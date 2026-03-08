package karma

import (
	"context"

	"serotonyl.ru/telegram-bot/internal/commands"
	"serotonyl.ru/telegram-bot/internal/config"
)

func RegisterCommands(r *commands.Router, h *Handler, cfg *config.Config) {
	if !cfg.FeatureKarmaEnabled {
		return
	}

	r.Register("карма", func(ctx context.Context, c commands.Context, args []string) {
		if cfg == nil || c.ChatID != cfg.MemberSourceChatID {
			return
		}
		h.HandleKarma(ctx, c, args)
	})

	r.Register("спасибо", func(ctx context.Context, c commands.Context, args []string) {
		if cfg == nil || c.ChatID != cfg.MemberSourceChatID {
			return
		}
		h.HandleThanksCommand(ctx, c, args)
	})
}
