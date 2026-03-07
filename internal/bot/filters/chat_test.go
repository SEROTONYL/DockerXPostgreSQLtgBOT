package filters

import (
	"context"
	"testing"
	"time"

	models "github.com/mymmrac/telego"

	"serotonyl.ru/telegram-bot/internal/bot"
	"serotonyl.ru/telegram-bot/internal/telegram"
)

type fakeTG struct{}

type fakeTGMember struct {
	status         string
	lastGetChatID  int64
	lastGetUserID  int64
	getMemberCalls int
}

func (f *fakeTG) SendMessage(chatID int64, text string, markup *models.InlineKeyboardMarkup) (int, error) {
	return 0, nil
}
func (f *fakeTG) EditMessage(chatID int64, messageID int, text string, markup *models.InlineKeyboardMarkup) error {
	return nil
}
func (f *fakeTG) AnswerCallbackQuery(callbackID string, text string, showAlert bool) error {
	return nil
}
func (f *fakeTG) GetChatMember(chatID int64, userID int64) (member models.ChatMember, err error) {
	return &models.ChatMemberLeft{Status: "left"}, nil
}
func (f *fakeTGMember) SendMessage(chatID int64, text string, markup *models.InlineKeyboardMarkup) (int, error) {
	return 0, nil
}
func (f *fakeTGMember) EditMessage(chatID int64, messageID int, text string, markup *models.InlineKeyboardMarkup) error {
	return nil
}
func (f *fakeTGMember) AnswerCallbackQuery(callbackID string, text string, showAlert bool) error {
	return nil
}
func (f *fakeTGMember) GetChatMember(chatID int64, userID int64) (member models.ChatMember, err error) {
	f.lastGetChatID = chatID
	f.lastGetUserID = userID
	f.getMemberCalls++
	status := f.status
	if status == "" {
		status = "member"
	}
	return &models.ChatMemberMember{Status: status, User: models.User{ID: userID}}, nil
}
func (f *fakeTGMember) EditReplyMarkup(chatID int64, messageID int, markup *models.InlineKeyboardMarkup) error {
	return nil
}
func (f *fakeTGMember) DeleteMessage(chatID int64, messageID int) error {
	return nil
}
func (f *fakeTG) EditReplyMarkup(chatID int64, messageID int, markup *models.InlineKeyboardMarkup) error {
	return nil
}
func (f *fakeTG) DeleteMessage(chatID int64, messageID int) error {
	return nil
}

type fakeMemberService struct {
	ensureActiveCalls int
}

func (f *fakeMemberService) EnsureActiveMemberSeen(ctx context.Context, userID int64, username, fullName string, isBot bool, now time.Time) error {
	f.ensureActiveCalls++
	return nil
}

func (f *fakeMemberService) UpsertActiveMember(ctx context.Context, userID int64, username, fullName string, isBot bool, now time.Time) error {
	return nil
}

func (f *fakeMemberService) MarkMemberLeft(ctx context.Context, userID int64, leftAt, purgeAt time.Time) error {
	return nil
}

func (f *fakeMemberService) HandleNewMember(ctx context.Context, userID int64, username, firstName, lastName string, isBot bool) error {
	return nil
}

func (f *fakeMemberService) CountMembersByStatus(ctx context.Context) (active int, left int, err error) {
	return 0, 0, nil
}

func (f *fakeMemberService) CountPendingPurge(ctx context.Context, now time.Time) (pending int, err error) {
	return 0, nil
}

var _ bot.MemberService = (*fakeMemberService)(nil)

func TestCheckAccess_AdminChatAlwaysAllowed(t *testing.T) {
	f := NewChatFilter(-1001, -2002, &fakeMemberService{}, telegram.NewOps(&fakeTG{}))
	msg := &models.Message{Chat: models.Chat{ID: -2002, Type: models.ChatTypeSupergroup}, From: &models.User{ID: 42}}

	if ok := f.CheckAccess(context.Background(), msg); !ok {
		t.Fatal("expected admin chat to be allowed")
	}
}

func TestCheckAccess_PrivateMember_PersistsExactlyOnce(t *testing.T) {
	memberSvc := &fakeMemberService{}
	tg := &fakeTGMember{status: "member"}
	f := NewChatFilter(-1001, -2002, memberSvc, telegram.NewOps(tg))
	msg := &models.Message{Chat: models.Chat{ID: 42, Type: models.ChatTypePrivate}, From: &models.User{ID: 88, Username: "u", FirstName: "U"}}

	if ok := f.CheckAccess(context.Background(), msg); !ok {
		t.Fatal("expected private member message to be allowed")
	}
	if memberSvc.ensureActiveCalls != 1 {
		t.Fatalf("expected one EnsureActiveMemberSeen call in private access flow, got %d", memberSvc.ensureActiveCalls)
	}
	if tg.getMemberCalls != 1 || tg.lastGetChatID != -1001 || tg.lastGetUserID != 88 {
		t.Fatalf("expected GetChatMember to use member source chat (-1001) and user 88, got calls=%d chat=%d user=%d", tg.getMemberCalls, tg.lastGetChatID, tg.lastGetUserID)
	}
}
