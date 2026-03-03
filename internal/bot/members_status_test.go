package bot

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"

	"serotonyl.ru/telegram-bot/internal/features/members"
	"serotonyl.ru/telegram-bot/internal/jobs"
)

type fakeTGStatus struct {
	sent []string
}

func (f *fakeTGStatus) SendMessage(chatID int64, text string, markup *models.InlineKeyboardMarkup) (int, error) {
	f.sent = append(f.sent, text)
	return len(f.sent), nil
}
func (f *fakeTGStatus) EditMessage(chatID int64, messageID int, text string, markup *models.InlineKeyboardMarkup) error {
	return nil
}
func (f *fakeTGStatus) AnswerCallback(callbackID string) error { return nil }
func (f *fakeTGStatus) GetChatMember(chatID int64, userID int64) (member models.ChatMember, err error) {
	return models.ChatMember{}, nil
}

type fakeMembersRepoStatus struct {
	active  int
	left    int
	pending int
}

func (f *fakeMembersRepoStatus) UpsertActiveMember(ctx context.Context, userID int64, username, name string, joinedAt time.Time) error {
	return nil
}
func (f *fakeMembersRepoStatus) MarkMemberLeft(ctx context.Context, userID int64, leftAt, deleteAfter time.Time) error {
	return nil
}
func (f *fakeMembersRepoStatus) IsActiveMember(ctx context.Context, userID int64) (bool, error) {
	return true, nil
}
func (f *fakeMembersRepoStatus) PurgeExpiredLeftMembers(ctx context.Context, now time.Time, limit int) (int, error) {
	return 0, nil
}
func (f *fakeMembersRepoStatus) GetByUserID(ctx context.Context, userID int64) (*members.Member, error) {
	return &members.Member{}, nil
}
func (f *fakeMembersRepoStatus) GetByUsername(ctx context.Context, username string) (*members.Member, error) {
	return &members.Member{}, nil
}
func (f *fakeMembersRepoStatus) TouchLastSeen(ctx context.Context, userID int64, seenAt time.Time) error {
	return nil
}
func (f *fakeMembersRepoStatus) CountMembersByStatus(ctx context.Context) (active int, left int, err error) {
	return f.active, f.left, nil
}
func (f *fakeMembersRepoStatus) CountPendingPurge(ctx context.Context, now time.Time) (int, error) {
	return f.pending, nil
}

type fakePurgeMetricsProvider struct {
	m jobs.PurgeMetrics
}

func (f fakePurgeMetricsProvider) GetPurgeMetrics() jobs.PurgeMetrics { return f.m }

func TestMembersStatusCommand_IgnoredOutsideAdminChat(t *testing.T) {
	tg := &fakeTGStatus{}
	repo := &fakeMembersRepoStatus{active: 10, left: 3, pending: 1}
	b := &Bot{tg: tg, memberService: members.NewService(repo)}

	b.routeCommand(context.Background(), UpdateContext{ChatID: 111, UserID: 42, IsAdminChat: false, Now: time.Now().UTC()}, "members_status", nil)

	if len(tg.sent) != 0 {
		t.Fatalf("expected no messages, got %d", len(tg.sent))
	}
}

func TestMembersStatusCommand_ReturnsDataInAdminChat(t *testing.T) {
	tg := &fakeTGStatus{}
	repo := &fakeMembersRepoStatus{active: 10, left: 3, pending: 2}
	now := time.Now().UTC().Truncate(time.Second)
	b := &Bot{tg: tg, memberService: members.NewService(repo), purgeMetricsProvider: fakePurgeMetricsProvider{m: jobs.PurgeMetrics{TotalDeleted: 99, LastRunAt: now, LastRunDeleted: 5, LastError: "boom"}}}

	b.routeCommand(context.Background(), UpdateContext{ChatID: 777, UserID: 77, IsAdminChat: true, Now: now}, "members_status", nil)

	if len(tg.sent) != 1 {
		t.Fatalf("expected one message, got %d", len(tg.sent))
	}
	msg := tg.sent[0]
	checks := []string{"Active: 10", "Left (grace): 3", "Pending purge: 2", "Last deleted: 5", "Total deleted: 99", "Last error: boom"}
	for _, c := range checks {
		if !strings.Contains(msg, c) {
			t.Fatalf("expected %q in message: %s", c, msg)
		}
	}
}
