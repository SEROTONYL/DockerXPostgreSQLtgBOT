// Package economy — handlers.go обрабатывает команды:
// !пленки (баланс), !отсыпать (перевод), !транзакции (история).
package economy

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"

	"serotonyl.ru/telegram-bot/internal/common"
	"serotonyl.ru/telegram-bot/internal/features/members"
	"serotonyl.ru/telegram-bot/internal/telegram"
)

// Handler обрабатывает команды экономики.
type Handler struct {
	service       *Service         // Сервис экономики
	memberService *members.Service // Сервис участников (для поиска получателя)
	tgOps         *telegram.Ops    // Единый слой Telegram операций
}

// NewHandler создаёт новый обработчик экономических команд.
func NewHandler(service *Service, memberService *members.Service, tgOps *telegram.Ops) *Handler {
	return &Handler{
		service:       service,
		memberService: memberService,
		tgOps:         tgOps,
	}
}

// HandleBalance обрабатывает команду !пленки — показывает баланс.
//
// Формат ответа:
//
//	💰 Баланс: 150 пленок
func (h *Handler) HandleBalance(ctx context.Context, chatID int64, userID int64) {
	balance, err := h.service.GetBalance(ctx, userID)
	if err != nil {
		log.WithError(err).Error("Ошибка получения баланса")
		h.sendMessage(chatID, "❌ Ошибка получения баланса")
		return
	}

	text := fmt.Sprintf("💰 Баланс: %s", common.FormatBalance(balance))
	h.sendMessage(chatID, text)
}

// HandleTransfer обрабатывает команду !отсыпать @username 100.
// Переводит указанную сумму от отправителя к получателю.
//
// Формат: !отсыпать @username 100
// или: !отсыпать username 100 (без @)
//
// Ответ при успехе:
//
//	✅ Переведено 100 пленок @username
//	Твой баланс: 50 пленок
func (h *Handler) HandleTransfer(ctx context.Context, chatID int64, fromUserID int64, args []string) {
	// Проверяем аргументы: нужен @username и сумма
	if len(args) < 2 {
		h.sendMessage(chatID, "❌ Формат: !отсыпать @username сумма")
		return
	}

	// Парсим username (убираем @ если есть)
	username := strings.TrimPrefix(args[0], "@")
	if username == "" {
		h.sendMessage(chatID, "❌ Укажите @username получателя")
		return
	}

	// Парсим сумму
	amount, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil || amount <= 0 {
		h.sendMessage(chatID, "❌ Сумма должна быть положительным числом")
		return
	}

	// Находим получателя по username
	recipient, err := h.memberService.GetByUsername(ctx, username)
	if err != nil {
		h.sendMessage(chatID, "❌ Пользователь не найден")
		return
	}

	// Выполняем перевод
	err = h.service.Transfer(ctx, fromUserID, recipient.UserID, amount)
	if err != nil {
		switch err {
		case common.ErrSelfTransfer:
			h.sendMessage(chatID, "❌ Нельзя переводить плёнки самому себе")
		case common.ErrInsufficientBalance:
			h.sendMessage(chatID, "❌ Недостаточно плёнок на счёте")
		case common.ErrInvalidAmount:
			h.sendMessage(chatID, "❌ Сумма должна быть положительной")
		default:
			log.WithError(err).Error("Ошибка перевода")
			h.sendMessage(chatID, "❌ Ошибка выполнения перевода")
		}
		return
	}

	// Получаем новый баланс отправителя
	newBalance, _ := h.service.GetBalance(ctx, fromUserID)

	text := fmt.Sprintf("✅ Переведено %s @%s\nТвой баланс: %s",
		common.FormatBalance(amount), username, common.FormatBalance(newBalance))
	h.sendMessage(chatID, text)
}

// HandleTransactions обрабатывает команду !транзакции — показывает историю.
func (h *Handler) HandleTransactions(ctx context.Context, chatID int64, userID int64) {
	history, err := h.service.GetTransactionHistory(ctx, userID)
	if err != nil {
		log.WithError(err).Error("Ошибка получения транзакций")
		h.sendMessage(chatID, "❌ Ошибка получения истории транзакций")
		return
	}

	// Отправляем с MarkdownV2 для поддержки спойлеров
	h.sendMessage(chatID, history)
}

// sendMessage — вспомогательный метод для отправки текстовых сообщений.
func (h *Handler) sendMessage(chatID int64, text string) {
	if _, err := h.tgOps.Send(context.Background(), chatID, text, nil); err != nil {
		log.WithError(err).Error("Ошибка отправки сообщения")
	}
}
