package admin

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	models "github.com/mymmrac/telego"

	"serotonyl.ru/telegram-bot/internal/audit"
	"serotonyl.ru/telegram-bot/internal/features/members"
	"serotonyl.ru/telegram-bot/internal/telegram"
)

type fakeRiddleRepo struct {
	nextID                int64
	riddle                *Riddle
	answers               []*RiddleAnswer
	claimCalls            int
	lastClaimNormalized   string
	listExpiredCalls      int
	cleanupCalls          int
	abortCalls            int
	abortErr              error
	expiredActiveSnapshot []*Riddle
}

func (f *fakeRiddleRepo) WithTx(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
	return fn(ctx, nil)
}

func (f *fakeRiddleRepo) CreatePublishingRiddleTx(ctx context.Context, tx pgx.Tx, adminID int64, postText string, reward int64, answers []RiddleDraftAnswer, now time.Time) (*Riddle, []*RiddleAnswer, error) {
	if f.riddle != nil && (f.riddle.State == riddleStateActive || f.riddle.State == riddleStatePublishing) {
		return nil, nil, ErrRiddleAlreadyActive
	}
	f.nextID++
	f.riddle = &Riddle{ID: f.nextID, State: riddleStatePublishing, PostText: postText, RewardAmount: reward, CreatedByAdminID: adminID, CreatedAt: now, ExpiresAt: now.Add(riddleTTL)}
	f.answers = make([]*RiddleAnswer, 0, len(answers))
	for i, ans := range answers {
		id := int64(i + 1)
		f.answers = append(f.answers, &RiddleAnswer{ID: id, RiddleID: f.riddle.ID, AnswerRaw: ans.Raw, AnswerNormalized: ans.Normalized})
	}
	return f.riddle, cloneAnswers(f.answers), nil
}

func (f *fakeRiddleRepo) ActivatePublishedRiddle(ctx context.Context, riddleID, groupChatID, messageID int64, publishedAt time.Time) error {
	if f.riddle == nil || f.riddle.ID != riddleID || f.riddle.State != riddleStatePublishing {
		return ErrRiddleStateConflict
	}
	f.riddle.State = riddleStateActive
	f.riddle.GroupChatID = &groupChatID
	f.riddle.MessageID = &messageID
	f.riddle.PublishedAt = &publishedAt
	f.riddle.ExpiresAt = publishedAt.Add(riddleTTL)
	return nil
}

func (f *fakeRiddleRepo) AbortPublishingRiddle(ctx context.Context, riddleID int64) error {
	f.abortCalls++
	if f.abortErr != nil {
		return f.abortErr
	}
	if f.riddle != nil && f.riddle.ID == riddleID && f.riddle.State == riddleStatePublishing {
		f.riddle = nil
		f.answers = nil
	}
	return nil
}

func (f *fakeRiddleRepo) StopActiveRiddleTx(ctx context.Context, tx pgx.Tx, now time.Time) (*Riddle, []*RiddleAnswer, error) {
	if f.riddle == nil || f.riddle.State != riddleStateActive {
		return nil, nil, nil
	}
	f.riddle.State = riddleStateStopped
	f.riddle.FinishedAt = ptrTime(now)
	f.riddle.ExpiresAt = now.Add(riddleTTL)
	return f.riddle, cloneAnswers(f.answers), nil
}

func (f *fakeRiddleRepo) ClaimAnswerAndMaybeCompleteTx(ctx context.Context, tx pgx.Tx, normalized, winnerDisplay string, userID int64, messageID int64, now time.Time) (*Riddle, []*RiddleAnswer, bool, error) {
	f.claimCalls++
	f.lastClaimNormalized = normalized
	if f.riddle == nil || f.riddle.State != riddleStateActive {
		return nil, nil, false, nil
	}
	for _, ans := range f.answers {
		if ans.AnswerNormalized != normalized || ans.WinnerUserID != nil {
			continue
		}
		ans.WinnerUserID = &userID
		ans.WinnerMessageID = &messageID
		ans.WinnerDisplay = &winnerDisplay
		ans.WonAt = &now
		done := true
		for _, item := range f.answers {
			if item.WinnerUserID == nil {
				done = false
				break
			}
		}
		if done {
			f.riddle.State = riddleStateCompleted
			f.riddle.FinishedAt = ptrTime(now)
			f.riddle.ExpiresAt = now.Add(riddleTTL)
			return f.riddle, cloneAnswers(f.answers), true, nil
		}
		return f.riddle, nil, false, nil
	}
	return nil, nil, false, nil
}

func (f *fakeRiddleRepo) GetActiveRiddle(ctx context.Context, now time.Time) (*Riddle, error) {
	if f.riddle == nil || f.riddle.State != riddleStateActive {
		return nil, nil
	}
	return f.riddle, nil
}

func (f *fakeRiddleRepo) ListExpiredActiveRiddles(ctx context.Context, now time.Time) ([]*Riddle, error) {
	f.listExpiredCalls++
	if f.expiredActiveSnapshot != nil {
		return cloneRiddles(f.expiredActiveSnapshot), nil
	}
	if f.riddle != nil && f.riddle.State == riddleStateActive && !f.riddle.ExpiresAt.After(now) {
		return []*Riddle{cloneRiddle(f.riddle)}, nil
	}
	return nil, nil
}

func (f *fakeRiddleRepo) CleanupExpired(ctx context.Context, now time.Time) (int64, error) {
	f.cleanupCalls++
	if f.riddle != nil && !f.riddle.ExpiresAt.After(now) {
		f.riddle = nil
		f.answers = nil
		return 1, nil
	}
	return 0, nil
}

type fakeRiddleEconomy struct {
	rewards []int64
	awardTo []int64
}

func (f *fakeRiddleEconomy) WithTransaction(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
	return fn(ctx, nil)
}

func (f *fakeRiddleEconomy) AddBalanceTx(ctx context.Context, tx pgx.Tx, userID int64, amount int64, txType, description string) error {
	f.awardTo = append(f.awardTo, userID)
	f.rewards = append(f.rewards, amount)
	return nil
}

func TestRiddleWizardValidationAndConfirm(t *testing.T) {
	tg := &fakeTG{}
	repo := &fakeMemberRepoHandlers{members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}}}
	h := newAdminHandlerForFlow(t, repo, tg)
	h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminRiddlesMenu))
	h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbRiddleCreate))

	if !h.HandleAdminMessage(context.Background(), 77, 77, 501, "   ") {
		t.Fatal("empty riddle text should be handled")
	}
	if last := tg.last("send"); last == nil || !strings.Contains(last.text, "Текст загадки не должен быть пустым.") {
		t.Fatalf("expected text validation error, got %#v", last)
	}

	_ = h.HandleAdminMessage(context.Background(), 77, 77, 502, "Текст загадки")
	_ = h.HandleAdminMessage(context.Background(), 77, 77, 503, "\napple\n \npear\n")
	_ = h.HandleAdminMessage(context.Background(), 77, 77, 504, "15")

	edit := tg.last("edit")
	if edit == nil || !strings.Contains(edit.text, "Текст загадки") || !strings.Contains(edit.text, "Ответов: 2") || !strings.Contains(edit.text, "Награда: 15") {
		t.Fatalf("expected confirm screen, got %#v", edit)
	}
}

func TestRiddleProcessGuessNormalizesOnceAndClaimsDirectly(t *testing.T) {
	repo := &fakeRiddleRepo{}
	econ := &fakeRiddleEconomy{}
	svc := NewRiddleService(repo, econ)
	now := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	pub, err := svc.CreatePublishing(context.Background(), 77, &RiddleDraftData{
		PostText:     "riddle",
		RewardAmount: 10,
		Answers:      []RiddleDraftAnswer{{Raw: "Apple", Normalized: "apple"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ActivatePublished(context.Background(), pub.Riddle.ID, -1001, 55); err != nil {
		t.Fatal(err)
	}

	res, matched, err := svc.ProcessGuess(context.Background(), &models.Message{
		MessageID: 101,
		Chat:      models.Chat{ID: -1001},
		From:      &models.User{ID: 1, Username: "alice"},
		Text:      "  APPLE \n",
	})
	if err != nil || !matched || res == nil || res.Riddle.State != riddleStateCompleted {
		t.Fatalf("expected completed single-answer riddle, matched=%v err=%v res=%+v", matched, err, res)
	}
	if repo.claimCalls != 1 || repo.lastClaimNormalized != "apple" {
		t.Fatalf("expected one direct normalized claim, calls=%d normalized=%q", repo.claimCalls, repo.lastClaimNormalized)
	}
	if len(econ.awardTo) != 1 || econ.awardTo[0] != 1 || econ.rewards[0] != 10 {
		t.Fatalf("unexpected rewards: %+v %+v", econ.awardTo, econ.rewards)
	}
}

func TestRiddleMultiAnswerAndRepeatedSameAnswer(t *testing.T) {
	repo := &fakeRiddleRepo{}
	econ := &fakeRiddleEconomy{}
	svc := NewRiddleService(repo, econ)
	now := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	pub, _ := svc.CreatePublishing(context.Background(), 77, &RiddleDraftData{
		PostText:     "riddle",
		RewardAmount: 7,
		Answers: []RiddleDraftAnswer{
			{Raw: "Apple", Normalized: "apple"},
			{Raw: "Pear", Normalized: "pear"},
		},
	})
	_ = svc.ActivatePublished(context.Background(), pub.Riddle.ID, -1001, 55)

	res, matched, err := svc.ProcessGuess(context.Background(), &models.Message{MessageID: 1, Chat: models.Chat{ID: -1001}, From: &models.User{ID: 10, Username: "u1"}, Text: "apple"})
	if err != nil || !matched || res != nil {
		t.Fatalf("first answer should match without completion: matched=%v err=%v res=%+v", matched, err, res)
	}
	res, matched, err = svc.ProcessGuess(context.Background(), &models.Message{MessageID: 2, Chat: models.Chat{ID: -1001}, From: &models.User{ID: 11, Username: "u2"}, Text: "apple"})
	if err != nil || matched || res != nil {
		t.Fatalf("repeated same answer must not win twice: matched=%v err=%v res=%+v", matched, err, res)
	}
	res, matched, err = svc.ProcessGuess(context.Background(), &models.Message{MessageID: 3, Chat: models.Chat{ID: -1001}, From: &models.User{ID: 10, Username: "u1"}, Text: "pear"})
	if err != nil || !matched || res == nil || res.Riddle.State != riddleStateCompleted {
		t.Fatalf("second distinct answer should complete: matched=%v err=%v res=%+v", matched, err, res)
	}
	if len(econ.awardTo) != 2 || econ.awardTo[0] != 10 || econ.awardTo[1] != 10 {
		t.Fatalf("same user should be allowed to win multiple answers, got %+v", econ.awardTo)
	}
}

func brokenEncodingTestRiddleCompletesWhenSameUserClaimsAllAnswerSlots(t *testing.T) {
	repo := &fakeRiddleRepo{}
	econ := &fakeRiddleEconomy{}
	svc := NewRiddleService(repo, econ)
	now := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	pub, _ := svc.CreatePublishing(context.Background(), 77, &RiddleDraftData{
		PostText:     "riddle",
		RewardAmount: 9,
		Answers: []RiddleDraftAnswer{
			{Raw: "Apple", Normalized: "apple"},
			{Raw: "Pear", Normalized: "pear"},
		},
	})
	_ = svc.ActivatePublished(context.Background(), pub.Riddle.ID, -1001, 55)

	res, matched, err := svc.ProcessGuess(context.Background(), &models.Message{MessageID: 1, Chat: models.Chat{ID: -1001}, From: &models.User{ID: 10, Username: "u1"}, Text: "apple"})
	if err != nil || !matched || res != nil {
		t.Fatalf("first answer should claim without completion: matched=%v err=%v res=%+v", matched, err, res)
	}
	res, matched, err = svc.ProcessGuess(context.Background(), &models.Message{MessageID: 2, Chat: models.Chat{ID: -1001}, From: &models.User{ID: 10, Username: "u1"}, Text: "pear"})
	if err != nil || !matched || res == nil || res.Riddle.State != riddleStateCompleted {
		t.Fatalf("second claimed slot should complete immediately: matched=%v err=%v res=%+v", matched, err, res)
	}
	if len(econ.awardTo) != 2 || econ.awardTo[0] != 10 || econ.awardTo[1] != 10 {
		t.Fatalf("reward issuance must stay exactly once per answer slot, got awardTo=%+v", econ.awardTo)
	}
	if len(econ.rewards) != 2 || econ.rewards[0] != 9 || econ.rewards[1] != 9 {
		t.Fatalf("unexpected reward amounts: %+v", econ.rewards)
	}
	if winners := summarizeRiddleWinners(res.Answers); len(winners) != 1 || winners[0] != "@u1 x2" {
		t.Fatalf("winner summary may aggregate repeated wins, got %+v", winners)
	}
}

func localizedStringVariantTestRiddleCompletesWhenSameUserClaimsAllAnswerSlots(t *testing.T) {
	repo := &fakeRiddleRepo{}
	econ := &fakeRiddleEconomy{}
	svc := NewRiddleService(repo, econ)
	now := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	pub, _ := svc.CreatePublishing(context.Background(), 77, &RiddleDraftData{
		PostText:     "riddle",
		RewardAmount: 9,
		Answers: []RiddleDraftAnswer{
			{Raw: "Apple", Normalized: "apple"},
			{Raw: "Pear", Normalized: "pear"},
		},
	})
	_ = svc.ActivatePublished(context.Background(), pub.Riddle.ID, -1001, 55)

	res, matched, err := svc.ProcessGuess(context.Background(), &models.Message{
		MessageID: 1,
		Chat:      models.Chat{ID: -1001},
		From:      &models.User{ID: 10, Username: "u1"},
		Text:      "apple",
	})
	if err != nil || !matched || res != nil {
		t.Fatalf("first answer should claim without completion: matched=%v err=%v res=%+v", matched, err, res)
	}

	res, matched, err = svc.ProcessGuess(context.Background(), &models.Message{
		MessageID: 2,
		Chat:      models.Chat{ID: -1001},
		From:      &models.User{ID: 10, Username: "u1"},
		Text:      "pear",
	})
	if err != nil || !matched || res == nil || res.Riddle.State != riddleStateCompleted {
		t.Fatalf("second claimed slot should complete immediately: matched=%v err=%v res=%+v", matched, err, res)
	}
	if len(econ.awardTo) != 2 || econ.awardTo[0] != 10 || econ.awardTo[1] != 10 {
		t.Fatalf("reward issuance must stay exactly once per answer slot, got awardTo=%+v", econ.awardTo)
	}
	if len(econ.rewards) != 2 || econ.rewards[0] != 9 || econ.rewards[1] != 9 {
		t.Fatalf("unexpected reward amounts: %+v", econ.rewards)
	}
	if winners := summarizeRiddleWinners(res.Answers); len(winners) != 1 || winners[0] != "@u1 x2" {
		t.Fatalf("winner summary may aggregate repeated wins, got %+v", winners)
	}
}

func TestRiddleCompletesWhenSameUserClaimsAllAnswerSlotsASCII(t *testing.T) {
	repo := &fakeRiddleRepo{}
	econ := &fakeRiddleEconomy{}
	svc := NewRiddleService(repo, econ)
	now := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	pub, err := svc.CreatePublishing(context.Background(), 77, &RiddleDraftData{
		PostText:     "riddle",
		RewardAmount: 9,
		Answers: []RiddleDraftAnswer{
			{Raw: "Apple", Normalized: "apple"},
			{Raw: "Pear", Normalized: "pear"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ActivatePublished(context.Background(), pub.Riddle.ID, -1001, 55); err != nil {
		t.Fatal(err)
	}

	res, matched, err := svc.ProcessGuess(context.Background(), &models.Message{
		MessageID: 1,
		Chat:      models.Chat{ID: -1001},
		From:      &models.User{ID: 10, Username: "u1"},
		Text:      "apple",
	})
	if err != nil || !matched || res != nil {
		t.Fatalf("first answer should claim without completion: matched=%v err=%v res=%+v", matched, err, res)
	}

	res, matched, err = svc.ProcessGuess(context.Background(), &models.Message{
		MessageID: 2,
		Chat:      models.Chat{ID: -1001},
		From:      &models.User{ID: 10, Username: "u1"},
		Text:      "pear",
	})
	if err != nil || !matched || res == nil || res.Riddle.State != riddleStateCompleted {
		t.Fatalf("second claimed slot should complete immediately: matched=%v err=%v res=%+v", matched, err, res)
	}
	if len(econ.awardTo) != 2 || econ.awardTo[0] != 10 || econ.awardTo[1] != 10 {
		t.Fatalf("reward issuance must stay exactly once per answer slot, got awardTo=%+v", econ.awardTo)
	}
	if len(econ.rewards) != 2 || econ.rewards[0] != 9 || econ.rewards[1] != 9 {
		t.Fatalf("unexpected reward amounts: %+v", econ.rewards)
	}
	if winners := summarizeRiddleWinners(res.Answers); len(winners) != 1 || winners[0] != "@u1 x2" {
		t.Fatalf("winner summary may aggregate repeated wins, got %+v", winners)
	}
}

func TestRiddleCompletionAndStopAreIdempotent(t *testing.T) {
	repo := &fakeRiddleRepo{}
	econ := &fakeRiddleEconomy{}
	svc := NewRiddleService(repo, econ)
	now := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	pub, _ := svc.CreatePublishing(context.Background(), 77, &RiddleDraftData{
		PostText:     "riddle",
		RewardAmount: 5,
		Answers:      []RiddleDraftAnswer{{Raw: "Apple", Normalized: "apple"}},
	})
	_ = svc.ActivatePublished(context.Background(), pub.Riddle.ID, -1001, 55)

	res, matched, err := svc.ProcessGuess(context.Background(), &models.Message{MessageID: 1, Chat: models.Chat{ID: -1001}, From: &models.User{ID: 1, Username: "u"}, Text: "apple"})
	if err != nil || !matched || res == nil || res.Riddle.State != riddleStateCompleted {
		t.Fatalf("expected completion on first guess, matched=%v err=%v res=%+v", matched, err, res)
	}
	if len(econ.awardTo) != 1 {
		t.Fatalf("expected one reward after completion, got %+v", econ.awardTo)
	}

	res, matched, err = svc.ProcessGuess(context.Background(), &models.Message{MessageID: 2, Chat: models.Chat{ID: -1001}, From: &models.User{ID: 1, Username: "u"}, Text: "apple"})
	if err != nil || matched || res != nil {
		t.Fatalf("completed riddle must ignore repeated completion path, matched=%v err=%v res=%+v", matched, err, res)
	}
	if len(econ.awardTo) != 1 {
		t.Fatalf("repeated completion must not duplicate rewards, got %+v", econ.awardTo)
	}

	stop, err := svc.StopActive(context.Background())
	if err != nil || stop != nil {
		t.Fatalf("repeat stop should be idempotent, err=%v stop=%+v", err, stop)
	}
}

func TestRiddleStopPreventsFurtherProcessingAndCleanupRemovesAnswers(t *testing.T) {
	repo := &fakeRiddleRepo{}
	econ := &fakeRiddleEconomy{}
	svc := NewRiddleService(repo, econ)
	now := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	pub, _ := svc.CreatePublishing(context.Background(), 77, &RiddleDraftData{PostText: "riddle", RewardAmount: 5, Answers: []RiddleDraftAnswer{{Raw: "Apple", Normalized: "apple"}}})
	_ = svc.ActivatePublished(context.Background(), pub.Riddle.ID, -1001, 55)

	stop, err := svc.StopActive(context.Background())
	if err != nil || stop == nil || stop.Riddle.State != riddleStateStopped {
		t.Fatalf("expected stopped riddle, got err=%v stop=%+v", err, stop)
	}
	res, matched, err := svc.ProcessGuess(context.Background(), &models.Message{MessageID: 1, Chat: models.Chat{ID: -1001}, From: &models.User{ID: 1, Username: "u"}, Text: "apple"})
	if err != nil || matched || res != nil {
		t.Fatalf("stopped riddle must ignore future answers: matched=%v err=%v res=%+v", matched, err, res)
	}

	repo.riddle.ExpiresAt = now.Add(-time.Minute)
	if err := svc.CleanupExpired(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if repo.riddle != nil || len(repo.answers) != 0 {
		t.Fatalf("cleanup should remove expired riddle and its answers")
	}
}

func TestRiddleCleanupUnpinsExpiredActiveBeforeDeleteBestEffort(t *testing.T) {
	repo := &fakeRiddleRepo{
		riddle: &Riddle{
			ID:          1,
			State:       riddleStateActive,
			GroupChatID: ptrInt64(-1001),
			MessageID:   ptrInt64(55),
			ExpiresAt:   time.Date(2026, 3, 9, 11, 0, 0, 0, time.UTC),
		},
		expiredActiveSnapshot: []*Riddle{{
			ID:          1,
			State:       riddleStateActive,
			GroupChatID: ptrInt64(-1001),
			MessageID:   ptrInt64(55),
			ExpiresAt:   time.Date(2026, 3, 9, 11, 0, 0, 0, time.UTC),
		}},
	}
	econ := &fakeRiddleEconomy{}
	tg := &fakeTG{unpinErr: errors.New("boom")}
	svc := NewRiddleService(repo, econ)
	svc.SetOps(telegram.NewOps(tg))
	now := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)

	if err := svc.CleanupExpired(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if tg.count("unpin") != 1 {
		t.Fatalf("expected best-effort unpin before delete, got %d", tg.count("unpin"))
	}
	if tg.count("delete") != 1 {
		t.Fatalf("expected best-effort delete before cleanup, got %d", tg.count("delete"))
	}
	if repo.listExpiredCalls != 1 || repo.cleanupCalls != 1 {
		t.Fatalf("expected cleanup flow to list expired and delete once, list=%d cleanup=%d", repo.listExpiredCalls, repo.cleanupCalls)
	}
}

func TestRiddleCleanupDeleteFailureDoesNotBlockCleanup(t *testing.T) {
	repo := &fakeRiddleRepo{
		riddle: &Riddle{
			ID:          1,
			State:       riddleStateActive,
			GroupChatID: ptrInt64(-1001),
			MessageID:   ptrInt64(55),
			ExpiresAt:   time.Date(2026, 3, 9, 11, 0, 0, 0, time.UTC),
		},
		expiredActiveSnapshot: []*Riddle{{
			ID:          1,
			State:       riddleStateActive,
			GroupChatID: ptrInt64(-1001),
			MessageID:   ptrInt64(55),
			ExpiresAt:   time.Date(2026, 3, 9, 11, 0, 0, 0, time.UTC),
		}},
	}
	econ := &fakeRiddleEconomy{}
	tg := &fakeTG{deleteErr: errors.New("missing")}
	svc := NewRiddleService(repo, econ)
	svc.SetOps(telegram.NewOps(tg))
	now := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)

	if err := svc.CleanupExpired(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	if tg.count("delete") != 1 {
		t.Fatalf("expected one delete attempt, got %d", tg.count("delete"))
	}
	if repo.cleanupCalls != 1 {
		t.Fatalf("expected cleanup to proceed after delete failure, got %d", repo.cleanupCalls)
	}
}

func TestRiddlePublishFailureAbortsPublishingRecord(t *testing.T) {
	tg := &fakeTG{sendErr: errors.New("send failed")}
	stateRepo := &fakeAdminRepoHandlers{hasSession: true}
	memberRepo := &fakeMemberRepoHandlers{members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}}}
	svc := NewService(stateRepo, memberRepo, nil)
	riddleRepo := &fakeRiddleRepo{}
	riddleSvc := NewRiddleService(riddleRepo, &fakeRiddleEconomy{})
	svc.SetRiddleService(riddleSvc)

	h := NewHandler(svc, nil, &fakeEconomy{}, telegram.NewOps(tg), -1001)
	h.riddleService = riddleSvc
	h.service.SetState(77, StateRiddleConfirm, &RiddleDraftData{
		PostText:     "riddle",
		RewardAmount: 10,
		Answers:      []RiddleDraftAnswer{{Raw: "Apple", Normalized: "apple"}},
	})

	h.handleRiddlePublish(context.Background(), 77, 77)

	if riddleRepo.abortCalls != 1 {
		t.Fatalf("expected publication abort on send failure, got %d", riddleRepo.abortCalls)
	}
	if riddleRepo.riddle != nil {
		t.Fatalf("send failure must not leave publishing riddle behind: %+v", riddleRepo.riddle)
	}
	if last := tg.last("send"); last == nil || !strings.Contains(last.text, "Не удалось опубликовать загадку в основном чате.") {
		t.Fatalf("expected publish failure message, got %#v", last)
	}
}

func TestRiddleActiveSurvivesRestart(t *testing.T) {
	repo := &fakeRiddleRepo{}
	econ := &fakeRiddleEconomy{}
	now := time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC)

	svc1 := NewRiddleService(repo, econ)
	svc1.now = func() time.Time { return now }
	pub, _ := svc1.CreatePublishing(context.Background(), 77, &RiddleDraftData{PostText: "riddle", RewardAmount: 8, Answers: []RiddleDraftAnswer{{Raw: "Apple", Normalized: "apple"}}})
	_ = svc1.ActivatePublished(context.Background(), pub.Riddle.ID, -1001, 55)

	svc2 := NewRiddleService(repo, econ)
	svc2.now = func() time.Time { return now }
	res, matched, err := svc2.ProcessGuess(context.Background(), &models.Message{MessageID: 1, Chat: models.Chat{ID: -1001}, From: &models.User{ID: 44, Username: "survivor"}, Text: "apple"})
	if err != nil || !matched || res == nil || res.Riddle.State != riddleStateCompleted {
		t.Fatalf("restart should not lose active riddle state: matched=%v err=%v res=%+v", matched, err, res)
	}
}

func cloneAnswers(src []*RiddleAnswer) []*RiddleAnswer {
	out := make([]*RiddleAnswer, 0, len(src))
	for _, item := range src {
		if item == nil {
			continue
		}
		cp := *item
		out = append(out, &cp)
	}
	return out
}

func cloneRiddles(src []*Riddle) []*Riddle {
	out := make([]*Riddle, 0, len(src))
	for _, item := range src {
		if item == nil {
			continue
		}
		out = append(out, cloneRiddle(item))
	}
	return out
}

func cloneRiddle(src *Riddle) *Riddle {
	if src == nil {
		return nil
	}
	cp := *src
	return &cp
}

func ptrInt64(v int64) *int64 {
	return &v
}

func TestRiddleAuditLogsCreatedAndEnded(t *testing.T) {
	repo := &fakeRiddleRepo{}
	tg := &fakeTG{}
	svc := NewRiddleService(repo, &fakeRiddleEconomy{})
	svc.SetAuditLogger(audit.NewLogger(telegram.NewOps(tg), 999), &fakeMemberRepoHandlers{
		members: map[int64]*members.Member{77: {UserID: 77, Username: "riddle_admin"}},
	})

	pub, err := svc.CreatePublishing(context.Background(), 77, &RiddleDraftData{
		PostText:     "riddle",
		RewardAmount: 60,
		Answers:      []RiddleDraftAnswer{{Raw: "Apple", Normalized: "apple"}},
	})
	if err != nil {
		t.Fatalf("create publishing failed: %v", err)
	}
	if err := svc.ActivatePublished(context.Background(), pub.Riddle.ID, -100, 10); err != nil {
		t.Fatalf("activate published failed: %v", err)
	}
	if !hasCallText(tg.calls, "send", "riddle: создана (60, winners=1) by @riddle_admin") {
		t.Fatalf("expected riddle created audit log, calls=%#v", tg.calls)
	}

	if _, err := svc.StopActive(context.Background()); err != nil {
		t.Fatalf("stop active failed: %v", err)
	}
	if !hasCallText(tg.calls, "send", "riddle_end: stopped winners=0 reward=60") {
		t.Fatalf("expected riddle stopped audit log, calls=%#v", tg.calls)
	}
}
