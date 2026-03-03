package filters

import (
	"context"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"

	"serotonyl.ru/telegram-bot/internal/features/members"
)

type fakeMemberRepo struct{}

func (f *fakeMemberRepo) UpsertActiveMember(ctx context.Context, userID int64, username, name string, joinedAt time.Time) error {
	return nil
}
func (f *fakeMemberRepo) MarkMemberLeft(ctx context.Context, userID int64, leftAt, deleteAfter time.Time) error {
	return nil
}
func (f *fakeMemberRepo) IsActiveMember(ctx context.Context, userID int64) (bool, error) {
	return false, nil
}
func (f *fakeMemberRepo) PurgeExpiredLeftMembers(ctx context.Context, now time.Time, limit int) (int, error) {
	return 0, nil
}
func (f *fakeMemberRepo) GetByUserID(ctx context.Context, userID int64) (*members.Member, error) {
	return nil, nil
}
func (f *fakeMemberRepo) GetByUsername(ctx context.Context, username string) (*members.Member, error) {
	return nil, nil
}
func (f *fakeMemberRepo) TouchLastSeen(ctx context.Context, userID int64, seenAt time.Time) error {
	return nil
}
func (f *fakeMemberRepo) CountMembersByStatus(ctx context.Context) (active int, left int, err error) {
	return 0, 0, nil
}
func (f *fakeMemberRepo) CountPendingPurge(ctx context.Context, now time.Time) (int, error) {
	return 0, nil
}

type fakeTG struct{}

func (f *fakeTG) SendMessage(chatID int64, text string, markup *models.InlineKeyboardMarkup) (int, error) {
	return 0, nil
}
func (f *fakeTG) EditMessage(chatID int64, messageID int, text string, markup *models.InlineKeyboardMarkup) error {
	return nil
}
func (f *fakeTG) AnswerCallback(callbackID string) error { return nil }
func (f *fakeTG) GetChatMember(chatID int64, userID int64) (member models.ChatMember, err error) {
	return models.ChatMember{Type: models.ChatMemberTypeLeft}, nil
}

func TestCheckAccess_AdminChatAlwaysAllowed(t *testing.T) {
	memberSvc := members.NewService(&fakeMemberRepo{})
	f := NewChatFilter(-1001, -2002, memberSvc, &fakeTG{})
	msg := &models.Message{Chat: models.Chat{ID: -2002, Type: models.ChatTypeSupergroup}, From: &models.User{ID: 42}}

	if ok := f.CheckAccess(context.Background(), msg); !ok {
		t.Fatal("expected admin chat to be allowed")
	}
}
