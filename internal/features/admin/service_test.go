package admin

import (
	"context"
	"testing"
	"time"

	"serotonyl.ru/telegram-bot/internal/config"
	"serotonyl.ru/telegram-bot/internal/features/members"
)

type fakeAdminRepo struct{}

func (f *fakeAdminRepo) CreateSession(ctx context.Context, session *AdminSession) error { return nil }
func (f *fakeAdminRepo) GetActiveSession(ctx context.Context, userID int64) (*AdminSession, error) {
	return nil, nil
}
func (f *fakeAdminRepo) DeactivateSession(ctx context.Context, userID int64) error        { return nil }
func (f *fakeAdminRepo) UpdateActivity(ctx context.Context, userID int64) error           { return nil }
func (f *fakeAdminRepo) LogAttempt(ctx context.Context, userID int64, success bool) error { return nil }
func (f *fakeAdminRepo) GetRecentAttempts(ctx context.Context, userID int64, period time.Duration) (int, error) {
	return 0, nil
}

type fakeMemberRepo struct {
	member           *members.Member
	getErr           error
	updateAdminCalls int
	updatedUserID    int64
	updatedIsAdmin   bool
}

func (f *fakeMemberRepo) GetByUserID(ctx context.Context, userID int64) (*members.Member, error) {
	return f.member, f.getErr
}
func (f *fakeMemberRepo) GetUsersWithoutRole(ctx context.Context) ([]*members.Member, error) {
	return nil, nil
}
func (f *fakeMemberRepo) GetUsersWithRole(ctx context.Context) ([]*members.Member, error) {
	return nil, nil
}
func (f *fakeMemberRepo) UpdateRole(ctx context.Context, userID int64, role string) error { return nil }
func (f *fakeMemberRepo) UpdateAdminFlag(ctx context.Context, userID int64, isAdmin bool) error {
	f.updateAdminCalls++
	f.updatedUserID = userID
	f.updatedIsAdmin = isAdmin
	return nil
}

func TestCanEnterAdmin_DenyWhenNotAdminAndNotInConfig(t *testing.T) {
	svc := NewService(&fakeAdminRepo{}, &fakeMemberRepo{member: &members.Member{IsAdmin: false}}, &config.Config{AdminIDs: []int64{111}})

	allowed := svc.CanEnterAdmin(context.Background(), 999)
	if allowed {
		t.Fatalf("expected deny")
	}
}

func TestCanEnterAdmin_AllowFromAdminIDsAndBootstrapFlag(t *testing.T) {
	mr := &fakeMemberRepo{}
	svc := NewService(&fakeAdminRepo{}, mr, &config.Config{AdminIDs: []int64{42}})

	allowed := svc.CanEnterAdmin(context.Background(), 42)
	if !allowed {
		t.Fatalf("expected allow")
	}
	if mr.updateAdminCalls != 1 {
		t.Fatalf("expected UpdateAdminFlag call, got %d", mr.updateAdminCalls)
	}
	if mr.updatedUserID != 42 || !mr.updatedIsAdmin {
		t.Fatalf("unexpected update args: user=%d is_admin=%v", mr.updatedUserID, mr.updatedIsAdmin)
	}
}

func TestCanEnterAdmin_AllowWhenMemberIsAdmin(t *testing.T) {
	svc := NewService(&fakeAdminRepo{}, &fakeMemberRepo{member: &members.Member{IsAdmin: true}}, &config.Config{})

	allowed := svc.CanEnterAdmin(context.Background(), 555)
	if !allowed {
		t.Fatalf("expected allow")
	}
}
