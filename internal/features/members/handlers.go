// Package members — handlers.go обрабатывает Telegram-события, связанные с участниками.
// Основное событие: новый пользователь вступил в чат (ChatMemberUpdated).
package members

import (
	"context"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	log "github.com/sirupsen/logrus"
)

// Handler обрабатывает события участников.
type Handler struct {
	service *Service // Сервис участников для бизнес-логики
}

// NewHandler создаёт новый обработчик событий участников.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// HandleNewChatMembers обрабатывает событие вступления новых пользователей.
// Telegram отправляет это событие, когда кто-то присоединяется к чату.
//
// Для каждого нового участника:
// 1. Регистрирует в таблице members
// 2. Создаёт связанные записи (баланс, стрик, карма) — через другие сервисы
func (h *Handler) HandleNewChatMembers(ctx context.Context, newMembers []tgbotapi.User) {
	for _, user := range newMembers {
		// Регистрируем каждого нового участника
		err := h.service.HandleNewMember(ctx, user.ID, user.UserName, user.FirstName, user.LastName)
		if err != nil {
			log.WithError(err).WithField("user_id", user.ID).Error("Ошибка регистрации нового участника")
		}
	}
}
