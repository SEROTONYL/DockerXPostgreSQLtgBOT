// Package admin — handlers.go обрабатывает взаимодействие с админ-панелью.
// Панель работает через Reply Keyboard в личных сообщениях.
// Поток: аутентификация → клавиатура → выбор действия → пошаговый диалог.
package admin

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	log "github.com/sirupsen/logrus"

	"serotonyl.ru/telegram-bot/internal/features/members"
)

const (
	userPickerPrevButton = "◀️"
	userPickerNextButton = "▶️"
	userPickerBackButton = "⬅️ Назад"
	userPickerPageSize   = 8

	cbAdminAssignRole = "admin:assign_role"
	cbAdminChangeRole = "admin:change_role"
	cbAdminStub       = "admin:stub"
	cbPickerPrefix    = "admin:picker:"
	cbPickerSelect    = "select"
	cbPickerPrev      = "prev"
	cbPickerNext      = "next"
	cbPickerBack      = "back"
	cbRoleInputBack   = "admin:role_input_back"
)

var userPickerIDPattern = regexp.MustCompile(`(?i)(?:id:|#)(\d+)`)
var userPickerPageLabelPattern = regexp.MustCompile(`(?i)^\s*стр\s*\d+\s*/\s*\d+\s*$`)

var editNeedlesNotModified = []string{"message is not modified"}
var editNeedlesNotFound = []string{"message to edit not found", "message not found"}
var editNeedlesForbidden = []string{"bot was blocked by the user", "chat not found", "forbidden", "not enough rights"}

// Handler обрабатывает админ-команды.
type Handler struct {
	service       *Service
	memberService *members.Service
	bot           *tgbotapi.BotAPI
	sendFn        func(c tgbotapi.Chattable) (tgbotapi.Message, error)
	editFn        func(chatID int64, messageID int, text string, keyboard tgbotapi.InlineKeyboardMarkup) error
}

type editErrorKind string

const (
	editErrNone        editErrorKind = "none"
	editErrNotModified editErrorKind = "not_modified"
	editErrNotFound    editErrorKind = "not_found"
	editErrForbidden   editErrorKind = "forbidden"
	editErrOther       editErrorKind = "other"
)

// NewHandler создаёт обработчик админ-панели.
func NewHandler(service *Service, memberService *members.Service, bot *tgbotapi.BotAPI) *Handler {
	return &Handler{
		service:       service,
		memberService: memberService,
		bot:           bot,
		sendFn:        bot.Send,
		editFn:        nil,
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
	case "Выдать плёнки", "Отнять плёнки", "Выдать кредит",
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
func (h *Handler) HandleAdminCallback(ctx context.Context, q *tgbotapi.CallbackQuery) bool {
	if q == nil || q.From == nil || q.Message == nil {
		return false
	}

	chatID := q.Message.Chat.ID
	userID := q.From.ID
	data := q.Data
	panelMsgID := q.Message.MessageID
	h.attachPanelMessageID(userID, panelMsgID)

	if !h.service.CanEnterAdmin(ctx, userID) {
		h.answerCallback(q.ID, "❌ Доступ запрещён")
		return true
	}

	if !h.service.HasActiveSession(ctx, userID) {
		h.answerCallback(q.ID, "Сессия истекла")
		h.sendMessage(chatID, "🔐 Введите пароль для доступа к админ-панели:")
		h.service.SetState(userID, StateAwaitingPassword, nil)
		return true
	}

	if err := h.service.repo.UpdateActivity(ctx, userID); err != nil {
		log.WithError(err).WithField("user_id", userID).Warn("ошибка обновления активности админ-сессии")
	}

	switch data {
	case cbAdminAssignRole:
		h.answerCallback(q.ID, "")
		h.startAssignRole(ctx, chatID, userID, panelMsgID)
		return true
	case cbAdminChangeRole:
		h.answerCallback(q.ID, "")
		h.startChangeRole(ctx, chatID, userID, panelMsgID)
		return true
	case cbAdminStub:
		h.answerCallback(q.ID, "Функция в разработке")
		return true
	case cbRoleInputBack:
		h.answerCallback(q.ID, "")
		h.handleRoleInputBack(chatID, userID, panelMsgID)
		return true
	}

	if strings.HasPrefix(data, cbPickerPrefix) {
		h.answerCallback(q.ID, "")
		h.handleUserPickerCallback(chatID, userID, panelMsgID, data)
		return true
	}

	h.answerCallback(q.ID, "Неизвестная кнопка")
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

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Назначить роль", cbAdminAssignRole),
			tgbotapi.NewInlineKeyboardButtonData("Сменить роль", cbAdminChangeRole),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Выдать плёнки", cbAdminStub),
			tgbotapi.NewInlineKeyboardButtonData("Отнять плёнки", cbAdminStub),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Выдать кредит", cbAdminStub),
			tgbotapi.NewInlineKeyboardButtonData("Аннулировать кредит", cbAdminStub),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("Создать сокращение", cbAdminStub),
			tgbotapi.NewInlineKeyboardButtonData("Удалить сокращение", cbAdminStub),
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

	h.sendMessage(chatID, fmt.Sprintf("✅ Роль назначена: %s → %s", selected.DisplayName(), role))
	h.service.ClearState(userID)
	h.showKeyboardSafe(chatID, userID, h.panelMessageIDFromState(userID))
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

	if err := h.service.AssignRole(ctx, selected.UserID, role); err != nil {
		h.sendMessage(chatID, fmt.Sprintf("❌ Ошибка: %s", err.Error()))
		h.service.ClearState(userID)
		h.showKeyboardSafe(chatID, userID, h.panelMessageIDFromState(userID))
		return
	}

	h.sendMessage(chatID, fmt.Sprintf("✅ Роль изменена: %s → %s", selected.DisplayName(), role))
	h.service.ClearState(userID)
	h.showKeyboardSafe(chatID, userID, h.panelMessageIDFromState(userID))
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

	rows := make([][]tgbotapi.InlineKeyboardButton, 0, (end-start)+2)
	for _, user := range data.UsersSnapshot[start:end] {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(formatUserPickerButton(user), pickerCallbackData(data.Mode, cbPickerSelect, user.UserID)),
		))
	}

	pageLabel := fmt.Sprintf("Стр %d/%d", data.PageIndex+1, totalPages)
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(userPickerPrevButton, pickerCallbackData(data.Mode, cbPickerPrev, 0)),
		tgbotapi.NewInlineKeyboardButtonData(pageLabel, pickerCallbackData(data.Mode, "page", 0)),
		tgbotapi.NewInlineKeyboardButtonData(userPickerNextButton, pickerCallbackData(data.Mode, cbPickerNext, 0)),
	))
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData(userPickerBackButton, pickerCallbackData(data.Mode, cbPickerBack, 0)),
	))

	keyboard := tgbotapi.NewInlineKeyboardMarkup(rows...)

	caption := "Выбери участника:"
	if data.Mode == UserPickerAssignWithoutRole {
		caption = "Выбери участника без роли:"
	} else if data.Mode == UserPickerChangeWithRole {
		caption = "Выбери участника с ролью:"
	}
	caption = fmt.Sprintf("%s\nФормат: [роль] @username, иначе [роль] [id].", caption)

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

func formatUserPickerButton(user *members.Member) string {
	return shortenForButton(formatMemberForPicker(user), 40)
}

func formatMemberForPicker(user *members.Member) string {
	role := "БЕЗ РОЛИ"
	if user.Role != nil && strings.TrimSpace(*user.Role) != "" {
		role = strings.TrimSpace(*user.Role)
	}
	username := strings.TrimSpace(user.Username)
	if username != "" {
		username = strings.TrimPrefix(username, "@")
		return fmt.Sprintf("[%s] @%s", role, username)
	}
	return fmt.Sprintf("[%s] [%d]", role, user.UserID)
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

func (h *Handler) answerCallback(callbackID, text string) {
	if h.bot == nil || callbackID == "" {
		return
	}
	cb := tgbotapi.NewCallback(callbackID, text)
	if _, err := h.bot.Request(cb); err != nil {
		log.WithError(err).Debug("ошибка ответа на callback")
	}
}

func (h *Handler) sendMessage(chatID int64, text string) {
	h.ensureSender()

	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := h.sendFn(msg); err != nil {
		log.WithError(err).Error("ошибка отправки сообщения")
	}
}

func (h *Handler) ensureSender() {
	if h.sendFn != nil {
		return
	}
	if h.bot != nil {
		h.sendFn = h.bot.Send
		return
	}

	h.sendFn = func(c tgbotapi.Chattable) (tgbotapi.Message, error) {
		return tgbotapi.Message{}, fmt.Errorf("send function is nil")
	}
}

func (h *Handler) editAdminScreen(chatID int64, messageID int, text string, keyboard tgbotapi.InlineKeyboardMarkup) error {
	if h.editFn != nil {
		return h.editFn(chatID, messageID, text, keyboard)
	}
	if h.bot == nil || messageID <= 0 {
		return fmt.Errorf("bot or message id is not available")
	}

	cfg := tgbotapi.NewEditMessageText(chatID, messageID, text)
	cfg.ReplyMarkup = &keyboard
	if _, err := h.bot.Send(cfg); err != nil {
		return err
	}
	return nil
}

func classifyEditError(err error) (editErrorKind, int, string) {
	if err == nil {
		return editErrNone, 0, ""
	}

	var tgErr *tgbotapi.Error
	if errors.As(err, &tgErr) {
		d := strings.ToLower(tgErr.Message)
		switch {
		case containsAny(d, editNeedlesNotModified):
			return editErrNotModified, tgErr.Code, tgErr.Message
		case containsAny(d, editNeedlesNotFound):
			return editErrNotFound, tgErr.Code, tgErr.Message
		case containsAny(d, editNeedlesForbidden):
			return editErrForbidden, tgErr.Code, tgErr.Message
		default:
			return editErrOther, tgErr.Code, tgErr.Message
		}
	}

	d := strings.ToLower(err.Error())
	switch {
	case containsAny(d, editNeedlesNotModified):
		return editErrNotModified, 0, err.Error()
	case containsAny(d, editNeedlesNotFound):
		return editErrNotFound, 0, err.Error()
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
		if err := h.renderAdminScreen(chatID, userID, panelMsgID, "role_input", text, tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData(userPickerBackButton, cbRoleInputBack)),
		)); err != nil {
			h.sendUIErrorHint(chatID, err)
		}
		return
	}
	if err := h.renderAdminScreen(chatID, userID, 0, "role_input", text, tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(tgbotapi.NewInlineKeyboardButtonData(userPickerBackButton, cbRoleInputBack)),
	)); err != nil {
		h.sendUIErrorHint(chatID, err)
	}
}

func (h *Handler) renderAdminScreen(chatID, userID int64, panelMsgID int, screenName, text string, keyboard tgbotapi.InlineKeyboardMarkup) error {
	h.ensureSender()

	if panelMsgID > 0 {
		err := h.editAdminScreen(chatID, panelMsgID, text, keyboard)
		if err == nil {
			h.attachPanelMessageID(userID, panelMsgID)
			return nil
		}

		kind, tgCode, tgText := classifyEditError(err)
		switch kind {
		case editErrNotModified:
			return nil
		case editErrNotFound:
			h.logAdminUIError(userID, chatID, panelMsgID, screenName, "edit", tgCode, tgText, err)
		case editErrForbidden, editErrOther:
			h.logAdminUIError(userID, chatID, panelMsgID, screenName, "edit", tgCode, tgText, err)
			return err
		}
	}

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = keyboard
	sent, err := h.sendFn(msg)
	if err != nil {
		_, tgCode, tgText := classifyEditError(err)
		h.logAdminUIError(userID, chatID, panelMsgID, screenName, "send", tgCode, tgText, err)
		return err
	}
	h.attachPanelMessageID(userID, sent.MessageID)
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
