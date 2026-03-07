package members

import (
	"context"
	"errors"
	"testing"
	"time"

	models "github.com/mymmrac/telego"

	"serotonyl.ru/telegram-bot/internal/telegram"
)

type fakeRepo struct {
	upsertCalled         bool
	markLeftCalled       bool
	isActiveCalled       bool
	upsertUserID         int64
	upsertUsername       string
	upsertName           string
	upsertJoinedAt       time.Time
	markLeftUserID       int64
	markLeftAt           time.Time
	markLeftDeleteAfter  time.Time
	isActiveResult       bool
	purgeCalled          bool
	purgeDeleted         int
	ensureSeenCalled     bool
	ensureSeenUserID     int64
	ensureSeenUsername   string
	ensureSeenName       string
	ensureSeenAt         time.Time
	ensureActiveCalled   bool
	touchCalled          bool
	touchUserID          int64
	touchSeenAt          time.Time
	countActive          int
	countLeft            int
	pendingPurge         int
	activeUserIDs        []int64
	refreshCandidateIDs  []int64
	updateTagBlocked     bool
	updateTagErrorByUser map[int64]error
	updateTagCalls       []int64
	upsertCalls          []int64
}

func (f *fakeRepo) UpsertActiveMember(ctx context.Context, userID int64, username, name string, isBot bool, joinedAt time.Time) error {
	f.upsertCalled = true
	f.upsertUserID = userID
	f.upsertUsername = username
	f.upsertName = name
	f.upsertJoinedAt = joinedAt
	f.upsertCalls = append(f.upsertCalls, userID)
	return nil
}

func (f *fakeRepo) MarkMemberLeft(ctx context.Context, userID int64, leftAt, deleteAfter time.Time) error {
	f.markLeftCalled = true
	f.markLeftUserID = userID
	f.markLeftAt = leftAt
	f.markLeftDeleteAfter = deleteAfter
	return nil
}

func (f *fakeRepo) IsActiveMember(ctx context.Context, userID int64) (bool, error) {
	f.isActiveCalled = true
	return f.isActiveResult, nil
}

func (f *fakeRepo) PurgeExpiredLeftMembers(ctx context.Context, now time.Time, limit int) (int, error) {
	f.purgeCalled = true
	return f.purgeDeleted, nil
}

func (f *fakeRepo) GetByUserID(ctx context.Context, userID int64) (*Member, error) { return nil, nil }
func (f *fakeRepo) GetByUsername(ctx context.Context, username string) (*Member, error) {
	return nil, nil
}
func (f *fakeRepo) EnsureMemberSeen(ctx context.Context, userID int64, username, name string, isBot bool, seenAt time.Time) error {
	f.ensureSeenCalled = true
	f.ensureSeenUserID = userID
	f.ensureSeenUsername = username
	f.ensureSeenName = name
	f.ensureSeenAt = seenAt
	return nil
}
func (f *fakeRepo) EnsureActiveMemberSeen(ctx context.Context, userID int64, username, name string, isBot bool, seenAt time.Time) error {
	f.ensureActiveCalled = true
	f.ensureSeenUserID = userID
	f.ensureSeenUsername = username
	f.ensureSeenName = name
	f.ensureSeenAt = seenAt
	return nil
}
func (f *fakeRepo) TouchLastSeen(ctx context.Context, userID int64, seenAt time.Time) error {
	f.touchCalled = true
	f.touchUserID = userID
	f.touchSeenAt = seenAt
	return nil
}
func (f *fakeRepo) CountMembersByStatus(ctx context.Context) (active int, left int, err error) {
	return f.countActive, f.countLeft, nil
}
func (f *fakeRepo) CountPendingPurge(ctx context.Context, now time.Time) (int, error) {
	return f.pendingPurge, nil
}
func (f *fakeRepo) ListActiveUserIDs(ctx context.Context) ([]int64, error) {
	return f.activeUserIDs, nil
}
func (f *fakeRepo) ListRefreshCandidateUserIDs(ctx context.Context) ([]int64, error) {
	return f.refreshCandidateIDs, nil
}
func (f *fakeRepo) UpdateMemberTag(ctx context.Context, userID int64, tag *string, updatedAt time.Time) error {
	f.updateTagCalls = append(f.updateTagCalls, userID)
	if f.updateTagBlocked {
		<-ctx.Done()
		return ctx.Err()
	}
	if err, ok := f.updateTagErrorByUser[userID]; ok {
		return err
	}
	return nil
}

func TestServiceUpsertActiveMember(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo)
	now := time.Now().UTC().Truncate(time.Second)

	if err := svc.UpsertActiveMember(context.Background(), 42, "john", "John", false, now); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !repo.upsertCalled {
		t.Fatal("expected UpsertActiveMember to be called")
	}
	if repo.upsertUserID != 42 || repo.upsertUsername != "john" || repo.upsertName != "John" || !repo.upsertJoinedAt.Equal(now) {
		t.Fatalf("unexpected upsert args: %+v", repo)
	}
}

func TestServiceMarkMemberLeftNow(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo)
	start := time.Now().UTC()

	if err := svc.MarkMemberLeftNow(context.Background(), 77); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !repo.markLeftCalled {
		t.Fatal("expected MarkMemberLeft to be called")
	}
	if repo.markLeftUserID != 77 {
		t.Fatalf("unexpected user id: %d", repo.markLeftUserID)
	}
	if repo.markLeftAt.Before(start.Add(-time.Second)) {
		t.Fatalf("unexpected leftAt: %v", repo.markLeftAt)
	}
	if got := repo.markLeftDeleteAfter.Sub(repo.markLeftAt); got != leftGracePeriod {
		t.Fatalf("expected grace period %s, got %s", leftGracePeriod, got)
	}
}

func TestServiceIsActiveMember(t *testing.T) {
	repo := &fakeRepo{isActiveResult: true}
	svc := NewService(repo)

	active, err := svc.IsActiveMember(context.Background(), 55)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !active {
		t.Fatal("expected active=true")
	}
	if !repo.isActiveCalled {
		t.Fatal("expected IsActiveMember to be called")
	}
}

func TestServicePurgeExpiredLeftMembers(t *testing.T) {
	repo := &fakeRepo{purgeDeleted: 3}
	svc := NewService(repo)

	deleted, err := svc.PurgeExpiredLeftMembers(context.Background(), time.Now().UTC(), 500)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deleted != 3 {
		t.Fatalf("deleted = %d, want 3", deleted)
	}
	if !repo.purgeCalled {
		t.Fatal("expected PurgeExpiredLeftMembers to be called")
	}
}

func TestServiceEnsureMemberSeen(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo)
	now := time.Now().UTC().Truncate(time.Second)

	if err := svc.EnsureMemberSeen(context.Background(), 12, "john", "John", false, now); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !repo.ensureSeenCalled || repo.ensureSeenUserID != 12 || repo.ensureSeenUsername != "john" || repo.ensureSeenName != "John" || !repo.ensureSeenAt.Equal(now) {
		t.Fatalf("unexpected ensure seen args: %+v", repo)
	}
}

func TestServiceEnsureActiveMemberSeen(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo)
	now := time.Now().UTC().Truncate(time.Second)

	if err := svc.EnsureActiveMemberSeen(context.Background(), 77, "neo", "Neo", false, now); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !repo.ensureActiveCalled || repo.ensureSeenUserID != 77 || !repo.ensureSeenAt.Equal(now) {
		t.Fatalf("unexpected ensure active args: %+v", repo)
	}
}

func TestServiceCountMembersByStatus(t *testing.T) {
	repo := &fakeRepo{countActive: 10, countLeft: 3}
	svc := NewService(repo)
	active, left, err := svc.CountMembersByStatus(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if active != 10 || left != 3 {
		t.Fatalf("unexpected counts: active=%d left=%d", active, left)
	}
}

func TestServiceCountPendingPurge(t *testing.T) {
	repo := &fakeRepo{pendingPurge: 7}
	svc := NewService(repo)
	pending, err := svc.CountPendingPurge(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pending != 7 {
		t.Fatalf("pending=%d, want 7", pending)
	}
}

func TestServiceTouchLastSeen(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo)
	now := time.Now().UTC().Truncate(time.Second)

	if err := svc.TouchLastSeen(context.Background(), 88, now); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !repo.touchCalled || repo.touchUserID != 88 || !repo.touchSeenAt.Equal(now) {
		t.Fatalf("unexpected touch args: %+v", repo)
	}
}

func TestScanAndUpdateMemberTags_ContextTimeoutReturnsError(t *testing.T) {
	repo := &fakeRepo{activeUserIDs: []int64{1}, refreshCandidateIDs: []int64{1}, updateTagBlocked: true}
	svc := NewService(repo)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	updated, err := svc.ScanAndUpdateMemberTags(ctx, telegram.NewOps(&fakeTelegramClient{}), 1, time.Now().UTC())
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}
	if updated != 0 {
		t.Fatalf("expected updated=0, got %d", updated)
	}
}

func TestScanAndUpdateMemberTags_CanceledContextReturnsError(t *testing.T) {
	repo := &fakeRepo{activeUserIDs: []int64{1}, refreshCandidateIDs: []int64{1}, updateTagBlocked: true}
	svc := NewService(repo)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	updated, err := svc.ScanAndUpdateMemberTags(ctx, telegram.NewOps(&fakeTelegramClient{}), 1, time.Now().UTC())
	if err == nil {
		t.Fatal("expected canceled error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected Canceled, got %v", err)
	}
	if updated != 0 {
		t.Fatalf("expected updated=0, got %d", updated)
	}
}

func TestScanAndUpdateMemberTags_NonContextErrorsDoNotAbortScan(t *testing.T) {
	repo := &fakeRepo{
		activeUserIDs:        []int64{1, 2, 3},
		refreshCandidateIDs:  []int64{1, 2, 3},
		updateTagErrorByUser: map[int64]error{2: errors.New("db write failed")},
	}
	tg := &fakeTelegramClient{getChatMemberErrByUser: map[int64]error{1: errors.New("temporary tg error")}}
	svc := NewService(repo)

	updated, err := svc.ScanAndUpdateMemberTags(context.Background(), telegram.NewOps(tg), 1, time.Now().UTC())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated != 1 {
		t.Fatalf("expected updated=1, got %d", updated)
	}
	if len(tg.getChatMemberCalls) != 3 {
		t.Fatalf("expected scan to continue for all users, calls=%v", tg.getChatMemberCalls)
	}
	if len(repo.updateTagCalls) != 2 {
		t.Fatalf("expected update attempts for users with fetched telegram members, calls=%v", repo.updateTagCalls)
	}
}

func TestScanAndUpdateMemberTags_RestoresMissingKnownMemberWhenTelegramConfirmsMembership(t *testing.T) {
	repo := &fakeRepo{activeUserIDs: []int64{1}, refreshCandidateIDs: []int64{1, 2}}
	svc := NewService(repo)

	updated, err := svc.ScanAndUpdateMemberTags(context.Background(), telegram.NewOps(&fakeTelegramClient{}), 1, time.Now().UTC())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated != 2 {
		t.Fatalf("expected updated=2, got %d", updated)
	}
	if len(repo.upsertCalls) != 2 || repo.upsertCalls[0] != 1 || repo.upsertCalls[1] != 2 {
		t.Fatalf("expected identity upsert for all bounded member-like candidates, got %v", repo.upsertCalls)
	}
}

func TestScanAndUpdateMemberTags_DoesNotRestoreKnownNonMemberStatus(t *testing.T) {
	repo := &fakeRepo{activeUserIDs: []int64{}, refreshCandidateIDs: []int64{2}}
	tg := &fakeTelegramClient{statusByUser: map[int64]string{2: "left"}}
	svc := NewService(repo)

	updated, err := svc.ScanAndUpdateMemberTags(context.Background(), telegram.NewOps(tg), 1, time.Now().UTC())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated != 0 {
		t.Fatalf("expected updated=0, got %d", updated)
	}
	if len(repo.upsertCalls) != 0 {
		t.Fatalf("expected no upserts, got %v", repo.upsertCalls)
	}
}

func TestScanAndUpdateMemberTags_DoesNotUseUnknownIDsOutsideKnownSources(t *testing.T) {
	repo := &fakeRepo{activeUserIDs: []int64{1}, refreshCandidateIDs: []int64{1, 2}}
	tg := &fakeTelegramClient{}
	svc := NewService(repo)

	_, err := svc.ScanAndUpdateMemberTags(context.Background(), telegram.NewOps(tg), 1, time.Now().UTC())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tg.getChatMemberCalls) != 2 {
		t.Fatalf("expected checks only for known candidates, got %v", tg.getChatMemberCalls)
	}
}

func TestScanAndUpdateMemberTags_RestrictedIsMemberLikeAndRestoredFromBoundedCandidates(t *testing.T) {
	repo := &fakeRepo{activeUserIDs: nil, refreshCandidateIDs: []int64{42}}
	tg := &fakeTelegramClient{statusByUser: map[int64]string{42: "restricted"}}
	svc := NewService(repo)

	updated, err := svc.ScanAndUpdateMemberTags(context.Background(), telegram.NewOps(tg), 1, time.Now().UTC())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated != 1 {
		t.Fatalf("expected updated=1, got %d", updated)
	}
	if len(repo.upsertCalls) != 1 || repo.upsertCalls[0] != 42 {
		t.Fatalf("expected restore upsert for restricted member, got %v", repo.upsertCalls)
	}
}

func TestScanAndUpdateMemberTags_DoesNotRestoreUnknownDMOnlyUserOutsideBoundedCandidates(t *testing.T) {
	repo := &fakeRepo{activeUserIDs: []int64{10}, refreshCandidateIDs: []int64{10}}
	tg := &fakeTelegramClient{}
	svc := NewService(repo)

	_, err := svc.ScanAndUpdateMemberTags(context.Background(), telegram.NewOps(tg), 1, time.Now().UTC())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tg.getChatMemberCalls) != 1 || tg.getChatMemberCalls[0] != 10 {
		t.Fatalf("expected only bounded candidate checks, got %v", tg.getChatMemberCalls)
	}
	if len(repo.upsertCalls) != 1 || repo.upsertCalls[0] != 10 {
		t.Fatalf("expected bounded identity upsert only for known candidate 10, got %v", repo.upsertCalls)
	}
}

func TestScanAndUpdateMemberTags_RepairsIsBotForAlreadyActivePersistedMember(t *testing.T) {
	repo := &fakeRepo{activeUserIDs: []int64{77}, refreshCandidateIDs: []int64{77}}
	tg := &fakeTelegramClient{memberUserByID: map[int64]models.User{77: {ID: 77, Username: "helperbot", FirstName: "Helper", IsBot: true}}}
	svc := NewService(repo)

	updated, err := svc.ScanAndUpdateMemberTags(context.Background(), telegram.NewOps(tg), 1, time.Now().UTC())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated != 1 {
		t.Fatalf("expected updated=1, got %d", updated)
	}
	if len(repo.upsertCalls) != 1 || repo.upsertCalls[0] != 77 {
		t.Fatalf("expected identity upsert for active candidate, got %v", repo.upsertCalls)
	}
	if repo.upsertUserID != 77 || repo.upsertUsername != "helperbot" {
		t.Fatalf("expected upsert with telegram identity, got userID=%d username=%q", repo.upsertUserID, repo.upsertUsername)
	}
}

type fakeTelegramClient struct {
	getChatMemberBlocked   bool
	getChatMemberErrByUser map[int64]error
	getChatMemberCalls     []int64
	statusByUser           map[int64]string
	memberUserByID         map[int64]models.User
}

func (f *fakeTelegramClient) SendMessage(chatID int64, text string, markup *models.InlineKeyboardMarkup) (messageID int, err error) {
	return 0, nil
}
func (f *fakeTelegramClient) EditMessage(chatID int64, messageID int, text string, markup *models.InlineKeyboardMarkup) error {
	return nil
}
func (f *fakeTelegramClient) EditReplyMarkup(chatID int64, messageID int, markup *models.InlineKeyboardMarkup) error {
	return nil
}
func (f *fakeTelegramClient) DeleteMessage(chatID int64, messageID int) error { return nil }
func (f *fakeTelegramClient) GetChatMember(chatID int64, userID int64) (models.ChatMember, error) {
	f.getChatMemberCalls = append(f.getChatMemberCalls, userID)
	if f.getChatMemberBlocked {
		return nil, context.DeadlineExceeded
	}
	if err, ok := f.getChatMemberErrByUser[userID]; ok {
		return nil, err
	}
	if status, ok := f.statusByUser[userID]; ok {
		switch status {
		case "left":
			return &models.ChatMemberLeft{User: models.User{ID: userID}}, nil
		case "restricted":
			return &models.ChatMemberRestricted{User: models.User{ID: userID}}, nil
		}
	}
	if u, ok := f.memberUserByID[userID]; ok {
		return &models.ChatMemberMember{User: u}, nil
	}
	return &models.ChatMemberMember{User: models.User{ID: userID}}, nil
}
