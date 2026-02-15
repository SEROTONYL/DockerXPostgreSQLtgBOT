// Package karma — service.go содержит бизнес-логику кармы.
package karma

import (
	"context"

	log "github.com/sirupsen/logrus"

	"telegram-bot/internal/common"
	"telegram-bot/internal/config"
)

// Service управляет системой кармы.
type Service struct {
	repo *Repository
	cfg  *config.Config
}

// NewService создаёт сервис кармы.
func NewService(repo *Repository, cfg *config.Config) *Service {
	return &Service{repo: repo, cfg: cfg}
}

// GiveKarma даёт +1 карму. Проверяет лимиты и ограничения.
func (s *Service) GiveKarma(ctx context.Context, fromUserID, toUserID int64) error {
	if fromUserID == toUserID {
		return common.ErrKarmaSelfGive
	}

	count, err := s.repo.GetTodayCount(ctx, fromUserID)
	if err != nil {
		return err
	}
	if count >= s.cfg.KarmaDailyLimit {
		return common.ErrKarmaDailyLimit
	}

	gave, err := s.repo.GaveToday(ctx, fromUserID, toUserID)
	if err != nil {
		return err
	}
	if gave {
		return common.ErrKarmaAlreadyGave
	}

	if err := s.repo.IncrementKarma(ctx, toUserID); err != nil {
		return err
	}

	if err := s.repo.LogKarma(ctx, fromUserID, toUserID, 1); err != nil {
		log.WithError(err).Error("Ошибка записи лога кармы")
	}

	return nil
}

// GetKarma возвращает карму пользователя.
func (s *Service) GetKarma(ctx context.Context, userID int64) (int, error) {
	k, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		return 0, err
	}
	return k.KarmaPoints, nil
}

// CreateKarma создаёт запись кармы для нового участника.
func (s *Service) CreateKarma(ctx context.Context, userID int64) error {
	return s.repo.Create(ctx, userID)
}
