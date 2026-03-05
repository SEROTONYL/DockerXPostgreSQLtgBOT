package filters

import (
	"context"
	"strings"
	"time"

	models "github.com/mymmrac/telego"
	log "github.com/sirupsen/logrus"

	"serotonyl.ru/telegram-bot/internal/bot"
	"serotonyl.ru/telegram-bot/internal/telegram"
)

type ChatFilter struct {
	floodChatID   int64
	adminChatID   int64
	memberService bot.MemberService
	tgOps         *telegram.Ops
}

func NewChatFilter(floodChatID int64, adminChatID int64, memberService bot.MemberService, ops *telegram.Ops) *ChatFilter {
	return &ChatFilter{
		floodChatID:   floodChatID,
		adminChatID:   adminChatID,
		memberService: memberService,
		tgOps:         ops,
	}
}

func (f *ChatFilter) CheckAccess(ctx context.Context, message *models.Message) bool {
	if message == nil {
		log.WithField("component", "ChatFilter").Warn("nil message/chat")
		return false
	}
	if message.From == nil {
		log.WithFields(log.Fields{
			"component": "ChatFilter",
			"chat_id":   message.Chat.ID,
			"chat_type": message.Chat.Type,
		}).Warn("nil message.From (service/channel message?)")
		return false
	}

	chatID := message.Chat.ID

	// Админ-чат — служебный контур, всегда пропускаем до любых прочих проверок.
	if chatID == f.adminChatID {
		log.WithFields(log.Fields{
			"component": "ChatFilter",
			"chat_id":   chatID,
			"chat_type": message.Chat.Type,
		}).Debug("allow: admin chat")
		return true
	}

	if f.memberService == nil {
		log.WithField("component", "ChatFilter").Error("memberService is nil")
		return false
	}
	if f.tgOps == nil {
		log.WithField("component", "ChatFilter").Error("tgOps is nil")
		return false
	}
	if f.floodChatID == 0 {
		log.WithField("component", "ChatFilter").Error("floodChatID is 0 (config bug)")
		return false
	}

	userID := message.From.ID

	logger := log.WithFields(log.Fields{
		"component":     "ChatFilter",
		"chat_id":       chatID,
		"chat_type":     message.Chat.Type,
		"user_id":       userID,
		"flood_chat_id": f.floodChatID,
	})

	// 1) Разрешённые чаты

	if chatID == f.floodChatID {
		logger.Debug("allow: flood chat")
		return true
	}

	// 2) Личка: проверяем членство через Telegram API
	if message.Chat.Type == models.ChatTypePrivate {
		cm, err := f.tgOps.GetChatMember(ctx, f.floodChatID, userID)
		if err != nil {
			logger.WithError(err).Error("member check failed (telegram GetChatMember)")
			return false
		}

		status := cm.MemberStatus()

		switch status {
		case "creator", "administrator", "member", "restricted":
			if err := f.memberService.EnsureActiveMemberSeen(
				ctx, userID,
				message.From.Username,
				buildDisplayName(message.From.FirstName, message.From.LastName),
				time.Now().UTC(),
			); err != nil {
				logger.WithError(err).Warn("failed to upsert active member in DB (access allowed despite failure)")
			}
			logger.WithField("tg_status", status).Info("allow: private (telegram member)")
			return true

		default:
			logger.WithField("tg_status", status).Info("deny: private (not a chat member)")
			if _, sendErr := f.tgOps.Send(ctx, chatID, "❌ Бот работает только для участников основного чата", nil); sendErr != nil {
				logger.WithError(sendErr).Warn("failed to send deny message")
			}
			return false
		}
	}

	// 3) Остальные чаты игнорируем
	logger.Info("deny: not flood chat and not private")
	return false
}

func buildDisplayName(firstName, lastName string) string {
	name := strings.TrimSpace(firstName)
	if ln := strings.TrimSpace(lastName); ln != "" {
		if name != "" {
			name += " "
		}
		name += ln
	}
	return name
}
