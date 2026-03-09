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

type fakeAdminRepo struct {
	mu     sync.Mutex
	states map[int64]*AdminState
	panels map[int64]AdminPanelMessage
}

func newFakeAdminRepo() *fakeAdminRepo {
	return &fakeAdminRepo{
		states: make(map[int64]*AdminState),
		panels: make(map[int64]AdminPanelMessage),
	}
}

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
func (f *fakeAdminRepo) CleanupStaleAuthState(ctx context.Context, now time.Time) (CleanupResult, error) {
	return CleanupResult{}, nil
}
func (f *fakeAdminRepo) ListBalanceDeltas(ctx context.Context, chatID int64) ([]*BalanceDelta, error) {
	return nil, nil
}
func (f *fakeAdminRepo) CreateBalanceDelta(ctx context.Context, chatID int64, name string, amount int64, createdBy int64) error {
	return nil
}
func (f *fakeAdminRepo) DeleteBalanceDelta(ctx context.Context, chatID int64, deltaID int64) error {
	return nil
}
func (f *fakeAdminRepo) SaveFlowState(ctx context.Context, userID int64, state *AdminState) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.states[userID] = state
	return nil
}
func (f *fakeAdminRepo) GetFlowState(ctx context.Context, userID int64) (*AdminState, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.states[userID], nil
}
func (f *fakeAdminRepo) ClearFlowState(ctx context.Context, userID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.states, userID)
	delete(f.panels, userID)
	return nil
}
func (f *fakeAdminRepo) SetPanelMessage(ctx context.Context, userID int64, panel AdminPanelMessage) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.panels[userID] = panel
	return nil
}
func (f *fakeAdminRepo) GetPanelMessage(ctx context.Context, userID int64) (AdminPanelMessage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.panels[userID], nil
}
func (f *fakeAdminRepo) ClearPanelMessage(ctx context.Context, userID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.panels, userID)
	return nil
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
	svc := NewService(newFakeAdminRepo(), &fakeMemberRepo{member: &members.Member{IsAdmin: false}}, &config.Config{AdminIDs: []int64{111}})

	allowed := svc.CanEnterAdmin(context.Background(), 999)
	if allowed {
		t.Fatalf("expected deny")
	}
}

func TestCanEnterAdmin_AllowFromAdminIDsAndBootstrapFlag(t *testing.T) {
	mr := &fakeMemberRepo{}
	svc := NewService(newFakeAdminRepo(), mr, &config.Config{AdminIDs: []int64{42}})

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
	svc := NewService(newFakeAdminRepo(), &fakeMemberRepo{member: &members.Member{IsAdmin: true}}, &config.Config{})

	allowed := svc.CanEnterAdmin(context.Background(), 555)
	if !allowed {
		t.Fatalf("expected allow")
	}
}

type fakeAdminRepoAttempts struct {
	*fakeAdminRepo
	attempts int
}

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
func (f *fakeAdminRepoAttempts) CleanupStaleAuthState(ctx context.Context, now time.Time) (CleanupResult, error) {
	return CleanupResult{}, nil
}
func (f *fakeAdminRepoAttempts) ListBalanceDeltas(ctx context.Context, chatID int64) ([]*BalanceDelta, error) {
	return nil, nil
}
func (f *fakeAdminRepoAttempts) CreateBalanceDelta(ctx context.Context, chatID int64, name string, amount int64, createdBy int64) error {
	return nil
}
func (f *fakeAdminRepoAttempts) DeleteBalanceDelta(ctx context.Context, chatID int64, deltaID int64) error {
	return nil
}

func TestVerifyPassword_NoMojibakeInErrors(t *testing.T) {
	svc := NewService(&fakeAdminRepoAttempts{fakeAdminRepo: newFakeAdminRepo(), attempts: 3}, &fakeMemberRepo{}, &config.Config{})
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
	svc := NewService(newFakeAdminRepo(), &fakeMemberRepo{}, &config.Config{})

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

func TestStateAndPanelPersistAcrossServiceInstances(t *testing.T) {
	repo := newFakeAdminRepo()
	svcA := NewService(repo, &fakeMemberRepo{}, &config.Config{})
	svcA.SetState(77, StateAwaitingPassword, nil)
	svcA.SetPanelMessage(77, 77, 42)

	svcB := NewService(repo, &fakeMemberRepo{}, &config.Config{})
	state := svcB.GetState(77)
	if state == nil || state.State != StateAwaitingPassword {
		t.Fatalf("expected persisted state, got %+v", state)
	}
	panel := svcB.GetPanelMessage(77)
	if panel.ChatID != 77 || panel.MessageID != 42 {
		t.Fatalf("expected persisted panel message, got %+v", panel)
	}
}
