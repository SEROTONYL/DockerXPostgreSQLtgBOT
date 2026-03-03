package karma

import (
	"context"

	"serotonyl.ru/telegram-bot/internal/commands"
	"serotonyl.ru/telegram-bot/internal/config"
)

// RegisterCommands регистрирует команды кармы.
func RegisterCommands(r *commands.Router, h *Handler, cfg *config.Config) {
	if !cfg.FeatureKarmaEnabled {
		return
	}

	r.Register("карма", func(ctx context.Context, c commands.Context, args []string) {
		h.HandleKarma(ctx, c.ChatID, c.UserID)
	})
}
