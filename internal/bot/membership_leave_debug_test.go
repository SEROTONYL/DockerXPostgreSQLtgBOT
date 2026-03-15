package bot

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	models "github.com/mymmrac/telego"
	log "github.com/sirupsen/logrus"

	"serotonyl.ru/telegram-bot/internal/config"
	"serotonyl.ru/telegram-bot/internal/telegram"
)

type leaveDebugSend struct {
	chatID int64
	text   string
}

type fakeTGLeaveDebug struct {
	sent    []leaveDebugSend
	sendErr error
}

func (f *fakeTGLeaveDebug) SendMessage(chatID int64, text string, markup *models.InlineKeyboardMarkup) (int, error) {
	if f.sendErr != nil {
		return 0, f.sendErr
	}
	f.sent = append(f.sent, leaveDebugSend{chatID: chatID, text: text})
	return len(f.sent), nil
}

func (f *fakeTGLeaveDebug) EditMessage(chatID int64, messageID int, text string, markup *models.InlineKeyboardMarkup) error {
	return nil
}

func (f *fakeTGLeaveDebug) EditReplyMarkup(chatID int64, messageID int, markup *models.InlineKeyboardMarkup) error {
	return nil
}

func (f *fakeTGLeaveDebug) DeleteMessage(chatID int64, messageID int) error {
	return nil
}

func (f *fakeTGLeaveDebug) PinChatMessage(chatID int64, messageID int, disableNotification bool) error {
	return nil
}

func (f *fakeTGLeaveDebug) UnpinChatMessage(chatID int64, messageID int) error {
	return nil
}

func (f *fakeTGLeaveDebug) GetChatMember(chatID int64, userID int64) (models.ChatMember, error) {
	return &models.ChatMemberMember{Status: "member", User: models.User{ID: userID}}, nil
}

type fakeLeaveDebugMemberService struct {
	markLeftCalls int
	markLeftErr   error
	role          *string
	tag           *string
	roleTagErr    error
}

func (f *fakeLeaveDebugMemberService) EnsureActiveMemberSeen(ctx context.Context, userID int64, username, fullName string, isBot bool, now time.Time) error {
	return nil
}

func (f *fakeLeaveDebugMemberService) UpsertActiveMember(ctx context.Context, userID int64, username, fullName string, isBot bool, now time.Time) error {
	return nil
}

func (f *fakeLeaveDebugMemberService) MarkMemberLeft(ctx context.Context, userID int64, leftAt, purgeAt time.Time) error {
	f.markLeftCalls++
	return f.markLeftErr
}

func (f *fakeLeaveDebugMemberService) GetRoleAndTag(ctx context.Context, userID int64) (role *string, tag *string, err error) {
	return f.role, f.tag, f.roleTagErr
}

func (f *fakeLeaveDebugMemberService) HandleNewMember(ctx context.Context, userID int64, username, firstName, lastName string, isBot bool) error {
	return nil
}

func (f *fakeLeaveDebugMemberService) CountMembersByStatus(ctx context.Context) (active int, left int, err error) {
	return 0, 0, nil
}

func (f *fakeLeaveDebugMemberService) CountPendingPurge(ctx context.Context, now time.Time) (pending int, err error) {
	return 0, nil
}

func TestHandleMembershipUpdate_LeaveDebugFormatting(t *testing.T) {
	tests := []struct {
		name     string
		role     *string
		tag      *string
		username string
		userID   int64
		want     string
	}{
		{name: "role and username", role: strPtr("Роль"), username: "username", userID: 101, want: "#Выход — Роль (@username)"},
		{name: "tag fallback", tag: strPtr("taggy"), username: "username", userID: 102, want: "#Выход — taggy (@username)"},
		{name: "no role no tag", username: "username", userID: 103, want: "#Выход — Без роли (@username)"},
		{name: "numeric identity fallback", role: strPtr("Роль"), userID: 104, want: "#Выход — Роль (104)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tg := &fakeTGLeaveDebug{}
			memberSvc := &fakeLeaveDebugMemberService{role: tt.role, tag: tt.tag}
			b := &Bot{
				cfg:           &config.Config{MemberSourceChatID: -1001, LeaveDebug: 555},
				ops:           telegram.NewOps(tg),
				memberService: memberSvc,
			}

			user := models.User{ID: tt.userID, Username: tt.username, FirstName: "Test"}
			handled := b.handleMembershipUpdate(context.Background(), UpdateContext{
				Now: time.Now().UTC(),
				ChatMember: &models.ChatMemberUpdated{
					Chat:          models.Chat{ID: -1001, Type: models.ChatTypeSupergroup},
					OldChatMember: &models.ChatMemberMember{Status: "member", User: user},
					NewChatMember: &models.ChatMemberLeft{Status: "left", User: user},
				},
			})

			if !handled {
				t.Fatal("expected membership update to be handled")
			}
			if memberSvc.markLeftCalls != 1 {
				t.Fatalf("expected MarkMemberLeft once, got %d", memberSvc.markLeftCalls)
			}
			if len(tg.sent) != 1 {
				t.Fatalf("expected one debug message, got %d", len(tg.sent))
			}
			if tg.sent[0].chatID != 555 {
				t.Fatalf("unexpected debug chat id: %d", tg.sent[0].chatID)
			}
			if tg.sent[0].text != tt.want {
				t.Fatalf("unexpected debug text: %q", tg.sent[0].text)
			}
		})
	}
}

func TestHandleMembershipUpdate_LeaveDebugDisabled(t *testing.T) {
	tg := &fakeTGLeaveDebug{}
	memberSvc := &fakeLeaveDebugMemberService{role: strPtr("Роль")}
	b := &Bot{
		cfg:           &config.Config{MemberSourceChatID: -1001, LeaveDebug: 0},
		ops:           telegram.NewOps(tg),
		memberService: memberSvc,
	}

	user := models.User{ID: 101, Username: "username"}
	b.handleMembershipUpdate(context.Background(), UpdateContext{
		Now: time.Now().UTC(),
		ChatMember: &models.ChatMemberUpdated{
			Chat:          models.Chat{ID: -1001, Type: models.ChatTypeSupergroup},
			OldChatMember: &models.ChatMemberMember{Status: "member", User: user},
			NewChatMember: &models.ChatMemberLeft{Status: "left", User: user},
		},
	})

	if memberSvc.markLeftCalls != 1 {
		t.Fatalf("expected MarkMemberLeft once, got %d", memberSvc.markLeftCalls)
	}
	if len(tg.sent) != 0 {
		t.Fatalf("expected no debug message, got %d", len(tg.sent))
	}
}

func TestHandleMembershipUpdate_LeaveDebugNotRepeatedForLeftToLeft(t *testing.T) {
	tg := &fakeTGLeaveDebug{}
	memberSvc := &fakeLeaveDebugMemberService{role: strPtr("Роль")}
	b := &Bot{
		cfg:           &config.Config{MemberSourceChatID: -1001, LeaveDebug: 555},
		ops:           telegram.NewOps(tg),
		memberService: memberSvc,
	}

	user := models.User{ID: 101, Username: "username"}
	b.handleMembershipUpdate(context.Background(), UpdateContext{
		Now: time.Now().UTC(),
		ChatMember: &models.ChatMemberUpdated{
			Chat:          models.Chat{ID: -1001, Type: models.ChatTypeSupergroup},
			OldChatMember: &models.ChatMemberLeft{Status: "left", User: user},
			NewChatMember: &models.ChatMemberLeft{Status: "left", User: user},
		},
	})

	if len(tg.sent) != 0 {
		t.Fatalf("expected no debug message, got %d", len(tg.sent))
	}
}

func TestHandleMembershipUpdate_LeaveDebugSendErrorDoesNotBreakMarkLeft(t *testing.T) {
	tg := &fakeTGLeaveDebug{sendErr: errors.New("send failed")}
	memberSvc := &fakeLeaveDebugMemberService{role: strPtr("Роль")}
	b := &Bot{
		cfg:           &config.Config{MemberSourceChatID: -1001, LeaveDebug: 555},
		ops:           telegram.NewOps(tg),
		memberService: memberSvc,
	}

	var logBuf bytes.Buffer
	prevOut := log.StandardLogger().Out
	prevLevel := log.StandardLogger().Level
	log.SetOutput(&logBuf)
	log.SetLevel(log.WarnLevel)
	defer log.SetOutput(prevOut)
	defer log.SetLevel(prevLevel)

	user := models.User{ID: 101, Username: "username"}
	b.handleMembershipUpdate(context.Background(), UpdateContext{
		Now: time.Now().UTC(),
		ChatMember: &models.ChatMemberUpdated{
			Chat:          models.Chat{ID: -1001, Type: models.ChatTypeSupergroup},
			OldChatMember: &models.ChatMemberMember{Status: "member", User: user},
			NewChatMember: &models.ChatMemberLeft{Status: "left", User: user},
		},
	})

	if memberSvc.markLeftCalls != 1 {
		t.Fatalf("expected MarkMemberLeft once, got %d", memberSvc.markLeftCalls)
	}
	logged := logBuf.String()
	if !strings.Contains(logged, "leave debug notification failed") {
		t.Fatalf("expected leave debug warning log, got %q", logged)
	}
	if !strings.Contains(logged, "leaving_user_id=101") || !strings.Contains(logged, "leave_debug_user_id=555") {
		t.Fatalf("expected leave debug log fields, got %q", logged)
	}
}

func TestHandleMembershipUpdate_LeaveDebugIgnoresOtherChats(t *testing.T) {
	tg := &fakeTGLeaveDebug{}
	memberSvc := &fakeLeaveDebugMemberService{role: strPtr("Роль")}
	b := &Bot{
		cfg:           &config.Config{MemberSourceChatID: -1001, LeaveDebug: 555},
		ops:           telegram.NewOps(tg),
		memberService: memberSvc,
	}

	user := models.User{ID: 101, Username: "username"}
	b.handleMembershipUpdate(context.Background(), UpdateContext{
		Now: time.Now().UTC(),
		ChatMember: &models.ChatMemberUpdated{
			Chat:          models.Chat{ID: -9999, Type: models.ChatTypeSupergroup},
			OldChatMember: &models.ChatMemberMember{Status: "member", User: user},
			NewChatMember: &models.ChatMemberLeft{Status: "left", User: user},
		},
	})

	if memberSvc.markLeftCalls != 0 {
		t.Fatalf("expected no MarkMemberLeft calls, got %d", memberSvc.markLeftCalls)
	}
	if len(tg.sent) != 0 {
		t.Fatalf("expected no debug message, got %d", len(tg.sent))
	}
}

func strPtr(v string) *string {
	return &v
}
