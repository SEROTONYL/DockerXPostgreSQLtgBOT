package admin

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"serotonyl.ru/telegram-bot/internal/config"
	"serotonyl.ru/telegram-bot/internal/features/members"
)

type fakeAdminRepoFlow struct {
	hasSession bool
}

func (f *fakeAdminRepoFlow) CreateSession(ctx context.Context, session *AdminSession) error {
	return nil
}
func (f *fakeAdminRepoFlow) GetActiveSession(ctx context.Context, userID int64) (*AdminSession, error) {
	if f.hasSession {
		return &AdminSession{UserID: userID}, nil
	}
	return nil, errors.New("no session")
}
func (f *fakeAdminRepoFlow) DeactivateSession(ctx context.Context, userID int64) error { return nil }
func (f *fakeAdminRepoFlow) UpdateActivity(ctx context.Context, userID int64) error    { return nil }
func (f *fakeAdminRepoFlow) LogAttempt(ctx context.Context, userID int64, success bool) error {
	return nil
}
func (f *fakeAdminRepoFlow) GetRecentAttempts(ctx context.Context, userID int64, period time.Duration) (int, error) {
	return 0, nil
}

type fakeMemberRepoFlow struct {
	member *members.Member
}

func (f *fakeMemberRepoFlow) GetByUserID(ctx context.Context, userID int64) (*members.Member, error) {
	if f.member != nil {
		return f.member, nil
	}
	return nil, errors.New("not found")
}
func (f *fakeMemberRepoFlow) GetUsersWithoutRole(ctx context.Context) ([]*members.Member, error) {
	return nil, nil
}
func (f *fakeMemberRepoFlow) GetUsersWithRole(ctx context.Context) ([]*members.Member, error) {
	return nil, nil
}
func (f *fakeMemberRepoFlow) UpdateRole(ctx context.Context, userID int64, role string) error {
	return nil
}
func (f *fakeMemberRepoFlow) UpdateAdminFlag(ctx context.Context, userID int64, isAdmin bool) error {
	return nil
}

func TestHandleAdminMessage_LoginDeniedResponds(t *testing.T) {
	svc := NewService(&fakeAdminRepoFlow{}, &fakeMemberRepoFlow{member: &members.Member{IsAdmin: false}}, &config.Config{})
	var sent []string
	h := &Handler{
		service: svc,
		sendFn: func(c tgbotapi.Chattable) (tgbotapi.Message, error) {
			if m, ok := c.(tgbotapi.MessageConfig); ok {
				sent = append(sent, m.Text)
			}
			return tgbotapi.Message{}, nil
		},
	}

	handled := h.HandleAdminMessage(context.Background(), 1, 77, "/login")
	if !handled {
		t.Fatalf("expected handled for /login deny")
	}
	if len(sent) != 1 || sent[0] != "❌ Доступ запрещён" {
		t.Fatalf("unexpected deny response: %#v", sent)
	}
}

func TestHandleAdminMessage_LoginAllowWithoutSessionAsksPassword(t *testing.T) {
	svc := NewService(&fakeAdminRepoFlow{hasSession: false}, &fakeMemberRepoFlow{member: &members.Member{IsAdmin: true}}, &config.Config{})
	var sent []string
	h := &Handler{
		service: svc,
		sendFn: func(c tgbotapi.Chattable) (tgbotapi.Message, error) {
			if m, ok := c.(tgbotapi.MessageConfig); ok {
				sent = append(sent, m.Text)
			}
			return tgbotapi.Message{}, nil
		},
	}

	handled := h.HandleAdminMessage(context.Background(), 1, 77, "/login")
	if !handled {
		t.Fatalf("expected handled")
	}
	if len(sent) != 1 || !strings.Contains(sent[0], "Введите пароль") {
		t.Fatalf("expected password prompt, got %#v", sent)
	}
	state := svc.GetState(77)
	if state == nil || state.State != StateAwaitingPassword {
		t.Fatalf("expected awaiting_password state, got %#v", state)
	}
}

func TestHandleAdminMessage_LoginAllowWithSessionShowsMenu(t *testing.T) {
	svc := NewService(&fakeAdminRepoFlow{hasSession: true}, &fakeMemberRepoFlow{member: &members.Member{IsAdmin: true}}, &config.Config{})
	var sent []string
	h := &Handler{
		service: svc,
		sendFn: func(c tgbotapi.Chattable) (tgbotapi.Message, error) {
			if m, ok := c.(tgbotapi.MessageConfig); ok {
				sent = append(sent, m.Text)
			}
			return tgbotapi.Message{}, nil
		},
	}

	handled := h.HandleAdminMessage(context.Background(), 1, 77, "/login")
	if !handled {
		t.Fatalf("expected handled")
	}
	if len(sent) != 1 {
		t.Fatalf("expected one menu message, got %#v", sent)
	}
	if strings.TrimSpace(sent[0]) == "" {
		t.Fatalf("expected non-empty menu text")
	}
}

func TestHandleAdminMessage_LoginRegressionAlwaysResponds(t *testing.T) {
	svc := NewService(&fakeAdminRepoFlow{}, &fakeMemberRepoFlow{member: &members.Member{IsAdmin: false}}, &config.Config{})
	messages := 0
	h := &Handler{
		service: svc,
		sendFn: func(c tgbotapi.Chattable) (tgbotapi.Message, error) {
			messages++
			return tgbotapi.Message{}, nil
		},
	}

	handled := h.HandleAdminMessage(context.Background(), 1, 99, "/login secret")
	if !handled {
		t.Fatalf("expected /login to be handled")
	}
	if messages == 0 {
		t.Fatalf("expected response message for /login")
	}
}

func TestHandleAdminMessage_LoginPrefixDoesNotMatchOtherCommands(t *testing.T) {
	svc := NewService(&fakeAdminRepoFlow{}, &fakeMemberRepoFlow{member: &members.Member{IsAdmin: false}}, &config.Config{})
	messages := 0
	h := &Handler{
		service: svc,
		sendFn: func(c tgbotapi.Chattable) (tgbotapi.Message, error) {
			messages++
			return tgbotapi.Message{}, nil
		},
	}

	handled := h.HandleAdminMessage(context.Background(), 1, 99, "/login123")
	if handled {
		t.Fatalf("expected /login123 not to be treated as /login")
	}
	if messages != 0 {
		t.Fatalf("expected no response for non-login command")
	}
}

func TestHandleAdminMessage_LoginSingleStepAttemptsPassword(t *testing.T) {
	svc := NewService(&fakeAdminRepoFlow{hasSession: false}, &fakeMemberRepoFlow{member: &members.Member{IsAdmin: true}}, &config.Config{})
	var sent []string
	h := &Handler{
		service: svc,
		sendFn: func(c tgbotapi.Chattable) (tgbotapi.Message, error) {
			if m, ok := c.(tgbotapi.MessageConfig); ok {
				sent = append(sent, m.Text)
			}
			return tgbotapi.Message{}, nil
		},
	}

	handled := h.HandleAdminMessage(context.Background(), 1, 77, "/login wrongpass")
	if !handled {
		t.Fatalf("expected handled")
	}
	if len(sent) != 1 || !strings.HasPrefix(sent[0], "❌") {
		t.Fatalf("expected invalid password response, got %#v", sent)
	}
	if state := svc.GetState(77); state != nil {
		t.Fatalf("expected no awaiting_password state after single-step attempt")
	}
}
