// Package admin — handlers.go обрабатывает взаимодействие с админ-панелью.
// Панель работает через Reply Keyboard в личных сообщениях.
// Поток: аутентификация → клавиатура → выбор действия → пошаговый диалог.
package admin

import (
	"context"
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
)

var userPickerIDPattern = regexp.MustCompile(`(?i)(?:id:|#)(\d+)`)

// Handler обрабатывает админ-команды.
type Handler struct {
	service       *Service
	memberService *members.Service
	bot           *tgbotapi.BotAPI
	sendFn        func(c tgbotapi.Chattable) (tgbotapi.Message, error)
}

// NewHandler создаёт обработчик админ-панели.
func NewHandler(service *Service, memberService *members.Service, bot *tgbotapi.BotAPI) *Handler {
	return &Handler{
		service:       service,
		memberService: memberService,
		bot:           bot,
		sendFn:        bot.Send,
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
			h.showKeyboard(chatID)
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
		h.startAssignRole(ctx, chatID, userID)
		return true
	case "Сменить роль":
		h.startChangeRole(ctx, chatID, userID)
		return true
	case "Выдать плёнки", "Отнять плёнки", "Выдать кредит",
		"Аннулировать кредит", "Создать сокращение", "Удалить сокращение":
		h.sendMessage(chatID, "🔧 Функция в разработке")
		return true
	case "Админ", "Панель", "админ", "панель":
		h.showKeyboard(chatID)
		return true
	}

	return false
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
	h.showKeyboard(chatID)
}

// showKeyboard отображает клавиатуру админ-панели.
func (h *Handler) showKeyboard(chatID int64) {
	h.ensureSender()

	keyboard := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("Назначить роль"),
			tgbotapi.NewKeyboardButton("Сменить роль"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("Выдать плёнки"),
			tgbotapi.NewKeyboardButton("Отнять плёнки"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("Выдать кредит"),
			tgbotapi.NewKeyboardButton("Аннулировать кредит"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("Создать сокращение"),
			tgbotapi.NewKeyboardButton("Удалить сокращение"),
		),
	)

	msg := tgbotapi.NewMessage(chatID, "✅ Админ-панель открыта")
	msg.ReplyMarkup = keyboard
	if _, err := h.sendFn(msg); err != nil {
		log.WithError(err).Error("ошибка отправки клавиатуры")
	}
}

// --- Назначить роль (3 шага) ---

// startAssignRole — Шаг 1: показать пользователей БЕЗ роли.
func (h *Handler) startAssignRole(ctx context.Context, chatID int64, userID int64) {
	users, err := h.service.GetUsersWithoutRole(ctx)
	if err != nil || len(users) == 0 {
		h.sendMessage(chatID, "Все пользователи уже имеют роли")
		return
	}

	h.startUserPicker(chatID, userID, StateAssignRoleSelect, UserPickerAssignWithoutRole, users)
}

// handleAssignRoleSelect — Шаг 2: пользователь выбрал кнопку.
func (h *Handler) handleAssignRoleSelect(ctx context.Context, chatID int64, userID int64, text string) {
	selected, ok := h.handleUserPickerInput(chatID, userID, StateAssignRoleSelect, text)
	if !ok {
		return
	}

	h.sendMessage(chatID, fmt.Sprintf("Введите роль для %s (максимум 64 символа):", selected.DisplayName()))
	h.service.SetState(userID, StateAssignRoleText, selected)
}

// handleAssignRoleText — Шаг 3: ввод текста роли.
func (h *Handler) handleAssignRoleText(ctx context.Context, chatID int64, userID int64, text string) {
	state := h.service.GetState(userID)
	if state == nil {
		h.sendMessage(chatID, "⚠️ Состояние сброшено. Вернитесь в админ-меню.")
		h.service.ClearState(userID)
		h.showKeyboard(chatID)
		return
	}

	selected, ok := state.Data.(*members.Member)
	if !ok || selected == nil {
		h.sendMessage(chatID, "⚠️ Состояние сброшено. Вернитесь в админ-меню.")
		h.service.ClearState(userID)
		h.showKeyboard(chatID)
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
		return
	}

	h.sendMessage(chatID, fmt.Sprintf("✅ Роль назначена: %s → %s", selected.DisplayName(), role))
	h.service.ClearState(userID)
}

// --- Сменить роль (3 шага) ---

func (h *Handler) startChangeRole(ctx context.Context, chatID int64, userID int64) {
	users, err := h.service.GetUsersWithRole(ctx)
	if err != nil || len(users) == 0 {
		h.sendMessage(chatID, "Нет пользователей с назначенными ролями")
		return
	}

	h.startUserPicker(chatID, userID, StateChangeRoleSelect, UserPickerChangeWithRole, users)
}

func (h *Handler) handleChangeRoleSelect(ctx context.Context, chatID int64, userID int64, text string) {
	selected, ok := h.handleUserPickerInput(chatID, userID, StateChangeRoleSelect, text)
	if !ok {
		return
	}

	currentRole := ""
	if selected.Role != nil {
		currentRole = *selected.Role
	}
	h.sendMessage(chatID, fmt.Sprintf("Текущая роль: %s\nВведите новую роль:", currentRole))
	h.service.SetState(userID, StateChangeRoleText, selected)
}

func (h *Handler) handleChangeRoleText(ctx context.Context, chatID int64, userID int64, text string) {
	state := h.service.GetState(userID)
	if state == nil {
		h.sendMessage(chatID, "⚠️ Состояние сброшено. Вернитесь в админ-меню.")
		h.service.ClearState(userID)
		h.showKeyboard(chatID)
		return
	}

	selected, ok := state.Data.(*members.Member)
	if !ok || selected == nil {
		h.sendMessage(chatID, "⚠️ Состояние сброшено. Вернитесь в админ-меню.")
		h.service.ClearState(userID)
		h.showKeyboard(chatID)
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
		return
	}

	h.sendMessage(chatID, fmt.Sprintf("✅ Роль изменена: %s → %s", selected.DisplayName(), role))
	h.service.ClearState(userID)
}

func (h *Handler) startUserPicker(chatID, userID int64, stateName string, mode UserPickerMode, users []*members.Member) {
	data := &UserPickerData{
		Mode:          mode,
		UsersSnapshot: users,
		PageIndex:     0,
		PageSize:      userPickerPageSize,
	}
	h.service.SetState(userID, stateName, data)
	h.renderUserPickerPage(chatID, userID, stateName)
}

func (h *Handler) renderUserPickerPage(chatID, userID int64, stateName string) {
	state := h.service.GetState(userID)
	if state == nil || state.State != stateName {
		h.sendMessage(chatID, "⚠️ Состояние сброшено. Вернитесь в админ-меню.")
		h.service.ClearState(userID)
		h.showKeyboard(chatID)
		return
	}

	data, ok := state.Data.(*UserPickerData)
	if !ok || data == nil || len(data.UsersSnapshot) == 0 {
		h.sendMessage(chatID, "⚠️ Список пользователей недоступен. Вернитесь в админ-меню.")
		h.service.ClearState(userID)
		h.showKeyboard(chatID)
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

	rows := make([][]tgbotapi.KeyboardButton, 0, (end-start)+2)
	for _, user := range data.UsersSnapshot[start:end] {
		rows = append(rows, tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(formatUserPickerButton(user)),
		))
	}

	pageLabel := fmt.Sprintf("Стр %d/%d", data.PageIndex+1, totalPages)
	rows = append(rows, tgbotapi.NewKeyboardButtonRow(
		tgbotapi.NewKeyboardButton(userPickerPrevButton),
		tgbotapi.NewKeyboardButton(pageLabel),
		tgbotapi.NewKeyboardButton(userPickerNextButton),
	))
	rows = append(rows, tgbotapi.NewKeyboardButtonRow(
		tgbotapi.NewKeyboardButton(userPickerBackButton),
	))

	keyboard := tgbotapi.NewReplyKeyboard(rows...)

	caption := "Выбери участника:"
	if data.Mode == UserPickerAssignWithoutRole {
		caption = "Выбери участника без роли:"
	} else if data.Mode == UserPickerChangeWithRole {
		caption = "Выбери участника с ролью:"
	}
	caption = fmt.Sprintf("%s\nПоказаны имена в кнопках (id — точный идентификатор).", caption)

	h.ensureSender()
	msg := tgbotapi.NewMessage(chatID, caption)
	msg.ReplyMarkup = keyboard
	if _, err := h.sendFn(msg); err != nil {
		log.WithError(err).Error("ошибка отправки user picker")
	}
}

func (h *Handler) handleUserPickerInput(chatID, userID int64, stateName, text string) (*members.Member, bool) {
	state := h.service.GetState(userID)
	if state == nil || state.State != stateName {
		h.sendMessage(chatID, "⚠️ Состояние сброшено. Вернитесь в админ-меню.")
		h.service.ClearState(userID)
		h.showKeyboard(chatID)
		return nil, false
	}

	data, ok := state.Data.(*UserPickerData)
	if !ok || data == nil || len(data.UsersSnapshot) == 0 {
		h.sendMessage(chatID, "⚠️ Список пользователей недоступен. Вернитесь в админ-меню.")
		h.service.ClearState(userID)
		h.showKeyboard(chatID)
		return nil, false
	}

	switch text {
	case userPickerBackButton:
		h.service.ClearState(userID)
		h.showKeyboard(chatID)
		return nil, false
	case userPickerPrevButton:
		if data.PageIndex > 0 {
			data.PageIndex--
		}
		h.renderUserPickerPage(chatID, userID, stateName)
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
		h.renderUserPickerPage(chatID, userID, stateName)
		return nil, false
	}

	pickedUserID, ok := parseUserIDFromButton(text)
	if !ok {
		h.sendMessage(chatID, "❌ Некорректный выбор. Используйте кнопки ниже.")
		h.renderUserPickerPage(chatID, userID, stateName)
		return nil, false
	}

	for _, user := range data.UsersSnapshot {
		if user.UserID == pickedUserID {
			data.SelectedUserID = pickedUserID
			return user, true
		}
	}

	h.sendMessage(chatID, "❌ Пользователь не найден в текущем списке. Выберите снова.")
	h.renderUserPickerPage(chatID, userID, stateName)
	return nil, false
}

func formatUserPickerButton(user *members.Member) string {
	display := user.DisplayName()
	if user.FirstName != "" && !strings.HasPrefix(display, "@") {
		display = user.FirstName
	}
	display = shortenForButton(display, 28)
	return fmt.Sprintf("👤 %s · id:%d", display, user.UserID)
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
