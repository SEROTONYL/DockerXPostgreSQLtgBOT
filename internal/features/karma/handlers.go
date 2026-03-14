package karma

import (
	"context"
	"errors"
	"fmt"
	"html"
	"strings"

	models "github.com/mymmrac/telego"
	log "github.com/sirupsen/logrus"

	"serotonyl.ru/telegram-bot/internal/commands"
	"serotonyl.ru/telegram-bot/internal/common"
	"serotonyl.ru/telegram-bot/internal/features/members"
	"serotonyl.ru/telegram-bot/internal/telegram"
)

type memberResolver interface {
	GetByUserID(ctx context.Context, userID int64) (*members.Member, error)
	GetByUsername(ctx context.Context, username string) (*members.Member, error)
}

type Handler struct {
	service       *Service
	memberService memberResolver
	tgOps         *telegram.Ops
}

func NewHandler(service *Service, memberService memberResolver, tgOps *telegram.Ops) *Handler {
	return &Handler{service: service, memberService: memberService, tgOps: tgOps}
}

func (h *Handler) HandleKarma(ctx context.Context, c commands.Context, args []string) {
	if len(args) != 0 {
		h.sendMessage(ctx, c.ChatID, "❌ Команда `!карма` не принимает аргументы.", c.MessageID)
		return
	}

	stats, err := h.service.GetThanksStats(ctx, c.UserID)
	if err != nil {
		log.WithError(err).Error("failed to get thanks stats")
		h.sendMessage(ctx, c.ChatID, "❌ Не удалось получить статистику благодарностей.", c.MessageID)
		return
	}

	text := fmt.Sprintf(
		"Твоя карма:\nСпасибо выдано: %d\nСпасибо получено: %d\nПолучено через спасибо: %s",
		stats.SentCount,
		stats.ReceivedCount,
		common.FormatBalance(stats.ReceivedReward),
	)
	h.sendMessage(ctx, c.ChatID, text, c.MessageID)
}

func (h *Handler) HandleThanksCommand(ctx context.Context, c commands.Context, args []string) {
	if c.Message == nil || c.Message.From == nil {
		h.sendMessage(ctx, c.ChatID, "❌ Не удалось прочитать сообщение команды.", c.MessageID)
		return
	}

	targetUserID, targetDisplay, err := h.resolveThanksTarget(ctx, c.Message, args)
	if err != nil {
		h.sendMessage(ctx, c.ChatID, userFacingThanksError(err), c.MessageID)
		return
	}

	if err := h.service.GiveThanks(ctx, c.UserID, targetUserID); err != nil {
		h.sendMessage(ctx, c.ChatID, userFacingThanksError(err), c.MessageID)
		return
	}

	h.sendThanksSuccessMessage(ctx, c.ChatID, c.MessageID, c.UserID, cleanUserLabel(visibleUserName(*c.Message.From)), targetUserID, cleanUserLabel(targetDisplay))
}

func (h *Handler) HandleThankYou(ctx context.Context, chatID int64, fromUserID, toUserID int64) {
	if err := h.service.GiveThanks(ctx, fromUserID, toUserID); err != nil {
		log.WithError(err).Debug("thanks not granted")
		return
	}

	h.sendThanksSuccessMessage(
		ctx,
		chatID,
		0,
		fromUserID,
		cleanUserLabel(h.resolveDisplayByUserID(ctx, fromUserID)),
		toUserID,
		cleanUserLabel(h.resolveDisplayByUserID(ctx, toUserID)),
	)
}

func (h *Handler) resolveThanksTarget(ctx context.Context, message *models.Message, args []string) (int64, string, error) {
	if len(args) > 1 {
		return 0, "", common.ErrThanksMalformedCommand
	}
	if len(args) == 1 {
		if !isUsernameToken(args[0]) {
			return 0, "", common.ErrThanksMalformedCommand
		}
		username := normalizeUsernameToken(args[0])
		if username == "" {
			return 0, "", common.ErrThanksMalformedCommand
		}
		member, err := h.memberService.GetByUsername(ctx, username)
		if err != nil || member == nil {
			return 0, "", common.ErrUserNotFound
		}
		return member.UserID, displayMember(member), nil
	}
	if message.ReplyToMessage != nil && message.ReplyToMessage.From != nil {
		member, err := h.memberService.GetByUserID(ctx, message.ReplyToMessage.From.ID)
		if err != nil || member == nil {
			return 0, "", common.ErrUserNotFound
		}
		return member.UserID, displayMember(member), nil
	}
	return 0, "", common.ErrThanksTargetMissing
}

func (h *Handler) resolveDisplayByUserID(ctx context.Context, userID int64) string {
	member, err := h.memberService.GetByUserID(ctx, userID)
	if err == nil && member != nil {
		return displayMember(member)
	}
	return fmt.Sprintf("id:%d", userID)
}

func userFacingThanksError(err error) string {
	switch {
	case errors.Is(err, common.ErrThanksTargetMissing):
		return "❌ Не указан получатель. Укажите `@username` или ответьте на сообщение пользователя."
	case errors.Is(err, common.ErrUserNotFound):
		return "❌ Получатель не найден."
	case errors.Is(err, common.ErrThanksSelfGive):
		return "❌ Нельзя благодарить самого себя."
	case errors.Is(err, common.ErrThanksTargetIsBot):
		return "❌ Нельзя благодарить ботов."
	case errors.Is(err, common.ErrThanksDailyLimit):
		return "❌ Вы исчерпали дневной лимит команды `спасибо`."
	case errors.Is(err, common.ErrThanksReciprocalCooldown):
		return "❌ Нельзя благодарить в ответ сразу. Подождите 5 минут."
	case errors.Is(err, common.ErrThanksMalformedCommand):
		return "❌ Некорректный формат. Используйте `!спасибо @username` или ответьте `!спасибо` на сообщение."
	default:
		return "❌ Не удалось выдать спасибо."
	}
}

func isUsernameToken(s string) bool {
	return strings.HasPrefix(strings.TrimSpace(s), "@")
}

func normalizeUsernameToken(s string) string {
	return strings.TrimPrefix(strings.TrimSpace(s), "@")
}

func visibleUserName(user models.User) string {
	username := strings.TrimPrefix(strings.TrimSpace(user.Username), "@")
	if username != "" {
		return "@" + username
	}
	name := strings.TrimSpace(strings.Join([]string{strings.TrimSpace(user.FirstName), strings.TrimSpace(user.LastName)}, " "))
	if name != "" {
		return name
	}
	return fmt.Sprintf("id:%d", user.ID)
}

func displayMember(member *members.Member) string {
	if member == nil {
		return "id:0"
	}
	username := strings.TrimPrefix(strings.TrimSpace(member.Username), "@")
	if username != "" {
		return "@" + username
	}
	name := strings.TrimSpace(strings.Join([]string{strings.TrimSpace(member.FirstName), strings.TrimSpace(member.LastName)}, " "))
	if name != "" {
		return name
	}
	if member.LastKnownName != nil && strings.TrimSpace(*member.LastKnownName) != "" {
		return strings.TrimSpace(*member.LastKnownName)
	}
	return fmt.Sprintf("id:%d", member.UserID)
}

func cleanUserLabel(label string) string {
	label = strings.TrimSpace(label)
	label = strings.TrimPrefix(label, "@")
	if label == "" {
		return "пользователь"
	}
	return label
}

func tgUserLink(userID int64, label string) string {
	return fmt.Sprintf(`<a href="tg://user?id=%d">%s</a>`, userID, html.EscapeString(cleanUserLabel(label)))
}

func (h *Handler) sendThanksSuccessMessage(ctx context.Context, chatID int64, replyToMessageID int, senderUserID int64, senderLabel string, targetUserID int64, targetLabel string) {
	remainingToday, dailyLimit, err := h.service.GetThanksDailyStatus(ctx, senderUserID)
	if err != nil {
		log.WithError(err).Debug("failed to get thanks daily status")
		remainingToday = 0
		dailyLimit = h.service.dailyLimit()
	}
	text := fmt.Sprintf(
		"❤️ %s сказал(а) спасибо - %s\n+%d%s. Сегодня осталось: %d/%d",
		tgUserLink(senderUserID, senderLabel),
		tgUserLink(targetUserID, targetLabel),
		ThanksReward,
		html.EscapeString(common.PluralizeFilms(ThanksReward)),
		remainingToday,
		dailyLimit,
	)
	_, _ = h.tgOps.SendWithOptions(ctx, telegram.SendOptions{
		ChatID:                chatID,
		Text:                  text,
		ParseMode:             telegram.ParseModeHTML,
		ReplyToMessageID:      replyToMessageID,
		DisableWebPagePreview: true,
	})
}

func (h *Handler) sendMessage(ctx context.Context, chatID int64, text string, replyToMessageID int) {
	_, _ = h.tgOps.SendWithOptions(ctx, telegram.SendOptions{
		ChatID:           chatID,
		Text:             text,
		ReplyToMessageID: replyToMessageID,
	})
}
