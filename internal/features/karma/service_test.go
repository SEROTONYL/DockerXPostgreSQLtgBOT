package karma

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"serotonyl.ru/telegram-bot/internal/common"
	"serotonyl.ru/telegram-bot/internal/config"
	"serotonyl.ru/telegram-bot/internal/features/members"
)

type fakeThanksRepo struct {
	sentCount         int
	reciprocalBlocked bool
	stats             *ThanksStats
	logged            struct {
		from   int64
		to     int64
		reward int64
	}
}

func (f *fakeThanksRepo) Create(context.Context, int64) error { return nil }
func (f *fakeThanksRepo) CountSentSince(context.Context, int64, time.Time) (int, error) {
	return f.sentCount, nil
}
func (f *fakeThanksRepo) HasReciprocalSince(context.Context, int64, int64, time.Time) (bool, error) {
	return f.reciprocalBlocked, nil
}
func (f *fakeThanksRepo) LogThanksTx(_ context.Context, _ pgx.Tx, fromUserID, toUserID, rewardAmount int64) error {
	f.logged.from = fromUserID
	f.logged.to = toUserID
	f.logged.reward = rewardAmount
	return nil
}
func (f *fakeThanksRepo) GetStats(context.Context, int64) (*ThanksStats, error) {
	if f.stats == nil {
		return &ThanksStats{}, nil
	}
	return f.stats, nil
}

type fakeRewarder struct {
	called bool
	userID int64
	amount int64
	txType string
	desc   string
}

func (f *fakeRewarder) AddBalanceWithHook(ctx context.Context, userID int64, amount int64, txType, description string, hook func(context.Context, pgx.Tx) error) error {
	f.called = true
	f.userID = userID
	f.amount = amount
	f.txType = txType
	f.desc = description
	if hook != nil {
		var tx pgx.Tx
		return hook(ctx, tx)
	}
	return nil
}

type fakeMemberLookup struct {
	byID map[int64]*members.Member
}

func (f fakeMemberLookup) GetByUserID(ctx context.Context, userID int64) (*members.Member, error) {
	member := f.byID[userID]
	if member == nil {
		return nil, errors.New("not found")
	}
	return member, nil
}

func TestServiceGiveThanksSuccess(t *testing.T) {
	repo := &fakeThanksRepo{}
	rewarder := &fakeRewarder{}
	service := &Service{
		repo:    repo,
		cfg:     &config.Config{ThanksDailyLimit: 3},
		economy: rewarder,
		members: fakeMemberLookup{byID: map[int64]*members.Member{2: {UserID: 2}}},
		now:     func() time.Time { return time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC) },
	}

	if err := service.GiveThanks(context.Background(), 1, 2); err != nil {
		t.Fatalf("GiveThanks() error = %v", err)
	}
	if !rewarder.called {
		t.Fatal("expected rewarder to be called")
	}
	if rewarder.userID != 2 || rewarder.amount != ThanksReward {
		t.Fatalf("unexpected reward call: user=%d amount=%d", rewarder.userID, rewarder.amount)
	}
	if repo.logged.from != 1 || repo.logged.to != 2 || repo.logged.reward != ThanksReward {
		t.Fatalf("unexpected logged thanks: %+v", repo.logged)
	}
}

func TestServiceGiveThanksDailyLimit(t *testing.T) {
	service := &Service{
		repo:    &fakeThanksRepo{sentCount: 3},
		cfg:     &config.Config{ThanksDailyLimit: 3},
		economy: &fakeRewarder{},
		members: fakeMemberLookup{byID: map[int64]*members.Member{2: {UserID: 2}}},
		now:     func() time.Time { return time.Now().UTC() },
	}

	err := service.GiveThanks(context.Background(), 1, 2)
	if !errors.Is(err, common.ErrThanksDailyLimit) {
		t.Fatalf("expected ErrThanksDailyLimit, got %v", err)
	}
}

func TestServiceGiveThanksReciprocalCooldown(t *testing.T) {
	service := &Service{
		repo:    &fakeThanksRepo{reciprocalBlocked: true},
		cfg:     &config.Config{ThanksDailyLimit: 3},
		economy: &fakeRewarder{},
		members: fakeMemberLookup{byID: map[int64]*members.Member{2: {UserID: 2}}},
		now:     func() time.Time { return time.Now().UTC() },
	}

	err := service.GiveThanks(context.Background(), 1, 2)
	if !errors.Is(err, common.ErrThanksReciprocalCooldown) {
		t.Fatalf("expected ErrThanksReciprocalCooldown, got %v", err)
	}
}

func TestServiceGiveThanksRejectsBot(t *testing.T) {
	service := &Service{
		repo:    &fakeThanksRepo{},
		cfg:     &config.Config{ThanksDailyLimit: 3},
		economy: &fakeRewarder{},
		members: fakeMemberLookup{byID: map[int64]*members.Member{2: {UserID: 2, IsBot: true}}},
		now:     func() time.Time { return time.Now().UTC() },
	}

	err := service.GiveThanks(context.Background(), 1, 2)
	if !errors.Is(err, common.ErrThanksTargetIsBot) {
		t.Fatalf("expected ErrThanksTargetIsBot, got %v", err)
	}
}
