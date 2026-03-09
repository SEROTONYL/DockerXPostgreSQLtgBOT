package bot

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	models "github.com/mymmrac/telego"
	log "github.com/sirupsen/logrus"

	"serotonyl.ru/telegram-bot/internal/bot/middleware"
	"serotonyl.ru/telegram-bot/internal/commands"
	"serotonyl.ru/telegram-bot/internal/config"
	"serotonyl.ru/telegram-bot/internal/features/admin"
	"serotonyl.ru/telegram-bot/internal/features/members"
	"serotonyl.ru/telegram-bot/internal/jobs"
	"serotonyl.ru/telegram-bot/internal/telegram"
)

type fakeTGStatus struct {
	sent              []string
	callbackAckCalls  int
	lastCallbackID    string
	lastCallbackText  string
	lastCallbackAlert bool
}

type policyChatFilterStatus struct {
	memberSourceChatID int64
	adminChatID        int64
}

func (f policyChatFilterStatus) CheckAccess(ctx context.Context, message *models.Message) bool {
	if message == nil || message.From == nil {
		return false
	}
	if message.Chat.ID == f.adminChatID || message.Chat.ID == f.memberSourceChatID {
		return true
	}
	return message.Chat.Type == models.ChatTypePrivate
}
func (f *fakeTGStatus) SendMessage(chatID int64, text string, markup *models.InlineKeyboardMarkup) (int, error) {
	f.sent = append(f.sent, text)
	return len(f.sent), nil
}
func (f *fakeTGStatus) EditMessage(chatID int64, messageID int, text string, markup *models.InlineKeyboardMarkup) error {
	return nil
}
func (f *fakeTGStatus) AnswerCallbackQuery(callbackID string, text string, showAlert bool) error {
	f.callbackAckCalls++
	f.lastCallbackID = callbackID
	f.lastCallbackText = text
	f.lastCallbackAlert = showAlert
	return nil
}
func (f *fakeTGStatus) GetChatMember(chatID int64, userID int64) (member models.ChatMember, err error) {
	return &models.ChatMemberMember{Status: "member", User: models.User{ID: userID}}, nil
}

func (f *fakeTGStatus) EditReplyMarkup(chatID int64, messageID int, markup *models.InlineKeyboardMarkup) error {
	return nil
}

func (f *fakeTGStatus) DeleteMessage(chatID int64, messageID int) error {
	return nil
}

type fakeMembersRepoStatus struct {
	active            int
	left              int
	pending           int
	ensureSeenCalls   int
	ensureActiveCalls int
	upsertCalls       int
	markLeftCalls     int
	callOrder         []string
}

func (f *fakeMembersRepoStatus) UpsertActiveMember(ctx context.Context, userID int64, username, name string, isBot bool, joinedAt time.Time) error {
	f.upsertCalls++
	return nil
}
func (f *fakeMembersRepoStatus) MarkMemberLeft(ctx context.Context, userID int64, leftAt, deleteAfter time.Time) error {
	f.markLeftCalls++
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
func (f *fakeMembersRepoStatus) FindByNickname(ctx context.Context, nickname string) (*members.Member, error) {
	return &members.Member{}, nil
}
func (f *fakeMembersRepoStatus) EnsureMemberSeen(ctx context.Context, userID int64, username, name string, isBot bool, seenAt time.Time) error {
	f.ensureSeenCalls++
	return nil
}
func (f *fakeMembersRepoStatus) EnsureActiveMemberSeen(ctx context.Context, userID int64, username, name string, isBot bool, seenAt time.Time) error {
	f.ensureActiveCalls++
	f.callOrder = append(f.callOrder, "ensure_active")
	return nil
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

func (f *fakeMembersRepoStatus) ListActiveUserIDs(ctx context.Context) ([]int64, error) {
	return nil, nil
}

func (f *fakeMembersRepoStatus) ListRefreshCandidateUserIDs(ctx context.Context) ([]int64, error) {
	return nil, nil
}

func (f *fakeMembersRepoStatus) UpdateMemberTag(ctx context.Context, userID int64, tag *string, updatedAt time.Time) error {
	return nil
}

type fakePurgeMetricsProvider struct {
	m jobs.PurgeMetrics
}

func (f fakePurgeMetricsProvider) GetPurgeMetrics() jobs.PurgeMetrics { return f.m }

type fakeStreakServiceStatus struct {
	calls     int
	callOrder *[]string
	err       error
}

func (s *fakeStreakServiceStatus) CountMessage(ctx context.Context, userID int64, messageID int64, text string) error {
	s.calls++
	if s.callOrder != nil {
		*s.callOrder = append(*s.callOrder, "count_message")
	}
	return s.err
}

func (s *fakeStreakServiceStatus) CreateStreak(ctx context.Context, userID int64) error { return nil }

func registerTestCommands(b *Bot) {
	admin.NewFeature(&config.Config{}, b.ops, b.adminHandler, b.memberService, func() jobs.PurgeMetrics {
		if b.purgeMetricsProvider != nil {
			return b.purgeMetricsProvider.GetPurgeMetrics()
		}
		return jobs.PurgeMetrics{}
	}).RegisterCommands(b.cmdRouter)
}

func TestMembersStatusCommand_IgnoredOutsideAdminChat(t *testing.T) {
	tg := &fakeTGStatus{}
	repo := &fakeMembersRepoStatus{active: 10, left: 3, pending: 1}
	b := &Bot{ops: telegram.NewOps(tg), memberService: members.NewService(repo), cfg: &config.Config{}, cmdRouter: commands.NewRouter()}
	registerTestCommands(b)

	b.routeCommand(context.Background(), UpdateContext{ChatID: 111, UserID: 42, IsAdminChat: false, Now: time.Now().UTC()}, "members_status", nil)

	if len(tg.sent) != 0 {
		t.Fatalf("expected no messages, got %d", len(tg.sent))
	}
}

func TestMembersStatusCommand_ReturnsDataInAdminChat(t *testing.T) {
	tg := &fakeTGStatus{}
	repo := &fakeMembersRepoStatus{active: 10, left: 3, pending: 2}
	now := time.Now().UTC().Truncate(time.Second)
	b := &Bot{ops: telegram.NewOps(tg), memberService: members.NewService(repo), cfg: &config.Config{}, cmdRouter: commands.NewRouter(), purgeMetricsProvider: fakePurgeMetricsProvider{m: jobs.PurgeMetrics{TotalDeleted: 99, LastRunAt: now, LastRunDeleted: 5, LastError: "boom"}}}
	registerTestCommands(b)

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
		cfg:           &config.Config{MemberSourceChatID: -1001, AdminChatID: -2002, RateLimitRequests: 100, RateLimitWindow: time.Minute},
		ops:           telegram.NewOps(tg),
		memberService: memberSvc,
		chatFilter:    policyChatFilterStatus{memberSourceChatID: -1001, adminChatID: -2002},
		rateLimiter:   middleware.NewRateLimiter(100, time.Minute),
		parser:        NewCommandParser(),
		cmdRouter:     commands.NewRouter(),
	}
	registerTestCommands(b)

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
		cfg:           &config.Config{MemberSourceChatID: -1001, AdminChatID: -2002, RateLimitRequests: 100, RateLimitWindow: time.Minute},
		ops:           telegram.NewOps(tg),
		memberService: memberSvc,
		chatFilter:    policyChatFilterStatus{memberSourceChatID: -1001, adminChatID: -2002},
		rateLimiter:   middleware.NewRateLimiter(100, time.Minute),
		parser:        NewCommandParser(),
		cmdRouter:     commands.NewRouter(),
	}
	registerTestCommands(b)

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
	b := &Bot{cfg: &config.Config{MemberSourceChatID: -1001, AdminChatID: -2002}}

	if !b.shouldTouchLastSeen(UpdateContext{ChatID: -1001, UserID: 10, Message: &models.Message{}}) {
		t.Fatal("expected member-source-chat message to touch last seen")
	}
	if !b.shouldTouchLastSeen(UpdateContext{ChatID: -1001, UserID: 10, Callback: &models.CallbackQuery{}}) {
		t.Fatal("expected member-source-chat callback to touch last seen")
	}
	if b.shouldTouchLastSeen(UpdateContext{ChatID: 10, UserID: 10, IsPrivate: true, Message: &models.Message{}}) {
		t.Fatal("expected private chat to not touch last seen in strict mode")
	}
	if b.shouldTouchLastSeen(UpdateContext{ChatID: -2002, UserID: 10, IsAdminChat: true, Message: &models.Message{}}) {
		t.Fatal("expected admin chat to not touch last seen")
	}
}

func TestHandleUpdate_DeniedByChatFilter_DoesNotWriteMemberSeen(t *testing.T) {
	tg := &fakeTGStatus{}
	repo := &fakeMembersRepoStatus{}
	memberSvc := members.NewService(repo)
	b := &Bot{
		cfg:           &config.Config{MemberSourceChatID: -1001, AdminChatID: -2002, RateLimitRequests: 100, RateLimitWindow: time.Minute},
		ops:           telegram.NewOps(tg),
		memberService: memberSvc,
		chatFilter:    policyChatFilterStatus{memberSourceChatID: -1001, adminChatID: -2002},
		rateLimiter:   middleware.NewRateLimiter(100, time.Minute),
		parser:        NewCommandParser(),
		cmdRouter:     commands.NewRouter(),
	}
	registerTestCommands(b)

	// Чат не member-source/admin и не private -> ChatFilter должен отклонить апдейт.
	upd := models.Update{Message: &models.Message{Chat: models.Chat{ID: -3003, Type: models.ChatTypeSupergroup}, From: &models.User{ID: 55, Username: "u"}, Text: "!пленки"}}
	b.handleUpdate(context.Background(), upd)

	if repo.ensureSeenCalls != 0 || repo.ensureActiveCalls != 0 {
		t.Fatalf("expected no member writes when chat filter denies update, got ensureSeen=%d ensureActive=%d", repo.ensureSeenCalls, repo.ensureActiveCalls)
	}
}

func TestHandleUpdate_MembershipUpdateHandledOnce(t *testing.T) {
	tg := &fakeTGStatus{}
	repo := &fakeMembersRepoStatus{}
	memberSvc := members.NewService(repo)
	b := &Bot{
		cfg:           &config.Config{MemberSourceChatID: -1001, AdminChatID: -2002, RateLimitRequests: 100, RateLimitWindow: time.Minute},
		ops:           telegram.NewOps(tg),
		memberService: memberSvc,
		chatFilter:    policyChatFilterStatus{memberSourceChatID: -1001, adminChatID: -2002},
		rateLimiter:   middleware.NewRateLimiter(100, time.Minute),
		parser:        NewCommandParser(),
		cmdRouter:     commands.NewRouter(),
	}
	registerTestCommands(b)

	upsertUser := &models.User{ID: 55, Username: "u", FirstName: "U"}
	upsertMember := &models.ChatMemberMember{Status: "member", User: *upsertUser}
	oldMember := &models.ChatMemberMember{Status: "member", User: *upsertUser}

	upd := models.Update{
		Message: &models.Message{Chat: models.Chat{ID: -1001, Type: models.ChatTypeSupergroup}, From: upsertUser, Text: "hello"},
		MyChatMember: &models.ChatMemberUpdated{
			Chat:          models.Chat{ID: -1001, Type: models.ChatTypeSupergroup},
			OldChatMember: oldMember,
			NewChatMember: upsertMember,
		},
	}

	b.handleUpdate(context.Background(), upd)

	if repo.upsertCalls != 1 {
		t.Fatalf("expected membership upsert once, got %d", repo.upsertCalls)
	}
	if repo.ensureSeenCalls != 0 || repo.ensureActiveCalls != 0 {
		t.Fatalf("expected no regular message member writes during membership update, got ensureSeen=%d ensureActive=%d", repo.ensureSeenCalls, repo.ensureActiveCalls)
	}
}

func TestHandleUpdate_MembershipUpdateOutsideMemberSourceChat_Ignored(t *testing.T) {
	tg := &fakeTGStatus{}
	repo := &fakeMembersRepoStatus{}
	memberSvc := members.NewService(repo)
	b := &Bot{
		cfg:           &config.Config{MemberSourceChatID: -1001, AdminChatID: -2002, RateLimitRequests: 100, RateLimitWindow: time.Minute},
		ops:           telegram.NewOps(tg),
		memberService: memberSvc,
		chatFilter:    policyChatFilterStatus{memberSourceChatID: -1001, adminChatID: -2002},
		rateLimiter:   middleware.NewRateLimiter(100, time.Minute),
		parser:        NewCommandParser(),
		cmdRouter:     commands.NewRouter(),
	}

	user := &models.User{ID: 55, Username: "u", FirstName: "U"}
	newMember := &models.ChatMemberMember{Status: "member", User: *user}
	oldMember := &models.ChatMemberMember{Status: "left", User: *user}

	upd := models.Update{
		MyChatMember: &models.ChatMemberUpdated{
			Chat:          models.Chat{ID: -9999, Type: models.ChatTypeSupergroup},
			OldChatMember: oldMember,
			NewChatMember: newMember,
		},
	}

	b.handleUpdate(context.Background(), upd)

	if repo.upsertCalls != 0 || repo.markLeftCalls != 0 {
		t.Fatalf("expected no membership writes outside member source chat, got upsert=%d markLeft=%d", repo.upsertCalls, repo.markLeftCalls)
	}
}

type adminHandlerRecorder struct {
	msgCalls int
}

func (a *adminHandlerRecorder) HandleAdminCallback(ctx context.Context, cb *models.CallbackQuery) bool {
	return false
}

func (a *adminHandlerRecorder) HandleAdminMessage(ctx context.Context, chatID int64, userID int64, messageID int, text string) bool {
	a.msgCalls++
	return false
}

func TestLoginCommand_GroupChat_IgnoredWithoutSideEffects(t *testing.T) {
	tg := &fakeTGStatus{}
	adminRecorder := &adminHandlerRecorder{}
	b := &Bot{ops: telegram.NewOps(tg), adminHandler: adminRecorder, cmdRouter: commands.NewRouter()}
	b.registerCoreCommands()

	b.routeCommand(context.Background(), UpdateContext{ChatID: -3001, UserID: 77, IsPrivate: false, Now: time.Now().UTC()}, "login", nil)

	if len(tg.sent) != 0 {
		t.Fatalf("expected no outgoing messages, got %d", len(tg.sent))
	}
	if adminRecorder.msgCalls != 0 {
		t.Fatalf("expected no admin handler calls, got %d", adminRecorder.msgCalls)
	}
}

func TestHandleUpdate_MessageWithoutSender_DoesNotPanicOrWrite(t *testing.T) {
	tg := &fakeTGStatus{}
	repo := &fakeMembersRepoStatus{}
	memberSvc := members.NewService(repo)
	b := &Bot{
		cfg:           &config.Config{MemberSourceChatID: -1001, AdminChatID: -2002, RateLimitRequests: 100, RateLimitWindow: time.Minute},
		ops:           telegram.NewOps(tg),
		memberService: memberSvc,
		chatFilter:    policyChatFilterStatus{memberSourceChatID: -1001, adminChatID: -2002},
		rateLimiter:   middleware.NewRateLimiter(100, time.Minute),
		parser:        NewCommandParser(),
		cmdRouter:     commands.NewRouter(),
	}
	registerTestCommands(b)

	upd := models.Update{Message: &models.Message{Chat: models.Chat{ID: -1001, Type: models.ChatTypeSupergroup}, Text: "hello"}}
	b.handleUpdate(context.Background(), upd)

	if repo.ensureSeenCalls != 0 || repo.ensureActiveCalls != 0 {
		t.Fatalf("expected no member writes for message without sender, got ensureSeen=%d ensureActive=%d", repo.ensureSeenCalls, repo.ensureActiveCalls)
	}
}

func TestHandleUpdate_MemberSourceChatHumanMessage_PersistsBeforeCountMessage(t *testing.T) {
	tg := &fakeTGStatus{}
	repo := &fakeMembersRepoStatus{}
	memberSvc := members.NewService(repo)
	streakSvc := &fakeStreakServiceStatus{callOrder: &repo.callOrder}
	b := &Bot{
		cfg: &config.Config{
			MemberSourceChatID:    -1001,
			AdminChatID:           -2002,
			RateLimitRequests:     100,
			RateLimitWindow:       time.Minute,
			FeatureStreaksEnabled: true,
		},
		ops:           telegram.NewOps(tg),
		memberService: memberSvc,
		streakService: streakSvc,
		chatFilter:    policyChatFilterStatus{memberSourceChatID: -1001, adminChatID: -2002},
		rateLimiter:   middleware.NewRateLimiter(100, time.Minute),
		parser:        NewCommandParser(),
		cmdRouter:     commands.NewRouter(),
	}

	upd := models.Update{Message: &models.Message{Chat: models.Chat{ID: -1001, Type: models.ChatTypeSupergroup}, From: &models.User{ID: 55, Username: "u", FirstName: "User", IsBot: false}, Text: "hello"}}
	b.handleUpdate(context.Background(), upd)

	if repo.ensureActiveCalls != 1 {
		t.Fatalf("expected one EnsureActiveMemberSeen call, got %d", repo.ensureActiveCalls)
	}
	if streakSvc.calls != 1 {
		t.Fatalf("expected one CountMessage call, got %d", streakSvc.calls)
	}
	if len(repo.callOrder) != 2 {
		t.Fatalf("expected 2 ordered calls, got %v", repo.callOrder)
	}
	if repo.callOrder[0] != "ensure_active" || repo.callOrder[1] != "count_message" {
		t.Fatalf("expected member persistence before count message, got %v", repo.callOrder)
	}
}

func TestHandleUpdate_MessageDrivenPipeline_UsesMemberSourceChat(t *testing.T) {
	tg := &fakeTGStatus{}
	repo := &fakeMembersRepoStatus{}
	memberSvc := members.NewService(repo)
	streakSvc := &fakeStreakServiceStatus{callOrder: &repo.callOrder}
	b := &Bot{
		cfg: &config.Config{
			MemberSourceChatID:    -1001,
			AdminChatID:           -2002,
			RateLimitRequests:     100,
			RateLimitWindow:       time.Minute,
			FeatureStreaksEnabled: true,
		},
		ops:           telegram.NewOps(tg),
		memberService: memberSvc,
		streakService: streakSvc,
		chatFilter:    policyChatFilterStatus{memberSourceChatID: -1001, adminChatID: -2002},
		rateLimiter:   middleware.NewRateLimiter(100, time.Minute),
		parser:        NewCommandParser(),
		cmdRouter:     commands.NewRouter(),
	}

	memberSourceMsg := models.Update{Message: &models.Message{Chat: models.Chat{ID: -1001, Type: models.ChatTypeSupergroup}, From: &models.User{ID: 55, Username: "u", FirstName: "User"}, Text: "hello"}}
	b.handleUpdate(context.Background(), memberSourceMsg)

	if repo.ensureActiveCalls != 1 || streakSvc.calls != 1 {
		t.Fatalf("expected member-source-chat message to drive both persistence and count once, got ensure=%d count=%d", repo.ensureActiveCalls, streakSvc.calls)
	}

	repo.ensureActiveCalls = 0
	streakSvc.calls = 0
	repo.callOrder = nil

	otherChatMsg := models.Update{Message: &models.Message{Chat: models.Chat{ID: -7777, Type: models.ChatTypeSupergroup}, From: &models.User{ID: 55, Username: "u", FirstName: "User"}, Text: "hello"}}
	b.handleUpdate(context.Background(), otherChatMsg)

	if repo.ensureActiveCalls != 0 || streakSvc.calls != 0 {
		t.Fatalf("expected non-member-source chat to not drive message-ingest pipeline, got ensure=%d count=%d", repo.ensureActiveCalls, streakSvc.calls)
	}
}

func TestHandleUpdate_MemberSourceChatCommand_DoesNotCountStreakMessage(t *testing.T) {
	tg := &fakeTGStatus{}
	repo := &fakeMembersRepoStatus{}
	memberSvc := members.NewService(repo)
	streakSvc := &fakeStreakServiceStatus{callOrder: &repo.callOrder}
	b := &Bot{
		cfg: &config.Config{
			MemberSourceChatID:    -1001,
			AdminChatID:           -2002,
			RateLimitRequests:     100,
			RateLimitWindow:       time.Minute,
			FeatureStreaksEnabled: true,
		},
		ops:           telegram.NewOps(tg),
		memberService: memberSvc,
		streakService: streakSvc,
		chatFilter:    policyChatFilterStatus{memberSourceChatID: -1001, adminChatID: -2002},
		rateLimiter:   middleware.NewRateLimiter(100, time.Minute),
		parser:        NewCommandParser(),
		cmdRouter:     commands.NewRouter(),
	}

	upd := models.Update{Message: &models.Message{Chat: models.Chat{ID: -1001, Type: models.ChatTypeSupergroup}, From: &models.User{ID: 55, Username: "u", FirstName: "User", IsBot: false}, Text: "!пленки"}}
	b.handleUpdate(context.Background(), upd)

	if repo.ensureActiveCalls != 1 {
		t.Fatalf("expected one EnsureActiveMemberSeen call, got %d", repo.ensureActiveCalls)
	}
	if streakSvc.calls != 0 {
		t.Fatalf("expected no CountMessage call for commands, got %d", streakSvc.calls)
	}
}

func TestHandleUpdate_MemberSourceChatNonTextMessage_DoesNotCountStreakMessage(t *testing.T) {
	tg := &fakeTGStatus{}
	repo := &fakeMembersRepoStatus{}
	memberSvc := members.NewService(repo)
	streakSvc := &fakeStreakServiceStatus{callOrder: &repo.callOrder}
	b := &Bot{
		cfg: &config.Config{
			MemberSourceChatID:    -1001,
			AdminChatID:           -2002,
			RateLimitRequests:     100,
			RateLimitWindow:       time.Minute,
			FeatureStreaksEnabled: true,
		},
		ops:           telegram.NewOps(tg),
		memberService: memberSvc,
		streakService: streakSvc,
		chatFilter:    policyChatFilterStatus{memberSourceChatID: -1001, adminChatID: -2002},
		rateLimiter:   middleware.NewRateLimiter(100, time.Minute),
		parser:        NewCommandParser(),
		cmdRouter:     commands.NewRouter(),
	}

	upd := models.Update{Message: &models.Message{Chat: models.Chat{ID: -1001, Type: models.ChatTypeSupergroup}, From: &models.User{ID: 55, Username: "u", FirstName: "User", IsBot: false}}}
	b.handleUpdate(context.Background(), upd)

	if repo.ensureActiveCalls != 1 {
		t.Fatalf("expected non-text message to persist member once, got %d", repo.ensureActiveCalls)
	}
	if streakSvc.calls != 0 {
		t.Fatalf("expected no CountMessage call for non-text messages, got %d", streakSvc.calls)
	}
}

func TestHandleUpdate_CallbackWithoutMessage_AcksAndReturns(t *testing.T) {
	tg := &fakeTGStatus{}
	b := &Bot{ops: telegram.NewOps(tg)}

	handled := b.handleCallbackUpdate(context.Background(), UpdateContext{
		Callback: &models.CallbackQuery{
			ID:   "cb-no-message",
			From: models.User{ID: 55},
		},
	})

	if !handled {
		t.Fatal("expected callback without message to be handled")
	}
	if tg.callbackAckCalls != 1 {
		t.Fatalf("expected one callback ack, got %d", tg.callbackAckCalls)
	}
	if tg.lastCallbackID != "cb-no-message" {
		t.Fatalf("unexpected callback id: %q", tg.lastCallbackID)
	}
	if tg.lastCallbackText != "" || tg.lastCallbackAlert {
		t.Fatalf("expected harmless ack, got text=%q alert=%v", tg.lastCallbackText, tg.lastCallbackAlert)
	}
}

func TestHandleUpdate_CallbackWithInaccessibleMessage_AcksAndReturns(t *testing.T) {
	tg := &fakeTGStatus{}
	b := &Bot{ops: telegram.NewOps(tg)}

	handled := b.handleCallbackUpdate(context.Background(), UpdateContext{
		Callback: &models.CallbackQuery{
			ID:   "cb-inaccessible",
			From: models.User{ID: 55},
			Message: &models.InaccessibleMessage{
				Chat:      models.Chat{ID: -1001, Type: models.ChatTypeSupergroup},
				MessageID: 42,
			},
		},
	})

	if !handled {
		t.Fatal("expected inaccessible callback to be handled")
	}
	if tg.callbackAckCalls != 1 {
		t.Fatalf("expected one callback ack, got %d", tg.callbackAckCalls)
	}
	if tg.lastCallbackID != "cb-inaccessible" {
		t.Fatalf("unexpected callback id: %q", tg.lastCallbackID)
	}
}

func TestHandleUpdate_CountMessageErrorIsLogged(t *testing.T) {
	tg := &fakeTGStatus{}
	repo := &fakeMembersRepoStatus{}
	memberSvc := members.NewService(repo)
	streakSvc := &fakeStreakServiceStatus{err: errors.New("count failed")}
	b := &Bot{
		cfg: &config.Config{
			MemberSourceChatID:    -1001,
			AdminChatID:           -2002,
			RateLimitRequests:     100,
			RateLimitWindow:       time.Minute,
			FeatureStreaksEnabled: true,
		},
		ops:           telegram.NewOps(tg),
		memberService: memberSvc,
		streakService: streakSvc,
		chatFilter:    policyChatFilterStatus{memberSourceChatID: -1001, adminChatID: -2002},
		rateLimiter:   middleware.NewRateLimiter(100, time.Minute),
		parser:        NewCommandParser(),
		cmdRouter:     commands.NewRouter(),
	}

	var logBuf bytes.Buffer
	prevOut := log.StandardLogger().Out
	prevLevel := log.StandardLogger().Level
	log.SetOutput(&logBuf)
	log.SetLevel(log.ErrorLevel)
	defer log.SetOutput(prevOut)
	defer log.SetLevel(prevLevel)

	upd := models.Update{Message: &models.Message{MessageID: 321, Chat: models.Chat{ID: -1001, Type: models.ChatTypeSupergroup}, From: &models.User{ID: 55, Username: "u", FirstName: "User"}, Text: "hello there now please"}}
	b.handleUpdate(context.Background(), upd)

	logged := logBuf.String()
	if !strings.Contains(logged, "streak count failed") {
		t.Fatalf("expected streak count error log, got %q", logged)
	}
	if !strings.Contains(logged, "user_id=55") || !strings.Contains(logged, "message_id=321") {
		t.Fatalf("expected user and message context in log, got %q", logged)
	}
}

func TestHandleUpdate_MemberSourceChatBotMessage_StaysExcludedFromRoleCandidates(t *testing.T) {
	tg := &fakeTGStatus{}
	repo := &fakeMembersRepoStatus{}
	memberSvc := members.NewService(repo)
	b := &Bot{
		cfg:           &config.Config{MemberSourceChatID: -1001, AdminChatID: -2002, RateLimitRequests: 100, RateLimitWindow: time.Minute},
		ops:           telegram.NewOps(tg),
		memberService: memberSvc,
		chatFilter:    policyChatFilterStatus{memberSourceChatID: -1001, adminChatID: -2002},
		rateLimiter:   middleware.NewRateLimiter(100, time.Minute),
		parser:        NewCommandParser(),
		cmdRouter:     commands.NewRouter(),
	}

	upd := models.Update{Message: &models.Message{Chat: models.Chat{ID: -1001, Type: models.ChatTypeSupergroup}, From: &models.User{ID: 77, Username: "helper_bot", FirstName: "Helper", IsBot: true}, Text: "auto"}}
	b.handleUpdate(context.Background(), upd)

	if repo.ensureActiveCalls != 1 {
		t.Fatalf("expected bot sender to be persisted once for membership state, got %d", repo.ensureActiveCalls)
	}
	if repo.ensureSeenCalls != 0 {
		t.Fatalf("expected no EnsureMemberSeen calls, got %d", repo.ensureSeenCalls)
	}
}

func TestHandleUpdate_MemberSourceChatNonTextMessage_PersistsMemberOnce(t *testing.T) {
	tg := &fakeTGStatus{}
	repo := &fakeMembersRepoStatus{}
	memberSvc := members.NewService(repo)
	b := &Bot{
		cfg:           &config.Config{MemberSourceChatID: -1001, AdminChatID: -2002, RateLimitRequests: 100, RateLimitWindow: time.Minute},
		ops:           telegram.NewOps(tg),
		memberService: memberSvc,
		chatFilter:    policyChatFilterStatus{memberSourceChatID: -1001, adminChatID: -2002},
		rateLimiter:   middleware.NewRateLimiter(100, time.Minute),
		parser:        NewCommandParser(),
		cmdRouter:     commands.NewRouter(),
	}

	upd := models.Update{Message: &models.Message{Chat: models.Chat{ID: -1001, Type: models.ChatTypeSupergroup}, From: &models.User{ID: 55, Username: "u", FirstName: "User", IsBot: false}}}
	b.handleUpdate(context.Background(), upd)

	if repo.ensureActiveCalls != 1 {
		t.Fatalf("expected non-text message to persist member once, got %d", repo.ensureActiveCalls)
	}
}

func TestHandleUpdate_NonTextPrivateMessage_SkipsTextOnlyPaths(t *testing.T) {
	tg := &fakeTGStatus{}
	adminRecorder := &adminHandlerRecorder{}
	repo := &fakeMembersRepoStatus{}
	b := &Bot{
		cfg:           &config.Config{MemberSourceChatID: -1001, AdminChatID: -2002, RateLimitRequests: 100, RateLimitWindow: time.Minute},
		ops:           telegram.NewOps(tg),
		memberService: members.NewService(repo),
		adminHandler:  adminRecorder,
		chatFilter:    policyChatFilterStatus{memberSourceChatID: -1001, adminChatID: -2002},
		rateLimiter:   middleware.NewRateLimiter(100, time.Minute),
		parser:        NewCommandParser(),
		cmdRouter:     commands.NewRouter(),
	}
	b.registerCoreCommands()

	b.handleUpdate(context.Background(), models.Update{Message: &models.Message{Chat: models.Chat{ID: 42, Type: models.ChatTypePrivate}, From: &models.User{ID: 55, Username: "u", FirstName: "User"}}})

	if adminRecorder.msgCalls != 0 {
		t.Fatalf("expected non-text private message to skip admin text handler, got %d calls", adminRecorder.msgCalls)
	}
	if repo.ensureActiveCalls != 0 {
		t.Fatalf("expected message handler to skip private member persistence, got %d", repo.ensureActiveCalls)
	}
}

func TestHandleUpdate_PrivateTextMessage_GoesThroughTextOnlyPath(t *testing.T) {
	tg := &fakeTGStatus{}
	adminRecorder := &adminHandlerRecorder{}
	repo := &fakeMembersRepoStatus{}
	b := &Bot{
		cfg:           &config.Config{MemberSourceChatID: -1001, AdminChatID: -2002, RateLimitRequests: 100, RateLimitWindow: time.Minute},
		ops:           telegram.NewOps(tg),
		memberService: members.NewService(repo),
		adminHandler:  adminRecorder,
		chatFilter:    policyChatFilterStatus{memberSourceChatID: -1001, adminChatID: -2002},
		rateLimiter:   middleware.NewRateLimiter(100, time.Minute),
		parser:        NewCommandParser(),
		cmdRouter:     commands.NewRouter(),
	}
	b.registerCoreCommands()

	b.handleUpdate(context.Background(), models.Update{Message: &models.Message{Chat: models.Chat{ID: 42, Type: models.ChatTypePrivate}, From: &models.User{ID: 55, Username: "u", FirstName: "User"}, Text: "hello"}})

	if adminRecorder.msgCalls != 1 {
		t.Fatalf("expected private text message to hit admin text handler once, got %d", adminRecorder.msgCalls)
	}
	if repo.ensureActiveCalls != 0 {
		t.Fatalf("expected message handler to skip private member persistence, got %d", repo.ensureActiveCalls)
	}
}

func (f *fakeMembersRepoStatus) GetUsersWithRole(ctx context.Context) ([]*members.Member, error) {
	return nil, nil
}

func (f *fakeMembersRepoStatus) GetUsersWithoutRole(ctx context.Context) ([]*members.Member, error) {
	return nil, nil
}
