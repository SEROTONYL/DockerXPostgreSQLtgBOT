package streak

import (
	"context"

	"serotonyl.ru/telegram-bot/internal/commands"
	"serotonyl.ru/telegram-bot/internal/config"
)

func RegisterCommands(r *commands.Router, h *Handler, cfg *config.Config) {
	if !cfg.FeatureStreaksEnabled {
		return
	}

	r.Register("огонек", func(ctx context.Context, c commands.Context, args []string) {
		if cfg == nil || c.ChatID != cfg.MemberSourceChatID {
			return
		}
		h.HandleOgonek(ctx, c.ChatID, c.UserID, c.MessageID)
	})

	r.Register("топогонек", func(ctx context.Context, c commands.Context, args []string) {
		if cfg == nil || c.ChatID != cfg.MemberSourceChatID {
			return
		}
		h.HandleTopOgonek(ctx, c.ChatID, c.MessageID)
	})
}
