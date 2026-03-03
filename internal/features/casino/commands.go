package casino

import (
	"context"

	"serotonyl.ru/telegram-bot/internal/commands"
	"serotonyl.ru/telegram-bot/internal/config"
)

// RegisterCommands регистрирует команды казино.
func RegisterCommands(r *commands.Router, h *Handler, cfg *config.Config) {
	if !cfg.FeatureCasinoEnabled {
		return
	}

	r.Register("слоты", func(ctx context.Context, c commands.Context, args []string) {
		h.HandleSlots(ctx, c.ChatID, c.UserID)
	})
	r.Register("статслоты", func(ctx context.Context, c commands.Context, args []string) {
		h.HandleSlotStats(ctx, c.ChatID, c.UserID)
	})
}
