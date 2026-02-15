// Package karma — repository.go выполняет операции с таблицами karma и karma_logs.
package karma

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"telegram-bot/internal/common"
)

// Repository работает с таблицами karma и karma_logs.
type Repository struct {
	db *pgxpool.Pool
}

// NewRepository создаёт репозиторий кармы.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// Create создаёт запись кармы для нового участника.
func (r *Repository) Create(ctx context.Context, userID int64) error {
	query := `
		INSERT INTO karma (user_id, karma_points, positive_received)
		VALUES ($1, 0, 0)
		ON CONFLICT (user_id) DO NOTHING
	`
	_, err := r.db.Exec(ctx, query, userID)
	return err
}

// GetByUserID возвращает карму пользователя.
func (r *Repository) GetByUserID(ctx context.Context, userID int64) (*Karma, error) {
	query := `
		SELECT id, user_id, karma_points, positive_received, created_at, updated_at
		FROM karma WHERE user_id = $1
	`
	var k Karma
	err := r.db.QueryRow(ctx, query, userID).Scan(
		&k.ID, &k.UserID, &k.KarmaPoints, &k.PositiveReceived,
		&k.CreatedAt, &k.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("карма не найдена: %w", err)
	}
	return &k, nil
}

// IncrementKarma увеличивает карму пользователя на 1.
func (r *Repository) IncrementKarma(ctx context.Context, toUserID int64) error {
	query := `
		UPDATE karma
		SET karma_points = karma_points + 1, positive_received = positive_received + 1,
		    updated_at = NOW()
		WHERE user_id = $1
	`
	_, err := r.db.Exec(ctx, query, toUserID)
	return err
}

// LogKarma записывает действие выдачи кармы.
func (r *Repository) LogKarma(ctx context.Context, fromUserID, toUserID int64, points int) error {
	query := `INSERT INTO karma_logs (from_user_id, to_user_id, points) VALUES ($1, $2, $3)`
	_, err := r.db.Exec(ctx, query, fromUserID, toUserID, points)
	return err
}

// GetTodayCount возвращает, сколько раз пользователь давал карму сегодня.
func (r *Repository) GetTodayCount(ctx context.Context, fromUserID int64) (int, error) {
	today := common.GetMoscowDate()
	query := `SELECT COUNT(*) FROM karma_logs WHERE from_user_id = $1 AND created_at >= $2`
	var count int
	err := r.db.QueryRow(ctx, query, fromUserID, today).Scan(&count)
	return count, err
}

// GaveToday проверяет, давал ли пользователь карму конкретному человеку сегодня.
func (r *Repository) GaveToday(ctx context.Context, fromUserID, toUserID int64) (bool, error) {
	today := common.GetMoscowDate()
	query := `
		SELECT EXISTS(
			SELECT 1 FROM karma_logs
			WHERE from_user_id = $1 AND to_user_id = $2 AND created_at >= $3
		)
	`
	var exists bool
	err := r.db.QueryRow(ctx, query, fromUserID, toUserID, today).Scan(&exists)
	return exists, err
}
