package karma

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, userID int64) error {
	query := `
		INSERT INTO karma (user_id, karma_points, positive_received)
		VALUES ($1, 0, 0)
		ON CONFLICT (user_id) DO NOTHING
	`
	_, err := r.db.Exec(ctx, query, userID)
	return err
}

func (r *Repository) CountSentSince(ctx context.Context, fromUserID int64, since time.Time) (int, error) {
	const query = `
		SELECT COUNT(*)
		FROM karma_logs
		WHERE from_user_id = $1 AND created_at >= $2
	`
	var count int
	err := r.db.QueryRow(ctx, query, fromUserID, since).Scan(&count)
	return count, err
}

func (r *Repository) HasReciprocalSince(ctx context.Context, fromUserID, toUserID int64, since time.Time) (bool, error) {
	const query = `
		SELECT EXISTS (
			SELECT 1
			FROM karma_logs
			WHERE from_user_id = $1 AND to_user_id = $2 AND created_at >= $3
		)
	`
	var exists bool
	err := r.db.QueryRow(ctx, query, fromUserID, toUserID, since).Scan(&exists)
	return exists, err
}

func (r *Repository) LogThanksTx(ctx context.Context, tx pgx.Tx, fromUserID, toUserID, rewardAmount int64) error {
	const query = `
		INSERT INTO karma_logs (from_user_id, to_user_id, points, reward_amount)
		VALUES ($1, $2, 1, $3)
	`
	_, err := tx.Exec(ctx, query, fromUserID, toUserID, rewardAmount)
	return err
}

func (r *Repository) GetStats(ctx context.Context, userID int64) (*ThanksStats, error) {
	const query = `
		SELECT
			COALESCE(COUNT(*) FILTER (WHERE from_user_id = $1), 0) AS sent_count,
			COALESCE(COUNT(*) FILTER (WHERE to_user_id = $1), 0) AS received_count,
			COALESCE(SUM(reward_amount) FILTER (WHERE to_user_id = $1), 0) AS received_reward
		FROM karma_logs
	`
	var stats ThanksStats
	err := r.db.QueryRow(ctx, query, userID).Scan(&stats.SentCount, &stats.ReceivedCount, &stats.ReceivedReward)
	if err != nil {
		return nil, err
	}
	return &stats, nil
}
