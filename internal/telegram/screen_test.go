package telegram

import (
	"context"
	"errors"
	"testing"
)

func TestRenderScreen_EditSuccess(t *testing.T) {
	client := &fakeClient{}
	ops := NewOps(client)

	msgID, usedEdit, err := RenderScreen(context.Background(), ops, Screen{ChatID: 1, MessageID: 10, Text: "ok"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if msgID != 10 {
		t.Fatalf("msgID = %d, want 10", msgID)
	}
	if !usedEdit {
		t.Fatal("usedEdit = false, want true")
	}
	if client.editCalls != 1 {
		t.Fatalf("editCalls = %d, want 1", client.editCalls)
	}
	if client.sendCalls != 0 {
		t.Fatalf("sendCalls = %d, want 0", client.sendCalls)
	}
}

func TestRenderScreen_EditNotModified(t *testing.T) {
	client := &fakeClient{editErr: errors.New("Bad Request: message is not modified")}
	ops := NewOps(client)

	msgID, usedEdit, err := RenderScreen(context.Background(), ops, Screen{ChatID: 1, MessageID: 10, Text: "ok"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if msgID != 10 {
		t.Fatalf("msgID = %d, want 10", msgID)
	}
	if !usedEdit {
		t.Fatal("usedEdit = false, want true")
	}
	if client.editCalls != 1 {
		t.Fatalf("editCalls = %d, want 1", client.editCalls)
	}
	if client.sendCalls != 0 {
		t.Fatalf("sendCalls = %d, want 0", client.sendCalls)
	}
}

func TestRenderScreen_EditNotFoundFallbackSend(t *testing.T) {
	client := &fakeClient{editErr: errors.New("Bad Request: message to edit not found")}
	ops := NewOps(client)

	msgID, usedEdit, err := RenderScreen(context.Background(), ops, Screen{ChatID: 1, MessageID: 10, Text: "ok"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if usedEdit {
		t.Fatal("usedEdit = true, want false")
	}
	if msgID == 10 {
		t.Fatalf("msgID = %d, want new message id", msgID)
	}
	if client.editCalls != 1 {
		t.Fatalf("editCalls = %d, want 1", client.editCalls)
	}
	if client.sendCalls != 1 {
		t.Fatalf("sendCalls = %d, want 1", client.sendCalls)
	}
}

func TestRenderScreen_EditCantBeEditedFallbackSend(t *testing.T) {
	client := &fakeClient{editErr: errors.New("Bad Request: message can't be edited")}
	ops := NewOps(client)

	msgID, usedEdit, err := RenderScreen(context.Background(), ops, Screen{ChatID: 1, MessageID: 10, Text: "ok"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if usedEdit {
		t.Fatal("usedEdit = true, want false")
	}
	if msgID == 10 {
		t.Fatalf("msgID = %d, want new message id", msgID)
	}
	if client.editCalls != 1 {
		t.Fatalf("editCalls = %d, want 1", client.editCalls)
	}
	if client.sendCalls != 1 {
		t.Fatalf("sendCalls = %d, want 1", client.sendCalls)
	}
}

func TestRenderScreen_EditForbiddenFallbackSend(t *testing.T) {
	client := &fakeClient{editErr: errors.New("Forbidden: bot was blocked by the user")}
	ops := NewOps(client)

	msgID, usedEdit, err := RenderScreen(context.Background(), ops, Screen{ChatID: 1, MessageID: 10, Text: "ok"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if usedEdit {
		t.Fatal("usedEdit = true, want false")
	}
	if msgID == 10 {
		t.Fatalf("msgID = %d, want new message id", msgID)
	}
	if client.editCalls != 1 {
		t.Fatalf("editCalls = %d, want 1", client.editCalls)
	}
	if client.sendCalls != 1 {
		t.Fatalf("sendCalls = %d, want 1", client.sendCalls)
	}
}
