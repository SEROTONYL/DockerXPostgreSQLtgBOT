package members

import (
	"context"
	"strconv"
	"strings"

	"serotonyl.ru/telegram-bot/internal/commands"
)

type Feature struct {
	h *Handler
}

func NewFeature(h *Handler) *Feature { return &Feature{h: h} }
func (f *Feature) Name() string      { return "members" }
func (f *Feature) RegisterCommands(r *commands.Router) {
	r.Register("список", func(ctx context.Context, c commands.Context, args []string) {
		limit, err := parseMembersListLimit(args)
		if err != nil {
			f.h.sendMembersListValidationError(ctx, c.ChatID, c.MessageID)
			return
		}
		f.h.HandleMembersList(ctx, c.ChatID, c.UserID, limit)
	})
}

func parseMembersListLimit(args []string) (int, error) {
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		token := strings.TrimSpace(arg)
		if token == "" {
			continue
		}
		filtered = append(filtered, token)
	}
	if len(filtered) == 0 {
		return 0, nil
	}
	if len(filtered) != 1 {
		return 0, strconv.ErrSyntax
	}
	value, err := strconv.Atoi(filtered[0])
	if err != nil || value <= 0 {
		return 0, strconv.ErrSyntax
	}
	return value, nil
}
