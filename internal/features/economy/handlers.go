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

type handlerService interface {
	GetBalance(ctx context.Context, userID int64) (int64, error)
	Transfer(ctx context.Context, fromUserID, toUserID, amount int64) error
	GetTransactionHistory(ctx context.Context, userID int64) (string, error)
}

// Handler обрабатывает команды экономики.
type Handler struct {
	service       handlerService   // Сервис экономики
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
func (h *Handler) HandleBalance(ctx context.Context, chatID int64, userID int64, replyToMessageID int) {
	balance, err := h.service.GetBalance(ctx, userID)
	if err != nil {
		log.WithError(err).Error("Ошибка получения баланса")
		h.sendMessage(ctx, chatID, "❌ Ошибка получения баланса", replyToMessageID)
		return
	}

	text := fmt.Sprintf("У вас: %d🎞️", balance)
	h.sendMessage(ctx, chatID, text, replyToMessageID)
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
		h.sendMessage(ctx, chatID, "❌ Формат: !отсыпать @username сумма", 0)
		return
	}

	// Парсим username (убираем @ если есть)
	username := strings.TrimPrefix(args[0], "@")
	if username == "" {
		h.sendMessage(ctx, chatID, "❌ Укажите @username получателя", 0)
		return
	}

	// Парсим сумму
	amount, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil || amount <= 0 {
		h.sendMessage(ctx, chatID, "❌ Сумма должна быть положительным числом", 0)
		return
	}

	// Находим получателя по username
	recipient, err := h.memberService.GetByUsername(ctx, username)
	if err != nil {
		h.sendMessage(ctx, chatID, "❌ Пользователь не найден", 0)
		return
	}

	// Выполняем перевод
	err = h.service.Transfer(ctx, fromUserID, recipient.UserID, amount)
	if err != nil {
		switch err {
		case common.ErrSelfTransfer:
			h.sendMessage(ctx, chatID, "❌ Нельзя переводить плёнки самому себе", 0)
		case common.ErrInsufficientBalance:
			h.sendMessage(ctx, chatID, "❌ Недостаточно плёнок на счёте", 0)
		case common.ErrInvalidAmount:
			h.sendMessage(ctx, chatID, "❌ Сумма должна быть положительной", 0)
		default:
			log.WithError(err).Error("Ошибка перевода")
			h.sendMessage(ctx, chatID, "❌ Ошибка выполнения перевода", 0)
		}
		return
	}

	// Получаем новый баланс отправителя
	newBalance, _ := h.service.GetBalance(ctx, fromUserID)

	text := fmt.Sprintf("✅ Переведено %s @%s\nТвой баланс: %s",
		common.FormatBalance(amount), username, common.FormatBalance(newBalance))
	h.sendMessage(ctx, chatID, text, 0)
}

// HandleTransactions обрабатывает команду !транзакции — показывает историю.
func (h *Handler) HandleTransactions(ctx context.Context, chatID int64, userID int64) {
	history, err := h.service.GetTransactionHistory(ctx, userID)
	if err != nil {
		log.WithError(err).Error("Ошибка получения транзакций")
		h.sendMessage(ctx, chatID, "❌ Ошибка получения истории транзакций", 0)
		return
	}

	// Отправляем с MarkdownV2 для поддержки спойлеров
	h.sendMessage(ctx, chatID, history, 0)
}

// sendMessage — вспомогательный метод для отправки текстовых сообщений.
func (h *Handler) sendMessage(ctx context.Context, chatID int64, text string, replyToMessageID int) {
	_, _ = h.tgOps.SendWithOptions(ctx, telegram.SendOptions{
		ChatID:           chatID,
		Text:             text,
		ReplyToMessageID: replyToMessageID,
	})
}
