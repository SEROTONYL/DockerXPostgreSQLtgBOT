package members

import (
	"context"
	"testing"

	"serotonyl.ru/telegram-bot/internal/commands"
	"serotonyl.ru/telegram-bot/internal/config"
	"serotonyl.ru/telegram-bot/internal/telegram"
)

func TestFeatureMembersListCommand_InvalidLimit_SendsValidationError(t *testing.T) {
	tg := &fakeMembersTGWithOptions{}
	h := NewHandler(&Service{}, noopBalanceProvider{}, telegram.NewOps(tg), &config.Config{MemberSourceChatID: 100})
	f := NewFeature(h)
	r := commands.NewRouter()
	f.RegisterCommands(r)

	handled := r.Dispatch(context.Background(), commands.Context{ChatID: 100, UserID: 77, MessageID: 12}, "список", []string{"0"})
	if !handled {
		t.Fatal("expected command handled")
	}
	if len(tg.sentOpts) != 1 {
		t.Fatalf("expected one validation message, got %d", len(tg.sentOpts))
	}
	if tg.sentOpts[0].ReplyToMessageID != 12 {
		t.Fatalf("expected reply to original message, got %d", tg.sentOpts[0].ReplyToMessageID)
	}
}

func TestParseMembersListLimit(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    int
		wantErr bool
	}{
		{name: "no args keeps old behavior", args: nil, want: 0},
		{name: "single valid arg", args: []string{"10"}, want: 10},
		{name: "extra spaces ignored", args: []string{"", " 10 ", ""}, want: 10},
		{name: "zero rejected", args: []string{"0"}, wantErr: true},
		{name: "negative rejected", args: []string{"-5"}, wantErr: true},
		{name: "text rejected", args: []string{"abc"}, wantErr: true},
		{name: "multiple args rejected", args: []string{"10", "20"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseMembersListLimit(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %d want %d", got, tt.want)
			}
		})
	}
}
