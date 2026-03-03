package streak

import (
	"context"

	"serotonyl.ru/telegram-bot/internal/commands"
	"serotonyl.ru/telegram-bot/internal/config"
)

// RegisterCommands регистрирует команды стриков.
func RegisterCommands(r *commands.Router, h *Handler, cfg *config.Config) {
	if !cfg.FeatureStreaksEnabled {
		return
	}

	r.Register("огонек", func(ctx context.Context, c commands.Context, args []string) {
		h.HandleOgonek(ctx, c.ChatID, c.UserID)
	})
}
