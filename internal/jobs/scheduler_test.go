package jobs

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"serotonyl.ru/telegram-bot/internal/telegram"
)

func TestCronLogMessages_DoNotContainMojibakeMarkers(t *testing.T) {
	messages := []string{
		cronWarnLoadLocation,
		cronInfoDailyReset,
		cronErrorDailyReset,
		cronDebugReminders,
		cronErrorReminders,
		cronInfoStarted,
		cronInfoStopped,
	}

	markers := []string{"Рќ", "РџСЂ", "РµСЂ", "РѕР»", "Р°Рґ", "PSP", "РѕР±", "РёСЃ"}

	for _, msg := range messages {
		for _, marker := range markers {
			if strings.Contains(msg, marker) {
				t.Fatalf("log message %q contains mojibake marker %q", msg, marker)
			}
		}
	}
}

type fakeMemberPurger struct {
	returns []int
	calls   int
}

func (f *fakeMemberPurger) PurgeExpiredLeftMembers(ctx context.Context, now time.Time, limit int) (int, error) {
	f.calls++
	if f.calls <= len(f.returns) {
		return f.returns[f.calls-1], nil
	}
	return 0, nil
}
func (f *fakeMemberPurger) ListActiveUserIDs(ctx context.Context) ([]int64, error) { return nil, nil }
func (f *fakeMemberPurger) UpdateMemberTag(ctx context.Context, userID int64, tag *string, updatedAt time.Time) error {
	return nil
}
func (f *fakeMemberPurger) ScanAndUpdateMemberTags(ctx context.Context, tgOps *telegram.Ops, memberSourceChatID int64, now time.Time) (int, error) {
	return 0, nil
}

type fakeAdminCleaner struct {
	calls int
	err   error
}

func (f *fakeAdminCleaner) CleanupStaleAuthState(ctx context.Context, now time.Time) error {
	f.calls++
	return f.err
}

func TestRunPurgeTick_LoopsUntilZero(t *testing.T) {
	purger := &fakeMemberPurger{returns: []int{500, 120, 0}}
	s := &Scheduler{memberService: purger}

	s.runPurgeTick(context.Background(), time.Now().UTC())

	if purger.calls != 3 {
		t.Fatalf("calls = %d, want 3", purger.calls)
	}
}

func TestRunPurgeTick_StopsAtMaxIterations(t *testing.T) {
	returns := make([]int, purgeMaxIterations+5)
	for i := range returns {
		returns[i] = 1
	}
	purger := &fakeMemberPurger{returns: returns}
	s := &Scheduler{memberService: purger}

	s.runPurgeTick(context.Background(), time.Now().UTC())

	if purger.calls != purgeMaxIterations {
		t.Fatalf("calls = %d, want %d", purger.calls, purgeMaxIterations)
	}
}

func TestRunPurgeTick_RunsAdminCleanupOnce(t *testing.T) {
	purger := &fakeMemberPurger{returns: []int{0}}
	cleaner := &fakeAdminCleaner{}
	s := &Scheduler{memberService: purger, adminService: cleaner}

	s.runPurgeTick(context.Background(), time.Now().UTC())

	if cleaner.calls != 1 {
		t.Fatalf("admin cleanup calls = %d, want 1", cleaner.calls)
	}
}

type fakeMemberPurgerErr struct {
	err error
}

func (f *fakeMemberPurgerErr) PurgeExpiredLeftMembers(ctx context.Context, now time.Time, limit int) (int, error) {
	return 0, f.err
}
func (f *fakeMemberPurgerErr) ListActiveUserIDs(ctx context.Context) ([]int64, error) {
	return nil, nil
}
func (f *fakeMemberPurgerErr) UpdateMemberTag(ctx context.Context, userID int64, tag *string, updatedAt time.Time) error {
	return nil
}
func (f *fakeMemberPurgerErr) ScanAndUpdateMemberTags(ctx context.Context, tgOps *telegram.Ops, memberSourceChatID int64, now time.Time) (int, error) {
	return 0, nil
}

func TestRunPurgeTick_UpdatesMetricsOnSuccess(t *testing.T) {
	now := time.Now().UTC()
	purger := &fakeMemberPurger{returns: []int{3, 2, 0}}
	s := &Scheduler{memberService: purger}

	s.runPurgeTick(context.Background(), now)
	m := s.GetPurgeMetrics()

	if !m.LastRunAt.Equal(now) {
		t.Fatalf("LastRunAt = %v, want %v", m.LastRunAt, now)
	}
	if m.LastRunDeleted != 5 {
		t.Fatalf("LastRunDeleted = %d, want 5", m.LastRunDeleted)
	}
	if m.TotalDeleted != 5 {
		t.Fatalf("TotalDeleted = %d, want 5", m.TotalDeleted)
	}
	if m.LastError != "" {
		t.Fatalf("LastError = %q, want empty", m.LastError)
	}
}

func TestRunPurgeTick_StoresLastError(t *testing.T) {
	now := time.Now().UTC()
	purger := &fakeMemberPurgerErr{err: context.Canceled}
	s := &Scheduler{memberService: purger}

	s.runPurgeTick(context.Background(), now)
	m := s.GetPurgeMetrics()

	if !m.LastRunAt.Equal(now) {
		t.Fatalf("LastRunAt = %v, want %v", m.LastRunAt, now)
	}
	if m.LastRunDeleted != 0 {
		t.Fatalf("LastRunDeleted = %d, want 0", m.LastRunDeleted)
	}
	if m.TotalDeleted != 0 {
		t.Fatalf("TotalDeleted = %d, want 0", m.TotalDeleted)
	}
	if m.LastError == "" {
		t.Fatal("LastError expected non-empty")
	}
}

func TestRunPurgeTick_StoresAdminCleanupError(t *testing.T) {
	now := time.Now().UTC()
	purger := &fakeMemberPurger{returns: []int{0}}
	cleaner := &fakeAdminCleaner{err: errors.New("admin cleanup failed")}
	s := &Scheduler{memberService: purger, adminService: cleaner}

	s.runPurgeTick(context.Background(), now)
	m := s.GetPurgeMetrics()

	if cleaner.calls != 1 {
		t.Fatalf("admin cleanup calls = %d, want 1", cleaner.calls)
	}
	if m.LastError == "" {
		t.Fatal("LastError expected non-empty")
	}
}
