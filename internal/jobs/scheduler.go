// Package jobs управляет фоновыми задачами (cron).
// scheduler.go настраивает расписание: ежедневный сброс стриков
// и ежечасные напоминания.
package jobs

import (
	"context"
	"time"

	"github.com/robfig/cron/v3"
	log "github.com/sirupsen/logrus"

	"telegram-bot/internal/features/streak"
)

// Scheduler управляет фоновыми задачами.
type Scheduler struct {
	cron          *cron.Cron
	streakService *streak.Service
	sendFunc      func(userID int64, text string)
}

// NewScheduler создаёт планировщик задач с московским часовым поясом.
func NewScheduler(streakService *streak.Service, sendFunc func(userID int64, text string)) *Scheduler {
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		log.WithError(err).Warn("Не удалось загрузить Europe/Moscow, используем UTC+3")
		loc = time.FixedZone("MSK", 3*60*60)
	}

	c := cron.New(cron.WithLocation(loc))

	return &Scheduler{
		cron:          c,
		streakService: streakService,
		sendFunc:      sendFunc,
	}
}

// Start запускает все фоновые задачи.
func (s *Scheduler) Start(ctx context.Context) {
	// Ежедневный сброс в 00:00 по Москве
	s.cron.AddFunc("0 0 * * *", func() {
		log.Info("[CRON] Ежедневный сброс стриков")
		if err := s.streakService.DailyReset(ctx); err != nil {
			log.WithError(err).Error("[CRON] Ошибка сброса")
		}
	})

	// Напоминания каждый час
	s.cron.AddFunc("0 * * * *", func() {
		log.Debug("[CRON] Проверка напоминаний")
		if err := s.streakService.SendReminders(ctx, s.sendFunc); err != nil {
			log.WithError(err).Error("[CRON] Ошибка напоминаний")
		}
	})

	s.cron.Start()
	log.Info("Планировщик задач запущен (Europe/Moscow)")
}

// Stop останавливает планировщик.
func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
	log.Info("Планировщик задач остановлен")
}
