package middleware

import (
	"github.com/go-telegram/bot/models"
	log "github.com/sirupsen/logrus"
)

// LogMessage логирует входящее сообщение.
// message.Chat/message.From могут быть nil на service/channel updates.
func LogMessage(message *models.Message) {
	if message == nil || message.From == nil {
		return
	}

	text := message.Text
	if len(text) > 80 {
		text = text[:80] + "..."
	}

	log.WithFields(log.Fields{
		"user_id":   message.From.ID,
		"chat_id":   message.Chat.ID,
		"chat_type": message.Chat.Type,
		"username":  message.From.Username,
		"text":      text,
	}).Debug("Входящее сообщение")
}
