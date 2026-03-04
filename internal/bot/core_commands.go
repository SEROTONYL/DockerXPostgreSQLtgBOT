package bot

import (
	"context"
	"fmt"
	"strings"
	"time"

	"serotonyl.ru/telegram-bot/internal/commands"
	"serotonyl.ru/telegram-bot/internal/jobs"
)

func (b *Bot) registerCoreCommands() {
	b.cmdRouter.Register("start", func(ctx context.Context, c commands.Context, args []string) {
		b.sendMessage(ctx, c.ChatID, "Я живой. Команды: /login <пароль> (админ), !плёнки, !карма, !слоты ...")
	})
	b.cmdRouter.Register("help", func(ctx context.Context, c commands.Context, args []string) {
		b.sendMessage(ctx, c.ChatID, "Я живой. Команды: /login <пароль> (админ), !плёнки, !карма, !слоты ...")
	})
	b.cmdRouter.Register("login", func(ctx context.Context, c commands.Context, args []string) {
		if b.adminHandler == nil || c.ChatID != c.UserID {
			return
		}
		b.adminHandler.HandleAdminMessage(ctx, c.ChatID, c.UserID, 0, "/login "+strings.Join(args, " "))
	})
	b.cmdRouter.Register("members_status", func(ctx context.Context, c commands.Context, args []string) {
		b.handleMembersStatusCommand(ctx, c)
	})
	b.cmdRouter.Register("members_stats", func(ctx context.Context, c commands.Context, args []string) {
		b.handleMembersStatusCommand(ctx, c)
	})
}

func (b *Bot) handleMembersStatusCommand(ctx context.Context, c commands.Context) {
	if b.memberService == nil || !c.IsAdminChat {
		return
	}
	active, left, err := b.memberService.CountMembersByStatus(ctx)
	if err != nil {
		return
	}
	pending, err := b.memberService.CountPendingPurge(ctx, c.Now)
	if err != nil {
		return
	}

	metrics := jobs.PurgeMetrics{}
	if b.purgeMetricsProvider != nil {
		metrics = b.purgeMetricsProvider.GetPurgeMetrics()
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
	b.sendMessage(ctx, c.ChatID, text)
}
