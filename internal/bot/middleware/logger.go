package middleware

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	log "github.com/sirupsen/logrus"
)

// LogMessage логирует входящее сообщение.
// message.Chat/message.From могут быть nil на service/channel updates.
func LogMessage(message *tgbotapi.Message) {
	if message == nil || message.Chat == nil || message.From == nil {
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
		"username":  message.From.UserName,
		"text":      text,
	}).Debug("Входящее сообщение")
}