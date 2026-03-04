package filters

import (
	"context"
	"testing"

	"github.com/go-telegram/bot/models"

	"serotonyl.ru/telegram-bot/internal/telegram"
)

type fakeMemberService struct{}

func (f *fakeMemberService) IsMember(ctx context.Context, userID int64) (bool, error) {
	return false, nil
}

func (f *fakeMemberService) EnsureMember(ctx context.Context, userID int64, username, firstName, lastName string) error {
	return nil
}

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

func TestCheckAccess_AdminChatAlwaysAllowed(t *testing.T) {
	f := NewChatFilter(-1001, -2002, &fakeMemberService{}, telegram.NewOps(&fakeTG{}))
	msg := &models.Message{Chat: models.Chat{ID: -2002, Type: models.ChatTypeSupergroup}, From: &models.User{ID: 42}}

	if ok := f.CheckAccess(context.Background(), msg); !ok {
		t.Fatal("expected admin chat to be allowed")
	}
}
