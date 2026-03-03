package economy

import (
	"context"

	"serotonyl.ru/telegram-bot/internal/commands"
	"serotonyl.ru/telegram-bot/internal/config"
)

// RegisterCommands регистрирует команды экономики.
func RegisterCommands(r *commands.Router, h *Handler, _ *config.Config) {
	r.Register("пленки", func(ctx context.Context, c commands.Context, args []string) {
		h.HandleBalance(ctx, c.ChatID, c.UserID)
	})
	r.Register("отсыпать", func(ctx context.Context, c commands.Context, args []string) {
		h.HandleTransfer(ctx, c.ChatID, c.UserID, args)
	})
	r.Register("транзакции", func(ctx context.Context, c commands.Context, args []string) {
		h.HandleTransactions(ctx, c.ChatID, c.UserID)
	})
}
