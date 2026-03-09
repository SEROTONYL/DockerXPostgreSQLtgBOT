package streak

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"serotonyl.ru/telegram-bot/internal/common"
	"serotonyl.ru/telegram-bot/internal/config"
	"serotonyl.ru/telegram-bot/internal/features/economy"
)

const streakRewardTxType = "streak_bonus"

type antiSpamState struct {
	LastNormalized string
	LastAt         time.Time
	CountedAt      []time.Time
}

type antiSpamDecision struct {
	normalized string
	now        time.Time
}

type streakRepository interface {
	Create(ctx context.Context, userID int64) error
	CreateTx(ctx context.Context, tx pgx.Tx, userID int64) error
	GetByUserID(ctx context.Context, userID int64) (*Streak, error)
	GetByUserIDForUpdateTx(ctx context.Context, tx pgx.Tx, userID int64) (*Streak, error)
	UpdateTx(ctx context.Context, tx pgx.Tx, s *Streak) error
	MarkProcessedMessageTx(ctx context.Context, tx pgx.Tx, userID, messageID int64, streakDay time.Time) error
	MarkReminderSentIfNotSentTodayTx(ctx context.Context, tx pgx.Tx, userID int64, progressDay time.Time) (bool, error)
	ClearReminderSentTx(ctx context.Context, tx pgx.Tx, userID int64, progressDay time.Time) error
	GetTop(ctx context.Context, limit int) ([]TopEntry, error)
	GetByMinStreak(ctx context.Context, minStreak int) ([]*Streak, error)
	ResetDaily(ctx context.Context, day time.Time) error
}

type rewardEconomy interface {
	WithTransaction(ctx context.Context, fn func(context.Context, pgx.Tx) error) error
	AddBalanceTx(ctx context.Context, tx pgx.Tx, userID int64, amount int64, txType, description string) error
}

type Service struct {
	repo           streakRepository
	economyService rewardEconomy
	cfg            *config.Config
	location       *time.Location
	now            func() time.Time

	mu       sync.Mutex
	antiSpam map[int64]*antiSpamState
}

func NewService(repo *Repository, economyService *economy.Service, cfg *config.Config) *Service {
	loc := time.UTC
	if cfg != nil && strings.TrimSpace(cfg.AppTimezone) != "" {
		if loaded, err := time.LoadLocation(cfg.AppTimezone); err == nil {
			loc = loaded
		}
	}
	return &Service{
		repo:           repo,
		economyService: economyService,
		cfg:            cfg,
		location:       loc,
		now:            time.Now,
		antiSpam:       make(map[int64]*antiSpamState),
	}
}

func (s *Service) CountMessage(ctx context.Context, userID, messageID int64, text string) error {
	if !IsValidForStreak(text) {
		return nil
	}

	now := s.now().In(s.location)
	decision, allowed := s.checkAntiSpam(userID, text, now)
	if !allowed {
		return nil
	}

	err := s.economyService.WithTransaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.repo.CreateTx(ctx, tx, userID); err != nil {
			return err
		}
		// TODO: Telegram message_id is chat-scoped. chat_id exists earlier in the update flow,
		// but CountMessage does not receive it yet, so dedupe remains user_id+message_id here.
		if err := s.repo.MarkProcessedMessageTx(ctx, tx, userID, messageID, s.dayStart(now)); err != nil {
			if errors.Is(err, errProcessedMessageDuplicate) {
				return nil
			}
			return err
		}

		st, err := s.repo.GetByUserIDForUpdateTx(ctx, tx, userID)
		if err != nil {
			return err
		}
		s.normalizeStateForDay(st, now)

		lastMessageAt := now.UTC()
		st.LastMessageAt = &lastMessageAt
		if st.QuotaCompletedToday {
			return s.repo.UpdateTx(ctx, tx, st)
		}

		if st.MessagesToday < dailyMessageTarget {
			st.MessagesToday++
		}
		if st.MessagesToday < dailyMessageTarget {
			return s.repo.UpdateTx(ctx, tx, st)
		}

		if st.QuotaCompletedToday || sameDayPtr(st.LastRewardedDay, s.dayStart(now)) {
			return s.repo.UpdateTx(ctx, tx, st)
		}

		st.QuotaCompletedToday = true
		completedDay := s.dayStart(now)
		st.LastQuotaCompletion = &completedDay
		st.LastRewardedDay = &completedDay
		st.CurrentStreak++
		if st.CurrentStreak > st.LongestStreak {
			st.LongestStreak = st.CurrentStreak
		}
		st.TotalQuotasCompleted++

		if err := s.repo.UpdateTx(ctx, tx, st); err != nil {
			return err
		}

		reward := CalculateReward(st.CurrentStreak - 1)
		description := FormatRewardDescription(st.CurrentStreak)
		return s.economyService.AddBalanceTx(ctx, tx, userID, reward, streakRewardTxType, description)
	})
	if err != nil {
		return err
	}

	s.commitAntiSpam(userID, decision)
	return nil
}

func (s *Service) GetStreak(ctx context.Context, userID int64) (*Streak, error) {
	if err := s.repo.Create(ctx, userID); err != nil {
		return nil, err
	}

	var out *Streak
	err := s.economyService.WithTransaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		st, err := s.repo.GetByUserIDForUpdateTx(ctx, tx, userID)
		if err != nil {
			return err
		}
		if s.normalizeStateForDay(st, s.now().In(s.location)) {
			if err := s.repo.UpdateTx(ctx, tx, st); err != nil {
				return err
			}
		}
		out = st
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Service) GetTop(ctx context.Context, limit int) ([]TopEntry, error) {
	return s.repo.GetTop(ctx, limit)
}

func (s *Service) CreateStreak(ctx context.Context, userID int64) error {
	return s.repo.Create(ctx, userID)
}

func (s *Service) DailyReset(ctx context.Context) error {
	return s.repo.ResetDaily(ctx, s.dayStart(s.now().In(s.location)))
}

func (s *Service) SendReminders(ctx context.Context, sendFunc func(context.Context, int64, string) error) error {
	longStreaks, err := s.repo.GetByMinStreak(ctx, s.cfg.StreakReminderThreshold)
	if err != nil {
		return err
	}

	now := s.now().In(s.location)
	progressDay := s.dayStart(now)
	for _, candidate := range longStreaks {
		var reminder *Streak
		err := s.economyService.WithTransaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
			st, err := s.repo.GetByUserIDForUpdateTx(ctx, tx, candidate.UserID)
			if err != nil {
				return err
			}

			if s.normalizeStateForDay(st, now) {
				if err := s.repo.UpdateTx(ctx, tx, st); err != nil {
					return err
				}
			}
			if st.CurrentStreak <= 0 || st.QuotaCompletedToday || st.ReminderSentToday {
				return nil
			}
			if st.LastMessageAt != nil && now.Sub(st.LastMessageAt.In(s.location)).Hours() < float64(s.cfg.StreakInactiveHours) {
				return nil
			}

			claimed, err := s.repo.MarkReminderSentIfNotSentTodayTx(ctx, tx, st.UserID, progressDay)
			if err != nil {
				return err
			}
			if !claimed {
				return nil
			}

			cp := *st
			reminder = &cp
			return nil
		})
		if err != nil {
			return err
		}
		if reminder == nil {
			continue
		}

		text := fmt.Sprintf("🔥 У тебя огонек %d %s! Не забудь добить 4/4.", reminder.CurrentStreak, common.PluralizeDays(reminder.CurrentStreak))
		if err := sendFunc(ctx, reminder.UserID, text); err != nil {
			// Keep the claim-first behavior for cross-instance duplicate suppression,
			// but release the claim again when Telegram delivery fails so the reminder is retried instead of lost.
			releaseErr := s.economyService.WithTransaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
				return s.repo.ClearReminderSentTx(ctx, tx, reminder.UserID, progressDay)
			})
			if releaseErr != nil {
				return fmt.Errorf("send reminder user_id=%d: %w; release claim: %v", reminder.UserID, err, releaseErr)
			}
			return fmt.Errorf("send reminder user_id=%d: %w", reminder.UserID, err)
		}
	}

	return nil
}

func (s *Service) normalizeStateForDay(st *Streak, now time.Time) bool {
	changed := false
	today := s.dayStart(now)

	if st.ProgressDate == nil || !sameDay(*st.ProgressDate, today) {
		st.ProgressDate = &today
		st.MessagesToday = 0
		st.QuotaCompletedToday = false
		st.ReminderSentToday = false
		changed = true
	}
	if s.isContinuityBroken(st, today) {
		if st.CurrentStreak != 0 {
			st.CurrentStreak = 0
			changed = true
		}
		if st.MessagesToday != 0 {
			st.MessagesToday = 0
			changed = true
		}
		if st.QuotaCompletedToday {
			st.QuotaCompletedToday = false
			changed = true
		}
		if st.ProgressDate == nil || !sameDay(*st.ProgressDate, today) {
			st.ProgressDate = &today
			changed = true
		}
	}

	return changed
}

func (s *Service) isContinuityBroken(st *Streak, today time.Time) bool {
	if st.CurrentStreak == 0 {
		return false
	}
	if st.LastQuotaCompletion == nil {
		return true
	}

	last := s.dayStart(st.LastQuotaCompletion.In(s.location))
	if sameDay(last, today) {
		return false
	}
	yesterday := today.AddDate(0, 0, -1)
	return !sameDay(last, yesterday)
}

func (s *Service) checkAntiSpam(userID int64, text string, now time.Time) (antiSpamDecision, bool) {
	normalized := normalizeMessageText(text)

	s.mu.Lock()
	defer s.mu.Unlock()

	state := s.antiSpam[userID]
	if state == nil {
		state = &antiSpamState{}
		s.antiSpam[userID] = state
	}

	if state.LastNormalized == normalized && !state.LastAt.IsZero() && now.Sub(state.LastAt) <= duplicateWindow {
		return antiSpamDecision{}, false
	}

	kept := make([]time.Time, 0, len(state.CountedAt))
	for _, ts := range state.CountedAt {
		if now.Sub(ts) < time.Minute {
			kept = append(kept, ts)
		}
	}
	if len(kept) >= maxValidPerMinute {
		return antiSpamDecision{}, false
	}

	return antiSpamDecision{normalized: normalized, now: now}, true
}

func (s *Service) commitAntiSpam(userID int64, decision antiSpamDecision) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state := s.antiSpam[userID]
	if state == nil {
		state = &antiSpamState{}
		s.antiSpam[userID] = state
	}

	kept := state.CountedAt[:0]
	for _, ts := range state.CountedAt {
		if decision.now.Sub(ts) < time.Minute {
			kept = append(kept, ts)
		}
	}
	state.CountedAt = kept
	state.LastNormalized = decision.normalized
	state.LastAt = decision.now
	state.CountedAt = append(state.CountedAt, decision.now)
}

func (s *Service) dayStart(t time.Time) time.Time {
	tt := t.In(s.location)
	return time.Date(tt.Year(), tt.Month(), tt.Day(), 0, 0, 0, 0, s.location)
}

func sameDay(a, b time.Time) bool {
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}

func sameDayPtr(a *time.Time, b time.Time) bool {
	return a != nil && sameDay(*a, b)
}

func CalculateReward(currentStreak int) int64 {
	return GetReward(currentStreak)
}

func FormatRewardDescription(day int) string {
	return fmt.Sprintf("Ogonek reward - day %d", day)
}
