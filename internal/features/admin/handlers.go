// Package admin — handlers.go обрабатывает взаимодействие с админ-панелью.
// Панель работает через Reply Keyboard в личных сообщениях.
// Поток: аутентификация → клавиатура → выбор действия → пошаговый диалог.
package admin

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	models "github.com/mymmrac/telego"
	log "github.com/sirupsen/logrus"

	"serotonyl.ru/telegram-bot/internal/audit"
	"serotonyl.ru/telegram-bot/internal/common"
	"serotonyl.ru/telegram-bot/internal/features/members"
	"serotonyl.ru/telegram-bot/internal/telegram"
)

const (
	userPickerPrevButton = "◀️"
	userPickerNextButton = "▶️"
	userPickerBackButton = "⬅️ Назад"
	userPickerPageSize   = 8

	cbAdminAssignRole        = "admin:assign_role"
	cbAdminChangeRole        = "admin:change_role"
	cbAdminBalanceAdjust     = "admin:balance_adjust"
	cbAdminParticipants      = "admin:participants"
	cbAdminParticipantsPage  = "admin:participants:page:"
	cbAdminDeltasMenu        = "admin:deltas"
	cbAdminDeltaAdd          = "admin:deltas:add"
	cbAdminDeltaDelete       = "admin:deltas:delete"
	cbPickerPrefix           = "admin:picker:"
	cbPickerSelect           = "select"
	cbPickerPrev             = "prev"
	cbPickerNext             = "next"
	cbPickerBack             = "back"
	cbAssignRefresh          = "admin:assign:refresh"
	cbRoleInputBack          = "admin:role_input_back"
	cbAdminCancelAction      = "admin:cancel_action"
	cbAdminUndoLast          = "admin:undo_last"
	cbAdminReturnPanel       = "admin:return_panel"
	manualRefreshTimeout     = 7 * time.Second
	assignRefreshFailureHint = "⚠️ Не удалось обновить теги участников, показываю текущий список."
)

var userPickerIDPattern = regexp.MustCompile(`(?i)(?:id:|#)(\d+)`)
var userPickerPageLabelPattern = regexp.MustCompile(`(?i)^\s*стр\s*\d+\s*/\s*\d+\s*$`)

// Handler обрабатывает админ-команды.
type economyService interface {
	AddBalance(ctx context.Context, userID int64, amount int64, txType, description string) error
	DeductBalance(ctx context.Context, userID int64, amount int64, txType, description string) error
	WithTransaction(ctx context.Context, fn func(context.Context, pgx.Tx) error) error
	AddBalanceTx(ctx context.Context, tx pgx.Tx, userID int64, amount int64, txType, description string) error
	GetBalance(ctx context.Context, userID int64) (int64, error)
}

type Handler struct {
	service            *Service
	memberService      *members.Service
	economyService     economyService
	riddleService      *RiddleService
	ops                *telegram.Ops
	audit              *audit.Logger
	memberSourceChatID int64
	undoMu             sync.Mutex
	lastRoleUndo       map[int64]*roleUndoData
	wizardCtx          context.Context
	refreshTimeout     time.Duration
}

type roleUndoData struct {
	targetUserID int64
	oldRole      string
	newRole      string
	ts           time.Time
}

// NewHandler создаёт обработчик админ-панели.
func NewHandler(service *Service, memberService *members.Service, economyService economyService, ops *telegram.Ops, memberSourceChatID int64) *Handler {
	var riddles *RiddleService
	if service != nil {
		riddles = service.riddles
	}
	return &Handler{
		service:            service,
		memberService:      memberService,
		economyService:     economyService,
		riddleService:      riddles,
		ops:                ops,
		memberSourceChatID: memberSourceChatID,
		lastRoleUndo:       make(map[int64]*roleUndoData),
		refreshTimeout:     manualRefreshTimeout,
	}
}

// HandleAdminMessage обрабатывает любое сообщение от администратора в DM.
// Определяет текущее состояние диалога и маршрутизирует сообщение.
func (h *Handler) HandleAdminMessage(ctx context.Context, chatID int64, userID int64, messageID int, text string) bool {
	fields := strings.Fields(strings.TrimSpace(text))
	isLoginCommand := len(fields) > 0 && strings.EqualFold(fields[0], "/login")

	// Единый gate: DB is_admin ИЛИ ADMIN_IDS
	if !h.service.CanEnterAdmin(ctx, userID) {
		if isLoginCommand {
			h.sendMessage(ctx, chatID, "❌ Доступ запрещён")
			return true
		}
		return false
	}

	// Проверяем состояние диалога
	state := h.service.GetState(userID)
	hasActiveSession := h.service.HasActiveSession(ctx, userID)

	if hasActiveSession {
		if isLoginCommand {
			if err := h.reopenAdminPanel(ctx, chatID, userID); err != nil {
				h.sendUIErrorHint(ctx, chatID, err)
			}
			return true
		}

		// Обновляем активность сессии
		if err := h.service.repo.UpdateActivity(ctx, userID); err != nil {
			log.WithError(err).WithField("user_id", userID).Warn("ошибка обновления активности админ-сессии")
		}
	} else {
		// Обрабатываем состояние ожидания пароля
		if state != nil && state.State == StateAwaitingPassword {
			h.handlePasswordInput(ctx, chatID, userID, text)
			return true
		}

		// Single-step логин: /login <пароль>
		if isLoginCommand && len(fields) > 1 {
			password := strings.Join(fields[1:], " ")
			h.handlePasswordInput(ctx, chatID, userID, password)
			return true
		}

		// Нет сессии — запрашиваем пароль
		h.sendMessage(ctx, chatID, "🔐 Введите пароль для доступа к админ-панели:")
		h.service.SetState(userID, StateAwaitingPassword, nil)
		return true
	}

	// Обрабатываем текущее состояние
	if state != nil {
		switch state.State {
		case StateAssignRoleSelect:
			if !h.service.CanManageRoles(ctx, userID) {
				h.denyInsufficientPermissions(ctx, chatID)
				h.service.ClearState(userID)
				return true
			}
			h.handleAssignRoleSelect(ctx, chatID, userID, text)
			h.deleteAdminInputMessage(ctx, chatID, messageID)
			return true
		case StateAssignRoleText:
			if !h.service.CanManageRoles(ctx, userID) {
				h.denyInsufficientPermissions(ctx, chatID)
				h.service.ClearState(userID)
				return true
			}
			h.handleAssignRoleText(ctx, chatID, userID, text)
			h.deleteAdminInputMessage(ctx, chatID, messageID)
			return true
		case StateChangeRoleSelect:
			if !h.service.CanManageRoles(ctx, userID) {
				h.denyInsufficientPermissions(ctx, chatID)
				h.service.ClearState(userID)
				return true
			}
			h.handleChangeRoleSelect(ctx, chatID, userID, text)
			h.deleteAdminInputMessage(ctx, chatID, messageID)
			return true
		case StateChangeRoleText:
			if !h.service.CanManageRoles(ctx, userID) {
				h.denyInsufficientPermissions(ctx, chatID)
				h.service.ClearState(userID)
				return true
			}
			h.handleChangeRoleText(ctx, chatID, userID, text)
			h.deleteAdminInputMessage(ctx, chatID, messageID)
			return true
		case StateBalanceAdjustAmount, StateBalanceDeltaName, StateBalanceDeltaAmount:
			if !h.service.CanManageBalance(ctx, userID) {
				h.denyInsufficientPermissions(ctx, chatID)
				h.service.ClearState(userID)
				return true
			}
			if h.handleBalanceAdjustManualAmount(ctx, chatID, userID, strings.TrimSpace(text)) {
				h.deleteAdminInputMessage(ctx, chatID, messageID)
				return true
			}
		}
		if h.service.CanManageRiddles(ctx, userID) && h.handleRiddleMessageInput(ctx, chatID, userID, messageID, text) {
			return true
		}
	}

	// Обрабатываем кнопки клавиатуры
	switch text {
	case "Назначить роль":
		if !h.service.CanManageRoles(ctx, userID) {
			h.denyInsufficientPermissions(ctx, chatID)
			return true
		}
		h.startAssignRole(ctx, chatID, userID, h.panelMessageIDFromState(userID))
		return true
	case "Сменить роль":
		if !h.service.CanManageRoles(ctx, userID) {
			h.denyInsufficientPermissions(ctx, chatID)
			return true
		}
		h.startChangeRole(ctx, chatID, userID, h.panelMessageIDFromState(userID))
		return true
	case "Изменить баланс", "️ Валюта":
		if !h.service.CanManageBalance(ctx, userID) {
			h.denyInsufficientPermissions(ctx, chatID)
			return true
		}
		h.startBalanceAdjustMode(ctx, chatID, userID, h.panelMessageIDFromState(userID))
		return true
	case "Участники":
		if !h.service.CanManageBalance(ctx, userID) {
			h.denyInsufficientPermissions(ctx, chatID)
			return true
		}
		h.showParticipantsPage(ctx, chatID, userID, h.panelMessageIDFromState(userID), 0)
		return true
	case "Дельты":
		if !h.service.CanManageBalance(ctx, userID) {
			h.denyInsufficientPermissions(ctx, chatID)
			return true
		}
		h.showDeltaMenu(ctx, chatID, userID, h.panelMessageIDFromState(userID))
		return true
	case "Админ", "Панель", "админ", "панель":
		if err := h.showKeyboard(ctx, chatID, userID, h.panelMessageIDFromState(userID)); err != nil {
			h.sendUIErrorHint(ctx, chatID, err)
		}
		return true
	}

	return false
}

// HandleAdminCallback обрабатывает callback_query кнопок админки.
func (h *Handler) HandleAdminCallback(ctx context.Context, q *models.CallbackQuery) bool {
	if q == nil {
		return false
	}
	if q.Data == cbAssignRefresh {
		h.answerCallback(ctx, q.ID, "Обновляю список…")
	} else {
		h.answerCallback(ctx, q.ID, "")
	}

	msg := callbackMessage(q)
	if msg == nil {
		return false
	}

	chatID := msg.Chat.ID
	userID := q.From.ID
	data := q.Data
	panelMsgID := msg.MessageID
	h.attachPanelMessage(userID, chatID, panelMsgID)

	if !h.service.CanEnterAdmin(ctx, userID) {
		return true
	}

	if !h.service.HasActiveSession(ctx, userID) {
		h.sendMessage(ctx, chatID, "🔐 Введите пароль для доступа к админ-панели:")
		h.service.SetState(userID, StateAwaitingPassword, nil)
		return true
	}

	if err := h.service.repo.UpdateActivity(ctx, userID); err != nil {
		log.WithError(err).WithField("user_id", userID).Warn("ошибка обновления активности админ-сессии")
	}

	switch data {
	case cbAdminAssignRole:
		if !h.service.CanManageRoles(ctx, userID) {
			h.denyInsufficientPermissions(ctx, chatID)
			return true
		}
		h.startAssignRole(ctx, chatID, userID, panelMsgID)
		return true
	case cbAdminChangeRole:
		if !h.service.CanManageRoles(ctx, userID) {
			h.denyInsufficientPermissions(ctx, chatID)
			return true
		}
		h.startChangeRole(ctx, chatID, userID, panelMsgID)
		return true
	case cbAdminBalanceAdjust:
		if !h.service.CanManageBalance(ctx, userID) {
			h.denyInsufficientPermissions(ctx, chatID)
			return true
		}
		h.startBalanceAdjustMode(ctx, chatID, userID, panelMsgID)
		return true
	case cbAdminParticipants:
		if !h.service.CanManageBalance(ctx, userID) {
			h.denyInsufficientPermissions(ctx, chatID)
			return true
		}
		h.showParticipantsPage(ctx, chatID, userID, panelMsgID, 0)
		return true
	case cbAdminDeltasMenu:
		if !h.service.CanManageBalance(ctx, userID) {
			h.denyInsufficientPermissions(ctx, chatID)
			return true
		}
		h.showDeltaMenu(ctx, chatID, userID, panelMsgID)
		return true
	case cbAdminDeltaAdd:
		if !h.service.CanManageBalance(ctx, userID) {
			h.denyInsufficientPermissions(ctx, chatID)
			return true
		}
		h.startDeltaCreate(ctx, chatID, userID, panelMsgID)
		return true
	case cbAdminDeltaDelete:
		if !h.service.CanManageBalance(ctx, userID) {
			h.denyInsufficientPermissions(ctx, chatID)
			return true
		}
		h.startDeltaDelete(ctx, chatID, userID, panelMsgID)
		return true
	case cbAdminRiddlesMenu:
		if !h.service.CanManageRiddles(ctx, userID) {
			h.denyInsufficientPermissions(ctx, chatID)
			return true
		}
		h.showRiddlesMenu(ctx, chatID, userID, panelMsgID)
		return true
	case cbRiddleCreate:
		if !h.service.CanManageRiddles(ctx, userID) {
			h.denyInsufficientPermissions(ctx, chatID)
			return true
		}
		h.startRiddleCreate(ctx, chatID, userID, panelMsgID)
		return true
	case cbRiddleStop:
		if !h.service.CanManageRiddles(ctx, userID) {
			h.denyInsufficientPermissions(ctx, chatID)
			return true
		}
		h.handleRiddleStop(ctx, chatID, userID)
		return true
	case cbRiddlePublish:
		if !h.service.CanManageRiddles(ctx, userID) {
			h.denyInsufficientPermissions(ctx, chatID)
			return true
		}
		h.handleRiddlePublish(ctx, chatID, userID)
		return true
	case cbRiddleCancelDraft:
		if !h.service.CanManageRiddles(ctx, userID) {
			h.denyInsufficientPermissions(ctx, chatID)
			return true
		}
		h.handleRiddleCancel(ctx, chatID, userID)
		return true
	case cbRoleInputBack:
		h.handleRoleInputBack(ctx, chatID, userID, panelMsgID)
		return true
	case cbAdminCancelAction:
		h.service.ClearState(userID)
		h.showKeyboardSafe(ctx, chatID, userID, panelMsgID)
		return true
	case cbAdminUndoLast:
		if !h.service.CanManageRoles(ctx, userID) {
			h.denyInsufficientPermissions(ctx, chatID)
			return true
		}
		h.handleUndoLastRole(ctx, chatID, userID, panelMsgID)
		return true
	case cbAdminReturnPanel:
		h.service.ClearState(userID)
		h.showKeyboardSafe(ctx, chatID, userID, panelMsgID)
		return true
	case cbAssignRefresh:
		if !h.service.CanManageRoles(ctx, userID) {
			h.denyInsufficientPermissions(ctx, chatID)
			return true
		}
		h.refreshAssignRolePicker(ctx, chatID, userID, panelMsgID)
		return true
	}

	if strings.HasPrefix(data, "admin:balmode:") || strings.HasPrefix(data, "admin:balpick:") || strings.HasPrefix(data, "admin:balamt:") || strings.HasPrefix(data, "admin:balconfirm:") || data == cbBalUndo {
		if !h.service.CanManageBalance(ctx, userID) {
			h.denyInsufficientPermissions(ctx, chatID)
			return true
		}
		h.handleBalanceAdjustCallback(ctx, chatID, userID, panelMsgID, data)
		return true
	}
	if strings.HasPrefix(data, cbAdminParticipantsPage) {
		if !h.service.CanManageBalance(ctx, userID) {
			h.denyInsufficientPermissions(ctx, chatID)
			return true
		}
		page, err := strconv.Atoi(strings.TrimPrefix(data, cbAdminParticipantsPage))
		if err != nil {
			page = 0
		}
		h.showParticipantsPage(ctx, chatID, userID, panelMsgID, page)
		return true
	}

	if strings.HasPrefix(data, cbPickerPrefix) {
		h.handleUserPickerCallback(ctx, chatID, userID, panelMsgID, data)
		return true
	}

	return true
}

// handlePasswordInput обрабатывает ввод пароля.
func (h *Handler) handlePasswordInput(ctx context.Context, chatID int64, userID int64, password string) {
	err := h.service.VerifyPassword(ctx, userID, password)
	if err != nil {
		h.sendMessage(ctx, chatID, fmt.Sprintf("❌ %s", err.Error()))
		h.service.ClearState(userID)
		return
	}

	h.service.ClearState(userID)
	if err := h.reopenAdminPanel(ctx, chatID, userID); err != nil {
		h.sendUIErrorHint(ctx, chatID, err)
	}
}

// showKeyboard отображает клавиатуру админ-панели.
func (h *Handler) showKeyboard(ctx context.Context, chatID int64, userID int64, panelMsgID int) error {
	if !h.service.CanManageRoles(ctx, userID) {
		keyboard := newInlineKeyboardMarkup(
			newInlineKeyboardRow(
				newInlineKeyboardButtonData("Создать загадку", cbRiddleCreate),
			),
			newInlineKeyboardRow(
				newInlineKeyboardButtonData("Остановить загадку", cbRiddleStop),
			),
		)
		return h.renderAdminScreen(ctx, chatID, userID, panelMsgID, "moderator_panel", "Панель модератора", keyboard)
	}

	keyboard := newInlineKeyboardMarkup(
		newInlineKeyboardRow(
			newInlineKeyboardButtonData("👤 Назначить роль", cbAdminAssignRole),
		),
		newInlineKeyboardRow(
			newInlineKeyboardButtonData("🔄 Сменить роль", cbAdminChangeRole),
		),
		newInlineKeyboardRow(
			newInlineKeyboardButtonData("🎞️ Валюта", cbAdminBalanceAdjust),
		),
		newInlineKeyboardRow(
			newInlineKeyboardButtonData("👥 Участники", cbAdminParticipants),
		),
		newInlineKeyboardRow(
			newInlineKeyboardButtonData("❓ Загадки", cbAdminRiddlesMenu),
		),
		newInlineKeyboardRow(
			newInlineKeyboardButtonData("➕ Дельты", cbAdminDeltasMenu),
		),
	)

	return h.renderAdminScreen(ctx, chatID, userID, panelMsgID, "panel", "✅ Админ-панель открыта", keyboard)
}

func (h *Handler) showDeltaMenu(ctx context.Context, chatID int64, userID int64, panelMsgID int) {
	h.service.ClearState(userID)
	keyboard := newInlineKeyboardMarkup(
		newInlineKeyboardRow(newInlineKeyboardButtonData("Добавить дельту", cbAdminDeltaAdd)),
		newInlineKeyboardRow(newInlineKeyboardButtonData("Удалить дельту", cbAdminDeltaDelete)),
		newInlineKeyboardRow(newInlineKeyboardButtonDataStyled("Назад", cbAdminReturnPanel, "danger")),
	)
	if err := h.renderAdminScreen(ctx, chatID, userID, panelMsgID, "delta_menu", "Дельты", keyboard); err != nil {
		h.sendUIErrorHint(ctx, chatID, err)
	}
}

func (h *Handler) showParticipantsPage(ctx context.Context, chatID, userID int64, panelMsgID int, page int) {
	withRole, err := h.service.GetUsersWithRole(ctx)
	if err != nil {
		h.sendUIErrorHint(ctx, chatID, err)
		return
	}
	withoutRole, err := h.service.GetUsersWithoutRole(ctx)
	if err != nil {
		h.sendUIErrorHint(ctx, chatID, err)
		return
	}
	ranked, err := members.RankMembersByBalance(ctx, withRole, withoutRole, h.economyService, 0)
	if err != nil {
		h.sendUIErrorHint(ctx, chatID, err)
		return
	}

	totalPages := 1
	if len(ranked) > 0 {
		totalPages = (len(ranked) + userPickerPageSize - 1) / userPickerPageSize
	}
	if page < 0 {
		page = 0
	}
	if page >= totalPages {
		page = totalPages - 1
	}

	start := page * userPickerPageSize
	end := start + userPickerPageSize
	if end > len(ranked) {
		end = len(ranked)
	}

	lines := []string{"Участники"}
	if len(ranked) == 0 {
		lines = append(lines, "", "Список пуст.")
	} else {
		lines = append(lines, "")
		for _, rm := range ranked[start:end] {
			lines = append(lines, fmt.Sprintf("%s – %s", members.FormatParticipantHTML(rm.Member), common.FormatBalance(rm.Balance)))
		}
	}

	rows := make([][]models.InlineKeyboardButton, 0, 2)
	if totalPages > 1 {
		prevPage := page - 1
		if prevPage < 0 {
			prevPage = 0
		}
		nextPage := page + 1
		if nextPage >= totalPages {
			nextPage = totalPages - 1
		}
		rows = append(rows, newInlineKeyboardRow(
			newInlineKeyboardButtonData(userPickerPrevButton, fmt.Sprintf("%s%d", cbAdminParticipantsPage, prevPage)),
			newInlineKeyboardButtonData(fmt.Sprintf("Стр %d/%d", page+1, totalPages), fmt.Sprintf("%s%d", cbAdminParticipantsPage, page)),
			newInlineKeyboardButtonData(userPickerNextButton, fmt.Sprintf("%s%d", cbAdminParticipantsPage, nextPage)),
		))
	}
	rows = append(rows, newInlineKeyboardRow(newInlineKeyboardButtonDataStyled("Назад", cbAdminReturnPanel, "danger")))
	if err := h.renderAdminScreenWithOptions(ctx, chatID, userID, panelMsgID, "participants", strings.Join(lines, "\n"), newInlineKeyboardMarkup(rows...), telegram.ParseModeHTML, true); err != nil {
		h.sendUIErrorHint(ctx, chatID, err)
	}
}

// --- Назначить роль (3 шага) ---

// startAssignRole — Шаг 1: показать пользователей БЕЗ роли.
func (h *Handler) startAssignRole(ctx context.Context, chatID int64, userID int64, panelMsgID int) {
	users, err := h.service.GetUsersWithoutRole(ctx)
	if err != nil {
		h.sendMessage(ctx, chatID, fmt.Sprintf("❌ Ошибка получения списка пользователей: %s", err.Error()))
		return
	}
	if len(users) == 0 {
		h.renderNoRoleCandidatesScreen(ctx, chatID, userID, panelMsgID)
		return
	}

	h.startUserPicker(ctx, chatID, userID, panelMsgID, StateAssignRoleSelect, UserPickerAssignWithoutRole, users)
}

func (h *Handler) renderNoRoleCandidatesScreen(ctx context.Context, chatID, userID int64, panelMsgID int) {
	h.service.ClearState(userID)
	if panelMsgID <= 0 {
		panelMsgID = h.panelMessageIDFromState(userID)
	}
	if err := h.renderAdminScreen(ctx, chatID, userID, panelMsgID, "assign_no_candidates", "ℹ️ Все пользователи уже имеют роли.", h.noRoleCandidatesMarkup()); err != nil {
		h.sendUIErrorHint(ctx, chatID, err)
	}
}

func (h *Handler) noRoleCandidatesMarkup() models.InlineKeyboardMarkup {
	return newInlineKeyboardMarkup(
		newInlineKeyboardRow(
			newInlineKeyboardButtonData("🔄 Обновить список", cbAssignRefresh),
		),
		newInlineKeyboardRow(
			newInlineKeyboardButtonDataStyled("✅ Вернуться в админку", cbAdminReturnPanel, "success"),
		),
	)
}

// handleAssignRoleSelect — Шаг 2: пользователь выбрал кнопку.
func (h *Handler) handleAssignRoleSelect(ctx context.Context, chatID int64, userID int64, text string) {
	selected, ok := h.handleUserPickerInput(ctx, chatID, userID, h.panelMessageIDFromState(userID), StateAssignRoleSelect, text)
	if !ok {
		return
	}

	h.renderAssignRoleInput(ctx, chatID, userID, selected)
}

// handleAssignRoleText — Шаг 3: ввод текста роли.
func (h *Handler) handleAssignRoleText(ctx context.Context, chatID int64, userID int64, text string) {
	state := h.service.GetState(userID)
	if state == nil {
		h.sendMessage(ctx, chatID, "⚠️ Состояние сброшено. Вернитесь в админ-меню.")
		h.service.ClearState(userID)
		h.showKeyboardSafe(ctx, chatID, userID, h.panelMessageIDFromState(userID))
		return
	}

	roleInput, ok := state.Data.(*RoleInputData)
	if !ok || roleInput == nil || roleInput.SelectedUser == nil {
		h.sendMessage(ctx, chatID, "⚠️ Состояние сброшено. Вернитесь в админ-меню.")
		h.service.ClearState(userID)
		h.showKeyboardSafe(ctx, chatID, userID, h.panelMessageIDFromState(userID))
		return
	}

	selected := roleInput.SelectedUser

	if strings.EqualFold(strings.TrimSpace(text), userPickerBackButton) {
		if roleInput.Picker != nil {
			h.service.SetState(userID, StateAssignRoleSelect, roleInput.Picker)
			h.renderUserPickerPage(ctx, chatID, userID, h.panelMessageIDFromState(userID), StateAssignRoleSelect)
			return
		}
		h.sendMessage(ctx, chatID, "⚠️ Невозможно вернуться назад. Вернитесь в админ-меню.")
		h.service.ClearState(userID)
		h.showKeyboardSafe(ctx, chatID, userID, h.panelMessageIDFromState(userID))
		return
	}

	role := strings.TrimSpace(text)
	if len([]rune(role)) > 64 {
		h.sendMessage(ctx, chatID, "❌ Роль слишком длинная (максимум 64 символа)")
		return
	}

	if err := h.service.AssignRole(ctx, selected.UserID, role); err != nil {
		h.sendMessage(ctx, chatID, fmt.Sprintf("❌ Ошибка: %s", err.Error()))
		h.service.ClearState(userID)
		h.showKeyboardSafe(ctx, chatID, userID, h.panelMessageIDFromState(userID))
		return
	}

	h.setUndoRoleChange(userID, selected.UserID, "", role)
	if h.audit != nil {
		h.audit.LogRoleAssign(ctx, h.auditActorLabel(ctx, userID), formatMemberIdentityCompact(selected), role)
	}
	h.sendRoleChangeSuccess(ctx, chatID, userID, h.panelMessageIDFromState(userID), fmt.Sprintf("✅ Роль назначена: %s → %s", normalizeRoleLabel(""), role))
	h.service.ClearState(userID)
}

// --- Сменить роль (3 шага) ---

func (h *Handler) startChangeRole(ctx context.Context, chatID int64, userID int64, panelMsgID int) {
	users, err := h.service.GetUsersWithRole(ctx)
	if err != nil {
		h.sendMessage(ctx, chatID, fmt.Sprintf("❌ Ошибка получения списка пользователей: %s", err.Error()))
		return
	}
	if len(users) == 0 {
		h.sendMessage(ctx, chatID, "Нет пользователей с назначенными ролями")
		return
	}

	h.startUserPicker(ctx, chatID, userID, panelMsgID, StateChangeRoleSelect, UserPickerChangeWithRole, users)
}

func (h *Handler) handleChangeRoleSelect(ctx context.Context, chatID int64, userID int64, text string) {
	selected, ok := h.handleUserPickerInput(ctx, chatID, userID, h.panelMessageIDFromState(userID), StateChangeRoleSelect, text)
	if !ok {
		return
	}

	h.renderChangeRoleInput(ctx, chatID, userID, selected)
}

func (h *Handler) handleChangeRoleText(ctx context.Context, chatID int64, userID int64, text string) {
	state := h.service.GetState(userID)
	if state == nil {
		h.sendMessage(ctx, chatID, "⚠️ Состояние сброшено. Вернитесь в админ-меню.")
		h.service.ClearState(userID)
		h.showKeyboardSafe(ctx, chatID, userID, h.panelMessageIDFromState(userID))
		return
	}

	roleInput, ok := state.Data.(*RoleInputData)
	if !ok || roleInput == nil || roleInput.SelectedUser == nil {
		h.sendMessage(ctx, chatID, "⚠️ Состояние сброшено. Вернитесь в админ-меню.")
		h.service.ClearState(userID)
		h.showKeyboardSafe(ctx, chatID, userID, h.panelMessageIDFromState(userID))
		return
	}

	selected := roleInput.SelectedUser

	if strings.EqualFold(strings.TrimSpace(text), userPickerBackButton) {
		if roleInput.Picker != nil {
			h.service.SetState(userID, StateChangeRoleSelect, roleInput.Picker)
			h.renderUserPickerPage(ctx, chatID, userID, h.panelMessageIDFromState(userID), StateChangeRoleSelect)
			return
		}
		h.sendMessage(ctx, chatID, "⚠️ Невозможно вернуться назад. Вернитесь в админ-меню.")
		h.service.ClearState(userID)
		h.showKeyboardSafe(ctx, chatID, userID, h.panelMessageIDFromState(userID))
		return
	}

	role := strings.TrimSpace(text)
	if len([]rune(role)) > 64 {
		h.sendMessage(ctx, chatID, "❌ Роль слишком длинная (максимум 64 символа)")
		return
	}

	oldRole := ""
	if currentMember, err := h.service.memberRepo.GetByUserID(ctx, selected.UserID); err == nil && currentMember != nil && currentMember.Role != nil {
		oldRole = strings.TrimSpace(*currentMember.Role)
	} else if selected.Role != nil {
		oldRole = strings.TrimSpace(*selected.Role)
	}

	if err := h.service.AssignRole(ctx, selected.UserID, role); err != nil {
		h.sendMessage(ctx, chatID, fmt.Sprintf("❌ Ошибка: %s", err.Error()))
		h.service.ClearState(userID)
		h.showKeyboardSafe(ctx, chatID, userID, h.panelMessageIDFromState(userID))
		return
	}

	h.setUndoRoleChange(userID, selected.UserID, oldRole, role)
	if h.audit != nil {
		h.audit.LogRoleChange(ctx, h.auditActorLabel(ctx, userID), formatMemberIdentityCompact(selected), oldRole, role)
	}
	h.sendRoleChangeSuccess(ctx, chatID, userID, h.panelMessageIDFromState(userID), fmt.Sprintf("✅ Роль изменена: %s → %s", normalizeRoleLabel(oldRole), role))
	h.service.ClearState(userID)
}

func (h *Handler) startUserPicker(ctx context.Context, chatID, userID int64, panelMsgID int, stateName string, mode UserPickerMode, users []*members.Member) {
	data := &UserPickerData{
		Mode:          mode,
		UsersSnapshot: users,
		PageIndex:     0,
		PageSize:      userPickerPageSize,
	}
	h.service.SetState(userID, stateName, data)
	h.attachPanelMessage(userID, chatID, panelMsgID)
	h.renderUserPickerPage(ctx, chatID, userID, panelMsgID, stateName)
}

func (h *Handler) renderUserPickerPage(ctx context.Context, chatID, userID int64, panelMsgID int, stateName string) {
	state := h.service.GetState(userID)
	if state == nil || state.State != stateName {
		h.sendMessage(ctx, chatID, "⚠️ Состояние сброшено. Вернитесь в админ-меню.")
		h.service.ClearState(userID)
		h.showKeyboardSafe(ctx, chatID, userID, panelMsgID)
		return
	}

	data, ok := state.Data.(*UserPickerData)
	if !ok || data == nil || len(data.UsersSnapshot) == 0 {
		h.sendMessage(ctx, chatID, "⚠️ Список пользователей недоступен. Вернитесь в админ-меню.")
		h.service.ClearState(userID)
		h.showKeyboardSafe(ctx, chatID, userID, panelMsgID)
		return
	}

	if data.PageSize <= 0 {
		data.PageSize = userPickerPageSize
	}

	totalPages := (len(data.UsersSnapshot) + data.PageSize - 1) / data.PageSize
	if totalPages == 0 {
		totalPages = 1
	}
	if data.PageIndex < 0 {
		data.PageIndex = 0
	}
	if data.PageIndex >= totalPages {
		data.PageIndex = totalPages - 1
	}

	start := data.PageIndex * data.PageSize
	end := start + data.PageSize
	if end > len(data.UsersSnapshot) {
		end = len(data.UsersSnapshot)
	}

	usersOnPage := data.UsersSnapshot[start:end]
	rows := make([][]models.InlineKeyboardButton, 0, len(usersOnPage)+3)
	for _, user := range usersOnPage {
		rows = append(rows, newInlineKeyboardRow(
			newInlineKeyboardButtonData(formatUserPickerButton(user, data.Mode), pickerCallbackData(data.Mode, cbPickerSelect, user.UserID)),
		))
	}

	pageLabel := fmt.Sprintf("Стр %d/%d", data.PageIndex+1, totalPages)
	rows = append(rows, newInlineKeyboardRow(
		newInlineKeyboardButtonData(userPickerPrevButton, pickerCallbackData(data.Mode, cbPickerPrev, 0)),
		newInlineKeyboardButtonData(pageLabel, pickerCallbackData(data.Mode, "page", 0)),
		newInlineKeyboardButtonData(userPickerNextButton, pickerCallbackData(data.Mode, cbPickerNext, 0)),
	))
	if data.Mode == UserPickerAssignWithoutRole {
		rows = append(rows, newInlineKeyboardRow(
			newInlineKeyboardButtonData("🔄 Обновить список", cbAssignRefresh),
		))
	}
	rows = append(rows, newInlineKeyboardRow(
		newInlineKeyboardButtonDataStyled(userPickerBackButton, pickerCallbackData(data.Mode, cbPickerBack, 0), "danger"),
	))

	keyboard := newInlineKeyboardMarkup(rows...)

	caption := "Выбери участника:"

	if panelMsgID <= 0 {
		panelMsgID = h.panelMessageIDFromState(userID)
	}
	if panelMsgID > 0 {
		if err := h.renderAdminScreen(ctx, chatID, userID, panelMsgID, "picker", caption, keyboard); err != nil {
			h.sendUIErrorHint(ctx, chatID, err)
		}
		return
	}
	if err := h.renderAdminScreen(ctx, chatID, userID, 0, "picker", caption, keyboard); err != nil {
		h.sendUIErrorHint(ctx, chatID, err)
	}
}

func (h *Handler) handleUserPickerCallback(ctx context.Context, chatID, userID int64, panelMsgID int, data string) {
	parts := strings.Split(data, ":")
	if len(parts) < 4 {
		return
	}

	mode := UserPickerMode(parts[2])
	if (mode == UserPickerAssignWithoutRole || mode == UserPickerChangeWithRole) && !h.service.CanManageRoles(ctx, userID) {
		h.denyInsufficientPermissions(ctx, chatID)
		return
	}
	action := parts[3]
	stateName := StateAssignRoleSelect
	if mode == UserPickerChangeWithRole {
		stateName = StateChangeRoleSelect
	}

	switch action {
	case cbPickerBack:
		h.service.ClearState(userID)
		h.showKeyboardSafe(ctx, chatID, userID, panelMsgID)
		return
	case cbPickerPrev:
		h.handleUserPickerInput(ctx, chatID, userID, panelMsgID, stateName, userPickerPrevButton)
		return
	case cbPickerNext:
		h.handleUserPickerInput(ctx, chatID, userID, panelMsgID, stateName, userPickerNextButton)
		return
	case "page":
		return
	case cbPickerSelect:
		if len(parts) != 5 {
			return
		}
		text := "id:" + parts[4]
		selected, ok := h.handleUserPickerInput(ctx, chatID, userID, panelMsgID, stateName, text)
		if !ok || selected == nil {
			return
		}
		if stateName == StateAssignRoleSelect {
			h.renderAssignRoleInput(ctx, chatID, userID, selected)
			return
		}
		h.renderChangeRoleInput(ctx, chatID, userID, selected)
	}
}

func (h *Handler) refreshAssignRolePicker(ctx context.Context, chatID, userID int64, panelMsgID int) {
	// Refresh всегда строит новый picker из свежего состояния репозитория и не должен
	// полагаться на старый UsersSnapshot, который мог устареть.
	h.service.ClearState(userID)
	if h.memberService != nil && h.ops != nil && h.memberSourceChatID != 0 {
		refreshCtx, cancel := context.WithTimeout(ctx, h.refreshTimeout)
		defer cancel()

		// Ручное обновление пересинхронизирует tag и может восстановить пропущенных
		// известных участников после GetChatMember-проверки. Telegram Bot API при этом
		// не позволяет перечислить всех участников группы с нуля, а DM сама по себе
		// не считается основанием для вставки active-записи.
		if _, err := h.memberService.ScanAndUpdateMemberTags(refreshCtx, h.ops, h.memberSourceChatID, time.Now().UTC()); err != nil {
			log.WithError(err).WithFields(log.Fields{
				"admin_id":   userID,
				"chat_id":    chatID,
				"group_id":   h.memberSourceChatID,
				"timeout_ms": h.refreshTimeout.Milliseconds(),
			}).Warn("assign role refresh: member tag sync failed")
			if ctx.Err() == nil {
				h.sendMessage(ctx, chatID, assignRefreshFailureHint)
			}
		}
	}
	h.startAssignRole(ctx, chatID, userID, panelMsgID)
}

func (h *Handler) handleUserPickerInput(ctx context.Context, chatID, userID int64, panelMsgID int, stateName, text string) (*members.Member, bool) {
	state := h.service.GetState(userID)
	if state == nil || state.State != stateName {
		h.sendMessage(ctx, chatID, "⚠️ Состояние сброшено. Вернитесь в админ-меню.")
		h.service.ClearState(userID)
		h.showKeyboardSafe(ctx, chatID, userID, panelMsgID)
		return nil, false
	}

	data, ok := state.Data.(*UserPickerData)
	if !ok || data == nil || len(data.UsersSnapshot) == 0 {
		h.sendMessage(ctx, chatID, "⚠️ Список пользователей недоступен. Вернитесь в админ-меню.")
		h.service.ClearState(userID)
		h.showKeyboardSafe(ctx, chatID, userID, panelMsgID)
		return nil, false
	}

	switch text {
	case userPickerBackButton:
		h.service.ClearState(userID)
		h.showKeyboardSafe(ctx, chatID, userID, panelMsgID)
		return nil, false
	case userPickerPrevButton:
		if data.PageIndex > 0 {
			data.PageIndex--
		}
		h.renderUserPickerPage(ctx, chatID, userID, panelMsgID, stateName)
		return nil, false
	case userPickerNextButton:
		if data.PageSize <= 0 {
			data.PageSize = userPickerPageSize
		}
		totalPages := (len(data.UsersSnapshot) + data.PageSize - 1) / data.PageSize
		if totalPages == 0 {
			totalPages = 1
		}
		if data.PageIndex < totalPages-1 {
			data.PageIndex++
		}
		h.renderUserPickerPage(ctx, chatID, userID, panelMsgID, stateName)
		return nil, false
	}

	if isUserPickerPageLabel(text) {
		return nil, false
	}

	pickedUserID, ok := parseUserIDFromButton(text)
	if !ok {
		h.sendMessage(ctx, chatID, "❌ Некорректный выбор. Используйте кнопки ниже.")
		h.renderUserPickerPage(ctx, chatID, userID, panelMsgID, stateName)
		return nil, false
	}

	for _, user := range data.UsersSnapshot {
		if user.UserID == pickedUserID {
			data.SelectedUserID = pickedUserID
			return user, true
		}
	}

	h.sendMessage(ctx, chatID, "❌ Пользователь не найден в текущем списке. Выберите снова.")
	h.renderUserPickerPage(ctx, chatID, userID, panelMsgID, stateName)
	return nil, false
}

func isUserPickerPageLabel(text string) bool {
	return userPickerPageLabelPattern.MatchString(strings.TrimSpace(text))
}

func formatUserPickerButton(user *members.Member, mode UserPickerMode) string {
	switch mode {
	case UserPickerChangeWithRole:
		return shortenForButton(formatMemberRoleOnly(user), 40)
	default:
		return shortenForButton(formatMemberTagOnly(user), 40)
	}
}

func formatMemberTagOnly(user *members.Member) string {
	if user != nil && user.Tag != nil {
		if tag := strings.TrimSpace(*user.Tag); tag != "" {
			return tag
		}
	}
	return "Без тега"
}

func formatMemberRoleOnly(user *members.Member) string {
	if user != nil && user.Role != nil {
		if role := strings.TrimSpace(*user.Role); role != "" {
			return role
		}
	}
	return "Без роли"
}

func formatMemberIdentityCompact(user *members.Member) string {
	if user == nil {
		return "id:0"
	}

	tag := ""
	if user.Tag != nil {
		tag = strings.TrimSpace(*user.Tag)
	}
	username := strings.TrimSpace(strings.TrimPrefix(user.Username, "@"))
	nick := strings.TrimSpace(strings.Join([]string{strings.TrimSpace(user.FirstName), strings.TrimSpace(user.LastName)}, " "))
	if nick == "" && user.LastKnownName != nil {
		nick = strings.TrimSpace(*user.LastKnownName)
	}
	id := fmt.Sprintf("id:%d", user.UserID)

	if username != "" {
		if tag != "" {
			return fmt.Sprintf("%s • @%s", tag, username)
		}
		return fmt.Sprintf("@%s", username)
	}

	if tag != "" && nick != "" {
		return fmt.Sprintf("%s • %s • %s", tag, nick, id)
	}
	if tag != "" {
		return fmt.Sprintf("%s • %s", tag, id)
	}
	if nick != "" {
		return fmt.Sprintf("%s • %s", nick, id)
	}

	return id
}

func shortenForButton(s string, max int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) == 0 {
		return "пользователь"
	}
	if len(r) <= max {
		return string(r)
	}
	if max <= 1 {
		return string(r[:1])
	}
	return string(r[:max-1]) + "…"
}

func parseUserIDFromButton(text string) (int64, bool) {
	matches := userPickerIDPattern.FindStringSubmatch(strings.TrimSpace(text))
	if len(matches) != 2 {
		return 0, false
	}

	var id int64
	_, err := fmt.Sscanf(matches[1], "%d", &id)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func pickerCallbackData(mode UserPickerMode, action string, userID int64) string {
	if action == cbPickerSelect {
		return fmt.Sprintf("%s%s:%s:%d", cbPickerPrefix, mode, action, userID)
	}
	return fmt.Sprintf("%s%s:%s", cbPickerPrefix, mode, action)
}

func (h *Handler) answerCallback(ctx context.Context, callbackID, text string) {
	if callbackID == "" {
		return
	}
	if err := h.ops.AnswerCallback(ctx, callbackID, text, false); err != nil {
		log.WithError(err).Debug("ошибка ответа на callback")
	}
}

func (h *Handler) sendMessage(ctx context.Context, chatID int64, text string) {
	_, _ = h.ops.Send(ctx, chatID, text, nil)
}

func (h *Handler) denyInsufficientPermissions(ctx context.Context, chatID int64) {
	h.sendMessage(ctx, chatID, "Недостаточно прав для этого действия.")
}

func (h *Handler) logAdminUIError(adminID, chatID int64, panelMessageID int, screenName, action string, tgCode int, tgText string, err error) {
	log.WithError(err).WithFields(log.Fields{
		"admin_id":         adminID,
		"chat_id":          chatID,
		"panel_message_id": panelMessageID,
		"screen_name":      screenName,
		"action":           action,
		"tg_error_code":    tgCode,
		"tg_error_text":    tgText,
	}).Warn("admin ui operation failed")
}

func (h *Handler) showKeyboardSafe(ctx context.Context, chatID, userID int64, panelMsgID int) {
	if err := h.showKeyboard(ctx, chatID, userID, panelMsgID); err != nil {
		h.sendUIErrorHint(ctx, chatID, err)
	}
}

func (h *Handler) sendUIErrorHint(ctx context.Context, chatID int64, err error) {
	if err == nil {
		return
	}
	d := strings.ToLower(err.Error())
	if strings.Contains(d, "forbidden") || strings.Contains(d, "blocked by the user") {
		return
	}
	if telegram.ShouldFallbackToSendOnEdit(err) {
		h.sendMessage(ctx, chatID, "⚠️ Панель устарела, попробуйте открыть снова.")
		return
	}
	h.sendMessage(ctx, chatID, "⚠️ Не удалось обновить панель. Попробуйте ещё раз.")
}

func (h *Handler) renderAssignRoleInput(ctx context.Context, chatID, userID int64, selected *members.Member) {
	picker := h.pickerDataFromState(userID)
	roleInput := &RoleInputData{SelectedUser: selected, Picker: picker}
	h.service.SetState(userID, StateAssignRoleText, roleInput)

	text := fmt.Sprintf("Введите роль для %s (максимум 64 символа):\n%s — назад к выбору участника.", selected.DisplayName(), userPickerBackButton)
	h.renderRoleInputScreen(ctx, chatID, userID, text)
}

func (h *Handler) renderChangeRoleInput(ctx context.Context, chatID, userID int64, selected *members.Member) {
	picker := h.pickerDataFromState(userID)
	roleInput := &RoleInputData{SelectedUser: selected, Picker: picker}
	h.service.SetState(userID, StateChangeRoleText, roleInput)

	currentRole := ""
	if selected.Role != nil {
		currentRole = *selected.Role
	}
	text := fmt.Sprintf("Текущая роль: %s\nВведите новую роль:\n%s — назад к выбору участника.", currentRole, userPickerBackButton)
	h.renderRoleInputScreen(ctx, chatID, userID, text)
}

func (h *Handler) renderRoleInputScreen(ctx context.Context, chatID, userID int64, text string) {
	panelMsgID := h.panelMessageIDFromState(userID)
	if panelMsgID > 0 {
		if err := h.renderAdminScreen(ctx, chatID, userID, panelMsgID, "role_input", text, newInlineKeyboardMarkup(
			newInlineKeyboardRow(newInlineKeyboardButtonData(userPickerBackButton, cbRoleInputBack)),
		)); err != nil {
			h.sendUIErrorHint(ctx, chatID, err)
		}
		return
	}
	if err := h.renderAdminScreen(ctx, chatID, userID, 0, "role_input", text, newInlineKeyboardMarkup(
		newInlineKeyboardRow(newInlineKeyboardButtonData(userPickerBackButton, cbRoleInputBack)),
	)); err != nil {
		h.sendUIErrorHint(ctx, chatID, err)
	}
}

func (h *Handler) sendRoleChangeSuccess(ctx context.Context, chatID, userID int64, panelMsgID int, text string) {
	if err := h.renderAdminScreen(ctx, chatID, userID, panelMsgID, "role_change_success", text, h.roleChangeSuccessActionsMarkup()); err != nil {
		h.sendUIErrorHint(ctx, chatID, err)
	}
}

func (h *Handler) roleChangeSuccessActionsMarkup() models.InlineKeyboardMarkup {
	return newInlineKeyboardMarkup(
		newInlineKeyboardRow(
			newInlineKeyboardButtonDataStyled("↩️ Отменить", cbAdminUndoLast, "danger"),
		),
		newInlineKeyboardRow(
			newInlineKeyboardButtonDataStyled("🏠 Админка", cbAdminReturnPanel, "success"),
		),
	)
}

func (h *Handler) roleChangeUndoDoneMarkup() models.InlineKeyboardMarkup {
	return newInlineKeyboardMarkup(
		newInlineKeyboardRow(
			newInlineKeyboardButtonDataStyled("🏠 Админка", cbAdminReturnPanel, "success"),
		),
	)
}

func (h *Handler) setUndoRoleChange(adminUserID, targetUserID int64, oldRole, newRole string) {
	h.undoMu.Lock()
	defer h.undoMu.Unlock()
	h.lastRoleUndo[adminUserID] = &roleUndoData{targetUserID: targetUserID, oldRole: oldRole, newRole: newRole, ts: time.Now()}
}

func (h *Handler) popUndoRoleChange(adminUserID int64) *roleUndoData {
	h.undoMu.Lock()
	defer h.undoMu.Unlock()
	data := h.lastRoleUndo[adminUserID]
	delete(h.lastRoleUndo, adminUserID)
	return data
}

func (h *Handler) handleUndoLastRole(ctx context.Context, chatID, userID int64, panelMsgID int) {
	undo := h.popUndoRoleChange(userID)
	if undo == nil {
		h.sendMessage(ctx, chatID, "Нет действия для отката")
		h.showKeyboardSafe(ctx, chatID, userID, panelMsgID)
		return
	}

	if err := h.service.AssignRole(ctx, undo.targetUserID, undo.oldRole); err != nil {
		h.sendMessage(ctx, chatID, fmt.Sprintf("❌ Ошибка отката: %s", err.Error()))
		return
	}

	if err := h.renderAdminScreen(ctx, chatID, userID, panelMsgID, "role_change_undo", fmt.Sprintf("↩️ Откат выполнен: %d %s → %s", undo.targetUserID, undo.newRole, normalizeRoleLabel(undo.oldRole)), h.roleChangeUndoDoneMarkup()); err != nil {
		h.sendUIErrorHint(ctx, chatID, err)
		return
	}
	h.service.ClearState(userID)
}

func (h *Handler) deleteAdminInputMessage(ctx context.Context, chatID int64, messageID int) {
	if messageID <= 0 {
		return
	}
	if err := h.ops.DeleteMessage(ctx, chatID, messageID); err != nil {
		log.WithError(err).WithFields(log.Fields{"chat_id": chatID, "message_id": messageID}).Debug("не удалось удалить сообщение с вводом администратора")
	}
}

func normalizeRoleLabel(role string) string {
	if strings.TrimSpace(role) == "" {
		return "—"
	}
	return strings.TrimSpace(role)
}

func (h *Handler) renderAdminScreen(ctx context.Context, chatID, userID int64, panelMsgID int, screenName, text string, keyboard models.InlineKeyboardMarkup) error {
	return h.renderAdminScreenWithOptions(ctx, chatID, userID, panelMsgID, screenName, text, keyboard, nil, false)
}

func (h *Handler) renderAdminScreenWithOptions(ctx context.Context, chatID, userID int64, panelMsgID int, screenName, text string, keyboard models.InlineKeyboardMarkup, parseMode *string, disableWebPagePreview bool) error {
	msgID, usedEdit, err := telegram.RenderScreen(ctx, h.ops, telegram.Screen{
		ChatID:                chatID,
		MessageID:             panelMsgID,
		Text:                  text,
		ReplyMarkup:           &keyboard,
		ParseMode:             parseMode,
		DisableWebPagePreview: disableWebPagePreview,
	})
	if err != nil {
		if telegram.IsEditNotModified(err) {
			log.WithError(err).WithFields(log.Fields{"chat_id": chatID, "panel_message_id": panelMsgID, "screen": screenName}).Debug("admin ui edit skipped: message is not modified")
			h.attachPanelMessage(userID, chatID, panelMsgID)
			return nil
		}
		action := "send"
		if usedEdit {
			action = "edit"
		}
		h.logAdminUIError(userID, chatID, panelMsgID, screenName, action, 0, err.Error(), err)
		return err
	}
	h.attachPanelMessage(userID, chatID, msgID)
	return nil
}

func (h *Handler) reopenAdminPanel(ctx context.Context, chatID, userID int64) error {
	panel := h.service.GetPanelMessage(userID)
	h.service.ClearState(userID)
	h.deletePanelMessage(ctx, userID, panel)
	if err := h.showKeyboard(ctx, chatID, userID, 0); err != nil {
		return err
	}
	if h.audit != nil {
		h.audit.LogLogin(ctx, h.auditActorLabel(ctx, userID))
	}
	return nil
}

func (h *Handler) deletePanelMessage(ctx context.Context, userID int64, panel AdminPanelMessage) {
	h.service.ClearPanelMessage(userID)
	if panel.ChatID == 0 || panel.MessageID <= 0 {
		return
	}
	if err := h.ops.DeleteMessage(ctx, panel.ChatID, panel.MessageID); err != nil {
		log.WithError(err).WithFields(log.Fields{
			"user_id":    userID,
			"chat_id":    panel.ChatID,
			"message_id": panel.MessageID,
		}).Debug("failed to delete previous admin panel message")
	}
}

func (h *Handler) attachPanelMessage(userID, chatID int64, panelMsgID int) {
	h.service.SetPanelMessage(userID, chatID, panelMsgID)
}

func (h *Handler) panelMessageIDFromState(userID int64) int {
	return h.service.GetPanelMessageID(userID)
}

func (h *Handler) pickerDataFromState(userID int64) *UserPickerData {
	state := h.service.GetState(userID)
	if state == nil {
		return nil
	}
	data, _ := state.Data.(*UserPickerData)
	return data
}

func (h *Handler) handleRoleInputBack(ctx context.Context, chatID, userID int64, panelMsgID int) {
	state := h.service.GetState(userID)
	if state == nil {
		h.showKeyboardSafe(ctx, chatID, userID, panelMsgID)
		return
	}

	roleInput, ok := state.Data.(*RoleInputData)
	if !ok || roleInput == nil || roleInput.Picker == nil {
		h.showKeyboardSafe(ctx, chatID, userID, panelMsgID)
		return
	}

	switch state.State {
	case StateAssignRoleText:
		h.service.SetState(userID, StateAssignRoleSelect, roleInput.Picker)
		h.renderUserPickerPage(ctx, chatID, userID, panelMsgID, StateAssignRoleSelect)
	case StateChangeRoleText:
		h.service.SetState(userID, StateChangeRoleSelect, roleInput.Picker)
		h.renderUserPickerPage(ctx, chatID, userID, panelMsgID, StateChangeRoleSelect)
	default:
		h.showKeyboardSafe(ctx, chatID, userID, panelMsgID)
	}
}

func (h *Handler) setWizardCtx(ctx context.Context) {
	h.wizardCtx = ctx
}

func (h *Handler) currentWizardCtx() context.Context {
	if h.wizardCtx != nil {
		return h.wizardCtx
	}
	return context.TODO()
}

func callbackMessage(q *models.CallbackQuery) *models.Message {
	if q == nil {
		return nil
	}
	return q.Message.Message()
}

func newInlineKeyboardMarkup(rows ...[]models.InlineKeyboardButton) models.InlineKeyboardMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func newInlineKeyboardRow(buttons ...models.InlineKeyboardButton) []models.InlineKeyboardButton {
	return buttons
}

func newInlineKeyboardButtonData(text, data string) models.InlineKeyboardButton {
	return models.InlineKeyboardButton{Text: strings.TrimSpace(text), CallbackData: data}
}

func newInlineKeyboardButtonDataStyled(text, data, style string) models.InlineKeyboardButton {
	b := newInlineKeyboardButtonData(text, data)
	b.Style = style
	return b
}
