package filters

import (
	"context"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"

	"serotonyl.ru/telegram-bot/internal/bot"
	"serotonyl.ru/telegram-bot/internal/telegram"
)

type fakeTG struct{}

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
	return models.ChatMember{Type: models.ChatMemberTypeLeft}, nil
}
func (f *fakeTG) EditReplyMarkup(chatID int64, messageID int, markup *models.InlineKeyboardMarkup) error {
	return nil
}
func (f *fakeTG) DeleteMessage(chatID int64, messageID int) error {
	return nil
}

type fakeMemberService struct{}

func (f *fakeMemberService) EnsureActiveMemberSeen(ctx context.Context, userID int64, username, fullName string, now time.Time) error {
	return nil
}

func (f *fakeMemberService) UpsertActiveMember(ctx context.Context, userID int64, username, fullName string, now time.Time) error {
	return nil
}

func (f *fakeMemberService) MarkMemberLeft(ctx context.Context, userID int64, leftAt, purgeAt time.Time) error {
	return nil
}

func (f *fakeMemberService) HandleNewMember(ctx context.Context, userID int64, username, firstName, lastName string) error {
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
