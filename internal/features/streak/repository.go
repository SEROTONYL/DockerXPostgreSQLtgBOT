// Package streak — repository.go выполняет операции с таблицей streaks.
package streak

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository предоставляет методы для работы с таблицей streaks.
type Repository struct {
	db *pgxpool.Pool
}

// NewRepository создаёт новый репозиторий стриков.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// Create создаёт начальную запись стрика для нового участника.
func (r *Repository) Create(ctx context.Context, userID int64) error {
	query := `
		INSERT INTO streaks (user_id, current_streak, longest_streak, messages_today,
		                     quota_completed_today, total_quotas_completed, reminder_sent_today)
		VALUES ($1, 0, 0, 0, FALSE, 0, FALSE)
		ON CONFLICT (user_id) DO NOTHING
	`
	_, err := r.db.Exec(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("ошибка создания стрика: %w", err)
	}
	return nil
}

// GetByUserID возвращает стрик пользователя.
func (r *Repository) GetByUserID(ctx context.Context, userID int64) (*Streak, error) {
	query := `
		SELECT id, user_id, current_streak, longest_streak, messages_today,
		       quota_completed_today, last_quota_completion, last_message_at,
		       total_quotas_completed, reminder_sent_today, created_at, updated_at
		FROM streaks
		WHERE user_id = $1
	`
	var s Streak
	err := r.db.QueryRow(ctx, query, userID).Scan(
		&s.ID, &s.UserID, &s.CurrentStreak, &s.LongestStreak,
		&s.MessagesToday, &s.QuotaCompletedToday, &s.LastQuotaCompletion,
		&s.LastMessageAt, &s.TotalQuotasCompleted, &s.ReminderSentToday,
		&s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("стрик не найден (user_id=%d): %w", userID, err)
	}
	return &s, nil
}

// Update обновляет запись стрика в БД.
func (r *Repository) Update(ctx context.Context, s *Streak) error {
	query := `
		UPDATE streaks
		SET current_streak = $2, longest_streak = $3, messages_today = $4,
		    quota_completed_today = $5, last_quota_completion = $6,
		    last_message_at = $7, total_quotas_completed = $8,
		    reminder_sent_today = $9, updated_at = NOW()
		WHERE user_id = $1
	`
	_, err := r.db.Exec(ctx, query,
		s.UserID, s.CurrentStreak, s.LongestStreak, s.MessagesToday,
		s.QuotaCompletedToday, s.LastQuotaCompletion, s.LastMessageAt,
		s.TotalQuotasCompleted, s.ReminderSentToday,
	)
	if err != nil {
		return fmt.Errorf("ошибка обновления стрика: %w", err)
	}
	return nil
}

// IncrementMessages увеличивает счётчик сообщений на 1 и обновляет last_message_at.
func (r *Repository) IncrementMessages(ctx context.Context, userID int64) (*Streak, error) {
	now := time.Now()
	query := `
		UPDATE streaks
		SET messages_today = messages_today + 1, last_message_at = $2, updated_at = NOW()
		WHERE user_id = $1
		RETURNING id, user_id, current_streak, longest_streak, messages_today,
		          quota_completed_today, last_quota_completion, last_message_at,
		          total_quotas_completed, reminder_sent_today, created_at, updated_at
	`
	var s Streak
	err := r.db.QueryRow(ctx, query, userID, now).Scan(
		&s.ID, &s.UserID, &s.CurrentStreak, &s.LongestStreak,
		&s.MessagesToday, &s.QuotaCompletedToday, &s.LastQuotaCompletion,
		&s.LastMessageAt, &s.TotalQuotasCompleted, &s.ReminderSentToday,
		&s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("ошибка обновления счётчика: %w", err)
	}
	return &s, nil
}

// CompleteQuota отмечает выполнение дневной нормы.
func (r *Repository) CompleteQuota(ctx context.Context, userID int64, newStreak, longestStreak, totalCompleted int, quotaDate time.Time) error {
	query := `
		UPDATE streaks
		SET quota_completed_today = TRUE,
		    current_streak = $2,
		    longest_streak = $3,
		    total_quotas_completed = $4,
		    last_quota_completion = $5,
		    updated_at = NOW()
		WHERE user_id = $1
	`
	_, err := r.db.Exec(ctx, query, userID, newStreak, longestStreak, totalCompleted, quotaDate)
	if err != nil {
		return fmt.Errorf("ошибка завершения нормы: %w", err)
	}
	return nil
}

// GetAll возвращает все стрики. Используется для ежедневного сброса.
func (r *Repository) GetAll(ctx context.Context) ([]*Streak, error) {
	query := `
		SELECT id, user_id, current_streak, longest_streak, messages_today,
		       quota_completed_today, last_quota_completion, last_message_at,
		       total_quotas_completed, reminder_sent_today, created_at, updated_at
		FROM streaks
	`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения стриков: %w", err)
	}
	defer rows.Close()

	var streaks []*Streak
	for rows.Next() {
		var s Streak
		err := rows.Scan(
			&s.ID, &s.UserID, &s.CurrentStreak, &s.LongestStreak,
			&s.MessagesToday, &s.QuotaCompletedToday, &s.LastQuotaCompletion,
			&s.LastMessageAt, &s.TotalQuotasCompleted, &s.ReminderSentToday,
			&s.CreatedAt, &s.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("ошибка сканирования: %w", err)
		}
		streaks = append(streaks, &s)
	}
	return streaks, nil
}

// GetByMinStreak возвращает стрики с серией >= minStreak.
// Используется для напоминаний (стрик >= 7 дней).
func (r *Repository) GetByMinStreak(ctx context.Context, minStreak int) ([]*Streak, error) {
	query := `
		SELECT id, user_id, current_streak, longest_streak, messages_today,
		       quota_completed_today, last_quota_completion, last_message_at,
		       total_quotas_completed, reminder_sent_today, created_at, updated_at
		FROM streaks
		WHERE current_streak >= $1
	`
	rows, err := r.db.Query(ctx, query, minStreak)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var streaks []*Streak
	for rows.Next() {
		var s Streak
		err := rows.Scan(
			&s.ID, &s.UserID, &s.CurrentStreak, &s.LongestStreak,
			&s.MessagesToday, &s.QuotaCompletedToday, &s.LastQuotaCompletion,
			&s.LastMessageAt, &s.TotalQuotasCompleted, &s.ReminderSentToday,
			&s.CreatedAt, &s.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		streaks = append(streaks, &s)
	}
	return streaks, nil
}

// ResetDaily сбрасывает дневные счётчики для всех пользователей.
// Вызывается кроном в 00:00 по Москве.
func (r *Repository) ResetDaily(ctx context.Context) error {
	query := `
		UPDATE streaks
		SET messages_today = 0, quota_completed_today = FALSE,
		    reminder_sent_today = FALSE, updated_at = NOW()
	`
	_, err := r.db.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("ошибка сброса дневных счётчиков: %w", err)
	}
	return nil
}

// BreakStreak обнуляет стрик пользователя (не выполнил норму).
func (r *Repository) BreakStreak(ctx context.Context, userID int64) error {
	query := `UPDATE streaks SET current_streak = 0, updated_at = NOW() WHERE user_id = $1`
	_, err := r.db.Exec(ctx, query, userID)
	return err
}

// MarkReminderSent помечает, что напоминание уже отправлено сегодня.
func (r *Repository) MarkReminderSent(ctx context.Context, userID int64) error {
	query := `UPDATE streaks SET reminder_sent_today = TRUE WHERE user_id = $1`
	_, err := r.db.Exec(ctx, query, userID)
	return err
}
