package telegram

import (
	"context"
	"errors"
	"testing"
)

func TestRenderScreen_EditSuccess(t *testing.T) {
	client := &fakeClient{}
	ops := NewOps(client)

	err := RenderScreen(context.Background(), ops, Screen{ChatID: 1, MessageID: 10, Text: "ok"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
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

	err := RenderScreen(context.Background(), ops, Screen{ChatID: 1, MessageID: 10, Text: "ok"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
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

	err := RenderScreen(context.Background(), ops, Screen{ChatID: 1, MessageID: 10, Text: "ok"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if client.editCalls != 1 {
		t.Fatalf("editCalls = %d, want 1", client.editCalls)
	}
	if client.sendCalls != 1 {
		t.Fatalf("sendCalls = %d, want 1", client.sendCalls)
	}
}
