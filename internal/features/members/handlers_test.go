package members

import (
	"context"
	"testing"

	models "github.com/mymmrac/telego"

	"serotonyl.ru/telegram-bot/internal/config"
	"serotonyl.ru/telegram-bot/internal/telegram"
)

type fakeMembersTG struct {
	callbackText string
	callbackID   string
}

func (f *fakeMembersTG) SendMessage(chatID int64, text string, markup *models.InlineKeyboardMarkup) (int, error) {
	return 1, nil
}
func (f *fakeMembersTG) EditMessage(chatID int64, messageID int, text string, markup *models.InlineKeyboardMarkup) error {
	return nil
}
func (f *fakeMembersTG) EditReplyMarkup(chatID int64, messageID int, markup *models.InlineKeyboardMarkup) error {
	return nil
}
func (f *fakeMembersTG) DeleteMessage(chatID int64, messageID int) error { return nil }
func (f *fakeMembersTG) GetChatMember(chatID int64, userID int64) (models.ChatMember, error) {
	return nil, nil
}
func (f *fakeMembersTG) AnswerCallbackQuery(callbackID string, text string, showAlert bool) error {
	f.callbackID = callbackID
	f.callbackText = text
	return nil
}

type noopBalanceProvider struct{}

func (noopBalanceProvider) GetBalance(ctx context.Context, userID int64) (int64, error) {
	return 0, nil
}

func TestHandleMembersList_WrongChat_NoPanicNoop(t *testing.T) {
	h := &Handler{cfg: &config.Config{MemberSourceChatID: 100}}
	h.HandleMembersList(context.Background(), 200, 77)
}

func TestHandleMembersCallback_NonOwnerGetsNotice(t *testing.T) {
	tg := &fakeMembersTG{}
	h := NewHandler(&Service{}, noopBalanceProvider{}, telegram.NewOps(tg), &config.Config{MemberSourceChatID: 100})

	ok := h.HandleMembersCallback(context.Background(), &models.CallbackQuery{
		ID:   "cb-1",
		From: models.User{ID: 999},
		Data: "members:list:77:1",
		Message: &models.Message{
			MessageID: 10,
			Chat:      models.Chat{ID: 100},
		},
	})
	if !ok {
		t.Fatal("expected members callback handled")
	}
	if tg.callbackID == "" {
		t.Fatal("expected callback answer")
	}
	if tg.callbackText == "" {
		t.Fatal("expected non-owner notice text")
	}
}
