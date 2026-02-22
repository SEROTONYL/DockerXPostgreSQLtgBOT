// Package streak вЂ” service.go СЃРѕРґРµСЂР¶РёС‚ РѕСЃРЅРѕРІРЅСѓСЋ Р±РёР·РЅРµСЃ-Р»РѕРіРёРєСѓ СЃС‚СЂРёРє-СЃРёСЃС‚РµРјС‹.
// РЎРµСЂРІРёСЃ СЃС‡РёС‚Р°РµС‚ СЃРѕРѕР±С‰РµРЅРёСЏ, РЅР°С‡РёСЃР»СЏРµС‚ Р±РѕРЅСѓСЃС‹ Рё СѓРїСЂР°РІР»СЏРµС‚ РµР¶РµРґРЅРµРІРЅС‹Рј СЃР±СЂРѕСЃРѕРј.
package streak

import (
	"context"
	"fmt"

	log "github.com/sirupsen/logrus"

	"serotonyl.ru/telegram-bot/internal/common"
	"serotonyl.ru/telegram-bot/internal/config"
	"serotonyl.ru/telegram-bot/internal/features/economy"
)

// Service СѓРїСЂР°РІР»СЏРµС‚ СЃС‚СЂРёРє-СЃРёСЃС‚РµРјРѕР№.
type Service struct {
	repo           *Repository      // Р РµРїРѕР·РёС‚РѕСЂРёР№ СЃС‚СЂРёРєРѕРІ
	economyService *economy.Service // РЎРµСЂРІРёСЃ СЌРєРѕРЅРѕРјРёРєРё РґР»СЏ РЅР°С‡РёСЃР»РµРЅРёСЏ Р±РѕРЅСѓСЃРѕРІ
	cfg            *config.Config   // РљРѕРЅС„РёРіСѓСЂР°С†РёСЏ
}

// NewService СЃРѕР·РґР°С‘С‚ РЅРѕРІС‹Р№ СЃРµСЂРІРёСЃ СЃС‚СЂРёРєРѕРІ.
func NewService(repo *Repository, economyService *economy.Service, cfg *config.Config) *Service {
	return &Service{
		repo:           repo,
		economyService: economyService,
		cfg:            cfg,
	}
}

// CountMessage РѕР±СЂР°Р±Р°С‚С‹РІР°РµС‚ РІС…РѕРґСЏС‰РµРµ СЃРѕРѕР±С‰РµРЅРёРµ РґР»СЏ РїРѕРґСЃС‡С‘С‚Р° СЃС‚СЂРёРєР°.
// Р’С‹Р·С‹РІР°РµС‚СЃСЏ РґР»СЏ РљРђР–Р”РћР“Рћ СЃРѕРѕР±С‰РµРЅРёСЏ РІ FLOOD_CHAT_ID.
//
// РђР»РіРѕСЂРёС‚Рј:
//  1. РџСЂРѕРІРµСЂСЏРµРј, СЃРѕРґРµСЂР¶РёС‚ Р»Рё СЃРѕРѕР±С‰РµРЅРёРµ 3+ СЃР»РѕРІ
//  2. РџСЂРѕРІРµСЂСЏРµРј, РЅРµ СЏРІР»СЏРµС‚СЃСЏ Р»Рё РєРѕРјР°РЅРґРѕР№
//  3. РџСЂРѕРІРµСЂСЏРµРј, РЅРµ РІС‹РїРѕР»РЅРµРЅР° Р»Рё СѓР¶Рµ РЅРѕСЂРјР°
//  4. РЈРІРµР»РёС‡РёРІР°РµРј СЃС‡С‘С‚С‡РёРє
//  5. Р•СЃР»Рё РґРѕСЃС‚РёРіРЅСѓС‚Р° РЅРѕСЂРјР° (50) вЂ” РЅР°С‡РёСЃР»СЏРµРј Р±РѕРЅСѓСЃ РњРћР›Р§Рђ
func (s *Service) CountMessage(ctx context.Context, userID int64, text string) error {
	// РЁР°Рі 1-2: РџСЂРѕРІРµСЂСЏРµРј, РїРѕРґС…РѕРґРёС‚ Р»Рё СЃРѕРѕР±С‰РµРЅРёРµ
	if !IsValidForStreak(text) {
		return nil // РЎРѕРѕР±С‰РµРЅРёРµ РЅРµ РїРѕРґС…РѕРґРёС‚ вЂ” РёРіРЅРѕСЂРёСЂСѓРµРј
	}

	// РЁР°Рі 3: РџРѕР»СѓС‡Р°РµРј С‚РµРєСѓС‰РёР№ СЃС‚СЂРёРє
	streak, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		// Р•СЃР»Рё СЃС‚СЂРёРє РЅРµ РЅР°Р№РґРµРЅ вЂ” СЃРѕР·РґР°С‘Рј (РїРµСЂРІРѕРµ СЃРѕРѕР±С‰РµРЅРёРµ РїРѕСЃР»Рµ СЂРµРіРёСЃС‚СЂР°С†РёРё)
		if err := s.repo.Create(ctx, userID); err != nil {
			return err
		}
		streak, err = s.repo.GetByUserID(ctx, userID)
		if err != nil {
			return err
		}
	}

	// Р•СЃР»Рё РЅРѕСЂРјР° СѓР¶Рµ РІС‹РїРѕР»РЅРµРЅР° вЂ” РЅРµ СЃС‡РёС‚Р°РµРј РґР°Р»СЊС€Рµ
	if streak.QuotaCompletedToday {
		return nil
	}

	// РЁР°Рі 4: РЈРІРµР»РёС‡РёРІР°РµРј СЃС‡С‘С‚С‡РёРє
	updated, err := s.repo.IncrementMessages(ctx, userID)
	if err != nil {
		return err
	}

	// РЁР°Рі 5: РџСЂРѕРІРµСЂСЏРµРј, РґРѕСЃС‚РёРіРЅСѓС‚Р° Р»Рё РЅРѕСЂРјР°
	if updated.MessagesToday >= s.cfg.StreakMessagesNeed {
		return s.completeQuota(ctx, userID, updated)
	}

	return nil
}

// completeQuota РІС‹РїРѕР»РЅСЏРµС‚ РЅРѕСЂРјСѓ РґРЅСЏ: СѓРІРµР»РёС‡РёРІР°РµС‚ СЃС‚СЂРёРє, РЅР°С‡РёСЃР»СЏРµС‚ Р±РѕРЅСѓСЃ.
// Р‘РѕРЅСѓСЃ РЅР°С‡РёСЃР»СЏРµС‚СЃСЏ РњРћР›Р§Рђ вЂ” РїРѕР»СЊР·РѕРІР°С‚РµР»СЊ РЅРµ РїРѕР»СѓС‡Р°РµС‚ СѓРІРµРґРѕРјР»РµРЅРёРµ.
func (s *Service) completeQuota(ctx context.Context, userID int64, streak *Streak) error {
	// Р Р°СЃСЃС‡РёС‚С‹РІР°РµРј Р±РѕРЅСѓСЃ РЅР° РѕСЃРЅРѕРІРµ С‚РµРєСѓС‰РµРіРѕ СЃС‚СЂРёРєР°
	bonus := CalculateReward(streak.CurrentStreak)
	newStreak := streak.CurrentStreak + 1
	longestStreak := streak.LongestStreak
	if newStreak > longestStreak {
		longestStreak = newStreak
	}
	totalCompleted := streak.TotalQuotasCompleted + 1
	quotaDate := common.GetMoscowDate()

	// РћР±РЅРѕРІР»СЏРµРј СЃС‚СЂРёРє РІ Р‘Р”
	if err := s.repo.CompleteQuota(ctx, userID, newStreak, longestStreak, totalCompleted, quotaDate); err != nil {
		return fmt.Errorf("РѕС€РёР±РєР° Р·Р°РІРµСЂС€РµРЅРёСЏ РЅРѕСЂРјС‹: %w", err)
	}

	// РќР°С‡РёСЃР»СЏРµРј Р±РѕРЅСѓСЃ РњРћР›Р§Рђ (Р±РµР· СѓРІРµРґРѕРјР»РµРЅРёСЏ)
	description := fmt.Sprintf("Streak bonus - Day %d", newStreak)
	if err := s.economyService.AddBalance(ctx, userID, bonus, "streak_bonus", description); err != nil {
		log.WithError(err).WithField("user_id", userID).Error("РћС€РёР±РєР° РЅР°С‡РёСЃР»РµРЅРёСЏ СЃС‚СЂРёРє-Р±РѕРЅСѓСЃР°")
		return err
	}

	log.WithFields(log.Fields{
		"user_id": userID,
		"day":     newStreak,
		"bonus":   bonus,
	}).Debug("РЎС‚СЂРёРє-Р±РѕРЅСѓСЃ РЅР°С‡РёСЃР»РµРЅ (РјРѕР»С‡Р°)")

	return nil
}

// GetStreak РІРѕР·РІСЂР°С‰Р°РµС‚ РёРЅС„РѕСЂРјР°С†РёСЋ Рѕ СЃС‚СЂРёРєРµ РїРѕР»СЊР·РѕРІР°С‚РµР»СЏ.
func (s *Service) GetStreak(ctx context.Context, userID int64) (*Streak, error) {
	return s.repo.GetByUserID(ctx, userID)
}

// CreateStreak СЃРѕР·РґР°С‘С‚ РЅР°С‡Р°Р»СЊРЅСѓСЋ Р·Р°РїРёСЃСЊ СЃС‚СЂРёРєР°.
func (s *Service) CreateStreak(ctx context.Context, userID int64) error {
	return s.repo.Create(ctx, userID)
}

// DailyReset СЃР±СЂР°СЃС‹РІР°РµС‚ РґРЅРµРІРЅС‹Рµ СЃС‡С‘С‚С‡РёРєРё Рё Р»РѕРјР°РµС‚ СЃС‚СЂРёРєРё С‚РµС…, РєС‚Рѕ РЅРµ РІС‹РїРѕР»РЅРёР» РЅРѕСЂРјСѓ.
// Р—Р°РїСѓСЃРєР°РµС‚СЃСЏ РєСЂРѕРЅРѕРј РІ 00:00 РїРѕ РњРѕСЃРєРІРµ.
func (s *Service) DailyReset(ctx context.Context) error {
	log.Info("Р—Р°РїСѓСЃРє РµР¶РµРґРЅРµРІРЅРѕРіРѕ СЃР±СЂРѕСЃР° СЃС‚СЂРёРєРѕРІ")

	// РџРѕР»СѓС‡Р°РµРј РІСЃРµ СЃС‚СЂРёРєРё
	streaks, err := s.repo.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("РѕС€РёР±РєР° РїРѕР»СѓС‡РµРЅРёСЏ СЃС‚СЂРёРєРѕРІ: %w", err)
	}

	brokenCount := 0
	for _, streak := range streaks {
		// Р•СЃР»Рё РЅРѕСЂРјР° РќР• Р±С‹Р»Р° РІС‹РїРѕР»РЅРµРЅР° РІС‡РµСЂР° Рё СЃС‚СЂРёРє > 0 вЂ” Р»РѕРјР°РµРј
		if !streak.QuotaCompletedToday && streak.CurrentStreak > 0 {
			if err := s.repo.BreakStreak(ctx, streak.UserID); err != nil {
				log.WithError(err).WithField("user_id", streak.UserID).Error("РћС€РёР±РєР° СЃР±СЂРѕСЃР° СЃС‚СЂРёРєР°")
			}
			brokenCount++
		}
	}

	// РЎР±СЂР°СЃС‹РІР°РµРј РґРЅРµРІРЅС‹Рµ СЃС‡С‘С‚С‡РёРєРё Сѓ РІСЃРµС…
	if err := s.repo.ResetDaily(ctx); err != nil {
		return fmt.Errorf("РѕС€РёР±РєР° СЃР±СЂРѕСЃР° РґРЅРµРІРЅС‹С… СЃС‡С‘С‚С‡РёРєРѕРІ: %w", err)
	}

	log.WithFields(log.Fields{
		"total":  len(streaks),
		"broken": brokenCount,
	}).Info("Р•Р¶РµРґРЅРµРІРЅС‹Р№ СЃР±СЂРѕСЃ Р·Р°РІРµСЂС€С‘РЅ")

	return nil
}

// SendReminders РѕС‚РїСЂР°РІР»СЏРµС‚ РЅР°РїРѕРјРёРЅР°РЅРёСЏ РїРѕР»СЊР·РѕРІР°С‚РµР»СЏРј СЃ РґР»РёРЅРЅС‹РјРё СЃС‚СЂРёРєР°РјРё.
// Р—Р°РїСѓСЃРєР°РµС‚СЃСЏ РєСЂРѕРЅРѕРј РєР°Р¶РґС‹Р№ С‡Р°СЃ.
func (s *Service) SendReminders(ctx context.Context, sendFunc func(userID int64, text string)) error {
	// РќР°С…РѕРґРёРј РїРѕР»СЊР·РѕРІР°С‚РµР»РµР№ СЃРѕ СЃС‚СЂРёРєРѕРј >= РїРѕСЂРѕРіР° (РїРѕ СѓРјРѕР»С‡Р°РЅРёСЋ 7)
	longStreaks, err := s.repo.GetByMinStreak(ctx, s.cfg.StreakReminderThreshold)
	if err != nil {
		return err
	}

	for _, streak := range longStreaks {
		// РџСЂРѕРїСѓСЃРєР°РµРј РµСЃР»Рё РЅРѕСЂРјР° СѓР¶Рµ РІС‹РїРѕР»РЅРµРЅР° РёР»Рё РЅР°РїРѕРјРёРЅР°РЅРёРµ СѓР¶Рµ РѕС‚РїСЂР°РІР»РµРЅРѕ
		if streak.QuotaCompletedToday || streak.ReminderSentToday {
			continue
		}

		// РџСЂРѕРІРµСЂСЏРµРј РЅРµР°РєС‚РёРІРЅРѕСЃС‚СЊ (10+ С‡Р°СЃРѕРІ Р±РµР· СЃРѕРѕР±С‰РµРЅРёР№)
		if streak.LastMessageAt != nil {
			inactiveHours := common.GetMoscowTime().Sub(*streak.LastMessageAt).Hours()
			if inactiveHours < float64(s.cfg.StreakInactiveHours) {
				continue // Р•С‰С‘ РЅРµ РїСЂРѕС€Р»Рѕ РґРѕСЃС‚Р°С‚РѕС‡РЅРѕ РІСЂРµРјРµРЅРё
			}
		}

		// РћС‚РїСЂР°РІР»СЏРµРј РЅР°РїРѕРјРёРЅР°РЅРёРµ
		msg := fmt.Sprintf("вљ пёЏ РЈ С‚РµР±СЏ РѕРіРѕРЅРµРє %d %s! РќРµ Р·Р°Р±СѓРґСЊ РЅР°РїРёСЃР°С‚СЊ СЃРѕРѕР±С‰РµРЅРёСЏ, С‡С‚РѕР±С‹ РЅРµ РїРѕС‚РµСЂСЏС‚СЊ РїСЂРѕРіСЂРµСЃСЃ!",
			streak.CurrentStreak, common.PluralizeDays(streak.CurrentStreak))
		sendFunc(streak.UserID, msg)

		// РџРѕРјРµС‡Р°РµРј, С‡С‚Рѕ РЅР°РїРѕРјРёРЅР°РЅРёРµ РѕС‚РїСЂР°РІР»РµРЅРѕ
		s.repo.MarkReminderSent(ctx, streak.UserID)
	}

	return nil
}
