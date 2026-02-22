// Package jobs СѓРїСЂР°РІР»СЏРµС‚ С„РѕРЅРѕРІС‹РјРё Р·Р°РґР°С‡Р°РјРё (cron).
// scheduler.go РЅР°СЃС‚СЂР°РёРІР°РµС‚ СЂР°СЃРїРёСЃР°РЅРёРµ: РµР¶РµРґРЅРµРІРЅС‹Р№ СЃР±СЂРѕСЃ СЃС‚СЂРёРєРѕРІ
// Рё РµР¶РµС‡Р°СЃРЅС‹Рµ РЅР°РїРѕРјРёРЅР°РЅРёСЏ.
package jobs

import (
	"context"
	"time"

	"github.com/robfig/cron/v3"
	log "github.com/sirupsen/logrus"

	"serotonyl.ru/telegram-bot/internal/features/streak"
)

// Scheduler СѓРїСЂР°РІР»СЏРµС‚ С„РѕРЅРѕРІС‹РјРё Р·Р°РґР°С‡Р°РјРё.
type Scheduler struct {
	cron          *cron.Cron
	streakService *streak.Service
	sendFunc      func(userID int64, text string)
}

// NewScheduler СЃРѕР·РґР°С‘С‚ РїР»Р°РЅРёСЂРѕРІС‰РёРє Р·Р°РґР°С‡ СЃ РјРѕСЃРєРѕРІСЃРєРёРј С‡Р°СЃРѕРІС‹Рј РїРѕСЏСЃРѕРј.
func NewScheduler(streakService *streak.Service, sendFunc func(userID int64, text string)) *Scheduler {
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		log.WithError(err).Warn("РќРµ СѓРґР°Р»РѕСЃСЊ Р·Р°РіСЂСѓР·РёС‚СЊ Europe/Moscow, РёСЃРїРѕР»СЊР·СѓРµРј UTC+3")
		loc = time.FixedZone("MSK", 3*60*60)
	}

	c := cron.New(cron.WithLocation(loc))

	return &Scheduler{
		cron:          c,
		streakService: streakService,
		sendFunc:      sendFunc,
	}
}

// Start Р·Р°РїСѓСЃРєР°РµС‚ РІСЃРµ С„РѕРЅРѕРІС‹Рµ Р·Р°РґР°С‡Рё.
func (s *Scheduler) Start(ctx context.Context) {
	// Р•Р¶РµРґРЅРµРІРЅС‹Р№ СЃР±СЂРѕСЃ РІ 00:00 РїРѕ РњРѕСЃРєРІРµ
	s.cron.AddFunc("0 0 * * *", func() {
		log.Info("[CRON] Р•Р¶РµРґРЅРµРІРЅС‹Р№ СЃР±СЂРѕСЃ СЃС‚СЂРёРєРѕРІ")
		if err := s.streakService.DailyReset(ctx); err != nil {
			log.WithError(err).Error("[CRON] РћС€РёР±РєР° СЃР±СЂРѕСЃР°")
		}
	})

	// РќР°РїРѕРјРёРЅР°РЅРёСЏ РєР°Р¶РґС‹Р№ С‡Р°СЃ
	s.cron.AddFunc("0 * * * *", func() {
		log.Debug("[CRON] РџСЂРѕРІРµСЂРєР° РЅР°РїРѕРјРёРЅР°РЅРёР№")
		if err := s.streakService.SendReminders(ctx, s.sendFunc); err != nil {
			log.WithError(err).Error("[CRON] РћС€РёР±РєР° РЅР°РїРѕРјРёРЅР°РЅРёР№")
		}
	})

	s.cron.Start()
	log.Info("РџР»Р°РЅРёСЂРѕРІС‰РёРє Р·Р°РґР°С‡ Р·Р°РїСѓС‰РµРЅ (Europe/Moscow)")
}

// Stop РѕСЃС‚Р°РЅР°РІР»РёРІР°РµС‚ РїР»Р°РЅРёСЂРѕРІС‰РёРє.
func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
	log.Info("РџР»Р°РЅРёСЂРѕРІС‰РёРє Р·Р°РґР°С‡ РѕСЃС‚Р°РЅРѕРІР»РµРЅ")
}
