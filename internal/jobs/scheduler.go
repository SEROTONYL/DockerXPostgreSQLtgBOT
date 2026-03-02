// Package jobs manages background cron tasks.
package jobs

import (
	"context"
	"time"

	"github.com/robfig/cron/v3"
	log "github.com/sirupsen/logrus"

	"serotonyl.ru/telegram-bot/internal/features/streak"
)

const (
	cronWarnLoadLocation = "Failed to load Europe/Moscow, using UTC+3"
	cronInfoDailyReset   = "[CRON] Daily streak reset"
	cronErrorDailyReset  = "[CRON] Daily reset failed"
	cronDebugReminders   = "[CRON] Checking reminders"
	cronErrorReminders   = "[CRON] Reminder run failed"
	cronInfoStarted      = "Scheduler started (Europe/Moscow)"
	cronInfoStopped      = "Scheduler stopped"
)

// Scheduler manages background tasks.
type Scheduler struct {
	cron          *cron.Cron
	streakService *streak.Service
	sendFunc      func(userID int64, text string)
}

// NewScheduler creates a scheduler configured for Europe/Moscow.
func NewScheduler(streakService *streak.Service, sendFunc func(userID int64, text string)) *Scheduler {
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		log.WithError(err).Warn(cronWarnLoadLocation)
		loc = time.FixedZone("MSK", 3*60*60)
	}

	c := cron.New(cron.WithLocation(loc))

	return &Scheduler{
		cron:          c,
		streakService: streakService,
		sendFunc:      sendFunc,
	}
}

// Start launches background tasks.
func (s *Scheduler) Start(ctx context.Context) {
	s.cron.AddFunc("0 0 * * *", func() {
		log.Info(cronInfoDailyReset)
		if err := s.streakService.DailyReset(ctx); err != nil {
			log.WithError(err).Error(cronErrorDailyReset)
		}
	})

	s.cron.AddFunc("0 * * * *", func() {
		log.Debug(cronDebugReminders)
		if err := s.streakService.SendReminders(ctx, s.sendFunc); err != nil {
			log.WithError(err).Error(cronErrorReminders)
		}
	})

	s.cron.Start()
	log.Info(cronInfoStarted)
}

// Stop gracefully stops scheduler.
func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
	log.Info(cronInfoStopped)
}
