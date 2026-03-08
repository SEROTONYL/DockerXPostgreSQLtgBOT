package members

import (
	"context"
	"testing"
	"time"
)

type lifecycleFakeRepo struct {
	member      Member
	balance     int64
	roleValue   string
	purgeCalled bool
}

func newLifecycleFakeRepo(userID int64) *lifecycleFakeRepo {
	return &lifecycleFakeRepo{
		member: Member{
			UserID: userID,
			Status: StatusActive,
		},
	}
}

func (f *lifecycleFakeRepo) UpsertActiveMember(ctx context.Context, userID int64, username, name string, isBot bool, joinedAt time.Time) error {
	f.member.UserID = userID
	f.member.Username = username
	f.member.FirstName = name
	f.member.Status = StatusActive
	f.member.LeftAt = nil
	f.member.DeleteAfter = nil
	f.member.LastSeenAt = &joinedAt
	return nil
}

func (f *lifecycleFakeRepo) MarkMemberLeft(ctx context.Context, userID int64, leftAt, deleteAfter time.Time) error {
	f.member.UserID = userID
	f.member.Status = StatusLeft
	f.member.LeftAt = &leftAt
	f.member.DeleteAfter = &deleteAfter
	return nil
}

func (f *lifecycleFakeRepo) IsActiveMember(ctx context.Context, userID int64) (bool, error) {
	return f.member.UserID == userID && f.member.Status == StatusActive, nil
}

func (f *lifecycleFakeRepo) PurgeExpiredLeftMembers(ctx context.Context, now time.Time, limit int) (int, error) {
	f.purgeCalled = true
	if f.member.Status == StatusLeft && f.member.DeleteAfter != nil && !f.member.DeleteAfter.After(now) {
		f.member = Member{}
		f.balance = 0
		f.roleValue = ""
		return 1, nil
	}
	return 0, nil
}

func (f *lifecycleFakeRepo) GetByUserID(ctx context.Context, userID int64) (*Member, error) {
	if f.member.UserID != userID {
		return nil, nil
	}
	m := f.member
	return &m, nil
}

func (f *lifecycleFakeRepo) GetByUsername(ctx context.Context, username string) (*Member, error) {
	if f.member.Username != username {
		return nil, nil
	}
	m := f.member
	return &m, nil
}

func (f *lifecycleFakeRepo) ListActiveUserIDs(ctx context.Context) ([]int64, error) {
	return nil, nil
}

func (f *lifecycleFakeRepo) ListRefreshCandidateUserIDs(ctx context.Context) ([]int64, error) {
	return nil, nil
}

func (f *lifecycleFakeRepo) UpdateMemberTag(ctx context.Context, userID int64, tag *string, updatedAt time.Time) error {
	return nil
}

func TestLifecycleTransitions_RestrictedToMember_BecomesActive(t *testing.T) {
	repo := newLifecycleFakeRepo(101)
	svc := NewService(repo)
	now := time.Now().UTC()

	if err := svc.MarkMemberLeft(context.Background(), 101, now, now.Add(leftGracePeriod)); err != nil {
		t.Fatalf("mark left: %v", err)
	}
	if repo.member.Status != StatusLeft || repo.member.DeleteAfter == nil {
		t.Fatalf("expected left status with delete_after, got: %+v", repo.member)
	}

	if err := svc.UpsertActiveMember(context.Background(), 101, "john", "John", false, now.Add(5*time.Minute)); err != nil {
		t.Fatalf("upsert active: %v", err)
	}

	if repo.member.Status != StatusActive {
		t.Fatalf("expected active status, got: %s", repo.member.Status)
	}
	if repo.member.DeleteAfter != nil {
		t.Fatalf("expected delete_after to be cleared, got: %v", repo.member.DeleteAfter)
	}
}

func TestLifecycleTransitions_MemberToRestricted_BecomesLeft(t *testing.T) {
	repo := newLifecycleFakeRepo(202)
	svc := NewService(repo)

	if err := svc.MarkMemberLeftNow(context.Background(), 202); err != nil {
		t.Fatalf("mark left now: %v", err)
	}

	if repo.member.Status != StatusLeft {
		t.Fatalf("expected left status, got: %s", repo.member.Status)
	}
	if repo.member.DeleteAfter == nil {
		t.Fatal("expected delete_after to be set")
	}
}

func TestLifecycleTransitions_LeftToMember_ClearsDeleteAfter(t *testing.T) {
	repo := newLifecycleFakeRepo(303)
	svc := NewService(repo)
	now := time.Now().UTC()

	if err := svc.MarkMemberLeft(context.Background(), 303, now, now.Add(leftGracePeriod)); err != nil {
		t.Fatalf("mark left: %v", err)
	}
	if repo.member.DeleteAfter == nil {
		t.Fatal("expected delete_after to be set")
	}

	if err := svc.UpsertActiveMember(context.Background(), 303, "jane", "Jane", false, now.Add(time.Hour)); err != nil {
		t.Fatalf("upsert active: %v", err)
	}

	if repo.member.Status != StatusActive {
		t.Fatalf("expected active status, got: %s", repo.member.Status)
	}
	if repo.member.DeleteAfter != nil {
		t.Fatalf("expected delete_after to be nil, got: %v", repo.member.DeleteAfter)
	}
}

func TestRejoinWithinGrace_RestoresDataWithoutPurge(t *testing.T) {
	repo := newLifecycleFakeRepo(404)
	repo.balance = 150
	repo.roleValue = "captain"
	svc := NewService(repo)
	now := time.Now().UTC()

	if err := svc.MarkMemberLeft(context.Background(), 404, now, now.Add(leftGracePeriod)); err != nil {
		t.Fatalf("mark left: %v", err)
	}

	if err := svc.UpsertActiveMember(context.Background(), 404, "neo", "Neo", false, now.Add(24*time.Hour)); err != nil {
		t.Fatalf("upsert active: %v", err)
	}

	member, err := svc.GetByUserID(context.Background(), 404)
	if err != nil {
		t.Fatalf("get by user id: %v", err)
	}
	if member == nil || member.Status != StatusActive {
		t.Fatalf("expected active member, got: %+v", member)
	}
	if member.DeleteAfter != nil {
		t.Fatalf("expected delete_after to be nil, got: %v", member.DeleteAfter)
	}
	if repo.balance != 150 {
		t.Fatalf("expected balance to be preserved, got: %d", repo.balance)
	}
	if repo.roleValue != "captain" {
		t.Fatalf("expected role to be preserved, got: %q", repo.roleValue)
	}
	if repo.purgeCalled {
		t.Fatal("purge should not run during rejoin within grace")
	}
}

func (f *lifecycleFakeRepo) EnsureMemberSeen(ctx context.Context, userID int64, username, name string, isBot bool, seenAt time.Time) error {
	if f.member.UserID == userID {
		f.member.Username = username
		f.member.FirstName = name
		f.member.LastSeenAt = &seenAt
	}
	return nil
}

func (f *lifecycleFakeRepo) EnsureActiveMemberSeen(ctx context.Context, userID int64, username, name string, isBot bool, seenAt time.Time) error {
	if f.member.UserID == 0 {
		f.member.UserID = userID
	}
	f.member.Status = StatusActive
	f.member.Username = username
	f.member.FirstName = name
	f.member.DeleteAfter = nil
	f.member.LeftAt = nil
	f.member.LastSeenAt = &seenAt
	return nil
}

func (f *lifecycleFakeRepo) TouchLastSeen(ctx context.Context, userID int64, seenAt time.Time) error {
	if f.member.UserID == userID {
		f.member.LastSeenAt = &seenAt
	}
	return nil
}

func (f *lifecycleFakeRepo) CountMembersByStatus(ctx context.Context) (active int, left int, err error) {
	if f.member.Status == StatusActive {
		return 1, 0, nil
	}
	if f.member.Status == StatusLeft {
		return 0, 1, nil
	}
	return 0, 0, nil
}

func (f *lifecycleFakeRepo) CountPendingPurge(ctx context.Context, now time.Time) (int, error) {
	if f.member.Status == StatusLeft && f.member.DeleteAfter != nil && !f.member.DeleteAfter.After(now) {
		return 1, nil
	}
	return 0, nil
}

type seenStateRepo struct {
	members map[int64]*Member
}

func newSeenStateRepo() *seenStateRepo {
	return &seenStateRepo{members: map[int64]*Member{}}
}

func (r *seenStateRepo) UpsertActiveMember(ctx context.Context, userID int64, username, name string, isBot bool, joinedAt time.Time) error {
	m := r.members[userID]
	if m == nil {
		m = &Member{UserID: userID}
		r.members[userID] = m
	}
	m.Status = StatusActive
	m.Username = username
	m.FirstName = name
	m.DeleteAfter = nil
	m.LeftAt = nil
	return nil
}
func (r *seenStateRepo) MarkMemberLeft(ctx context.Context, userID int64, leftAt, deleteAfter time.Time) error {
	return nil
}
func (r *seenStateRepo) IsActiveMember(ctx context.Context, userID int64) (bool, error) {
	m := r.members[userID]
	return m != nil && m.Status == StatusActive, nil
}
func (r *seenStateRepo) PurgeExpiredLeftMembers(ctx context.Context, now time.Time, limit int) (int, error) {
	return 0, nil
}
func (r *seenStateRepo) GetByUserID(ctx context.Context, userID int64) (*Member, error) {
	m := r.members[userID]
	if m == nil {
		return nil, nil
	}
	cp := *m
	return &cp, nil
}
func (r *seenStateRepo) GetByUsername(ctx context.Context, username string) (*Member, error) {
	return nil, nil
}
func (r *seenStateRepo) EnsureMemberSeen(ctx context.Context, userID int64, username, name string, isBot bool, seenAt time.Time) error {
	m := r.members[userID]
	if m == nil {
		return nil
	}
	m.Username = username
	m.FirstName = name
	if m.LastSeenAt == nil || m.LastSeenAt.Before(seenAt.Add(-5*time.Minute)) {
		t := seenAt
		m.LastSeenAt = &t
	}
	return nil
}
func (r *seenStateRepo) EnsureActiveMemberSeen(ctx context.Context, userID int64, username, name string, isBot bool, seenAt time.Time) error {
	m := r.members[userID]
	if m == nil {
		m = &Member{UserID: userID}
		r.members[userID] = m
	}
	m.Status = StatusActive
	m.Username = username
	m.FirstName = name
	if m.LastSeenAt == nil || m.LastSeenAt.Before(seenAt.Add(-5*time.Minute)) {
		t := seenAt
		m.LastSeenAt = &t
	}
	return nil
}
func (r *seenStateRepo) TouchLastSeen(ctx context.Context, userID int64, seenAt time.Time) error {
	m := r.members[userID]
	if m == nil {
		return nil
	}
	if m.LastSeenAt == nil || m.LastSeenAt.Before(seenAt.Add(-5*time.Minute)) {
		t := seenAt
		m.LastSeenAt = &t
	}
	return nil
}

func (r *seenStateRepo) CountMembersByStatus(ctx context.Context) (active int, left int, err error) {
	return 0, 0, nil
}
func (r *seenStateRepo) CountPendingPurge(ctx context.Context, now time.Time) (int, error) {
	return 0, nil
}
func (r *seenStateRepo) ListActiveUserIDs(ctx context.Context) ([]int64, error) { return nil, nil }
func (r *seenStateRepo) ListRefreshCandidateUserIDs(ctx context.Context) ([]int64, error) {
	return nil, nil
}
func (r *seenStateRepo) UpdateMemberTag(ctx context.Context, userID int64, tag *string, updatedAt time.Time) error {
	return nil
}

func TestEnsureMemberSeen_ThrottleBehavior(t *testing.T) {
	repo := newSeenStateRepo()
	repo.members[500] = &Member{UserID: 500, Status: StatusActive}
	svc := NewService(repo)
	base := time.Now().UTC().Truncate(time.Second)

	if err := svc.EnsureMemberSeen(context.Background(), 500, "u", "User", false, base); err != nil {
		t.Fatalf("first seen err: %v", err)
	}
	first := repo.members[500].LastSeenAt
	if first == nil || !first.Equal(base) {
		t.Fatalf("first seen mismatch: %v", first)
	}

	if err := svc.EnsureMemberSeen(context.Background(), 500, "u", "User", false, base.Add(time.Minute)); err != nil {
		t.Fatalf("second seen err: %v", err)
	}
	second := repo.members[500].LastSeenAt
	if second == nil || !second.Equal(base) {
		t.Fatalf("second seen should stay base, got: %v", second)
	}

	if err := svc.EnsureMemberSeen(context.Background(), 500, "u", "User", false, base.Add(6*time.Minute)); err != nil {
		t.Fatalf("third seen err: %v", err)
	}
	third := repo.members[500].LastSeenAt
	if third == nil || !third.Equal(base.Add(6*time.Minute)) {
		t.Fatalf("third seen should update to +6m, got: %v", third)
	}
}

func TestEnsureMemberSeen_PrivateNoCreateWhenMissing(t *testing.T) {
	repo := newSeenStateRepo()
	svc := NewService(repo)

	if err := svc.EnsureMemberSeen(context.Background(), 999, "ghost", "Ghost", false, time.Now().UTC()); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if repo.members[999] != nil {
		t.Fatal("expected no member to be created in strict private mode")
	}
}

func (f *lifecycleFakeRepo) GetUsersWithRole(ctx context.Context) ([]*Member, error) { return nil, nil }
func (f *lifecycleFakeRepo) GetUsersWithoutRole(ctx context.Context) ([]*Member, error) {
	return nil, nil
}
func (r *seenStateRepo) GetUsersWithRole(ctx context.Context) ([]*Member, error)    { return nil, nil }
func (r *seenStateRepo) GetUsersWithoutRole(ctx context.Context) ([]*Member, error) { return nil, nil }
