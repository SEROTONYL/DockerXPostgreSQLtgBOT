package bot

import (
	"context"
	"time"

	models "github.com/mymmrac/telego"
	log "github.com/sirupsen/logrus"

	"serotonyl.ru/telegram-bot/internal/bot/middleware"
)

func (b *Bot) shouldTouchLastSeen(uc UpdateContext) bool {
	if uc.IsAdminChat || uc.UserID == 0 || uc.ChatMember != nil {
		return false
	}
	if uc.ChatID == b.cfg.MainGroupID && (uc.Message != nil || uc.Callback != nil) {
		return true
	}
	return false
}

// handleUpdate обрабатывает одно обновление от Telegram.
func (b *Bot) handleUpdate(ctx context.Context, update models.Update) {
	defer middleware.RecoverFromPanic()

	uc := BuildUpdateContext(update, time.Now().UTC(), b.cfg)

	if uc.IsAdminChat {
		b.handleAdminChatUpdate(ctx, uc)
		return
	}

	if b.handleMembershipUpdate(ctx, uc) {
		return
	}

	if b.handleCallbackUpdate(ctx, uc) {
		return
	}

	b.handleMessageUpdate(ctx, uc)
}

func (b *Bot) handleAdminChatUpdate(ctx context.Context, uc UpdateContext) {
	if uc.Message == nil {
		return
	}
	cmd, args, isCommand := b.parser.ParseCommand(uc.Message.Text)
	if isCommand && isAdminChatAllowedCommand(cmd) {
		b.routeCommand(ctx, uc, cmd, args)
	}
}

func (b *Bot) handleCallbackUpdate(ctx context.Context, uc UpdateContext) bool {
	if uc.Callback == nil {
		return false
	}
	if uc.Callback.Message == nil {
		return true
	}

	message := uc.Callback.Message.Message()
	if message == nil {
		return true
	}
	middleware.LogMessage(message)
	if !b.chatFilter.CheckAccess(ctx, message) {
		return true
	}
	if b.shouldTouchLastSeen(uc) {
		if err := b.memberService.EnsureActiveMemberSeen(ctx, uc.UserID, uc.Username, uc.FullName, uc.Now); err != nil {
			log.WithError(err).WithField("user_id", uc.UserID).Debug("EnsureActiveMemberSeen failed")
		}
	}
	if b.adminHandler.HandleAdminCallback(ctx, uc.Callback) {
		return true
	}

	return false
}

func (b *Bot) handleMessageUpdate(ctx context.Context, uc UpdateContext) {
	if uc.Message == nil || uc.Message.Text == "" {
		return
	}

	message := uc.Message
	middleware.LogMessage(message)

	if !b.chatFilter.CheckAccess(ctx, message) {
		return
	}

	if message.From != nil && !b.rateLimiter.Allow(message.From.ID) {
		log.WithField("user_id", message.From.ID).Debug("rate limited")
		return
	}

	if message.From == nil {
		log.WithField("chat_id", message.Chat.ID).Debug("skip update without sender")
		return
	}

	chatID := message.Chat.ID
	userID := message.From.ID

	if b.shouldTouchLastSeen(uc) {
		if err := b.memberService.EnsureActiveMemberSeen(ctx, userID, message.From.Username, buildDisplayName(message.From.FirstName, message.From.LastName), uc.Now); err != nil {
			log.WithError(err).WithField("user_id", userID).Debug("EnsureActiveMemberSeen failed")
		}
	}

	if uc.IsPrivate {
		handled := b.adminHandler.HandleAdminMessage(ctx, chatID, userID, message.MessageID, message.Text)
		if handled {
			return
		}
	}

	if b.cfg.FeatureKarmaEnabled && message.ReplyToMessage != nil && message.ReplyToMessage.From != nil {
		if b.thankYou != nil && b.thankYou.IsThankYou(message.Text) {
			b.karmaHandler.HandleThankYou(ctx, chatID, userID, message.ReplyToMessage.From.ID)
			return
		}
	}

	cmd, args, isCommand := b.parser.ParseCommand(message.Text)
	log.WithFields(log.Fields{
		"isCommand": isCommand,
		"cmd":       cmd,
		"args":      args,
		"text":      message.Text,
	}).Debug("parsed command")

	if isCommand {
		b.routeCommand(ctx, uc, cmd, args)
		return
	} else if chatID == b.cfg.FloodChatID {
		if b.cfg.FeatureStreaksEnabled {
			b.streakService.CountMessage(ctx, userID, message.Text)
		}
	}
}
