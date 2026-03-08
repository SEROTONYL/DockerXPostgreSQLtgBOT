package economy

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	models "github.com/mymmrac/telego"

	"serotonyl.ru/telegram-bot/internal/commands"
	"serotonyl.ru/telegram-bot/internal/common"
	"serotonyl.ru/telegram-bot/internal/features/members"
	"serotonyl.ru/telegram-bot/internal/telegram"
)

type fakeEconomyService struct {
	balance         int64
	balanceErr      error
	transferErr     error
	transferCalls   int
	lastTransferTo  int64
	lastTransferAmt int64
}

func (f *fakeEconomyService) GetBalance(ctx context.Context, userID int64) (int64, error) {
	return f.balance, f.balanceErr
}

func (f *fakeEconomyService) Transfer(ctx context.Context, fromUserID, toUserID, amount int64) error {
	f.transferCalls++
	f.lastTransferTo = toUserID
	f.lastTransferAmt = amount
	return f.transferErr
}

func (f *fakeEconomyService) GetTransactionHistory(ctx context.Context, userID int64) (string, error) {
	return "", nil
}

type fakeMemberLookup struct {
	member *members.Member
	err    error
}

func (f *fakeMemberLookup) GetByUsername(ctx context.Context, username string) (*members.Member, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.member == nil || !strings.EqualFold(f.member.Username, username) {
		return nil, errors.New("not found")
	}
	return f.member, nil
}

type fakeEconomyTG struct {
	sent         []telegram.SendOptions
	editedText   []telegram.EditOptions
	callbackID   string
	callbackText string
	nextMsgID    int
}

func (f *fakeEconomyTG) SendMessage(chatID int64, text string, markup *models.InlineKeyboardMarkup) (int, error) {
	f.nextMsgID++
	return f.nextMsgID, nil
}
func (f *fakeEconomyTG) EditMessage(chatID int64, messageID int, text string, markup *models.InlineKeyboardMarkup) error {
	f.editedText = append(f.editedText, telegram.EditOptions{ChatID: chatID, MessageID: messageID, Text: text, ReplyMarkup: markup})
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
	f.nextMsgID++
	return f.nextMsgID, nil
}
func (f *fakeEconomyTG) EditMessageWithOptions(opts telegram.EditOptions) error {
	f.editedText = append(f.editedText, opts)
	return nil
}
func (f *fakeEconomyTG) AnswerCallbackQuery(callbackID string, text string, showAlert bool) error {
	f.callbackID = callbackID
	f.callbackText = text
	return nil
}

func TestHandleBalance_SendsReplyWithFilmEmoji(t *testing.T) {
	tg := &fakeEconomyTG{}
	h := &Handler{service: &fakeEconomyService{balance: 384655}, tgOps: telegram.NewOps(tg), confirmBy: map[string]*transferConfirmation{}, now: func() time.Time { return time.Unix(0, 0).UTC() }}

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
}

func TestHandleTransferCommand_ReplyFlowSendsFirstConfirmation(t *testing.T) {
	tg := &fakeEconomyTG{}
	h := &Handler{
		service:       &fakeEconomyService{balance: 10},
		memberService: &fakeMemberLookup{},
		tgOps:         telegram.NewOps(tg),
		confirmBy:     map[string]*transferConfirmation{},
		now:           func() time.Time { return time.Unix(100, 0).UTC() },
	}

	msg := &models.Message{
		MessageID: 77,
		Chat:      models.Chat{ID: -100},
		From:      &models.User{ID: 10, Username: "sender"},
		Text:      "!отсыпать 3",
		ReplyToMessage: &models.Message{
			From: &models.User{ID: 20, Username: "Dora_2270"},
		},
	}

	h.HandleTransferCommand(context.Background(), commands.Context{ChatID: -100, UserID: 10, MessageID: 77, Message: msg}, []string{"3"})

	if len(tg.sent) != 1 {
		t.Fatalf("expected one confirmation message, got %d", len(tg.sent))
	}
	if tg.sent[0].Text != "Вы уверены, что хотите передать 3  пользователю @Dora_2270?" {
		t.Fatalf("unexpected confirmation text: %q", tg.sent[0].Text)
	}
	if tg.sent[0].ReplyMarkup == nil || len(tg.sent[0].ReplyMarkup.InlineKeyboard) != 1 || len(tg.sent[0].ReplyMarkup.InlineKeyboard[0]) != 2 {
		t.Fatal("expected two inline buttons on one row")
	}
	yes := tg.sent[0].ReplyMarkup.InlineKeyboard[0][0]
	no := tg.sent[0].ReplyMarkup.InlineKeyboard[0][1]
	if yes.IconCustomEmojiID != confirmEmojiID {
		t.Fatalf("unexpected yes custom emoji id: %q", yes.IconCustomEmojiID)
	}
	if no.IconCustomEmojiID != cancelEmojiID {
		t.Fatalf("unexpected no custom emoji id: %q", no.IconCustomEmojiID)
	}
}

func TestHandleEconomyMessage_ParsesUsernamePhrase(t *testing.T) {
	tg := &fakeEconomyTG{}
	h := &Handler{
		service:       &fakeEconomyService{balance: 50},
		memberService: &fakeMemberLookup{member: &members.Member{UserID: 20, Username: "Dora_2270"}},
		tgOps:         telegram.NewOps(tg),
		confirmBy:     map[string]*transferConfirmation{},
		now:           func() time.Time { return time.Unix(100, 0).UTC() },
	}

	handled := h.HandleEconomyMessage(context.Background(), &models.Message{
		MessageID: 5,
		Chat:      models.Chat{ID: -10},
		From:      &models.User{ID: 1, Username: "sender"},
		Text:      "передать плёнки 5 @Dora_2270",
	})
	if !handled {
		t.Fatal("expected transfer phrase to be handled")
	}
	if len(tg.sent) != 1 {
		t.Fatalf("expected confirmation message, got %d", len(tg.sent))
	}
	if !strings.Contains(tg.sent[0].Text, "@Dora_2270") {
		t.Fatalf("unexpected text: %q", tg.sent[0].Text)
	}
}

func TestHandleEconomyCallback_RequiresOwnerAndRunsTwoSteps(t *testing.T) {
	tg := &fakeEconomyTG{}
	svc := &fakeEconomyService{balance: 50}
	h := &Handler{
		service:       svc,
		memberService: &fakeMemberLookup{},
		tgOps:         telegram.NewOps(tg),
		confirmBy:     map[string]*transferConfirmation{},
		now:           func() time.Time { return time.Unix(100, 0).UTC() },
	}
	entry := &transferConfirmation{
		Token:            "abc",
		ChatID:           -10,
		MessageID:        42,
		OwnerUserID:      1,
		FromUserID:       1,
		ToUserID:         2,
		Amount:           7,
		RecipientDisplay: "@Dora_2270",
		State:            transferStateAwaitFirst,
		ExpiresAt:        time.Unix(200, 0).UTC(),
	}
	h.confirmBy[entry.Token] = entry

	wrongUserHandled := h.HandleEconomyCallback(context.Background(), callback(-10, 42, 99, transferCallbackPrefix+"abc:"+transferConfirmYes))
	if !wrongUserHandled {
		t.Fatal("expected callback handled")
	}
	if tg.callbackText == "" {
		t.Fatal("expected callback rejection text")
	}
	if len(tg.editedText) != 0 {
		t.Fatal("wrong user must not edit message")
	}

	tg.callbackText = ""
	if !h.HandleEconomyCallback(context.Background(), callback(-10, 42, 1, transferCallbackPrefix+"abc:"+transferConfirmYes)) {
		t.Fatal("expected first confirm handled")
	}
	if len(tg.editedText) != 1 {
		t.Fatalf("expected one edit after first confirm, got %d", len(tg.editedText))
	}
	if tg.editedText[0].Text != "Вы точно уверены, что хотите передать 7  пользователю @Dora_2270?" {
		t.Fatalf("unexpected second step text: %q", tg.editedText[0].Text)
	}
	if entry.State != transferStateAwaitSecond {
		t.Fatalf("expected second-step state, got %q", entry.State)
	}

	if !h.HandleEconomyCallback(context.Background(), callback(-10, 42, 1, transferCallbackPrefix+"abc:"+transferConfirmYes)) {
		t.Fatal("expected second confirm handled")
	}
	if svc.transferCalls != 1 {
		t.Fatalf("expected transfer once, got %d", svc.transferCalls)
	}
	if len(tg.editedText) != 2 {
		t.Fatalf("expected final edit, got %d edits", len(tg.editedText))
	}
	if tg.editedText[1].Text != "✅ Передано 7  пользователю @Dora_2270." {
		t.Fatalf("unexpected final text: %q", tg.editedText[1].Text)
	}
}

func TestHandleTransferCommand_UsernameOverridesReply(t *testing.T) {
	tg := &fakeEconomyTG{}
	h := &Handler{
		service:       &fakeEconomyService{balance: 50},
		memberService: &fakeMemberLookup{member: &members.Member{UserID: 99, Username: "named_user"}},
		tgOps:         telegram.NewOps(tg),
		confirmBy:     map[string]*transferConfirmation{},
		now:           func() time.Time { return time.Unix(100, 0).UTC() },
	}

	msg := &models.Message{
		MessageID: 77,
		Chat:      models.Chat{ID: -100},
		From:      &models.User{ID: 10, Username: "sender"},
		Text:      "!отсыпать 3 @named_user",
		ReplyToMessage: &models.Message{
			From: &models.User{ID: 20, Username: "reply_user"},
		},
	}

	h.HandleTransferCommand(context.Background(), commands.Context{ChatID: -100, UserID: 10, MessageID: 77, Message: msg}, []string{"3", "@named_user"})

	if len(tg.sent) != 1 {
		t.Fatalf("expected one confirmation, got %d", len(tg.sent))
	}
	if !strings.Contains(tg.sent[0].Text, "@named_user") {
		t.Fatalf("expected explicit username to win, got %q", tg.sent[0].Text)
	}
}

func TestHandleTransferCommand_ShowsInsufficientBalance(t *testing.T) {
	tg := &fakeEconomyTG{}
	h := &Handler{
		service:       &fakeEconomyService{balance: 1},
		memberService: &fakeMemberLookup{},
		tgOps:         telegram.NewOps(tg),
		confirmBy:     map[string]*transferConfirmation{},
		now:           func() time.Time { return time.Unix(100, 0).UTC() },
	}

	msg := &models.Message{
		MessageID: 10,
		Chat:      models.Chat{ID: -100},
		From:      &models.User{ID: 10, Username: "sender"},
		Text:      "!отсыпать 5",
		ReplyToMessage: &models.Message{
			From: &models.User{ID: 20, Username: "user2"},
		},
	}

	h.HandleTransferCommand(context.Background(), commands.Context{ChatID: -100, UserID: 10, MessageID: 10, Message: msg}, []string{"5"})

	if len(tg.sent) != 1 {
		t.Fatalf("expected one error message, got %d", len(tg.sent))
	}
	if tg.sent[0].Text != "❌ Недостаточно плёнок для перевода." {
		t.Fatalf("unexpected error text: %q", tg.sent[0].Text)
	}
}

func TestHandleEconomyCallback_ExecutionErrorEditsMessage(t *testing.T) {
	tg := &fakeEconomyTG{}
	svc := &fakeEconomyService{balance: 50, transferErr: common.ErrInsufficientBalance}
	h := &Handler{
		service:       svc,
		memberService: &fakeMemberLookup{},
		tgOps:         telegram.NewOps(tg),
		confirmBy:     map[string]*transferConfirmation{},
		now:           func() time.Time { return time.Unix(100, 0).UTC() },
	}
	h.confirmBy["abc"] = &transferConfirmation{
		Token:            "abc",
		ChatID:           -10,
		MessageID:        42,
		OwnerUserID:      1,
		FromUserID:       1,
		ToUserID:         2,
		Amount:           7,
		RecipientDisplay: "@Dora_2270",
		State:            transferStateAwaitSecond,
		ExpiresAt:        time.Unix(200, 0).UTC(),
	}

	h.HandleEconomyCallback(context.Background(), callback(-10, 42, 1, transferCallbackPrefix+"abc:"+transferConfirmYes))

	if len(tg.editedText) != 1 {
		t.Fatalf("expected one final edit, got %d", len(tg.editedText))
	}
	if tg.editedText[0].Text != "❌ Недостаточно плёнок для перевода." {
		t.Fatalf("unexpected final text: %q", tg.editedText[0].Text)
	}
}

func callback(chatID int64, msgID int, userID int64, data string) *models.CallbackQuery {
	return &models.CallbackQuery{
		ID:   "cb-id",
		From: models.User{ID: userID, Username: "user"},
		Data: data,
		Message: &models.Message{
			MessageID: msgID,
			Chat:      models.Chat{ID: chatID},
		},
	}
}
