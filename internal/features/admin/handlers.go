// Package admin — handlers.go обрабатывает взаимодействие с админ-панелью.
// Панель работает через Reply Keyboard в личных сообщениях.
// Поток: аутентификация → клавиатура → выбор действия → пошаговый диалог.
package admin

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	log "github.com/sirupsen/logrus"

	"telegram-bot/internal/features/members"
)

// Handler обрабатывает админ-команды.
type Handler struct {
	service       *Service
	memberService *members.Service
	bot           *tgbotapi.BotAPI
}

// NewHandler создаёт обработчик админ-панели.
func NewHandler(service *Service, memberService *members.Service, bot *tgbotapi.BotAPI) *Handler {
	return &Handler{
		service:       service,
		memberService: memberService,
		bot:           bot,
	}
}

// HandleAdminMessage обрабатывает любое сообщение от администратора в DM.
// Определяет текущее состояние диалога и маршрутизирует сообщение.
func (h *Handler) HandleAdminMessage(ctx context.Context, chatID int64, userID int64, text string) bool {
	// Проверяем, является ли пользователь админом
	member, err := h.memberService.GetByUserID(ctx, userID)
	if err != nil || !member.IsAdmin {
		return false // Не админ
	}

	// Проверяем состояние диалога
	state := h.service.GetState(userID)

	// Обрабатываем состояние ожидания пароля
	if state != nil && state.State == StateAwaitingPassword {
		h.handlePasswordInput(ctx, chatID, userID, text)
		return true
	}

	// Проверяем активную сессию
	if !h.service.HasActiveSession(ctx, userID) {
		// Нет сессии — запрашиваем пароль
		h.sendMessage(chatID, "🔐 Введите пароль для доступа к админ-панели:")
		h.service.SetState(userID, StateAwaitingPassword, nil)
		return true
	}

	// Обновляем активность сессии
	h.service.repo.UpdateActivity(ctx, userID)

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
	if _, err := h.bot.Send(msg); err != nil {
		log.WithError(err).Error("Ошибка отправки клавиатуры")
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

	var sb strings.Builder
	sb.WriteString("Выберите пользователя (отправьте номер):\n\n")
	for i, user := range users {
		name := user.DisplayName()
		sb.WriteString(fmt.Sprintf("%d. %s (%s)\n", i+1, name, user.FirstName))
	}

	h.sendMessage(chatID, sb.String())
	h.service.SetState(userID, StateAssignRoleSelect, users)
}

// handleAssignRoleSelect — Шаг 2: пользователь выбрал номер.
func (h *Handler) handleAssignRoleSelect(ctx context.Context, chatID int64, userID int64, text string) {
	state := h.service.GetState(userID)
	users := state.Data.([]*members.Member)

	num, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || num < 1 || num > len(users) {
		h.sendMessage(chatID, "❌ Неверный номер. Попробуйте ещё раз.")
		return
	}

	selected := users[num-1]
	h.sendMessage(chatID, fmt.Sprintf("Введите роль для %s (максимум 64 символа):", selected.DisplayName()))
	h.service.SetState(userID, StateAssignRoleText, selected)
}

// handleAssignRoleText — Шаг 3: ввод текста роли.
func (h *Handler) handleAssignRoleText(ctx context.Context, chatID int64, userID int64, text string) {
	state := h.service.GetState(userID)
	selected := state.Data.(*members.Member)

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

	var sb strings.Builder
	sb.WriteString("Выберите пользователя (отправьте номер):\n\n")
	for i, user := range users {
		role := ""
		if user.Role != nil {
			role = *user.Role
		}
		sb.WriteString(fmt.Sprintf("%d. %s - %s\n", i+1, user.DisplayName(), role))
	}

	h.sendMessage(chatID, sb.String())
	h.service.SetState(userID, StateChangeRoleSelect, users)
}

func (h *Handler) handleChangeRoleSelect(ctx context.Context, chatID int64, userID int64, text string) {
	state := h.service.GetState(userID)
	users := state.Data.([]*members.Member)

	num, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || num < 1 || num > len(users) {
		h.sendMessage(chatID, "❌ Неверный номер")
		return
	}

	selected := users[num-1]
	currentRole := ""
	if selected.Role != nil {
		currentRole = *selected.Role
	}
	h.sendMessage(chatID, fmt.Sprintf("Текущая роль: %s\nВведите новую роль:", currentRole))
	h.service.SetState(userID, StateChangeRoleText, selected)
}

func (h *Handler) handleChangeRoleText(ctx context.Context, chatID int64, userID int64, text string) {
	state := h.service.GetState(userID)
	selected := state.Data.(*members.Member)

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

func (h *Handler) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := h.bot.Send(msg); err != nil {
		log.WithError(err).Error("Ошибка отправки сообщения")
	}
}
