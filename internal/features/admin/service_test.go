package admin

import (
	"context"
	"strings"
	"sync"
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

type fakeAdminRepoAttempts struct{ attempts int }

func (f *fakeAdminRepoAttempts) CreateSession(ctx context.Context, session *AdminSession) error {
	return nil
}
func (f *fakeAdminRepoAttempts) GetActiveSession(ctx context.Context, userID int64) (*AdminSession, error) {
	return nil, nil
}
func (f *fakeAdminRepoAttempts) DeactivateSession(ctx context.Context, userID int64) error {
	return nil
}
func (f *fakeAdminRepoAttempts) UpdateActivity(ctx context.Context, userID int64) error { return nil }
func (f *fakeAdminRepoAttempts) LogAttempt(ctx context.Context, userID int64, success bool) error {
	return nil
}
func (f *fakeAdminRepoAttempts) GetRecentAttempts(ctx context.Context, userID int64, period time.Duration) (int, error) {
	return f.attempts, nil
}

func TestVerifyPassword_NoMojibakeInErrors(t *testing.T) {
	svc := NewService(&fakeAdminRepoAttempts{attempts: 3}, &fakeMemberRepo{}, &config.Config{})
	err := svc.VerifyPassword(context.Background(), 1, "any")
	if err == nil {
		t.Fatalf("expected error")
	}
	for _, bad := range []string{"Не", "ол", "ер", "ад"} {
		if strings.Contains(err.Error(), bad) {
			t.Fatalf("error contains mojibake marker %q: %q", bad, err.Error())
		}
	}
	if err.Error() != "слишком много попыток, подождите 1 час" {
		t.Fatalf("unexpected text: %q", err.Error())
	}
}

func TestPanelMessageID_ConcurrentSetGetRaceSafe(t *testing.T) {
	svc := NewService(&fakeAdminRepo{}, &fakeMemberRepo{}, &config.Config{})

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				uid := int64((worker % 5) + 1)
				svc.SetPanelMessageID(uid, worker*1000+j+1)
				_ = svc.GetPanelMessageID(uid)
			}
		}(i)
	}
	wg.Wait()
}
