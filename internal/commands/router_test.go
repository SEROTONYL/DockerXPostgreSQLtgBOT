package commands

import (
	"context"
	"testing"
)

func TestRouterDispatchUnknownReturnsFalse(t *testing.T) {
	r := NewRouter()
	if ok := r.Dispatch(context.Background(), Context{}, "missing", nil); ok {
		t.Fatal("expected false for unknown command")
	}
}

func TestRouterDispatchCallsHandler(t *testing.T) {
	r := NewRouter()
	called := false
	r.Register("Test", func(ctx context.Context, c Context, args []string) {
		called = true
	})

	if ok := r.Dispatch(context.Background(), Context{}, " test ", nil); !ok {
		t.Fatal("expected true for registered command")
	}
	if !called {
		t.Fatal("expected handler to be called")
	}
}

func TestRouterRegisterDuplicatePanics(t *testing.T) {
	r := NewRouter()
	r.Register("cmd", func(ctx context.Context, c Context, args []string) {})

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on duplicate registration")
		}
	}()

	r.Register(" CMD ", func(ctx context.Context, c Context, args []string) {})
}

func TestRouterNormalizeTreatsYoAsYe(t *testing.T) {
	r := NewRouter()
	called := false
	r.Register("пленки", func(ctx context.Context, c Context, args []string) { called = true })
	if ok := r.Dispatch(context.Background(), Context{}, "плёнки", nil); !ok {
		t.Fatal("expected command alias with ё to be dispatched")
	}
	if !called {
		t.Fatal("expected handler called")
	}
}
