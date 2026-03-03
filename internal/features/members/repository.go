// Package members — repository.go отвечает за все операции с таблицей members в БД.
// Каждая функция выполняет один SQL-запрос и возвращает результат или ошибку.
package members

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	StatusActive = "active"
	StatusLeft   = "left"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// Create добавляет нового участника в таблицу members.
// На конфликте по user_id обновляет имя/username и активирует участника.
func (r *Repository) Create(ctx context.Context, m *Member) error {
	joinedAt := time.Now().UTC()
	return r.UpsertActiveMember(ctx, m.UserID, m.Username, m.DisplayName(), joinedAt)
}

// UpsertActiveMember вставляет/обновляет участника и помечает его как active.
func (r *Repository) UpsertActiveMember(ctx context.Context, userID int64, username, name string, joinedAt time.Time) error {
	query := `
		INSERT INTO members (user_id, username, first_name, status, joined_at, left_at, delete_after, last_seen_at, last_known_name)
		VALUES ($1, $2, $3, $4, $5, NULL, NULL, NOW(), $3)
		ON CONFLICT (user_id) DO UPDATE
		SET username = EXCLUDED.username,
		    first_name = EXCLUDED.first_name,
		    status = $4,
		    joined_at = COALESCE(members.joined_at, EXCLUDED.joined_at),
		    left_at = NULL,
		    delete_after = NULL,
		    last_seen_at = NOW(),
		    last_known_name = EXCLUDED.last_known_name,
		    updated_at = NOW()
	`
	if _, err := r.db.Exec(ctx, query, userID, username, name, StatusActive, joinedAt.UTC()); err != nil {
		return fmt.Errorf("ошибка upsert активного участника: %w", err)
	}
	return nil
}

// MarkMemberLeft помечает участника как вышедшего.
func (r *Repository) MarkMemberLeft(ctx context.Context, userID int64, leftAt, deleteAfter time.Time) error {
	query := `
		UPDATE members
		SET status = $2,
		    left_at = $3,
		    delete_after = $4,
		    updated_at = NOW()
		WHERE user_id = $1
	`
	if _, err := r.db.Exec(ctx, query, userID, StatusLeft, leftAt.UTC(), deleteAfter.UTC()); err != nil {
		return fmt.Errorf("ошибка установки статуса left: %w", err)
	}
	return nil
}

// IsActiveMember возвращает true, если участник в статусе active.
func (r *Repository) IsActiveMember(ctx context.Context, userID int64) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM members WHERE user_id = $1 AND status = $2)`
	var isActive bool
	if err := r.db.QueryRow(ctx, query, userID, StatusActive).Scan(&isActive); err != nil {
		return false, fmt.Errorf("ошибка проверки active-статуса: %w", err)
	}
	return isActive, nil
}

// ListActiveMembers возвращает список активных участников.
func (r *Repository) ListActiveMembers(ctx context.Context) ([]*Member, error) {
	query := `
		SELECT id, user_id, username, first_name, last_name, role, is_admin, is_banned,
		       status, joined_at, left_at, delete_after, last_seen_at, last_known_name, created_at, updated_at
		FROM members
		WHERE status = $1
		ORDER BY first_name
	`
	return r.queryMembers(ctx, query, StatusActive)
}

// PurgeExpiredLeftMembers удаляет пользователей со статусом left, у которых истёк delete_after.
// Удаление выполняется транзакционно и включает связанные записи из доменных таблиц.
func (r *Repository) PurgeExpiredLeftMembers(ctx context.Context, now time.Time, limit int) (int, error) {
	if limit <= 0 {
		return 0, fmt.Errorf("limit должен быть > 0")
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("ошибка начала транзакции purge: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	rows, err := tx.Query(ctx, purgeSelectionQuery(), StatusLeft, now.UTC(), limit)
	if err != nil {
		return 0, fmt.Errorf("ошибка выборки кандидатов purge: %w", err)
	}

	userIDs := make([]int64, 0, limit)
	for rows.Next() {
		var userID int64
		if err := rows.Scan(&userID); err != nil {
			rows.Close()
			return 0, fmt.Errorf("ошибка чтения кандидатов purge: %w", err)
		}
		userIDs = append(userIDs, userID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, fmt.Errorf("ошибка итерации кандидатов purge: %w", err)
	}
	rows.Close()

	if len(userIDs) == 0 {
		if err := tx.Commit(ctx); err != nil {
			return 0, fmt.Errorf("ошибка commit пустого purge: %w", err)
		}
		return 0, nil
	}

	for _, query := range purgeDeleteQueries() {
		if _, err := tx.Exec(ctx, query, userIDs); err != nil {
			return 0, fmt.Errorf("ошибка purge query %q: %w", query, err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("ошибка commit purge: %w", err)
	}

	return len(userIDs), nil
}

// GetByUserID: если не найден — ошибка с pgx.ErrNoRows (errors.Is(err, pgx.ErrNoRows) == true)
func (r *Repository) GetByUserID(ctx context.Context, userID int64) (*Member, error) {
	query := `
		SELECT id, user_id, username, first_name, last_name, role, is_admin, is_banned,
		       status, joined_at, left_at, delete_after, last_seen_at, last_known_name, created_at, updated_at
		FROM members
		WHERE user_id = $1
	`
	var m Member
	err := r.db.QueryRow(ctx, query, userID).Scan(
		&m.ID, &m.UserID, &m.Username, &m.FirstName, &m.LastName,
		&m.Role, &m.IsAdmin, &m.IsBanned,
		&m.Status, &m.JoinedAt, &m.LeftAt, &m.DeleteAfter, &m.LastSeenAt, &m.LastKnownName, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("участник не найден (user_id=%d): %w", userID, err)
		}
		return nil, fmt.Errorf("ошибка чтения участника (user_id=%d): %w", userID, err)
	}
	return &m, nil
}

// GetByUsername: если не найден — ошибка с pgx.ErrNoRows
func (r *Repository) GetByUsername(ctx context.Context, username string) (*Member, error) {
	query := `
		SELECT id, user_id, username, first_name, last_name, role, is_admin, is_banned,
		       status, joined_at, left_at, delete_after, last_seen_at, last_known_name, created_at, updated_at
		FROM members
		WHERE LOWER(username) = LOWER($1) AND status = $2
	`
	var m Member
	err := r.db.QueryRow(ctx, query, username, StatusActive).Scan(
		&m.ID, &m.UserID, &m.Username, &m.FirstName, &m.LastName,
		&m.Role, &m.IsAdmin, &m.IsBanned,
		&m.Status, &m.JoinedAt, &m.LeftAt, &m.DeleteAfter, &m.LastSeenAt, &m.LastKnownName, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("участник не найден (username=%s): %w", username, err)
		}
		return nil, fmt.Errorf("ошибка чтения участника (username=%s): %w", username, err)
	}
	return &m, nil
}

func (r *Repository) Exists(ctx context.Context, userID int64) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM members WHERE user_id = $1 AND status = $2)`
	var exists bool
	if err := r.db.QueryRow(ctx, query, userID, StatusActive).Scan(&exists); err != nil {
		return false, fmt.Errorf("ошибка проверки существования: %w", err)
	}
	return exists, nil
}

func (r *Repository) UpdateInfo(ctx context.Context, userID int64, info UpdateInfo) error {
	query := `
		UPDATE members
		SET username = $2, first_name = $3, last_name = $4, last_seen_at = NOW(), updated_at = NOW()
		WHERE user_id = $1
	`
	if _, err := r.db.Exec(ctx, query, userID, info.Username, info.FirstName, info.LastName); err != nil {
		return fmt.Errorf("ошибка обновления данных участника: %w", err)
	}
	return nil
}

func (r *Repository) UpdateRole(ctx context.Context, userID int64, role string) error {
	query := `UPDATE members SET role = $2, updated_at = NOW() WHERE user_id = $1 AND status = $3`
	if _, err := r.db.Exec(ctx, query, userID, role, StatusActive); err != nil {
		return fmt.Errorf("ошибка обновления роли: %w", err)
	}
	return nil
}

func (r *Repository) UpdateAdminFlag(ctx context.Context, userID int64, isAdmin bool) error {
	query := `UPDATE members SET is_admin = $2, updated_at = NOW() WHERE user_id = $1 AND status = $3`
	if _, err := r.db.Exec(ctx, query, userID, isAdmin, StatusActive); err != nil {
		return fmt.Errorf("ошибка обновления флага администратора: %w", err)
	}
	return nil
}

func (r *Repository) GetUsersWithoutRole(ctx context.Context) ([]*Member, error) {
	query := `
		SELECT id, user_id, username, first_name, last_name, role, is_admin, is_banned,
		       status, joined_at, left_at, delete_after, last_seen_at, last_known_name, created_at, updated_at
		FROM members
		WHERE role IS NULL AND is_banned = FALSE AND status = $1
		ORDER BY first_name
	`
	return r.queryMembers(ctx, query, StatusActive)
}

func (r *Repository) GetUsersWithRole(ctx context.Context) ([]*Member, error) {
	query := `
		SELECT id, user_id, username, first_name, last_name, role, is_admin, is_banned,
		       status, joined_at, left_at, delete_after, last_seen_at, last_known_name, created_at, updated_at
		FROM members
		WHERE role IS NOT NULL AND is_banned = FALSE AND status = $1
		ORDER BY first_name
	`
	return r.queryMembers(ctx, query, StatusActive)
}

func purgeSelectionQuery() string {
	return `
		SELECT user_id
		FROM members
		WHERE status = $1 AND delete_after IS NOT NULL AND delete_after <= $2
		ORDER BY delete_after, user_id
		LIMIT $3
		FOR UPDATE SKIP LOCKED
	`
}

func purgeDeleteQueries() []string {
	return []string{
		`DELETE FROM transactions WHERE from_user_id = ANY($1) OR to_user_id = ANY($1)`,
		`DELETE FROM karma_logs WHERE from_user_id = ANY($1) OR to_user_id = ANY($1)`,
		`DELETE FROM admin_sessions WHERE user_id = ANY($1)`,
		`DELETE FROM admin_login_attempts WHERE user_id = ANY($1)`,
		`DELETE FROM casino_games WHERE user_id = ANY($1)`,
		`DELETE FROM casino_stats WHERE user_id = ANY($1)`,
		`DELETE FROM balances WHERE user_id = ANY($1)`,
		`DELETE FROM streaks WHERE user_id = ANY($1)`,
		`DELETE FROM karma WHERE user_id = ANY($1)`,
		`DELETE FROM members WHERE user_id = ANY($1)`,
	}
}

func (r *Repository) queryMembers(ctx context.Context, query string, args ...interface{}) ([]*Member, error) {
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("ошибка запроса участников: %w", err)
	}
	defer rows.Close()

	var out []*Member
	for rows.Next() {
		var m Member
		if err := rows.Scan(
			&m.ID, &m.UserID, &m.Username, &m.FirstName, &m.LastName,
			&m.Role, &m.IsAdmin, &m.IsBanned,
			&m.Status, &m.JoinedAt, &m.LeftAt, &m.DeleteAfter, &m.LastSeenAt, &m.LastKnownName, &m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("ошибка сканирования строки: %w", err)
		}
		out = append(out, &m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ошибка чтения строк: %w", err)
	}

	return out, nil
}
