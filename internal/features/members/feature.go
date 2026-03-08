package members

import (
	"context"

	"serotonyl.ru/telegram-bot/internal/commands"
)

type Feature struct {
	h *Handler
}

func NewFeature(h *Handler) *Feature { return &Feature{h: h} }
func (f *Feature) Name() string      { return "members" }
func (f *Feature) RegisterCommands(r *commands.Router) {
	r.Register("список", func(ctx context.Context, c commands.Context, args []string) {
		f.h.HandleMembersList(ctx, c.ChatID, c.UserID)
	})
}
