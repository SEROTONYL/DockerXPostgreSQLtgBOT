// Package karma — handlers.go обрабатывает команду !карма и «спасибо».
package karma

import (
	"context"
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	log "github.com/sirupsen/logrus"
)

// Handler обрабатывает события кармы.
type Handler struct {
	service *Service
	bot     *tgbotapi.BotAPI
}

// NewHandler создаёт обработчик кармы.
func NewHandler(service *Service, bot *tgbotapi.BotAPI) *Handler {
	return &Handler{service: service, bot: bot}
}

// HandleKarma — команда !карма. Показывает ТОЛЬКО свою карму.
func (h *Handler) HandleKarma(ctx context.Context, chatID int64, userID int64) {
	karma, err := h.service.GetKarma(ctx, userID)
	if err != nil {
		log.WithError(err).Error("Ошибка получения кармы")
		h.sendMessage(chatID, "❌ Ошибка получения кармы")
		return
	}
	h.sendMessage(chatID, fmt.Sprintf("⭐ Твоя карма: %d", karma))
}

// HandleThankYou обрабатывает «спасибо» в ответе на сообщение.
func (h *Handler) HandleThankYou(ctx context.Context, chatID int64, fromUserID, toUserID int64) {
	err := h.service.GiveKarma(ctx, fromUserID, toUserID)
	if err != nil {
		log.WithError(err).Debug("Карма не дана")
		return
	}
	h.sendMessage(chatID, "⭐ +1 к карме!")
}

func (h *Handler) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := h.bot.Send(msg); err != nil {
		log.WithError(err).Error("Ошибка отправки сообщения")
	}
}
