package filters

import (
	"context"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	log "github.com/sirupsen/logrus"

	"serotonyl.ru/telegram-bot/internal/features/members"
)

type ChatFilter struct {
	floodChatID   int64
	memberService *members.Service
	bot           *tgbotapi.BotAPI
}

func NewChatFilter(floodChatID int64, memberService *members.Service, bot *tgbotapi.BotAPI) *ChatFilter {
	return &ChatFilter{
		floodChatID:   floodChatID,
		memberService: memberService,
		bot:           bot,
	}
}

func (f *ChatFilter) CheckAccess(ctx context.Context, message *tgbotapi.Message) bool {
	if message == nil || message.Chat == nil {
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
	if f.memberService == nil {
		log.WithField("component", "ChatFilter").Error("memberService is nil")
		return false
	}
	if f.bot == nil {
		log.WithField("component", "ChatFilter").Error("bot is nil")
		return false
	}
	if f.floodChatID == 0 {
		log.WithField("component", "ChatFilter").Error("floodChatID is 0 (config bug)")
		return false
	}

	chatID := message.Chat.ID
	userID := message.From.ID

	logger := log.WithFields(log.Fields{
		"component":     "ChatFilter",
		"chat_id":       chatID,
		"chat_type":     message.Chat.Type,
		"user_id":       userID,
		"flood_chat_id": f.floodChatID,
	})

	// 1) Разрешённый чат
	if chatID == f.floodChatID {
		logger.Debug("allow: flood chat")
		return true
	}

	// 2) Личка: сначала быстро по БД
	if message.Chat.IsPrivate() {
		isMember, err := f.memberService.IsMember(ctx, userID)
		if err != nil {
			logger.WithError(err).Error("member check failed (db)")
			return false
		}
		if isMember {
			logger.Debug("allow: private (db member)")
			return true
		}

		// 2.1) БД не знает пользователя: проверяем членство через Telegram API
		cm, err := f.bot.GetChatMember(tgbotapi.GetChatMemberConfig{
			ChatConfigWithUser: tgbotapi.ChatConfigWithUser{
				ChatID: f.floodChatID,
				UserID: userID, // <-- ВАЖНО: int64, без int(...)
			},
		})
		if err != nil {
			logger.WithError(err).Error("member check failed (telegram GetChatMember)")
			return false
		}

		switch cm.Status {
		case "creator", "administrator", "member", "restricted":
			if err := f.memberService.EnsureMember(
				ctx, userID,
				message.From.UserName,
				message.From.FirstName,
				message.From.LastName,
			); err != nil {
				logger.WithError(err).Warn("failed to backfill member to DB (allowing anyway)")
			}
			logger.WithField("tg_status", cm.Status).Info("allow: private (telegram member, backfilled)")
			return true

		default:
			logger.WithField("tg_status", cm.Status).Info("deny: private (not a chat member)")
			msg := tgbotapi.NewMessage(chatID, "❌ Бот работает только для участников основного чата")
			if _, sendErr := f.bot.Send(msg); sendErr != nil {
				logger.WithError(sendErr).Warn("failed to send deny message")
			}
			return false
		}
	}

	// 3) Остальные чаты игнорируем
	logger.Info("deny: not flood chat and not private")
	return false
}
