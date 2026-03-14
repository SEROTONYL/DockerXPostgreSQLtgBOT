package audit

import (
	"context"
	"errors"
	"testing"

	models "github.com/mymmrac/telego"

	"serotonyl.ru/telegram-bot/internal/telegram"
)

type fakeTG struct {
	sent          []int64
	texts         []string
	sendErrByChat map[int64]error
}

func (f *fakeTG) SendMessage(chatID int64, text string, markup *models.InlineKeyboardMarkup) (int, error) {
	f.sent = append(f.sent, chatID)
	f.texts = append(f.texts, text)
	if f.sendErrByChat != nil {
		if err := f.sendErrByChat[chatID]; err != nil {
			return 0, err
		}
	}
	return len(f.sent), nil
}
func (f *fakeTG) EditMessage(chatID int64, messageID int, text string, markup *models.InlineKeyboardMarkup) error {
	return nil
}
func (f *fakeTG) EditReplyMarkup(chatID int64, messageID int, markup *models.InlineKeyboardMarkup) error {
	return nil
}
func (f *fakeTG) DeleteMessage(chatID int64, messageID int) error { return nil }
func (f *fakeTG) PinChatMessage(chatID int64, messageID int, disableNotification bool) error {
	return nil
}
func (f *fakeTG) UnpinChatMessage(chatID int64, messageID int) error { return nil }
func (f *fakeTG) AnswerCallbackQuery(callbackID string, text string, showAlert bool) error {
	return nil
}
func (f *fakeTG) GetChatMember(chatID int64, userID int64) (models.ChatMember, error) {
	return nil, nil
}

func TestLoggerUsesAdminChatID(t *testing.T) {
	tg := &fakeTG{}
	logger := NewLogger(telegram.NewOps(tg), 555)

	logger.LogLogin(context.Background(), "@actor")

	if len(tg.sent) != 1 || tg.sent[0] != 555 {
		t.Fatalf("expected log to go to admin chat 555, got %#v", tg.sent)
	}
}

func TestLoggerSkipsZeroAdminChatID(t *testing.T) {
	tg := &fakeTG{}
	logger := NewLogger(telegram.NewOps(tg), 0)

	logger.LogLogin(context.Background(), "@actor")

	if len(tg.sent) != 0 {
		t.Fatalf("expected no sends when admin chat is zero, got %#v", tg.sent)
	}
}

func TestLoggerSwallowsSendFailure(t *testing.T) {
	tg := &fakeTG{sendErrByChat: map[int64]error{555: errors.New("boom")}}
	logger := NewLogger(telegram.NewOps(tg), 555)

	logger.LogLogin(context.Background(), "@actor")

	if len(tg.sent) != 1 {
		t.Fatalf("expected attempted send, got %#v", tg.sent)
	}
}
