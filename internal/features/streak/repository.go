package streak

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var errProcessedMessageDuplicate = errors.New("streak processed message duplicate")

type streakDBTX interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, userID int64) error {
	return r.create(ctx, r.db, userID)
}

func (r *Repository) CreateTx(ctx context.Context, tx pgx.Tx, userID int64) error {
	return r.create(ctx, tx, userID)
}

func (r *Repository) create(ctx context.Context, db streakDBTX, userID int64) error {
	_, err := db.Exec(ctx, `
		INSERT INTO streaks (
			user_id,
			current_streak,
			longest_streak,
			messages_today,
			quota_completed_today,
			total_quotas_completed,
			reminder_sent_today
		)
		VALUES ($1, 0, 0, 0, FALSE, 0, FALSE)
		ON CONFLICT (user_id) DO NOTHING
	`, userID)
	if err != nil {
		return fmt.Errorf("create streak: %w", err)
	}
	return nil
}

func (r *Repository) GetByUserID(ctx context.Context, userID int64) (*Streak, error) {
	return r.getByUserID(ctx, r.db, userID, false)
}

func (r *Repository) GetByUserIDForUpdateTx(ctx context.Context, tx pgx.Tx, userID int64) (*Streak, error) {
	return r.getByUserID(ctx, tx, userID, true)
}

func (r *Repository) getByUserID(ctx context.Context, db streakDBTX, userID int64, forUpdate bool) (*Streak, error) {
	query := `
		SELECT id, user_id, current_streak, longest_streak, messages_today,
		       quota_completed_today, last_quota_completion, progress_date,
		       last_rewarded_day, last_message_at, total_quotas_completed,
		       reminder_sent_today, created_at, updated_at
		FROM streaks
		WHERE user_id = $1
	`
	if forUpdate {
		query += ` FOR UPDATE`
	}

	var s Streak
	err := db.QueryRow(ctx, query, userID).Scan(
		&s.ID, &s.UserID, &s.CurrentStreak, &s.LongestStreak,
		&s.MessagesToday, &s.QuotaCompletedToday, &s.LastQuotaCompletion,
		&s.ProgressDate, &s.LastRewardedDay, &s.LastMessageAt,
		&s.TotalQuotasCompleted, &s.ReminderSentToday, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get streak user_id=%d: %w", userID, err)
	}
	return &s, nil
}

func (r *Repository) UpdateTx(ctx context.Context, tx pgx.Tx, s *Streak) error {
	_, err := tx.Exec(ctx, `
		UPDATE streaks
		SET current_streak = $2,
		    longest_streak = $3,
		    messages_today = $4,
		    quota_completed_today = $5,
		    last_quota_completion = $6,
		    progress_date = $7,
		    last_rewarded_day = $8,
		    last_message_at = $9,
		    total_quotas_completed = $10,
		    reminder_sent_today = $11,
		    updated_at = NOW()
		WHERE user_id = $1
	`, s.UserID, s.CurrentStreak, s.LongestStreak, s.MessagesToday, s.QuotaCompletedToday,
		s.LastQuotaCompletion, s.ProgressDate, s.LastRewardedDay, s.LastMessageAt,
		s.TotalQuotasCompleted, s.ReminderSentToday)
	if err != nil {
		return fmt.Errorf("update streak: %w", err)
	}
	return nil
}

func (r *Repository) MarkProcessedMessageTx(ctx context.Context, tx pgx.Tx, userID, messageID int64, streakDay time.Time) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO streak_processed_messages (user_id, message_id, streak_day)
		VALUES ($1, $2, $3)
	`, userID, messageID, streakDay)
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return errProcessedMessageDuplicate
	}
	return fmt.Errorf("mark processed message: %w", err)
}

func (r *Repository) MarkReminderSentIfNotSentTodayTx(ctx context.Context, tx pgx.Tx, userID int64, progressDay time.Time) (bool, error) {
	tag, err := tx.Exec(ctx, `
		UPDATE streaks
		SET reminder_sent_today = TRUE,
		    updated_at = NOW()
		WHERE user_id = $1
		  AND progress_date = $2
		  AND reminder_sent_today = FALSE
	`, userID, progressDay)
	if err != nil {
		return false, fmt.Errorf("mark reminder sent conditionally: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func (r *Repository) ClearReminderSentTx(ctx context.Context, tx pgx.Tx, userID int64, progressDay time.Time) error {
	_, err := tx.Exec(ctx, `
		UPDATE streaks
		SET reminder_sent_today = FALSE,
		    updated_at = NOW()
		WHERE user_id = $1
		  AND progress_date = $2
		  AND reminder_sent_today = TRUE
	`, userID, progressDay)
	if err != nil {
		return fmt.Errorf("clear reminder sent: %w", err)
	}
	return nil
}

func (r *Repository) GetTop(ctx context.Context, limit int) ([]TopEntry, error) {
	rows, err := r.db.Query(ctx, `
		SELECT user_id, current_streak
		FROM streaks
		WHERE current_streak > 0
		ORDER BY current_streak DESC, user_id ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("get top streaks: %w", err)
	}
	defer rows.Close()

	out := make([]TopEntry, 0, limit)
	for rows.Next() {
		var entry TopEntry
		if err := rows.Scan(&entry.UserID, &entry.CurrentStreak); err != nil {
			return nil, fmt.Errorf("scan top streak: %w", err)
		}
		out = append(out, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate top streaks: %w", err)
	}
	return out, nil
}

func (r *Repository) GetByMinStreak(ctx context.Context, minStreak int) ([]*Streak, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, user_id, current_streak, longest_streak, messages_today,
		       quota_completed_today, last_quota_completion, progress_date,
		       last_rewarded_day, last_message_at, total_quotas_completed,
		       reminder_sent_today, created_at, updated_at
		FROM streaks
		WHERE current_streak >= $1
	`, minStreak)
	if err != nil {
		return nil, fmt.Errorf("get streaks by min streak: %w", err)
	}
	defer rows.Close()

	var out []*Streak
	for rows.Next() {
		var s Streak
		if err := rows.Scan(
			&s.ID, &s.UserID, &s.CurrentStreak, &s.LongestStreak,
			&s.MessagesToday, &s.QuotaCompletedToday, &s.LastQuotaCompletion,
			&s.ProgressDate, &s.LastRewardedDay, &s.LastMessageAt,
			&s.TotalQuotasCompleted, &s.ReminderSentToday, &s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan streak: %w", err)
		}
		out = append(out, &s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate streaks: %w", err)
	}
	return out, nil
}

func (r *Repository) ResetDaily(ctx context.Context, day time.Time) error {
	_, err := r.db.Exec(ctx, `
		UPDATE streaks
		SET messages_today = CASE WHEN progress_date = $1 THEN messages_today ELSE 0 END,
		    quota_completed_today = CASE WHEN progress_date = $1 THEN quota_completed_today ELSE FALSE END,
		    reminder_sent_today = FALSE,
		    updated_at = NOW()
	`, day)
	if err != nil {
		return fmt.Errorf("reset daily streak fields: %w", err)
	}

	retentionCutoff := day.AddDate(0, 0, -7)
	if _, err := r.db.Exec(ctx, `
		DELETE FROM streak_processed_messages
		WHERE streak_day < $1
	`, retentionCutoff); err != nil {
		return fmt.Errorf("cleanup processed streak messages: %w", err)
	}
	return nil
}
