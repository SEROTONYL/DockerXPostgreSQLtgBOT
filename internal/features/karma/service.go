package karma

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"serotonyl.ru/telegram-bot/internal/common"
	"serotonyl.ru/telegram-bot/internal/config"
	"serotonyl.ru/telegram-bot/internal/features/economy"
	"serotonyl.ru/telegram-bot/internal/features/members"
)

type memberLookup interface {
	GetByUserID(ctx context.Context, userID int64) (*members.Member, error)
}

type thanksRepository interface {
	Create(ctx context.Context, userID int64) error
	CountSentSince(ctx context.Context, fromUserID int64, since time.Time) (int, error)
	HasReciprocalSince(ctx context.Context, fromUserID, toUserID int64, since time.Time) (bool, error)
	LogThanksTx(ctx context.Context, tx pgx.Tx, fromUserID, toUserID, rewardAmount int64) error
	GetStats(ctx context.Context, userID int64) (*ThanksStats, error)
}

type balanceRewarder interface {
	AddBalanceWithHook(ctx context.Context, userID int64, amount int64, txType, description string, hook func(context.Context, pgx.Tx) error) error
}

type Service struct {
	repo    thanksRepository
	cfg     *config.Config
	economy balanceRewarder
	members memberLookup
	now     func() time.Time
}

func NewService(repo *Repository, economyService *economy.Service, membersService *members.Service, cfg *config.Config) *Service {
	return &Service{
		repo:    repo,
		cfg:     cfg,
		economy: economyService,
		members: membersService,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

func (s *Service) GiveThanks(ctx context.Context, fromUserID, toUserID int64) error {
	if fromUserID == toUserID {
		return common.ErrThanksSelfGive
	}

	target, err := s.members.GetByUserID(ctx, toUserID)
	if err != nil || target == nil {
		return common.ErrUserNotFound
	}
	if target.IsBot {
		return common.ErrThanksTargetIsBot
	}

	sentToday, err := s.repo.CountSentSince(ctx, fromUserID, common.GetMoscowDate())
	if err != nil {
		return err
	}
	if sentToday >= s.dailyLimit() {
		return common.ErrThanksDailyLimit
	}

	reciprocalBlocked, err := s.repo.HasReciprocalSince(ctx, toUserID, fromUserID, s.now().Add(-ThanksReciprocalCooldown))
	if err != nil {
		return err
	}
	if reciprocalBlocked {
		return common.ErrThanksReciprocalCooldown
	}

	description := fmt.Sprintf("Спасибо от %d", fromUserID)
	return s.economy.AddBalanceWithHook(ctx, toUserID, ThanksReward, thanksRewardTxType, description, func(ctx context.Context, tx pgx.Tx) error {
		return s.repo.LogThanksTx(ctx, tx, fromUserID, toUserID, ThanksReward)
	})
}

func (s *Service) GetThanksStats(ctx context.Context, userID int64) (*ThanksStats, error) {
	return s.repo.GetStats(ctx, userID)
}

func (s *Service) CreateKarma(ctx context.Context, userID int64) error {
	return s.repo.Create(ctx, userID)
}

func (s *Service) dailyLimit() int {
	if s.cfg != nil && s.cfg.ThanksDailyLimit > 0 {
		return s.cfg.ThanksDailyLimit
	}
	return DefaultThanksDailyLimit
}
