package streak

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"serotonyl.ru/telegram-bot/internal/config"
)

type fakeRepo struct {
	byUser                  map[int64]*Streak
	processed               map[string]struct{}
	updateCalls             map[int64]int
	reminderClaimCalls      map[int64]int
	reminderClaimShouldFail map[int64]bool
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		byUser:                  make(map[int64]*Streak),
		processed:               make(map[string]struct{}),
		updateCalls:             make(map[int64]int),
		reminderClaimCalls:      make(map[int64]int),
		reminderClaimShouldFail: make(map[int64]bool),
	}
}

func (r *fakeRepo) Create(ctx context.Context, userID int64) error {
	if _, ok := r.byUser[userID]; !ok {
		r.byUser[userID] = &Streak{UserID: userID}
	}
	return nil
}

func (r *fakeRepo) CreateTx(ctx context.Context, tx pgx.Tx, userID int64) error {
	return r.Create(ctx, userID)
}

func (r *fakeRepo) GetByUserID(ctx context.Context, userID int64) (*Streak, error) {
	st, ok := r.byUser[userID]
	if !ok {
		return nil, fmt.Errorf("missing streak")
	}
	cp := *st
	return &cp, nil
}

func (r *fakeRepo) GetByUserIDForUpdateTx(ctx context.Context, tx pgx.Tx, userID int64) (*Streak, error) {
	return r.GetByUserID(ctx, userID)
}

func (r *fakeRepo) UpdateTx(ctx context.Context, tx pgx.Tx, s *Streak) error {
	cp := *s
	r.byUser[s.UserID] = &cp
	r.updateCalls[s.UserID]++
	return nil
}

func (r *fakeRepo) MarkProcessedMessageTx(ctx context.Context, tx pgx.Tx, userID, messageID int64, streakDay time.Time) error {
	key := fmt.Sprintf("%d:%d", userID, messageID)
	if _, ok := r.processed[key]; ok {
		return errProcessedMessageDuplicate
	}
	r.processed[key] = struct{}{}
	return nil
}

func (r *fakeRepo) MarkReminderSentIfNotSentTodayTx(ctx context.Context, tx pgx.Tx, userID int64, progressDay time.Time) (bool, error) {
	r.reminderClaimCalls[userID]++
	if r.reminderClaimShouldFail[userID] {
		return false, nil
	}
	st, ok := r.byUser[userID]
	if !ok {
		return false, fmt.Errorf("missing streak")
	}
	if st.ProgressDate == nil || !sameDay(*st.ProgressDate, progressDay) || st.ReminderSentToday {
		return false, nil
	}
	st.ReminderSentToday = true
	return true, nil
}

func (r *fakeRepo) ClearReminderSentTx(ctx context.Context, tx pgx.Tx, userID int64, progressDay time.Time) error {
	st, ok := r.byUser[userID]
	if !ok {
		return fmt.Errorf("missing streak")
	}
	if st.ProgressDate == nil || !sameDay(*st.ProgressDate, progressDay) {
		return nil
	}
	st.ReminderSentToday = false
	return nil
}

func (r *fakeRepo) GetTop(ctx context.Context, limit int) ([]TopEntry, error) {
	top := make([]TopEntry, 0, len(r.byUser))
	for _, st := range r.byUser {
		if st.CurrentStreak > 0 {
			top = append(top, TopEntry{UserID: st.UserID, CurrentStreak: st.CurrentStreak})
		}
	}
	sort.Slice(top, func(i, j int) bool {
		if top[i].CurrentStreak == top[j].CurrentStreak {
			return top[i].UserID < top[j].UserID
		}
		return top[i].CurrentStreak > top[j].CurrentStreak
	})
	if len(top) > limit {
		top = top[:limit]
	}
	return top, nil
}

func (r *fakeRepo) GetByMinStreak(ctx context.Context, minStreak int) ([]*Streak, error) {
	var out []*Streak
	for _, st := range r.byUser {
		if st.CurrentStreak >= minStreak {
			cp := *st
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (r *fakeRepo) ResetDaily(ctx context.Context, day time.Time) error { return nil }

type fakeEconomy struct {
	awards          []int64
	failAfterTx     bool
	failAddBalance  bool
	withTxCalls     int
	addBalanceCalls int
}

func (e *fakeEconomy) WithTransaction(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
	e.withTxCalls++
	if err := fn(ctx, nil); err != nil {
		return err
	}
	if e.failAfterTx {
		return errors.New("transaction failed")
	}
	return nil
}

func (e *fakeEconomy) AddBalanceTx(ctx context.Context, tx pgx.Tx, userID int64, amount int64, txType, description string) error {
	e.addBalanceCalls++
	if e.failAddBalance {
		return errors.New("add balance failed")
	}
	e.awards = append(e.awards, amount)
	return nil
}

func newTestService(now time.Time) (*Service, *fakeRepo, *fakeEconomy, *time.Time) {
	repo := newFakeRepo()
	econ := &fakeEconomy{}
	current := now
	svc := &Service{
		repo:           repo,
		economyService: econ,
		cfg:            &config.Config{AppTimezone: "Europe/Moscow", StreakReminderThreshold: 7, StreakInactiveHours: 10},
		location:       time.FixedZone("MSK", 3*60*60),
		now:            func() time.Time { return current },
		antiSpam:       make(map[int64]*antiSpamState),
	}
	return svc, repo, econ, &current
}

func TestIsValidForStreakRules(t *testing.T) {
	if IsValidForStreak("/ogonek check this message please now") {
		t.Fatal("slash command must not count")
	}
	if IsValidForStreak("😀 😀 😀 😀 😀") {
		t.Fatal("emoji-only message must not count")
	}
	if !IsValidForStreak("one two three four five") {
		t.Fatal("5-word message must count")
	}
}

func TestCountMessage_CompletesDayAndRewardsOnce(t *testing.T) {
	now := time.Date(2026, 3, 8, 12, 0, 0, 0, time.FixedZone("MSK", 3*60*60))
	svc, repo, econ, current := newTestService(now)

	texts := []string{
		"раз два три четыре пять",
		"раз два три четыре шесть",
		"раз два три четыре семь",
		"раз два три четыре восемь",
	}
	for i, text := range texts {
		*current = now.Add(time.Duration(i) * time.Minute)
		if err := svc.CountMessage(context.Background(), 1, int64(i+1), text); err != nil {
			t.Fatalf("count message %d: %v", i+1, err)
		}
	}

	st := repo.byUser[1]
	if st.MessagesToday != 4 || !st.QuotaCompletedToday || st.CurrentStreak != 1 {
		t.Fatalf("unexpected streak state: %+v", st)
	}
	if len(econ.awards) != 1 || econ.awards[0] != 10 {
		t.Fatalf("unexpected awards: %v", econ.awards)
	}

	if err := svc.CountMessage(context.Background(), 1, 4, texts[3]); err != nil {
		t.Fatalf("duplicate message id: %v", err)
	}
	if len(econ.awards) != 1 {
		t.Fatalf("reward must stay single, got %v", econ.awards)
	}
}

func TestCountMessage_DuplicateSpamExcluded(t *testing.T) {
	now := time.Date(2026, 3, 8, 12, 0, 0, 0, time.FixedZone("MSK", 3*60*60))
	svc, repo, _, _ := newTestService(now)

	text := "раз два три четыре пять"
	if err := svc.CountMessage(context.Background(), 2, 1, text); err != nil {
		t.Fatal(err)
	}
	if err := svc.CountMessage(context.Background(), 2, 2, text); err != nil {
		t.Fatal(err)
	}
	if got := repo.byUser[2].MessagesToday; got != 1 {
		t.Fatalf("expected 1 counted message, got %d", got)
	}
}

func TestCountMessage_RateLimitTwoPerMinute(t *testing.T) {
	now := time.Date(2026, 3, 8, 12, 0, 0, 0, time.FixedZone("MSK", 3*60*60))
	svc, repo, _, _ := newTestService(now)

	for i := 0; i < 3; i++ {
		if err := svc.CountMessage(context.Background(), 3, int64(i+1), fmt.Sprintf("раз два три четыре %d", i)); err != nil {
			t.Fatal(err)
		}
	}
	if got := repo.byUser[3].MessagesToday; got != 2 {
		t.Fatalf("expected 2 counted messages, got %d", got)
	}
}

func TestCountMessage_RewardCapAtSeventy(t *testing.T) {
	now := time.Date(2026, 3, 8, 12, 0, 0, 0, time.FixedZone("MSK", 3*60*60))
	svc, repo, econ, _ := newTestService(now)
	lastCompleted := time.Date(2026, 3, 7, 0, 0, 0, 0, time.FixedZone("MSK", 3*60*60))
	progressDate := time.Date(2026, 3, 8, 0, 0, 0, 0, time.FixedZone("MSK", 3*60*60))
	repo.byUser[4] = &Streak{
		UserID:              4,
		CurrentStreak:       7,
		LongestStreak:       7,
		MessagesToday:       3,
		ProgressDate:        &progressDate,
		LastQuotaCompletion: &lastCompleted,
	}

	if err := svc.CountMessage(context.Background(), 4, 1, "раз два три четыре пять"); err != nil {
		t.Fatal(err)
	}
	if repo.byUser[4].CurrentStreak != 8 {
		t.Fatalf("expected streak to continue, got %d", repo.byUser[4].CurrentStreak)
	}
	if len(econ.awards) != 1 || econ.awards[0] != 70 {
		t.Fatalf("expected capped 70 reward, got %v", econ.awards)
	}
}

func TestGetStreak_ResetsBrokenContinuityAndPersists(t *testing.T) {
	now := time.Date(2026, 3, 8, 12, 0, 0, 0, time.FixedZone("MSK", 3*60*60))
	svc, repo, _, _ := newTestService(now)
	oldDay := time.Date(2026, 3, 6, 0, 0, 0, 0, time.FixedZone("MSK", 3*60*60))
	repo.byUser[5] = &Streak{
		UserID:              5,
		CurrentStreak:       4,
		LongestStreak:       4,
		MessagesToday:       3,
		LastQuotaCompletion: &oldDay,
		ProgressDate:        &oldDay,
	}

	st, err := svc.GetStreak(context.Background(), 5)
	if err != nil {
		t.Fatal(err)
	}
	if st.CurrentStreak != 0 || st.MessagesToday != 0 || st.QuotaCompletedToday {
		t.Fatalf("expected reset streak, got %+v", st)
	}
	if repo.byUser[5].CurrentStreak != 0 {
		t.Fatalf("expected persisted reset, got %+v", repo.byUser[5])
	}
	if repo.updateCalls[5] == 0 {
		t.Fatal("expected normalized state to be persisted")
	}
}

func TestContinuityAllowsYesterdayButNotTwoDaysAgo(t *testing.T) {
	now := time.Date(2026, 3, 8, 12, 0, 0, 0, time.FixedZone("MSK", 3*60*60))
	svc, _, _, _ := newTestService(now)
	today := svc.dayStart(now)
	yesterday := today.AddDate(0, 0, -1)
	twoDaysAgo := today.AddDate(0, 0, -2)

	if svc.isContinuityBroken(&Streak{CurrentStreak: 3, LastQuotaCompletion: &today}, today) {
		t.Fatal("today completion must keep continuity")
	}
	if svc.isContinuityBroken(&Streak{CurrentStreak: 3, LastQuotaCompletion: &yesterday}, today) {
		t.Fatal("yesterday completion must keep continuity")
	}
	if !svc.isContinuityBroken(&Streak{CurrentStreak: 3, LastQuotaCompletion: &twoDaysAgo}, today) {
		t.Fatal("two-day gap must break continuity")
	}
}

func TestSendReminders_PersistsNormalizationAndSkipsStaleStreak(t *testing.T) {
	now := time.Date(2026, 3, 8, 12, 0, 0, 0, time.FixedZone("MSK", 3*60*60))
	svc, repo, _, _ := newTestService(now)
	oldDay := time.Date(2026, 3, 6, 0, 0, 0, 0, time.FixedZone("MSK", 3*60*60))
	lastMessageAt := now.Add(-12 * time.Hour).UTC()
	repo.byUser[7] = &Streak{
		UserID:              7,
		CurrentStreak:       8,
		LongestStreak:       8,
		MessagesToday:       4,
		QuotaCompletedToday: true,
		LastQuotaCompletion: &oldDay,
		ProgressDate:        &oldDay,
		LastMessageAt:       &lastMessageAt,
	}

	var sent []int64
	if err := svc.SendReminders(context.Background(), func(ctx context.Context, userID int64, text string) error {
		sent = append(sent, userID)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if len(sent) != 0 {
		t.Fatalf("expected stale streak to be normalized and skipped, got %v", sent)
	}
	if repo.byUser[7].CurrentStreak != 0 {
		t.Fatalf("expected normalized streak persisted, got %+v", repo.byUser[7])
	}
	if repo.updateCalls[7] == 0 {
		t.Fatal("expected reminder path to persist normalization")
	}
}

func TestSendReminders_ConditionalClaimPreventsDuplicateSend(t *testing.T) {
	now := time.Date(2026, 3, 8, 12, 0, 0, 0, time.FixedZone("MSK", 3*60*60))
	svc, repo, _, _ := newTestService(now)
	progressDate := time.Date(2026, 3, 8, 0, 0, 0, 0, time.FixedZone("MSK", 3*60*60))
	lastCompleted := time.Date(2026, 3, 7, 0, 0, 0, 0, time.FixedZone("MSK", 3*60*60))
	lastMessageAt := now.Add(-12 * time.Hour).UTC()
	repo.byUser[8] = &Streak{
		UserID:              8,
		CurrentStreak:       8,
		LongestStreak:       8,
		ProgressDate:        &progressDate,
		LastQuotaCompletion: &lastCompleted,
		LastMessageAt:       &lastMessageAt,
	}
	repo.reminderClaimShouldFail[8] = true

	var sent []int64
	if err := svc.SendReminders(context.Background(), func(ctx context.Context, userID int64, text string) error {
		sent = append(sent, userID)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if len(sent) != 0 {
		t.Fatalf("expected claim failure to skip send, got %v", sent)
	}
	if repo.reminderClaimCalls[8] != 1 {
		t.Fatalf("expected one claim attempt, got %d", repo.reminderClaimCalls[8])
	}
}

func TestSendReminders_SendFailureReleasesClaimAndReturnsError(t *testing.T) {
	now := time.Date(2026, 3, 8, 12, 0, 0, 0, time.FixedZone("MSK", 3*60*60))
	svc, repo, _, _ := newTestService(now)
	progressDate := time.Date(2026, 3, 8, 0, 0, 0, 0, time.FixedZone("MSK", 3*60*60))
	lastCompleted := time.Date(2026, 3, 7, 0, 0, 0, 0, time.FixedZone("MSK", 3*60*60))
	lastMessageAt := now.Add(-12 * time.Hour).UTC()
	repo.byUser[11] = &Streak{
		UserID:              11,
		CurrentStreak:       8,
		LongestStreak:       8,
		ProgressDate:        &progressDate,
		LastQuotaCompletion: &lastCompleted,
		LastMessageAt:       &lastMessageAt,
	}

	err := svc.SendReminders(context.Background(), func(ctx context.Context, userID int64, text string) error {
		return errors.New("telegram down")
	})
	if err == nil {
		t.Fatal("expected send error")
	}
	if !strings.Contains(err.Error(), "send reminder user_id=11") {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.byUser[11].ReminderSentToday {
		t.Fatalf("expected reminder claim released after send error, got %+v", repo.byUser[11])
	}
}

func TestSendReminders_UsesReadableReminderText(t *testing.T) {
	now := time.Date(2026, 3, 8, 12, 0, 0, 0, time.FixedZone("MSK", 3*60*60))
	svc, repo, _, _ := newTestService(now)
	progressDate := time.Date(2026, 3, 8, 0, 0, 0, 0, time.FixedZone("MSK", 3*60*60))
	lastCompleted := time.Date(2026, 3, 7, 0, 0, 0, 0, time.FixedZone("MSK", 3*60*60))
	lastMessageAt := now.Add(-12 * time.Hour).UTC()
	repo.byUser[12] = &Streak{
		UserID:              12,
		CurrentStreak:       8,
		LongestStreak:       8,
		ProgressDate:        &progressDate,
		LastQuotaCompletion: &lastCompleted,
		LastMessageAt:       &lastMessageAt,
	}

	var got string
	if err := svc.SendReminders(context.Background(), func(ctx context.Context, userID int64, text string) error {
		got = text
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(got, "У тебя огонек 8") {
		t.Fatalf("unexpected reminder text: %q", got)
	}
	if strings.Contains(got, "Р") {
		t.Fatalf("reminder text still looks mojibake: %q", got)
	}
}

func TestCountMessage_AntiSpamNotConsumedOnFailedTransaction(t *testing.T) {
	now := time.Date(2026, 3, 8, 12, 0, 0, 0, time.FixedZone("MSK", 3*60*60))
	svc, repo, econ, current := newTestService(now)
	econ.failAfterTx = true

	text := "раз два три четыре пять"
	err := svc.CountMessage(context.Background(), 9, 1, text)
	if err == nil {
		t.Fatal("expected transaction failure")
	}
	if repo.byUser[9].MessagesToday != 1 {
		t.Fatalf("expected state change from failed fake transaction, got %+v", repo.byUser[9])
	}
	if state := svc.antiSpam[9]; state != nil && (state.LastNormalized != "" || len(state.CountedAt) != 0 || !state.LastAt.IsZero()) {
		t.Fatalf("expected anti-spam state to stay uncommitted, got %+v", state)
	}

	repo.byUser[9] = &Streak{UserID: 9}
	repo.processed = make(map[string]struct{})
	econ.failAfterTx = false
	*current = now.Add(2 * time.Second)
	if err := svc.CountMessage(context.Background(), 9, 2, text); err != nil {
		t.Fatalf("expected retry to pass after failed tx, got %v", err)
	}
	if repo.byUser[9].MessagesToday != 1 {
		t.Fatalf("expected retry message to count, got %+v", repo.byUser[9])
	}
}

func TestGetTop_SortsByCurrentStreakThenUserID(t *testing.T) {
	svc, repo, _, _ := newTestService(time.Now())
	repo.byUser[10] = &Streak{UserID: 10, CurrentStreak: 3}
	repo.byUser[5] = &Streak{UserID: 5, CurrentStreak: 5}
	repo.byUser[7] = &Streak{UserID: 7, CurrentStreak: 5}

	top, err := svc.GetTop(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(top) != 3 {
		t.Fatalf("unexpected top len: %d", len(top))
	}
	if top[0].UserID != 5 || top[1].UserID != 7 || top[2].UserID != 10 {
		t.Fatalf("unexpected order: %+v", top)
	}
}
