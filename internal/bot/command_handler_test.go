package bot

import (
	"context"
	"testing"

	models "github.com/mymmrac/telego"

	"serotonyl.ru/telegram-bot/internal/commands"
)

func TestRouteCommand_PropagatesTriggerMessageID(t *testing.T) {
	r := commands.NewRouter()
	got := 0
	r.Register("пленки", func(ctx context.Context, c commands.Context, args []string) {
		got = c.MessageID
	})

	b := &Bot{cmdRouter: r}
	uc := UpdateContext{ChatID: 1, UserID: 2, Message: &models.Message{MessageID: 321}}
	b.routeCommand(context.Background(), uc, "пленки", nil)

	if got != 321 {
		t.Fatalf("message_id = %d, want 321", got)
	}
}
