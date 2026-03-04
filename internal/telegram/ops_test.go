package telegram

import (
	"context"
	"errors"
	"testing"

	"github.com/go-telegram/bot/models"
)

type fakeClient struct {
	sendCalls int
	editCalls int

	sendErr error
	editErr error
}

func (f *fakeClient) SendMessage(chatID int64, text string, markup *models.InlineKeyboardMarkup) (int, error) {
	f.sendCalls++
	if f.sendErr != nil {
		return 0, f.sendErr
	}
	return 1000 + f.sendCalls, nil
}

func (f *fakeClient) EditMessage(chatID int64, messageID int, text string, markup *models.InlineKeyboardMarkup) error {
	f.editCalls++
	return f.editErr
}

func (f *fakeClient) EditReplyMarkup(chatID int64, messageID int, markup *models.InlineKeyboardMarkup) error {
	return nil
}

func (f *fakeClient) DeleteMessage(chatID int64, messageID int) error {
	return nil
}

func (f *fakeClient) AnswerCallback(callbackID string) error {
	return nil
}

func (f *fakeClient) AnswerCallbackQuery(callbackID string, text string, showAlert bool) error {
	return nil
}

func (f *fakeClient) GetChatMember(chatID int64, userID int64) (models.ChatMember, error) {
	return models.ChatMember{}, nil
}

func TestOpsEditOrSend_NotModified_NoFallbackSend(t *testing.T) {
	client := &fakeClient{editErr: errors.New("Bad Request: message is not modified")}
	op := NewOps(client)
	markup := models.InlineKeyboardMarkup{}

	msgID, usedEdit, err := op.EditOrSend(context.Background(), 1, 42, "same", markup)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if msgID != 42 {
		t.Fatalf("msgID = %d, want 42", msgID)
	}
	if !usedEdit {
		t.Fatalf("usedEdit = false, want true")
	}
	if client.editCalls != 1 {
		t.Fatalf("editCalls = %d, want 1", client.editCalls)
	}
	if client.sendCalls != 0 {
		t.Fatalf("sendCalls = %d, want 0", client.sendCalls)
	}
}

func TestOpsEditOrSend_NotFound_FallbackToSend(t *testing.T) {
	client := &fakeClient{editErr: errors.New("Bad Request: message to edit not found")}
	op := NewOps(client)
	markup := models.InlineKeyboardMarkup{}

	msgID, usedEdit, err := op.EditOrSend(context.Background(), 1, 42, "text", markup)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if usedEdit {
		t.Fatalf("usedEdit = true, want false")
	}
	if msgID == 42 {
		t.Fatalf("msgID = %d, want new message id", msgID)
	}
	if client.editCalls != 1 {
		t.Fatalf("editCalls = %d, want 1", client.editCalls)
	}
	if client.sendCalls != 1 {
		t.Fatalf("sendCalls = %d, want 1", client.sendCalls)
	}
}

func TestOpsEditOrSend_CantBeEdited_FallbackToSend(t *testing.T) {
	client := &fakeClient{editErr: errors.New("Bad Request: message can't be edited")}
	op := NewOps(client)
	markup := models.InlineKeyboardMarkup{}

	_, usedEdit, err := op.EditOrSend(context.Background(), 1, 42, "text", markup)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if usedEdit {
		t.Fatalf("usedEdit = true, want false")
	}
	if client.sendCalls != 1 {
		t.Fatalf("sendCalls = %d, want 1", client.sendCalls)
	}
}

func TestOpsEditOrSend_Forbidden_FallbackToSend(t *testing.T) {
	client := &fakeClient{editErr: errors.New("Forbidden: bot was blocked by the user")}
	op := NewOps(client)
	markup := models.InlineKeyboardMarkup{}

	_, usedEdit, err := op.EditOrSend(context.Background(), 1, 42, "text", markup)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if usedEdit {
		t.Fatalf("usedEdit = true, want false")
	}
	if client.sendCalls != 1 {
		t.Fatalf("sendCalls = %d, want 1", client.sendCalls)
	}
}

type callbackPreferenceClient struct {
	callbackCalls      int
	callbackQueryCalls int
	lastText           string
	lastShowAlert      bool
}

func (c *callbackPreferenceClient) SendMessage(chatID int64, text string, markup *models.InlineKeyboardMarkup) (int, error) {
	return 0, nil
}

func (c *callbackPreferenceClient) EditMessage(chatID int64, messageID int, text string, markup *models.InlineKeyboardMarkup) error {
	return nil
}

func (c *callbackPreferenceClient) EditReplyMarkup(chatID int64, messageID int, markup *models.InlineKeyboardMarkup) error {
	return nil
}

func (c *callbackPreferenceClient) DeleteMessage(chatID int64, messageID int) error {
	return nil
}

func (c *callbackPreferenceClient) AnswerCallback(callbackID string) error {
	c.callbackCalls++
	return nil
}

func (c *callbackPreferenceClient) AnswerCallbackQuery(callbackID string, text string, showAlert bool) error {
	c.callbackQueryCalls++
	c.lastText = text
	c.lastShowAlert = showAlert
	return nil
}

func (c *callbackPreferenceClient) GetChatMember(chatID int64, userID int64) (models.ChatMember, error) {
	return models.ChatMember{}, nil
}

func TestOpsAnswerCallback_WithTextPrefersCallbackQuery(t *testing.T) {
	client := &callbackPreferenceClient{}
	ops := NewOps(client)

	err := ops.AnswerCallback(context.Background(), "cb-id", "hi", true)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if client.callbackQueryCalls != 1 {
		t.Fatalf("callbackQueryCalls = %d, want 1", client.callbackQueryCalls)
	}
	if client.callbackCalls != 0 {
		t.Fatalf("callbackCalls = %d, want 0", client.callbackCalls)
	}
	if client.lastText != "hi" {
		t.Fatalf("lastText = %q, want hi", client.lastText)
	}
	if !client.lastShowAlert {
		t.Fatal("lastShowAlert = false, want true")
	}
}

func TestOpsAnswerCallback_WithoutTextPrefersSimpleCallback(t *testing.T) {
	client := &callbackPreferenceClient{}
	ops := NewOps(client)

	err := ops.AnswerCallback(context.Background(), "cb-id")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if client.callbackCalls != 1 {
		t.Fatalf("callbackCalls = %d, want 1", client.callbackCalls)
	}
	if client.callbackQueryCalls != 0 {
		t.Fatalf("callbackQueryCalls = %d, want 0", client.callbackQueryCalls)
	}
}
