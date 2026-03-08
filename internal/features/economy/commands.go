package economy

import (
	"context"

	"serotonyl.ru/telegram-bot/internal/commands"
	"serotonyl.ru/telegram-bot/internal/config"
)

// RegisterCommands регистрирует команды экономики.
func RegisterCommands(r *commands.Router, h *Handler, cfg *config.Config) {
	r.Register("пленки", func(ctx context.Context, c commands.Context, args []string) {
		if cfg == nil || c.ChatID != cfg.MemberSourceChatID {
			return
		}
		h.HandleBalance(ctx, c.ChatID, c.UserID, c.MessageID)
	})
	r.Register("твои", func(ctx context.Context, c commands.Context, args []string) {
		if cfg == nil || c.ChatID != cfg.MemberSourceChatID {
			return
		}
		h.HandleTargetBalanceCommand(ctx, c, args)
	})
	r.Register("отсыпать", func(ctx context.Context, c commands.Context, args []string) {
		h.HandleTransferCommand(ctx, c, args)
	})
	r.Register("транзакции", func(ctx context.Context, c commands.Context, args []string) {
		h.HandleTransactions(ctx, c.ChatID, c.UserID)
	})
}
