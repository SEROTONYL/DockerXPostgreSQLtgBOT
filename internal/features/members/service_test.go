package members

import (
	"context"
	"testing"
	"time"
)

type fakeRepo struct {
	upsertCalled        bool
	markLeftCalled      bool
	isActiveCalled      bool
	upsertUserID        int64
	upsertUsername      string
	upsertName          string
	upsertJoinedAt      time.Time
	markLeftUserID      int64
	markLeftAt          time.Time
	markLeftDeleteAfter time.Time
	isActiveResult      bool
	purgeCalled         bool
	purgeDeleted        int
	ensureSeenCalled    bool
	ensureSeenUserID    int64
	ensureSeenUsername  string
	ensureSeenName      string
	ensureSeenAt        time.Time
	ensureActiveCalled  bool
	touchCalled         bool
	touchUserID         int64
	touchSeenAt         time.Time
	countActive         int
	countLeft           int
	pendingPurge        int
}

func (f *fakeRepo) UpsertActiveMember(ctx context.Context, userID int64, username, name string, joinedAt time.Time) error {
	f.upsertCalled = true
	f.upsertUserID = userID
	f.upsertUsername = username
	f.upsertName = name
	f.upsertJoinedAt = joinedAt
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
func (f *fakeRepo) EnsureMemberSeen(ctx context.Context, userID int64, username, name string, seenAt time.Time) error {
	f.ensureSeenCalled = true
	f.ensureSeenUserID = userID
	f.ensureSeenUsername = username
	f.ensureSeenName = name
	f.ensureSeenAt = seenAt
	return nil
}
func (f *fakeRepo) EnsureActiveMemberSeen(ctx context.Context, userID int64, username, name string, seenAt time.Time) error {
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
func (f *fakeRepo) ListActiveUserIDs(ctx context.Context) ([]int64, error) { return nil, nil }
func (f *fakeRepo) UpdateMemberTag(ctx context.Context, userID int64, tag *string, updatedAt time.Time) error {
	return nil
}

func TestServiceUpsertActiveMember(t *testing.T) {
	repo := &fakeRepo{}
	svc := NewService(repo)
	now := time.Now().UTC().Truncate(time.Second)

	if err := svc.UpsertActiveMember(context.Background(), 42, "john", "John", now); err != nil {
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

	if err := svc.EnsureMemberSeen(context.Background(), 12, "john", "John", now); err != nil {
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

	if err := svc.EnsureActiveMemberSeen(context.Background(), 77, "neo", "Neo", now); err != nil {
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
