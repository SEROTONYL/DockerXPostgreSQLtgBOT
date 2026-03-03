// Package karma — handlers.go обрабатывает команду !карма и «спасибо».
package karma

import (
	"context"
	"fmt"

	log "github.com/sirupsen/logrus"

	"serotonyl.ru/telegram-bot/internal/telegram"
)

// Handler обрабатывает события кармы.
type Handler struct {
	service *Service
	tgOps   *telegram.Ops
}

// NewHandler создаёт обработчик кармы.
func NewHandler(service *Service, tgOps *telegram.Ops) *Handler {
	return &Handler{service: service, tgOps: tgOps}
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
	if _, err := h.tgOps.Send(context.Background(), chatID, text, nil); err != nil {
		log.WithError(err).Error("Ошибка отправки сообщения")
	}
}
