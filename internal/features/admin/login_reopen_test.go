package admin

import (
	"context"
	"errors"
	"testing"

	"serotonyl.ru/telegram-bot/internal/features/members"
)

func TestHandleAdminMessage_LoginWithActiveSession_ReplacesTrackedPanel(t *testing.T) {
	tg := &fakeTG{}
	repo := &fakeMemberRepoHandlers{members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}}}
	h := newAdminHandlerForFlow(t, repo, tg)
	h.service.SetPanelMessage(77, 77, 42)

	handled := h.HandleAdminMessage(context.Background(), 77, 77, 0, "/login")
	if !handled {
		t.Fatalf("expected handled=true")
	}
	if tg.count("delete") != 1 {
		t.Fatalf("expected previous panel delete, got %d", tg.count("delete"))
	}
	if d := tg.last("delete"); d == nil || d.chatID != 77 || d.messageID != 42 {
		t.Fatalf("unexpected delete call: %#v", d)
	}
	if tg.count("edit") != 0 {
		t.Fatalf("did not expect panel reopen via edit")
	}
	if tg.count("send") == 0 {
		t.Fatalf("expected fresh panel send")
	}

	panel := h.service.GetPanelMessage(77)
	if panel.ChatID != 77 || panel.MessageID == 0 || panel.MessageID == 42 {
		t.Fatalf("expected new tracked panel, got %#v", panel)
	}
}

func TestHandleAdminMessage_LoginWithActiveSession_DeleteFailureStillSendsFreshPanel(t *testing.T) {
	tg := &fakeTG{deleteErr: errors.New("message not found")}
	repo := &fakeMemberRepoHandlers{members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}}}
	h := newAdminHandlerForFlow(t, repo, tg)
	h.service.SetPanelMessage(77, 77, 42)

	handled := h.HandleAdminMessage(context.Background(), 77, 77, 0, "/login")
	if !handled {
		t.Fatalf("expected handled=true")
	}
	if tg.count("delete") != 1 {
		t.Fatalf("expected one delete attempt, got %d", tg.count("delete"))
	}
	if tg.count("edit") != 0 {
		t.Fatalf("did not expect panel reopen via edit")
	}
	if tg.count("send") == 0 {
		t.Fatalf("expected fresh panel send after delete failure")
	}

	panel := h.service.GetPanelMessage(77)
	if panel.ChatID != 77 || panel.MessageID == 0 || panel.MessageID == 42 {
		t.Fatalf("expected new tracked panel after delete failure, got %#v", panel)
	}
}
