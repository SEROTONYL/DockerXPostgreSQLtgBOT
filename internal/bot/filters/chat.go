package filters

import (
	"context"

	"github.com/go-telegram/bot/models"
	log "github.com/sirupsen/logrus"

	"serotonyl.ru/telegram-bot/internal/telegram"
)

type MemberService interface {
	IsMember(ctx context.Context, userID int64) (bool, error)
	EnsureMember(ctx context.Context, userID int64, username, firstName, lastName string) error
}

type ChatFilter struct {
	floodChatID   int64
	adminChatID   int64
	memberService MemberService
	tgOps         *telegram.Ops
}

func NewChatFilter(floodChatID int64, adminChatID int64, memberService MemberService, ops *telegram.Ops) *ChatFilter {
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

	// 2) Личка: сначала быстро по БД
	if message.Chat.Type == models.ChatTypePrivate {
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
		cm, err := f.tgOps.GetChatMember(ctx, f.floodChatID, userID)
		if err != nil {
			logger.WithError(err).Error("member check failed (telegram GetChatMember)")
			return false
		}

		switch cm.Type {
		case models.ChatMemberTypeOwner, models.ChatMemberTypeAdministrator, models.ChatMemberTypeMember, models.ChatMemberTypeRestricted:
			if err := f.memberService.EnsureMember(
				ctx, userID,
				message.From.Username,
				message.From.FirstName,
				message.From.LastName,
			); err != nil {
				logger.WithError(err).Warn("failed to backfill member to DB (access allowed despite failure)")
			}
			logger.WithField("tg_status", cm.Type).Info("allow: private (telegram member, backfilled)")
			return true

		default:
			logger.WithField("tg_status", cm.Type).Info("deny: private (not a chat member)")
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
