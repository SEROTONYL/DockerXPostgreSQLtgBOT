package admin

import (
	"context"
	"testing"

	"serotonyl.ru/telegram-bot/internal/features/members"
)

func TestModeratorForbiddenUndoCallbackIsRejected(t *testing.T) {
	tg := &fakeTG{}
	h := newModeratorHandlerForFlow(t, &fakeMemberRepoHandlers{members: map[int64]*members.Member{}}, tg)
	h.setUndoRoleChange(77, 1001, "old", "new")

	if !h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminUndoLast)) {
		t.Fatalf("expected callback handled")
	}
	if tg.count("send") == 0 {
		t.Fatalf("expected callback denial message")
	}
	if undo := h.popUndoRoleChange(77); undo == nil {
		t.Fatalf("forbidden undo callback must not consume undo state")
	}
}
