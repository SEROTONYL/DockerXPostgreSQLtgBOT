package karma

import (
	"context"
	"errors"
	"testing"

	models "github.com/mymmrac/telego"

	"serotonyl.ru/telegram-bot/internal/common"
	"serotonyl.ru/telegram-bot/internal/features/members"
)

type fakeMemberResolver struct {
	byID       map[int64]*members.Member
	byUsername map[string]*members.Member
}

func (f fakeMemberResolver) GetByUserID(context.Context, int64) (*members.Member, error) {
	return nil, errors.New("not found")
}

func (f fakeMemberResolver) GetByUsername(_ context.Context, username string) (*members.Member, error) {
	member := f.byUsername[username]
	if member == nil {
		return nil, errors.New("not found")
	}
	return member, nil
}

func TestResolveThanksTargetExplicitUsernameWinsOverReply(t *testing.T) {
	h := &Handler{
		memberService: fakeMemberResolver{
			byUsername: map[string]*members.Member{
				"target": {UserID: 42, Username: "target"},
			},
		},
	}

	msg := &models.Message{
		ReplyToMessage: &models.Message{
			From: &models.User{ID: 99, Username: "reply_user"},
		},
	}

	userID, display, err := h.resolveThanksTarget(context.Background(), msg, []string{"@target"})
	if err != nil {
		t.Fatalf("resolveThanksTarget() error = %v", err)
	}
	if userID != 42 || display != "@target" {
		t.Fatalf("unexpected target: userID=%d display=%q", userID, display)
	}
}

func TestResolveThanksTargetMissing(t *testing.T) {
	h := &Handler{memberService: fakeMemberResolver{}}

	_, _, err := h.resolveThanksTarget(context.Background(), &models.Message{}, nil)
	if !errors.Is(err, common.ErrThanksTargetMissing) {
		t.Fatalf("expected ErrThanksTargetMissing, got %v", err)
	}
}
