package bot

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"

	"serotonyl.ru/telegram-bot/internal/bot/filters"
	"serotonyl.ru/telegram-bot/internal/bot/middleware"
	"serotonyl.ru/telegram-bot/internal/config"
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
	active            int
	left              int
	pending           int
	ensureSeenCalls   int
	ensureActiveCalls int
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
func (f *fakeMembersRepoStatus) EnsureMemberSeen(ctx context.Context, userID int64, username, name string, seenAt time.Time) error {
	f.ensureSeenCalls++
	return nil
}
func (f *fakeMembersRepoStatus) EnsureActiveMemberSeen(ctx context.Context, userID int64, username, name string, seenAt time.Time) error {
	f.ensureActiveCalls++
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

func TestIsAdminChatAllowedCommand(t *testing.T) {
	if !isAdminChatAllowedCommand("members_status") || !isAdminChatAllowedCommand("members_stats") {
		t.Fatal("expected admin status commands to be allowed")
	}
	if isAdminChatAllowedCommand("пленки") {
		t.Fatal("expected non-admin command to be blocked")
	}
}

func TestHandleUpdate_AdminChatIgnoresNonAdminCommands(t *testing.T) {
	tg := &fakeTGStatus{}
	repo := &fakeMembersRepoStatus{}
	memberSvc := members.NewService(repo)
	b := &Bot{
		cfg:           &config.Config{MainGroupID: -1001, FloodChatID: -1001, AdminChatID: -2002, RateLimitRequests: 100, RateLimitWindow: time.Minute},
		tg:            tg,
		memberService: memberSvc,
		chatFilter:    filters.NewChatFilter(-1001, -2002, memberSvc, tg),
		rateLimiter:   middleware.NewRateLimiter(100, time.Minute),
		parser:        NewCommandParser(),
	}

	upd := models.Update{Message: &models.Message{Chat: models.Chat{ID: -2002, Type: models.ChatTypeSupergroup}, From: &models.User{ID: 42, Username: "u"}, Text: "/пленки"}}
	b.handleUpdate(context.Background(), upd)

	if len(tg.sent) != 0 {
		t.Fatalf("expected no outgoing messages for non-admin command in admin chat, got %d", len(tg.sent))
	}
	if repo.ensureSeenCalls != 0 || repo.ensureActiveCalls != 0 {
		t.Fatalf("expected no member writes in admin chat, got ensureSeen=%d ensureActive=%d", repo.ensureSeenCalls, repo.ensureActiveCalls)
	}
}

func TestHandleUpdate_AdminChatIgnoresPlainMessages(t *testing.T) {
	tg := &fakeTGStatus{}
	repo := &fakeMembersRepoStatus{}
	memberSvc := members.NewService(repo)
	b := &Bot{
		cfg:           &config.Config{MainGroupID: -1001, FloodChatID: -1001, AdminChatID: -2002, RateLimitRequests: 100, RateLimitWindow: time.Minute},
		tg:            tg,
		memberService: memberSvc,
		chatFilter:    filters.NewChatFilter(-1001, -2002, memberSvc, tg),
		rateLimiter:   middleware.NewRateLimiter(100, time.Minute),
		parser:        NewCommandParser(),
	}

	upd := models.Update{Message: &models.Message{Chat: models.Chat{ID: -2002, Type: models.ChatTypeSupergroup}, From: &models.User{ID: 42, Username: "u"}, Text: "hello admin chat"}}
	b.handleUpdate(context.Background(), upd)

	if len(tg.sent) != 0 {
		t.Fatalf("expected no outgoing messages for plain admin-chat message, got %d", len(tg.sent))
	}
	if repo.ensureSeenCalls != 0 || repo.ensureActiveCalls != 0 {
		t.Fatalf("expected no member writes in admin chat, got ensureSeen=%d ensureActive=%d", repo.ensureSeenCalls, repo.ensureActiveCalls)
	}
}

func TestShouldTouchLastSeen_UsesUpdateTypeAndChatOnly(t *testing.T) {
	b := &Bot{cfg: &config.Config{MainGroupID: -1001, AdminChatID: -2002}}
	ctx := context.Background()

	if !b.shouldTouchLastSeen(ctx, UpdateContext{ChatID: -1001, UserID: 10, Message: &models.Message{}}) {
		t.Fatal("expected main-group message to touch last seen")
	}
	if !b.shouldTouchLastSeen(ctx, UpdateContext{ChatID: -1001, UserID: 10, Callback: &models.CallbackQuery{}}) {
		t.Fatal("expected main-group callback to touch last seen")
	}
	if b.shouldTouchLastSeen(ctx, UpdateContext{ChatID: 10, UserID: 10, IsPrivate: true, Message: &models.Message{}}) {
		t.Fatal("expected private chat to not touch last seen in strict mode")
	}
	if b.shouldTouchLastSeen(ctx, UpdateContext{ChatID: -2002, UserID: 10, IsAdminChat: true, Message: &models.Message{}}) {
		t.Fatal("expected admin chat to not touch last seen")
	}
}
