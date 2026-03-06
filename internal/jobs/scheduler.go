// Package jobs manages background cron tasks.
package jobs

import (
	"context"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	log "github.com/sirupsen/logrus"

	"serotonyl.ru/telegram-bot/internal/config"
	"serotonyl.ru/telegram-bot/internal/features/members"
	"serotonyl.ru/telegram-bot/internal/features/streak"
	"serotonyl.ru/telegram-bot/internal/telegram"
)

const (
	cronWarnLoadLocation = "Failed to load Europe/Moscow, using UTC+3"
	cronInfoDailyReset   = "[CRON] Daily streak reset"
	cronErrorDailyReset  = "[CRON] Daily reset failed"
	cronDebugReminders   = "[CRON] Checking reminders"
	cronErrorReminders   = "[CRON] Reminder run failed"
	cronInfoStarted      = "Scheduler started (Europe/Moscow)"
	cronInfoStopped      = "Scheduler stopped"

	purgeTickInterval    = time.Hour
	purgeBatchLimit      = 500
	purgeMaxIterations   = 10
	scanMemberTagsPeriod = 4 * time.Hour
)

type memberPurger interface {
	PurgeExpiredLeftMembers(ctx context.Context, now time.Time, limit int) (int, error)
	ScanAndUpdateMemberTags(ctx context.Context, tgOps *telegram.Ops, mainGroupID int64, now time.Time) (int, error)
}

type PurgeMetrics struct {
	TotalDeleted   int64
	LastRunAt      time.Time
	LastRunDeleted int
	LastError      string
}

type purgeMetricsStore struct {
	mu sync.RWMutex
	v  PurgeMetrics
}

func (s *purgeMetricsStore) snapshot() PurgeMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.v
}

func (s *purgeMetricsStore) markRun(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.v.LastRunAt = now
	s.v.LastRunDeleted = 0
	s.v.LastError = ""
}

func (s *purgeMetricsStore) markResult(deleted int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.v.LastRunDeleted = deleted
	s.v.TotalDeleted += int64(deleted)
}

func (s *purgeMetricsStore) markError(err error) {
	if err == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.v.LastError = err.Error()
}

// Scheduler manages background tasks.
type Scheduler struct {
	cron          *cron.Cron
	streakService *streak.Service
	memberService memberPurger
	sendFunc      func(userID int64, text string)
	tgOps         *telegram.Ops
	mainGroupID   int64

	purgeCancel context.CancelFunc
	purgeWG     sync.WaitGroup
	purgeState  purgeMetricsStore
}

// NewScheduler creates a scheduler configured for Europe/Moscow.
func NewScheduler(cfg *config.Config, streakService *streak.Service, memberService *members.Service, sendFunc func(userID int64, text string), tgOps *telegram.Ops) *Scheduler {
	loc, err := time.LoadLocation("Europe/Moscow")
	if err != nil {
		log.WithError(err).Warn(cronWarnLoadLocation)
		loc = time.FixedZone("MSK", 3*60*60)
	}

	c := cron.New(cron.WithLocation(loc))

	var mainGroupID int64
	if cfg != nil {
		mainGroupID = cfg.MainGroupID
	}

	return &Scheduler{
		cron:          c,
		streakService: streakService,
		memberService: memberService,
		sendFunc:      sendFunc,
		tgOps:         tgOps,
		mainGroupID:   mainGroupID,
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

	purgeCtx, cancel := context.WithCancel(ctx)
	s.purgeCancel = cancel
	s.purgeWG.Add(1)
	go func() {
		defer s.purgeWG.Done()
		s.runPurgeWorker(purgeCtx)
	}()

	s.purgeWG.Add(1)
	go func() {
		defer s.purgeWG.Done()
		s.runMemberTagScanWorker(purgeCtx)
	}()
}

func (s *Scheduler) runPurgeWorker(ctx context.Context) {
	ticker := time.NewTicker(purgeTickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runPurgeTick(ctx, time.Now().UTC())
		}
	}
}

func (s *Scheduler) runPurgeTick(ctx context.Context, now time.Time) {
	s.purgeState.markRun(now)
	totalDeleted := 0
	var runErr error
	for i := 0; i < purgeMaxIterations; i++ {
		deleted, err := s.memberService.PurgeExpiredLeftMembers(ctx, now, purgeBatchLimit)
		if err != nil {
			runErr = err
			log.WithError(err).WithField("iteration", i+1).Error("purge expired members failed")
			break
		}
		totalDeleted += deleted
		if deleted == 0 {
			break
		}
	}

	s.purgeState.markResult(totalDeleted)
	if runErr != nil {
		s.purgeState.markError(runErr)
	}
	log.WithField("deleted", totalDeleted).Info("purge expired members: deleted=N")
}

func (s *Scheduler) runMemberTagScanWorker(ctx context.Context) {
	ticker := time.NewTicker(scanMemberTagsPeriod)
	defer ticker.Stop()

	s.scanMemberTags(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.scanMemberTags(ctx)
		}
	}
}

func (s *Scheduler) scanMemberTags(ctx context.Context) {
	if s.tgOps == nil || s.mainGroupID == 0 {
		return
	}

	log.Debug("[CRON] ScanMemberTags started")
	updated, err := s.memberService.ScanAndUpdateMemberTags(ctx, s.tgOps, s.mainGroupID, time.Now().UTC())
	if err != nil {
		log.WithError(err).Warn("[CRON] ScanMemberTags failed")
		return
	}
	log.WithField("updated", updated).Info("[CRON] ScanMemberTags completed")
}

func (s *Scheduler) GetPurgeMetrics() PurgeMetrics {
	return s.purgeState.snapshot()
}

// Stop gracefully stops scheduler.
func (s *Scheduler) Stop() {
	if s.purgeCancel != nil {
		s.purgeCancel()
	}
	s.purgeWG.Wait()

	ctx := s.cron.Stop()
	<-ctx.Done()
	log.Info(cronInfoStopped)
}
