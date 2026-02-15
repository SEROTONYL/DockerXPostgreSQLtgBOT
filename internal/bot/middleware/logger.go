// Package middleware содержит промежуточные обработчики для логирования,
// восстановления после паники и rate-limiting.
package middleware

import (
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	log "github.com/sirupsen/logrus"
)

// LogMessage логирует входящее сообщение.
// Записывает: user_id, chat_id, username, текст (первые 50 символов).
func LogMessage(message *tgbotapi.Message) {
	if message == nil {
		return
	}

	text := message.Text
	if len(text) > 50 {
		text = text[:50] + "..."
	}

	log.WithFields(log.Fields{
		"user_id":  message.From.ID,
		"chat_id":  message.Chat.ID,
		"username": message.From.UserName,
		"text":     text,
		"time":     time.Now().Format("15:04:05"),
	}).Debug("Входящее сообщение")
}
