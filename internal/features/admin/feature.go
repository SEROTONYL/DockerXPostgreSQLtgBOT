package admin

import (
	"context"
	"fmt"
	"strings"
	"time"

	"serotonyl.ru/telegram-bot/internal/commands"
	"serotonyl.ru/telegram-bot/internal/config"
	"serotonyl.ru/telegram-bot/internal/jobs"
	"serotonyl.ru/telegram-bot/internal/telegram"
)

type adminMessageHandler interface {
	HandleAdminMessage(ctx context.Context, chatID int64, userID int64, messageID int, text string) bool
}

type membersStatusService interface {
	CountMembersByStatus(ctx context.Context) (active int, left int, err error)
	CountPendingPurge(ctx context.Context, now time.Time) (int, error)
}

type Feature struct {
	cfg          *config.Config
	ops          *telegram.Ops
	adminHandler adminMessageHandler
	memberSvc    membersStatusService
	purgeMetrics func() jobs.PurgeMetrics
}

func NewFeature(cfg *config.Config, ops *telegram.Ops, adminHandler adminMessageHandler, memberSvc membersStatusService, purgeMetrics func() jobs.PurgeMetrics) *Feature {
	return &Feature{cfg: cfg, ops: ops, adminHandler: adminHandler, memberSvc: memberSvc, purgeMetrics: purgeMetrics}
}

func (f *Feature) Name() string { return "admin" }

func (f *Feature) RegisterCommands(r *commands.Router) {
	r.Register("login", func(ctx context.Context, c commands.Context, args []string) {
		if f.adminHandler == nil || !c.IsPrivate {
			return
		}
		f.adminHandler.HandleAdminMessage(ctx, c.ChatID, c.UserID, 0, "/login "+strings.Join(args, " "))
	})
	r.Register("members_status", func(ctx context.Context, c commands.Context, args []string) {
		f.handleMembersStatusCommand(ctx, c)
	})
	r.Register("members_stats", func(ctx context.Context, c commands.Context, args []string) {
		f.handleMembersStatusCommand(ctx, c)
	})
}

func (f *Feature) handleMembersStatusCommand(ctx context.Context, c commands.Context) {
	if f.memberSvc == nil || !c.IsAdminChat {
		return
	}

	active, left, err := f.memberSvc.CountMembersByStatus(ctx)
	if err != nil {
		return
	}
	pending, err := f.memberSvc.CountPendingPurge(ctx, c.Now)
	if err != nil {
		return
	}

	metrics := jobs.PurgeMetrics{}
	if f.purgeMetrics != nil {
		metrics = f.purgeMetrics()
	}
	lastRun := "never"
	if !metrics.LastRunAt.IsZero() {
		lastRun = metrics.LastRunAt.Format(time.RFC3339)
	}
	lastError := metrics.LastError
	if strings.TrimSpace(lastError) == "" {
		lastError = "none"
	}

	text := fmt.Sprintf("Members:\n- Active: %d\n- Left (grace): %d\n- Pending purge: %d\n\nPurge:\n- Last run: %s\n- Last deleted: %d\n- Total deleted: %d\n- Last error: %s", active, left, pending, lastRun, metrics.LastRunDeleted, metrics.TotalDeleted, lastError)
	f.sendMessage(ctx, c.ChatID, text)
}

func (f *Feature) sendMessage(ctx context.Context, chatID int64, text string) {
	if f.ops == nil {
		return
	}
	_, _ = f.ops.Send(ctx, chatID, text, nil)
}
