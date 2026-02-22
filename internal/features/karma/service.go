// Package karma вЂ” service.go СЃРѕРґРµСЂР¶РёС‚ Р±РёР·РЅРµСЃ-Р»РѕРіРёРєСѓ РєР°СЂРјС‹.
package karma

import (
	"context"

	log "github.com/sirupsen/logrus"

	"serotonyl.ru/telegram-bot/internal/common"
	"serotonyl.ru/telegram-bot/internal/config"
)

// Service СѓРїСЂР°РІР»СЏРµС‚ СЃРёСЃС‚РµРјРѕР№ РєР°СЂРјС‹.
type Service struct {
	repo *Repository
	cfg  *config.Config
}

// NewService СЃРѕР·РґР°С‘С‚ СЃРµСЂРІРёСЃ РєР°СЂРјС‹.
func NewService(repo *Repository, cfg *config.Config) *Service {
	return &Service{repo: repo, cfg: cfg}
}

// GiveKarma РґР°С‘С‚ +1 РєР°СЂРјСѓ. РџСЂРѕРІРµСЂСЏРµС‚ Р»РёРјРёС‚С‹ Рё РѕРіСЂР°РЅРёС‡РµРЅРёСЏ.
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
		log.WithError(err).Error("РћС€РёР±РєР° Р·Р°РїРёСЃРё Р»РѕРіР° РєР°СЂРјС‹")
	}

	return nil
}

// GetKarma РІРѕР·РІСЂР°С‰Р°РµС‚ РєР°СЂРјСѓ РїРѕР»СЊР·РѕРІР°С‚РµР»СЏ.
func (s *Service) GetKarma(ctx context.Context, userID int64) (int, error) {
	k, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		return 0, err
	}
	return k.KarmaPoints, nil
}

// CreateKarma СЃРѕР·РґР°С‘С‚ Р·Р°РїРёСЃСЊ РєР°СЂРјС‹ РґР»СЏ РЅРѕРІРѕРіРѕ СѓС‡Р°СЃС‚РЅРёРєР°.
func (s *Service) CreateKarma(ctx context.Context, userID int64) error {
	return s.repo.Create(ctx, userID)
}
