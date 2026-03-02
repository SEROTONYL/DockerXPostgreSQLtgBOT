package uiwizard

import (
	"errors"
	"testing"

	"github.com/go-telegram/bot/models"
)

type fakeRenderer struct {
	editErr   error
	editCalls int
	sendCalls int
	sendID    int
}

func (f *fakeRenderer) EditMessageText(chatID int64, messageID int, text string, markup *models.InlineKeyboardMarkup) error {
	f.editCalls++
	return f.editErr
}

func (f *fakeRenderer) SendMessage(chatID int64, text string, markup *models.InlineKeyboardMarkup) (newMessageID int, err error) {
	f.sendCalls++
	if f.sendID == 0 {
		f.sendID = 555
	}
	return f.sendID, nil
}

func TestRenderFallbackToSend(t *testing.T) {
	st := &WizardState{ChatID: 1, MessageID: 10}
	r := &fakeRenderer{editErr: errors.New("message not found")}
	err := Render(r, st, Output{Text: "x"}, func(error) bool { return true }, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.editCalls != 1 || r.sendCalls != 1 || st.MessageID != 555 {
		t.Fatalf("unexpected render behavior")
	}
}

func TestResetAndTransition(t *testing.T) {
	st := &WizardState{ChatID: 1, MessageID: 2, Step: "a", AwaitTextFor: "amount"}
	if !EnsureStep(st, "a") {
		t.Fatalf("ensure step failed")
	}
	Transition(st, "b")
	if st.Step != "b" {
		t.Fatalf("transition failed")
	}
	Reset(st)
	if st.Step != "" || st.MessageID != 0 || IsAwaitingText(st) {
		t.Fatalf("reset failed")
	}
}
