package economy

import (
	"context"
	"errors"
	"strings"
	"testing"

	models "github.com/mymmrac/telego"

	"serotonyl.ru/telegram-bot/internal/telegram"
)

type fakeEconomyService struct {
	balance    int64
	balanceErr error
}

func (f *fakeEconomyService) GetBalance(ctx context.Context, userID int64) (int64, error) {
	return f.balance, f.balanceErr
}

func (f *fakeEconomyService) Transfer(ctx context.Context, fromUserID, toUserID, amount int64) error {
	return nil
}

func (f *fakeEconomyService) GetTransactionHistory(ctx context.Context, userID int64) (string, error) {
	return "", nil
}

type fakeEconomyTG struct {
	sent []telegram.SendOptions
}

func (f *fakeEconomyTG) SendMessage(chatID int64, text string, markup *models.InlineKeyboardMarkup) (int, error) {
	return 0, nil
}
func (f *fakeEconomyTG) EditMessage(chatID int64, messageID int, text string, markup *models.InlineKeyboardMarkup) error {
	return nil
}
func (f *fakeEconomyTG) EditReplyMarkup(chatID int64, messageID int, markup *models.InlineKeyboardMarkup) error {
	return nil
}
func (f *fakeEconomyTG) DeleteMessage(chatID int64, messageID int) error { return nil }
func (f *fakeEconomyTG) GetChatMember(chatID int64, userID int64) (models.ChatMember, error) {
	return nil, nil
}
func (f *fakeEconomyTG) SendMessageWithOptions(opts telegram.SendOptions) (int, error) {
	f.sent = append(f.sent, opts)
	return 1, nil
}

func TestHandleBalance_SendsReplyWithFilmEmoji(t *testing.T) {
	tg := &fakeEconomyTG{}
	h := &Handler{service: &fakeEconomyService{balance: 384655}, tgOps: telegram.NewOps(tg)}

	h.HandleBalance(context.Background(), 100, 55, 777)

	if len(tg.sent) != 1 {
		t.Fatalf("expected one message, got %d", len(tg.sent))
	}
	if tg.sent[0].ReplyToMessageID != 777 {
		t.Fatalf("expected reply to triggering message, got %d", tg.sent[0].ReplyToMessageID)
	}
	if tg.sent[0].Text != "У вас: 384655🎞️" {
		t.Fatalf("unexpected text: %q", tg.sent[0].Text)
	}
	if strings.Contains(tg.sent[0].Text, "📼") {
		t.Fatalf("unexpected old emoji in text: %q", tg.sent[0].Text)
	}
}

func TestHandleBalance_ErrorPathSendsErrorMessage(t *testing.T) {
	tg := &fakeEconomyTG{}
	h := &Handler{service: &fakeEconomyService{balanceErr: errors.New("db down")}, tgOps: telegram.NewOps(tg)}

	h.HandleBalance(context.Background(), 100, 55, 888)

	if len(tg.sent) != 1 {
		t.Fatalf("expected one message, got %d", len(tg.sent))
	}
	if tg.sent[0].ReplyToMessageID != 888 {
		t.Fatalf("expected reply to triggering message on error, got %d", tg.sent[0].ReplyToMessageID)
	}
	if tg.sent[0].Text != "❌ Ошибка получения баланса" {
		t.Fatalf("unexpected error text: %q", tg.sent[0].Text)
	}
}
