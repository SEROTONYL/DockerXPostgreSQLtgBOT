// Package admin: repository.go работает с таблицами admin_sessions и admin_login_attempts.
package admin

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
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

const (
	adminLoginAttemptRetention = 30 * 24 * time.Hour
	adminFlowStateRetention    = 30 * 24 * time.Hour
)

type CleanupResult struct {
	ExpiredSessions  int64
	OldLoginAttempts int64
	StaleFlowStates  int64
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

func (r *Repository) CleanupStaleAuthState(ctx context.Context, now time.Time) (CleanupResult, error) {
	var result CleanupResult

	sessionsCmd, err := r.db.Exec(ctx, `
		DELETE FROM admin_sessions
		WHERE expires_at IS NOT NULL AND expires_at < $1
	`, now.UTC())
	if err != nil {
		return result, fmt.Errorf("cleanup expired admin sessions: %w", err)
	}
	result.ExpiredSessions = sessionsCmd.RowsAffected()

	attemptsCmd, err := r.db.Exec(ctx, `
		DELETE FROM admin_login_attempts
		WHERE attempt_time < $1
	`, now.UTC().Add(-adminLoginAttemptRetention))
	if err != nil {
		return result, fmt.Errorf("cleanup old admin login attempts: %w", err)
	}
	result.OldLoginAttempts = attemptsCmd.RowsAffected()

	flowCmd, err := r.db.Exec(ctx, `
		DELETE FROM admin_flow_states
		WHERE (state_expires_at IS NOT NULL AND state_expires_at < $1)
		   OR (
		        state_expires_at IS NULL
		    AND COALESCE(state_name, '') = ''
		    AND COALESCE(panel_updated_at, updated_at) < $2
		   )
	`, now.UTC(), now.UTC().Add(-adminFlowStateRetention))
	if err != nil {
		return result, fmt.Errorf("cleanup stale admin flow states: %w", err)
	}
	result.StaleFlowStates = flowCmd.RowsAffected()

	return result, nil
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ошибка итерации дельт: %w", err)
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

func (r *Repository) DeleteBalanceDelta(ctx context.Context, chatID int64, deltaID int64) error {
	cmd, err := r.db.Exec(ctx, `
		DELETE FROM admin_balance_deltas
		WHERE chat_id = $1 AND id = $2
	`, chatID, deltaID)
	if err != nil {
		return fmt.Errorf("ошибка удаления дельты: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("дельта не найдена")
	}
	return nil
}

func (r *Repository) SaveFlowState(ctx context.Context, userID int64, state *AdminState) error {
	if state == nil {
		return r.ClearFlowState(ctx, userID)
	}

	payload, err := marshalAdminStateData(state.State, state.Data)
	if err != nil {
		return fmt.Errorf("marshal admin flow state: %w", err)
	}

	_, err = r.db.Exec(ctx, `
		INSERT INTO admin_flow_states (user_id, state_name, state_payload, state_expires_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (user_id) DO UPDATE
		SET state_name = EXCLUDED.state_name,
		    state_payload = EXCLUDED.state_payload,
		    state_expires_at = EXCLUDED.state_expires_at,
		    updated_at = NOW()
	`, userID, state.State, payload, state.ExpiresAt)
	if err != nil {
		return fmt.Errorf("save admin flow state: %w", err)
	}
	return nil
}

func (r *Repository) GetFlowState(ctx context.Context, userID int64) (*AdminState, error) {
	var (
		stateName string
		payload   []byte
		expiresAt *time.Time
	)
	err := r.db.QueryRow(ctx, `
		SELECT state_name, state_payload, state_expires_at
		FROM admin_flow_states
		WHERE user_id = $1
	`, userID).Scan(&stateName, &payload, &expiresAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get admin flow state: %w", err)
	}
	if stateName == "" || expiresAt == nil {
		return nil, nil
	}
	data, err := unmarshalAdminStateData(stateName, payload)
	if err != nil {
		return nil, fmt.Errorf("unmarshal admin flow state: %w", err)
	}
	return &AdminState{State: stateName, Data: data, ExpiresAt: *expiresAt}, nil
}

func (r *Repository) ClearFlowState(ctx context.Context, userID int64) error {
	_, err := r.db.Exec(ctx, `
		DELETE FROM admin_flow_states
		WHERE user_id = $1
	`, userID)
	if err != nil {
		return fmt.Errorf("clear admin flow state: %w", err)
	}
	return nil
}

func (r *Repository) SetPanelMessage(ctx context.Context, userID int64, panel AdminPanelMessage) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO admin_flow_states (user_id, panel_chat_id, panel_message_id, panel_updated_at, updated_at)
		VALUES ($1, $2, $3, NOW(), NOW())
		ON CONFLICT (user_id) DO UPDATE
		SET panel_chat_id = EXCLUDED.panel_chat_id,
		    panel_message_id = EXCLUDED.panel_message_id,
		    panel_updated_at = NOW(),
		    updated_at = NOW()
	`, userID, panel.ChatID, panel.MessageID)
	if err != nil {
		return fmt.Errorf("set admin panel message: %w", err)
	}
	return nil
}

func (r *Repository) GetPanelMessage(ctx context.Context, userID int64) (AdminPanelMessage, error) {
	var panel AdminPanelMessage
	err := r.db.QueryRow(ctx, `
		SELECT COALESCE(panel_chat_id, 0), COALESCE(panel_message_id, 0)
		FROM admin_flow_states
		WHERE user_id = $1
	`, userID).Scan(&panel.ChatID, &panel.MessageID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return AdminPanelMessage{}, nil
		}
		return AdminPanelMessage{}, fmt.Errorf("get admin panel message: %w", err)
	}
	return panel, nil
}

func (r *Repository) ClearPanelMessage(ctx context.Context, userID int64) error {
	_, err := r.db.Exec(ctx, `
		UPDATE admin_flow_states
		SET panel_chat_id = NULL,
		    panel_message_id = NULL,
		    panel_updated_at = NULL,
		    updated_at = NOW()
		WHERE user_id = $1
	`, userID)
	if err != nil {
		return fmt.Errorf("clear admin panel message: %w", err)
	}
	return nil
}
