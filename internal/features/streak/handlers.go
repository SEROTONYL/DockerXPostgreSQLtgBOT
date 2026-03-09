package streak

import (
	"context"
	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"

	"serotonyl.ru/telegram-bot/internal/common"
	"serotonyl.ru/telegram-bot/internal/config"
	"serotonyl.ru/telegram-bot/internal/features/members"
	"serotonyl.ru/telegram-bot/internal/telegram"
)

type memberLookup interface {
	GetByUserID(ctx context.Context, userID int64) (*members.Member, error)
}

type Handler struct {
	service *Service
	members memberLookup
	tgOps   *telegram.Ops
	cfg     *config.Config
}

func NewHandler(service *Service, members memberLookup, tgOps *telegram.Ops, cfg *config.Config) *Handler {
	return &Handler{service: service, members: members, tgOps: tgOps, cfg: cfg}
}

func (h *Handler) HandleOgonek(ctx context.Context, chatID int64, userID int64, replyToMessageID int) {
	st, err := h.service.GetStreak(ctx, userID)
	if err != nil {
		log.WithError(err).Error("get streak failed")
		h.sendMessage(ctx, chatID, "❌ Не удалось получить огонёк.", replyToMessageID)
		return
	}

	nextReward := CalculateReward(st.CurrentStreak)
	if st.QuotaCompletedToday {
		nextReward = CalculateReward(st.CurrentStreak)
	}

	text := fmt.Sprintf(
		"🔥 Огонёк: %d %s\nСегодня: %d/%d\nСтатус: %s\nСледующая награда: %s",
		st.CurrentStreak,
		common.PluralizeDays(st.CurrentStreak),
		st.MessagesToday,
		dailyMessageTarget,
		streakStatusText(st),
		common.FormatBalance(nextReward),
	)
	h.sendMessage(ctx, chatID, text, replyToMessageID)
}

func (h *Handler) HandleTopOgonek(ctx context.Context, chatID int64, replyToMessageID int) {
	top, err := h.service.GetTop(ctx, 10)
	if err != nil {
		log.WithError(err).Error("get top streaks failed")
		h.sendMessage(ctx, chatID, "❌ Не удалось получить топ огонька.", replyToMessageID)
		return
	}
	if len(top) == 0 {
		h.sendMessage(ctx, chatID, "🔥 Топ огонька пока пуст.", replyToMessageID)
		return
	}

	var lines []string
	lines = append(lines, "🔥 Топ огонька")
	for i, entry := range top {
		lines = append(lines, fmt.Sprintf("%d. %s — %d %s", i+1, h.displayName(ctx, entry.UserID), entry.CurrentStreak, common.PluralizeDays(entry.CurrentStreak)))
	}
	h.sendMessage(ctx, chatID, strings.Join(lines, "\n"), replyToMessageID)
}

func streakStatusText(st *Streak) string {
	if st.QuotaCompletedToday {
		return "сегодня закрыт"
	}
	return "в процессе"
}

func (h *Handler) displayName(ctx context.Context, userID int64) string {
	if h.members == nil {
		return fmt.Sprintf("id:%d", userID)
	}
	member, err := h.members.GetByUserID(ctx, userID)
	if err != nil || member == nil {
		return fmt.Sprintf("id:%d", userID)
	}
	username := strings.TrimSpace(strings.TrimPrefix(member.Username, "@"))
	if username != "" {
		return "@" + username
	}
	fullName := strings.TrimSpace(strings.Join([]string{strings.TrimSpace(member.FirstName), strings.TrimSpace(member.LastName)}, " "))
	if fullName != "" {
		return fullName
	}
	if member.LastKnownName != nil && strings.TrimSpace(*member.LastKnownName) != "" {
		return strings.TrimSpace(*member.LastKnownName)
	}
	return fmt.Sprintf("id:%d", userID)
}

func (h *Handler) sendMessage(ctx context.Context, chatID int64, text string, replyToMessageID int) {
	_, _ = h.tgOps.SendWithOptions(ctx, telegram.SendOptions{
		ChatID:           chatID,
		Text:             text,
		ReplyToMessageID: replyToMessageID,
	})
}
