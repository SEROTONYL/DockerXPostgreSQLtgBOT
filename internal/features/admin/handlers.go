// Package admin — handlers.go обрабатывает взаимодействие с админ-панелью.
// Панель работает через Reply Keyboard в личных сообщениях.
// Поток: аутентификация → клавиатура → выбор действия → пошаговый диалог.
package admin

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-telegram/bot/models"
	log "github.com/sirupsen/logrus"

	"serotonyl.ru/telegram-bot/internal/features/members"
	"serotonyl.ru/telegram-bot/internal/telegram"
)

const (
	userPickerPrevButton = "◀️"
	userPickerNextButton = "▶️"
	userPickerBackButton = "⬅️ Назад"
	userPickerPageSize   = 8

	cbAdminAssignRole    = "admin:assign_role"
	cbAdminChangeRole    = "admin:change_role"
	cbAdminStub          = "admin:stub"
	cbAdminBalanceAdjust = "admin:balance_adjust"
	cbPickerPrefix       = "admin:picker:"
	cbPickerSelect       = "select"
	cbPickerPrev         = "prev"
	cbPickerNext         = "next"
	cbPickerBack         = "back"
	cbRoleInputBack      = "admin:role_input_back"
	cbAdminCancelAction  = "admin:cancel_action"
	cbAdminUndoLast      = "admin:undo_last"
	cbAdminReturnPanel   = "admin:return_panel"
)

var userPickerIDPattern = regexp.MustCompile(`(?i)(?:id:|#)(\d+)`)
var userPickerPageLabelPattern = regexp.MustCompile(`(?i)^\s*стр\s*\d+\s*/\s*\d+\s*$`)

var editNeedlesNotModified = []string{"message is not modified"}
var editNeedlesNotFound = []string{"message to edit not found", "message not found"}
var editNeedlesCantBeEdited = []string{"message can't be edited", "message can\u2019t be edited"}
var editNeedlesForbidden = []string{"bot was blocked by the user", "chat not found", "forbidden", "not enough rights"}

// Handler обрабатывает админ-команды.
type economyService interface {
	AddBalance(ctx context.Context, userID int64, amount int64, txType, description string) error
	DeductBalance(ctx context.Context, userID int64, amount int64, txType, description string) error
}

type Handler struct {
	service        *Service
	memberService  *members.Service
	economyService economyService
	bot            telegram.Client
	sendFn         func(chatID int64, text string, markup *models.InlineKeyboardMarkup) (int, error)
	editFn         func(chatID int64, messageID int, text string, keyboard models.InlineKeyboardMarkup) error
	undoMu         sync.Mutex
	lastRoleUndo   map[int64]*roleUndoData
}

type roleUndoData struct {
	targetUserID int64
	oldRole      string
	newRole      string
	ts           time.Time
}

type editErrorKind string

const (
	editErrNone         editErrorKind = "none"
	editErrNotModified  editErrorKind = "not_modified"
	editErrNotFound     editErrorKind = "not_found"
	editErrCantBeEdited editErrorKind = "cant_be_edited"
	editErrForbidden    editErrorKind = "forbidden"
	editErrOther        editErrorKind = "other"
)

// NewHandler создаёт обработчик админ-панели.
func NewHandler(service *Service, memberService *members.Service, economyService economyService, bot telegram.Client) *Handler {
	return &Handler{
		service:        service,
		memberService:  memberService,
		economyService: economyService,
		bot:            bot,
		sendFn:         bot.SendMessage,
		editFn:         nil,
		lastRoleUndo:   make(map[int64]*roleUndoData),
	}
}

// HandleAdminMessage обрабатывает любое сообщение от администратора в DM.
// Определяет текущее состояние диалога и маршрутизирует сообщение.
func (h *Handler) HandleAdminMessage(ctx context.Context, chatID int64, userID int64, text string) bool {
	fields := strings.Fields(strings.TrimSpace(text))
	isLoginCommand := len(fields) > 0 && strings.EqualFold(fields[0], "/login")

	// Единый gate: DB is_admin ИЛИ ADMIN_IDS
	if !h.service.CanEnterAdmin(ctx, userID) {
		if isLoginCommand {
			h.sendMessage(chatID, "❌ Доступ запрещён")
			return true
		}
		return false
	}

	// Проверяем состояние диалога
	state := h.service.GetState(userID)
	hasActiveSession := h.service.HasActiveSession(ctx, userID)

	if hasActiveSession {
		if isLoginCommand {
			if err := h.showKeyboard(chatID, userID, h.panelMessageIDFromState(userID)); err != nil {
				h.sendUIErrorHint(chatID, err)
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
		h.sendMessage(chatID, "🔐 Введите пароль для доступа к админ-панели:")
		h.service.SetState(userID, StateAwaitingPassword, nil)
		return true
	}

	// Обрабатываем текущее состояние
	if state != nil {
		switch state.State {
		case StateAssignRoleSelect:
			h.handleAssignRoleSelect(ctx, chatID, userID, text)
			return true
		case StateAssignRoleText:
			h.handleAssignRoleText(ctx, chatID, userID, text)
			return true
		case StateChangeRoleSelect:
			h.handleChangeRoleSelect(ctx, chatID, userID, text)
			return true
		case StateChangeRoleText:
			h.handleChangeRoleText(ctx, chatID, userID, text)
			return true
		case StateBalanceAdjustAmount:
			if h.handleBalanceAdjustManualAmount(ctx, chatID, userID, strings.TrimSpace(text)) {
				return true
			}
		}
	}

	// Обрабатываем кнопки клавиатуры
	switch text {
	case "Назначить роль":
		h.startAssignRole(ctx, chatID, userID, h.panelMessageIDFromState(userID))
		return true
	case "Сменить роль":
		h.startChangeRole(ctx, chatID, userID, h.panelMessageIDFromState(userID))
		return true
	case "Изменить баланс":
		h.startBalanceAdjustMode(chatID, userID, h.panelMessageIDFromState(userID))
		return true
	case "Выдать кредит",
		"Аннулировать кредит", "Создать сокращение", "Удалить сокращение":
		h.sendMessage(chatID, "🔧 Функция в разработке")
		return true
	case "Админ", "Панель", "админ", "панель":
		if err := h.showKeyboard(chatID, userID, h.panelMessageIDFromState(userID)); err != nil {
			h.sendUIErrorHint(chatID, err)
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
	h.answerCallback(q.ID, "")

	msg := callbackMessage(q)
	if msg == nil {
		return false
	}

	chatID := msg.Chat.ID
	userID := q.From.ID
	data := q.Data
	panelMsgID := msg.ID
	h.attachPanelMessageID(userID, panelMsgID)

	if !h.service.CanEnterAdmin(ctx, userID) {
		h.answerCallback(q.ID, "")
		return true
	}

	if !h.service.HasActiveSession(ctx, userID) {
		h.answerCallback(q.ID, "")
		h.sendMessage(chatID, "🔐 Введите пароль для доступа к админ-панели:")
		h.service.SetState(userID, StateAwaitingPassword, nil)
		return true
	}

	if err := h.service.repo.UpdateActivity(ctx, userID); err != nil {
		log.WithError(err).WithField("user_id", userID).Warn("ошибка обновления активности админ-сессии")
	}

	switch data {
	case cbAdminAssignRole:
		h.startAssignRole(ctx, chatID, userID, panelMsgID)
		return true
	case cbAdminChangeRole:
		h.startChangeRole(ctx, chatID, userID, panelMsgID)
		return true
	case cbAdminBalanceAdjust:
		h.startBalanceAdjustMode(chatID, userID, panelMsgID)
		return true
	case cbAdminStub:
		h.answerCallback(q.ID, "")
		return true
	case cbRoleInputBack:
		h.handleRoleInputBack(chatID, userID, panelMsgID)
		return true
	case cbAdminCancelAction:
		h.service.ClearState(userID)
		h.showKeyboardSafe(chatID, userID, panelMsgID)
		return true
	case cbAdminUndoLast:
		h.handleUndoLastRole(ctx, chatID, userID, panelMsgID)
		return true
	case cbAdminReturnPanel:
		h.service.ClearState(userID)
		h.showKeyboardSafe(chatID, userID, panelMsgID)
		return true
	}

	if strings.HasPrefix(data, "admin:balmode:") || strings.HasPrefix(data, "admin:balpick:") || strings.HasPrefix(data, "admin:balamt:") || strings.HasPrefix(data, "admin:balconfirm:") || data == cbBalUndo {
		h.handleBalanceAdjustCallback(ctx, chatID, userID, panelMsgID, data)
		return true
	}

	if strings.HasPrefix(data, cbPickerPrefix) {
		h.handleUserPickerCallback(chatID, userID, panelMsgID, data)
		return true
	}

	h.answerCallback(q.ID, "")
	return true
}

// handlePasswordInput обрабатывает ввод пароля.
func (h *Handler) handlePasswordInput(ctx context.Context, chatID int64, userID int64, password string) {
	err := h.service.VerifyPassword(ctx, userID, password)
	if err != nil {
		h.sendMessage(chatID, fmt.Sprintf("❌ %s", err.Error()))
		h.service.ClearState(userID)
		return
	}

	h.service.ClearState(userID)
	h.sendMessage(chatID, "✅ Аутентификация успешна!")
	if err := h.showKeyboard(chatID, userID, 0); err != nil {
		h.sendUIErrorHint(chatID, err)
	}
}

// showKeyboard отображает клавиатуру админ-панели.
func (h *Handler) showKeyboard(chatID int64, userID int64, panelMsgID int) error {
	h.ensureSender()

	keyboard := newInlineKeyboardMarkup(
		newInlineKeyboardRow(
			newInlineKeyboardButtonData("Назначить роль", cbAdminAssignRole),
			newInlineKeyboardButtonData("Сменить роль", cbAdminChangeRole),
		),
		newInlineKeyboardRow(
			newInlineKeyboardButtonData("💸 Изменить баланс", cbAdminBalanceAdjust),
			newInlineKeyboardButtonData("Выдать кредит", cbAdminStub),
		),
		newInlineKeyboardRow(
			newInlineKeyboardButtonData("Аннулировать кредит", cbAdminStub),
		),
		newInlineKeyboardRow(
			newInlineKeyboardButtonData("Создать сокращение", cbAdminStub),
			newInlineKeyboardButtonData("Удалить сокращение", cbAdminStub),
		),
	)

	return h.renderAdminScreen(chatID, userID, panelMsgID, "panel", "✅ Админ-панель открыта", keyboard)
}

// --- Назначить роль (3 шага) ---

// startAssignRole — Шаг 1: показать пользователей БЕЗ роли.
func (h *Handler) startAssignRole(ctx context.Context, chatID int64, userID int64, panelMsgID int) {
	users, err := h.service.GetUsersWithoutRole(ctx)
	if err != nil {
		h.sendMessage(chatID, fmt.Sprintf("❌ Ошибка получения списка пользователей: %s", err.Error()))
		return
	}
	if len(users) == 0 {
		h.sendMessage(chatID, "Все пользователи уже имеют роли")
		return
	}

	h.startUserPicker(chatID, userID, panelMsgID, StateAssignRoleSelect, UserPickerAssignWithoutRole, users)
}

// handleAssignRoleSelect — Шаг 2: пользователь выбрал кнопку.
func (h *Handler) handleAssignRoleSelect(ctx context.Context, chatID int64, userID int64, text string) {
	selected, ok := h.handleUserPickerInput(chatID, userID, h.panelMessageIDFromState(userID), StateAssignRoleSelect, text)
	if !ok {
		return
	}

	h.renderAssignRoleInput(chatID, userID, selected)
}

// handleAssignRoleText — Шаг 3: ввод текста роли.
func (h *Handler) handleAssignRoleText(ctx context.Context, chatID int64, userID int64, text string) {
	state := h.service.GetState(userID)
	if state == nil {
		h.sendMessage(chatID, "⚠️ Состояние сброшено. Вернитесь в админ-меню.")
		h.service.ClearState(userID)
		h.showKeyboardSafe(chatID, userID, h.panelMessageIDFromState(userID))
		return
	}

	roleInput, ok := state.Data.(*RoleInputData)
	if !ok || roleInput == nil || roleInput.SelectedUser == nil {
		h.sendMessage(chatID, "⚠️ Состояние сброшено. Вернитесь в админ-меню.")
		h.service.ClearState(userID)
		h.showKeyboardSafe(chatID, userID, h.panelMessageIDFromState(userID))
		return
	}

	selected := roleInput.SelectedUser

	if strings.EqualFold(strings.TrimSpace(text), userPickerBackButton) {
		if roleInput.Picker != nil {
			h.service.SetState(userID, StateAssignRoleSelect, roleInput.Picker)
			h.renderUserPickerPage(chatID, userID, h.panelMessageIDFromState(userID), StateAssignRoleSelect)
			return
		}
		h.sendMessage(chatID, "⚠️ Невозможно вернуться назад. Вернитесь в админ-меню.")
		h.service.ClearState(userID)
		h.showKeyboardSafe(chatID, userID, h.panelMessageIDFromState(userID))
		return
	}

	role := strings.TrimSpace(text)
	if len([]rune(role)) > 64 {
		h.sendMessage(chatID, "❌ Роль слишком длинная (максимум 64 символа)")
		return
	}

	if err := h.service.AssignRole(ctx, selected.UserID, role); err != nil {
		h.sendMessage(chatID, fmt.Sprintf("❌ Ошибка: %s", err.Error()))
		h.service.ClearState(userID)
		h.showKeyboardSafe(chatID, userID, h.panelMessageIDFromState(userID))
		return
	}

	h.setUndoRoleChange(userID, selected.UserID, "", role)
	h.sendRoleChangeSuccess(chatID, userID, h.panelMessageIDFromState(userID), fmt.Sprintf("✅ Роль назначена: %s → %s", selected.DisplayName(), role))
	h.service.ClearState(userID)
}

// --- Сменить роль (3 шага) ---

func (h *Handler) startChangeRole(ctx context.Context, chatID int64, userID int64, panelMsgID int) {
	users, err := h.service.GetUsersWithRole(ctx)
	if err != nil {
		h.sendMessage(chatID, fmt.Sprintf("❌ Ошибка получения списка пользователей: %s", err.Error()))
		return
	}
	if len(users) == 0 {
		h.sendMessage(chatID, "Нет пользователей с назначенными ролями")
		return
	}

	h.startUserPicker(chatID, userID, panelMsgID, StateChangeRoleSelect, UserPickerChangeWithRole, users)
}

func (h *Handler) handleChangeRoleSelect(ctx context.Context, chatID int64, userID int64, text string) {
	selected, ok := h.handleUserPickerInput(chatID, userID, h.panelMessageIDFromState(userID), StateChangeRoleSelect, text)
	if !ok {
		return
	}

	h.renderChangeRoleInput(chatID, userID, selected)
}

func (h *Handler) handleChangeRoleText(ctx context.Context, chatID int64, userID int64, text string) {
	state := h.service.GetState(userID)
	if state == nil {
		h.sendMessage(chatID, "⚠️ Состояние сброшено. Вернитесь в админ-меню.")
		h.service.ClearState(userID)
		h.showKeyboardSafe(chatID, userID, h.panelMessageIDFromState(userID))
		return
	}

	roleInput, ok := state.Data.(*RoleInputData)
	if !ok || roleInput == nil || roleInput.SelectedUser == nil {
		h.sendMessage(chatID, "⚠️ Состояние сброшено. Вернитесь в админ-меню.")
		h.service.ClearState(userID)
		h.showKeyboardSafe(chatID, userID, h.panelMessageIDFromState(userID))
		return
	}

	selected := roleInput.SelectedUser

	if strings.EqualFold(strings.TrimSpace(text), userPickerBackButton) {
		if roleInput.Picker != nil {
			h.service.SetState(userID, StateChangeRoleSelect, roleInput.Picker)
			h.renderUserPickerPage(chatID, userID, h.panelMessageIDFromState(userID), StateChangeRoleSelect)
			return
		}
		h.sendMessage(chatID, "⚠️ Невозможно вернуться назад. Вернитесь в админ-меню.")
		h.service.ClearState(userID)
		h.showKeyboardSafe(chatID, userID, h.panelMessageIDFromState(userID))
		return
	}

	role := strings.TrimSpace(text)
	if len([]rune(role)) > 64 {
		h.sendMessage(chatID, "❌ Роль слишком длинная (максимум 64 символа)")
		return
	}

	oldRole := ""
	if selected.Role != nil {
		oldRole = strings.TrimSpace(*selected.Role)
	}

	if err := h.service.AssignRole(ctx, selected.UserID, role); err != nil {
		h.sendMessage(chatID, fmt.Sprintf("❌ Ошибка: %s", err.Error()))
		h.service.ClearState(userID)
		h.showKeyboardSafe(chatID, userID, h.panelMessageIDFromState(userID))
		return
	}

	h.setUndoRoleChange(userID, selected.UserID, oldRole, role)
	h.sendRoleChangeSuccess(chatID, userID, h.panelMessageIDFromState(userID), fmt.Sprintf("✅ Роль изменена: %s → %s", selected.DisplayName(), role))
	h.service.ClearState(userID)
}

func (h *Handler) startUserPicker(chatID, userID int64, panelMsgID int, stateName string, mode UserPickerMode, users []*members.Member) {
	data := &UserPickerData{
		Mode:          mode,
		UsersSnapshot: users,
		PageIndex:     0,
		PageSize:      userPickerPageSize,
	}
	h.service.SetState(userID, stateName, data)
	h.attachPanelMessageID(userID, panelMsgID)
	h.renderUserPickerPage(chatID, userID, panelMsgID, stateName)
}

func (h *Handler) renderUserPickerPage(chatID, userID int64, panelMsgID int, stateName string) {
	state := h.service.GetState(userID)
	if state == nil || state.State != stateName {
		h.sendMessage(chatID, "⚠️ Состояние сброшено. Вернитесь в админ-меню.")
		h.service.ClearState(userID)
		h.showKeyboardSafe(chatID, userID, panelMsgID)
		return
	}

	data, ok := state.Data.(*UserPickerData)
	if !ok || data == nil || len(data.UsersSnapshot) == 0 {
		h.sendMessage(chatID, "⚠️ Список пользователей недоступен. Вернитесь в админ-меню.")
		h.service.ClearState(userID)
		h.showKeyboardSafe(chatID, userID, panelMsgID)
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
	for i, user := range usersOnPage {
		style := "primary"
		if i%2 != 0 {
			style = "success"
		}
		rows = append(rows, newInlineKeyboardRow(
			newInlineKeyboardButtonDataStyled(formatUserPickerButton(user, data.Mode), pickerCallbackData(data.Mode, cbPickerSelect, user.UserID), style),
		))
	}

	pageLabel := fmt.Sprintf("Стр %d/%d", data.PageIndex+1, totalPages)
	rows = append(rows, newInlineKeyboardRow(
		newInlineKeyboardButtonData(userPickerPrevButton, pickerCallbackData(data.Mode, cbPickerPrev, 0)),
		newInlineKeyboardButtonData(pageLabel, pickerCallbackData(data.Mode, "page", 0)),
		newInlineKeyboardButtonData(userPickerNextButton, pickerCallbackData(data.Mode, cbPickerNext, 0)),
	))
	rows = append(rows, newInlineKeyboardRow(
		newInlineKeyboardButtonDataStyled(userPickerBackButton, pickerCallbackData(data.Mode, cbPickerBack, 0), "danger"),
	))

	keyboard := newInlineKeyboardMarkup(rows...)

	caption := "Выбери участника:"
	if data.Mode == UserPickerAssignWithoutRole {
		caption = "Выбери участника:\nФормат: @username • id, иначе Имя • id."
	} else if data.Mode == UserPickerChangeWithRole {
		caption = "Выбери участника:\nФормат: роль • @username, иначе роль • id."
	}

	if panelMsgID <= 0 {
		panelMsgID = h.panelMessageIDFromState(userID)
	}
	if panelMsgID > 0 {
		if err := h.renderAdminScreen(chatID, userID, panelMsgID, "picker", caption, keyboard); err != nil {
			h.sendUIErrorHint(chatID, err)
		}
		return
	}
	if err := h.renderAdminScreen(chatID, userID, 0, "picker", caption, keyboard); err != nil {
		h.sendUIErrorHint(chatID, err)
	}
}

func (h *Handler) handleUserPickerCallback(chatID, userID int64, panelMsgID int, data string) {
	parts := strings.Split(data, ":")
	if len(parts) < 4 {
		return
	}

	mode := UserPickerMode(parts[2])
	action := parts[3]
	stateName := StateAssignRoleSelect
	if mode == UserPickerChangeWithRole {
		stateName = StateChangeRoleSelect
	}

	switch action {
	case cbPickerBack:
		h.service.ClearState(userID)
		h.showKeyboardSafe(chatID, userID, panelMsgID)
		return
	case cbPickerPrev:
		h.handleUserPickerInput(chatID, userID, panelMsgID, stateName, userPickerPrevButton)
		return
	case cbPickerNext:
		h.handleUserPickerInput(chatID, userID, panelMsgID, stateName, userPickerNextButton)
		return
	case "page":
		return
	case cbPickerSelect:
		if len(parts) != 5 {
			return
		}
		text := "id:" + parts[4]
		selected, ok := h.handleUserPickerInput(chatID, userID, panelMsgID, stateName, text)
		if !ok || selected == nil {
			return
		}
		if stateName == StateAssignRoleSelect {
			h.renderAssignRoleInput(chatID, userID, selected)
			return
		}
		h.renderChangeRoleInput(chatID, userID, selected)
	}
}

func (h *Handler) handleUserPickerInput(chatID, userID int64, panelMsgID int, stateName, text string) (*members.Member, bool) {
	state := h.service.GetState(userID)
	if state == nil || state.State != stateName {
		h.sendMessage(chatID, "⚠️ Состояние сброшено. Вернитесь в админ-меню.")
		h.service.ClearState(userID)
		h.showKeyboardSafe(chatID, userID, panelMsgID)
		return nil, false
	}

	data, ok := state.Data.(*UserPickerData)
	if !ok || data == nil || len(data.UsersSnapshot) == 0 {
		h.sendMessage(chatID, "⚠️ Список пользователей недоступен. Вернитесь в админ-меню.")
		h.service.ClearState(userID)
		h.showKeyboardSafe(chatID, userID, panelMsgID)
		return nil, false
	}

	switch text {
	case userPickerBackButton:
		h.service.ClearState(userID)
		h.showKeyboardSafe(chatID, userID, panelMsgID)
		return nil, false
	case userPickerPrevButton:
		if data.PageIndex > 0 {
			data.PageIndex--
		}
		h.renderUserPickerPage(chatID, userID, panelMsgID, stateName)
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
		h.renderUserPickerPage(chatID, userID, panelMsgID, stateName)
		return nil, false
	}

	if isUserPickerPageLabel(text) {
		return nil, false
	}

	pickedUserID, ok := parseUserIDFromButton(text)
	if !ok {
		h.sendMessage(chatID, "❌ Некорректный выбор. Используйте кнопки ниже.")
		h.renderUserPickerPage(chatID, userID, panelMsgID, stateName)
		return nil, false
	}

	for _, user := range data.UsersSnapshot {
		if user.UserID == pickedUserID {
			data.SelectedUserID = pickedUserID
			return user, true
		}
	}

	h.sendMessage(chatID, "❌ Пользователь не найден в текущем списке. Выберите снова.")
	h.renderUserPickerPage(chatID, userID, panelMsgID, stateName)
	return nil, false
}

func isUserPickerPageLabel(text string) bool {
	return userPickerPageLabelPattern.MatchString(strings.TrimSpace(text))
}

func formatUserPickerButton(user *members.Member, mode UserPickerMode) string {
	if mode == UserPickerAssignWithoutRole {
		return shortenForButton(formatMemberForAssignPicker(user), 40)
	}
	return shortenForButton(formatMemberForRolePicker(user), 40)
}

func formatMemberForRolePicker(user *members.Member) string {
	role := "без роли"
	if user.Role != nil && strings.TrimSpace(*user.Role) != "" {
		role = strings.TrimSpace(*user.Role)
	}
	username := strings.TrimSpace(user.Username)
	if username != "" {
		username = strings.TrimPrefix(username, "@")
		return fmt.Sprintf("%s • @%s", role, username)
	}
	return fmt.Sprintf("%s • %d", role, user.UserID)
}

func formatMemberForAssignPicker(user *members.Member) string {
	username := strings.TrimSpace(user.Username)
	if username != "" {
		username = strings.TrimPrefix(username, "@")
		return fmt.Sprintf("@%s • %d", username, user.UserID)
	}

	name := strings.TrimSpace(strings.Join([]string{strings.TrimSpace(user.FirstName), strings.TrimSpace(user.LastName)}, " "))
	if name != "" {
		return fmt.Sprintf("%s • %d", name, user.UserID)
	}

	return fmt.Sprintf("%d", user.UserID)
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

func (h *Handler) answerCallback(callbackID, _ string) {
	if callbackID == "" {
		return
	}
	if err := h.bot.AnswerCallback(callbackID); err != nil {
		log.WithError(err).Debug("ошибка ответа на callback")
	}
}

func (h *Handler) sendMessage(chatID int64, text string) {
	h.ensureSender()

	if _, err := h.sendFn(chatID, text, nil); err != nil {
		log.WithError(err).Error("ошибка отправки сообщения")
	}
}

func (h *Handler) ensureSender() {
	if h.sendFn != nil {
		return
	}
	if h.bot != nil {
		h.sendFn = h.bot.SendMessage
		return
	}

	h.sendFn = func(chatID int64, text string, markup *models.InlineKeyboardMarkup) (int, error) {
		return 0, fmt.Errorf("send function is nil")
	}
}

func (h *Handler) editAdminScreen(chatID int64, messageID int, text string, keyboard models.InlineKeyboardMarkup) error {
	if h.editFn != nil {
		return h.editFn(chatID, messageID, text, keyboard)
	}
	if h.bot == nil || messageID <= 0 {
		return fmt.Errorf("bot or message id is not available")
	}

	return h.bot.EditMessage(chatID, messageID, text, &keyboard)
}

func classifyEditError(err error) (editErrorKind, int, string) {
	if err == nil {
		return editErrNone, 0, ""
	}

	d := strings.ToLower(err.Error())
	switch {
	case containsAny(d, editNeedlesNotModified):
		return editErrNotModified, 0, err.Error()
	case containsAny(d, editNeedlesNotFound):
		return editErrNotFound, 0, err.Error()
	case containsAny(d, editNeedlesCantBeEdited):
		return editErrCantBeEdited, 0, err.Error()
	case containsAny(d, editNeedlesForbidden):
		return editErrForbidden, 0, err.Error()
	default:
		return editErrOther, 0, err.Error()
	}
}

func containsAny(s string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
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

func (h *Handler) showKeyboardSafe(chatID, userID int64, panelMsgID int) {
	if err := h.showKeyboard(chatID, userID, panelMsgID); err != nil {
		h.sendUIErrorHint(chatID, err)
	}
}

func (h *Handler) sendUIErrorHint(chatID int64, err error) {
	kind, _, _ := classifyEditError(err)
	switch kind {
	case editErrForbidden:
		return
	case editErrNotFound:
		h.sendMessage(chatID, "⚠️ Панель устарела, попробуйте открыть снова.")
	default:
		h.sendMessage(chatID, "⚠️ Не удалось обновить панель. Попробуйте ещё раз.")
	}
}

func (h *Handler) renderAssignRoleInput(chatID, userID int64, selected *members.Member) {
	picker := h.pickerDataFromState(userID)
	roleInput := &RoleInputData{SelectedUser: selected, Picker: picker}
	h.service.SetState(userID, StateAssignRoleText, roleInput)

	text := fmt.Sprintf("Введите роль для %s (максимум 64 символа):\n%s — назад к выбору участника.", selected.DisplayName(), userPickerBackButton)
	h.renderRoleInputScreen(chatID, userID, text)
}

func (h *Handler) renderChangeRoleInput(chatID, userID int64, selected *members.Member) {
	picker := h.pickerDataFromState(userID)
	roleInput := &RoleInputData{SelectedUser: selected, Picker: picker}
	h.service.SetState(userID, StateChangeRoleText, roleInput)

	currentRole := ""
	if selected.Role != nil {
		currentRole = *selected.Role
	}
	text := fmt.Sprintf("Текущая роль: %s\nВведите новую роль:\n%s — назад к выбору участника.", currentRole, userPickerBackButton)
	h.renderRoleInputScreen(chatID, userID, text)
}

func (h *Handler) renderRoleInputScreen(chatID, userID int64, text string) {
	panelMsgID := h.panelMessageIDFromState(userID)
	if panelMsgID > 0 {
		if err := h.renderAdminScreen(chatID, userID, panelMsgID, "role_input", text, newInlineKeyboardMarkup(
			newInlineKeyboardRow(newInlineKeyboardButtonData(userPickerBackButton, cbRoleInputBack)),
		)); err != nil {
			h.sendUIErrorHint(chatID, err)
		}
		return
	}
	if err := h.renderAdminScreen(chatID, userID, 0, "role_input", text, newInlineKeyboardMarkup(
		newInlineKeyboardRow(newInlineKeyboardButtonData(userPickerBackButton, cbRoleInputBack)),
	)); err != nil {
		h.sendUIErrorHint(chatID, err)
	}
}

func (h *Handler) sendRoleChangeSuccess(chatID, userID int64, panelMsgID int, text string) {
	if err := h.renderAdminScreen(chatID, userID, panelMsgID, "role_change_success", text, h.roleChangeSuccessActionsMarkup()); err != nil {
		h.sendUIErrorHint(chatID, err)
	}
}

func (h *Handler) roleChangeSuccessActionsMarkup() models.InlineKeyboardMarkup {
	return newInlineKeyboardMarkup(
		newInlineKeyboardRow(
			newInlineKeyboardButtonDataStyled("↩️ Отменить изменение", cbAdminUndoLast, "danger"),
			newInlineKeyboardButtonDataStyled("✅ Вернуться в админ-панель", cbAdminReturnPanel, "success"),
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
		h.sendMessage(chatID, "Нет действия для отката")
		h.showKeyboardSafe(chatID, userID, panelMsgID)
		return
	}

	if err := h.service.AssignRole(ctx, undo.targetUserID, undo.oldRole); err != nil {
		h.sendMessage(chatID, fmt.Sprintf("❌ Ошибка отката: %s", err.Error()))
		return
	}

	if err := h.renderAdminScreen(chatID, userID, panelMsgID, "role_change_undo", fmt.Sprintf("↩️ Откат выполнен: %d %s → %s", undo.targetUserID, undo.newRole, undo.oldRole), h.roleChangeSuccessActionsMarkup()); err != nil {
		h.sendUIErrorHint(chatID, err)
		return
	}
	h.service.ClearState(userID)
}

func (h *Handler) renderAdminScreen(chatID, userID int64, panelMsgID int, screenName, text string, keyboard models.InlineKeyboardMarkup) error {
	h.ensureSender()

	if panelMsgID > 0 {
		err := h.editAdminScreen(chatID, panelMsgID, text, keyboard)
		kind, tgCode, tgText := classifyEditError(err)
		if kind == editErrNone || kind == editErrNotModified {
			h.attachPanelMessageID(userID, panelMsgID)
			return nil
		}

		switch kind {
		case editErrNotFound, editErrCantBeEdited:
			h.logAdminUIError(userID, chatID, panelMsgID, screenName, "edit", tgCode, tgText, err)
		case editErrForbidden, editErrOther:
			h.logAdminUIError(userID, chatID, panelMsgID, screenName, "edit", tgCode, tgText, err)
			return err
		}
	}

	sent, err := h.sendFn(chatID, text, &keyboard)
	if err != nil {
		_, tgCode, tgText := classifyEditError(err)
		h.logAdminUIError(userID, chatID, panelMsgID, screenName, "send", tgCode, tgText, err)
		return err
	}
	h.attachPanelMessageID(userID, sent)
	return nil
}

func (h *Handler) attachPanelMessageID(userID int64, panelMsgID int) {
	h.service.SetPanelMessageID(userID, panelMsgID)
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

func (h *Handler) handleRoleInputBack(chatID, userID int64, panelMsgID int) {
	state := h.service.GetState(userID)
	if state == nil {
		h.showKeyboardSafe(chatID, userID, panelMsgID)
		return
	}

	roleInput, ok := state.Data.(*RoleInputData)
	if !ok || roleInput == nil || roleInput.Picker == nil {
		h.showKeyboardSafe(chatID, userID, panelMsgID)
		return
	}

	switch state.State {
	case StateAssignRoleText:
		h.service.SetState(userID, StateAssignRoleSelect, roleInput.Picker)
		h.renderUserPickerPage(chatID, userID, panelMsgID, StateAssignRoleSelect)
	case StateChangeRoleText:
		h.service.SetState(userID, StateChangeRoleSelect, roleInput.Picker)
		h.renderUserPickerPage(chatID, userID, panelMsgID, StateChangeRoleSelect)
	default:
		h.showKeyboardSafe(chatID, userID, panelMsgID)
	}
}

func callbackMessage(q *models.CallbackQuery) *models.Message {
	if q == nil {
		return nil
	}
	return q.Message.Message
}

func shouldFallbackToSend(kind editErrorKind) bool {
	return kind == editErrNotFound || kind == editErrCantBeEdited
}

func newInlineKeyboardMarkup(rows ...[]models.InlineKeyboardButton) models.InlineKeyboardMarkup {
	return models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

func newInlineKeyboardRow(buttons ...models.InlineKeyboardButton) []models.InlineKeyboardButton {
	return buttons
}

func newInlineKeyboardButtonData(text, data string) models.InlineKeyboardButton {
	return models.InlineKeyboardButton{Text: text, CallbackData: data}
}

func newInlineKeyboardButtonDataStyled(text, data, style string) models.InlineKeyboardButton {
	b := newInlineKeyboardButtonData(text, data)
	b.Style = style
	return b
}
