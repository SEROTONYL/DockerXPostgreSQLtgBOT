// Package admin — repository.go работает с таблицами admin_sessions и admin_login_attempts.
package admin

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository работает с админ-таблицами.
type Repository struct {
	db *pgxpool.Pool
}

// NewRepository создаёт репозиторий.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// CreateSession создаёт новую сессию администратора.
func (r *Repository) CreateSession(ctx context.Context, session *AdminSession) error {
	query := `
		INSERT INTO admin_sessions (user_id, session_token, expires_at, is_active)
		VALUES ($1, $2, $3, TRUE)
	`
	_, err := r.db.Exec(ctx, query, session.UserID, session.SessionToken, session.ExpiresAt)
	if err != nil {
		return fmt.Errorf("ошибка создания сессии: %w", err)
	}
	return nil
}

// GetActiveSession возвращает активную сессию пользователя.
func (r *Repository) GetActiveSession(ctx context.Context, userID int64) (*AdminSession, error) {
	query := `
		SELECT id, user_id, session_token, authenticated_at, expires_at, last_activity, is_active
		FROM admin_sessions
		WHERE user_id = $1 AND is_active = TRUE AND expires_at > NOW()
		ORDER BY authenticated_at DESC
		LIMIT 1
	`
	var s AdminSession
	err := r.db.QueryRow(ctx, query, userID).Scan(
		&s.ID, &s.UserID, &s.SessionToken, &s.AuthenticatedAt,
		&s.ExpiresAt, &s.LastActivity, &s.IsActive,
	)
	if err != nil {
		return nil, fmt.Errorf("активная сессия не найдена: %w", err)
	}
	return &s, nil
}

// DeactivateSession деактивирует сессию.
func (r *Repository) DeactivateSession(ctx context.Context, userID int64) error {
	query := `UPDATE admin_sessions SET is_active = FALSE WHERE user_id = $1`
	_, err := r.db.Exec(ctx, query, userID)
	return err
}

// UpdateActivity обновляет время последней активности.
func (r *Repository) UpdateActivity(ctx context.Context, userID int64) error {
	query := `UPDATE admin_sessions SET last_activity = NOW() WHERE user_id = $1 AND is_active = TRUE`
	_, err := r.db.Exec(ctx, query, userID)
	return err
}

// LogAttempt записывает попытку входа.
func (r *Repository) LogAttempt(ctx context.Context, userID int64, success bool) error {
	query := `INSERT INTO admin_login_attempts (user_id, success) VALUES ($1, $2)`
	_, err := r.db.Exec(ctx, query, userID, success)
	return err
}

// GetRecentAttempts возвращает количество неудачных попыток за указанный период.
func (r *Repository) GetRecentAttempts(ctx context.Context, userID int64, period time.Duration) (int, error) {
	since := time.Now().Add(-period)
	query := `
		SELECT COUNT(*) FROM admin_login_attempts
		WHERE user_id = $1 AND success = FALSE AND attempt_time >= $2
	`
	var count int
	err := r.db.QueryRow(ctx, query, userID, since).Scan(&count)
	return count, err
}

func (r *Repository) ListBalanceDeltas(ctx context.Context, chatID int64) ([]*BalanceDelta, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, chat_id, name, amount, created_by, created_at
		FROM admin_balance_deltas
		WHERE chat_id = $1
		ORDER BY created_at ASC, id ASC
	`, chatID)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения дельт: %w", err)
	}
	defer rows.Close()

	result := make([]*BalanceDelta, 0)
	for rows.Next() {
		var d BalanceDelta
		if err := rows.Scan(&d.ID, &d.ChatID, &d.Name, &d.Amount, &d.CreatedBy, &d.CreatedAt); err != nil {
			return nil, fmt.Errorf("ошибка чтения дельты: %w", err)
		}
		result = append(result, &d)
	}
	return result, nil
}

func (r *Repository) CreateBalanceDelta(ctx context.Context, chatID int64, name string, amount int64, createdBy int64) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO admin_balance_deltas (chat_id, name, amount, created_by)
		VALUES ($1, $2, $3, $4)
	`, chatID, name, amount, createdBy)
	if err != nil {
		return fmt.Errorf("ошибка создания дельты: %w", err)
	}
	return nil
}
