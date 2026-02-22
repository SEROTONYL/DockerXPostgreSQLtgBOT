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

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// Create добавляет нового участника в таблицу members.
// На конфликте по user_id обновляет только имя/username (не трогает роль/бан/админку).
func (r *Repository) Create(ctx context.Context, m *Member) error {
	query := `
		INSERT INTO members (user_id, username, first_name, last_name, role, is_admin, is_banned, joined_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (user_id) DO UPDATE
		SET username = EXCLUDED.username,
		    first_name = EXCLUDED.first_name,
		    last_name = EXCLUDED.last_name,
		    updated_at = NOW()
	`
	_, err := r.db.Exec(ctx, query,
		m.UserID, m.Username, m.FirstName, m.LastName,
		m.Role, m.IsAdmin, m.IsBanned, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("ошибка создания/обновления участника: %w", err)
	}
	return nil
}

// GetByUserID: если не найден — ошибка с pgx.ErrNoRows (errors.Is(err, pgx.ErrNoRows) == true)
func (r *Repository) GetByUserID(ctx context.Context, userID int64) (*Member, error) {
	query := `
		SELECT id, user_id, username, first_name, last_name, role, is_admin, is_banned,
		       joined_at, created_at, updated_at
		FROM members
		WHERE user_id = $1
	`
	var m Member
	err := r.db.QueryRow(ctx, query, userID).Scan(
		&m.ID, &m.UserID, &m.Username, &m.FirstName, &m.LastName,
		&m.Role, &m.IsAdmin, &m.IsBanned,
		&m.JoinedAt, &m.CreatedAt, &m.UpdatedAt,
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
		       joined_at, created_at, updated_at
		FROM members
		WHERE LOWER(username) = LOWER($1)
	`
	var m Member
	err := r.db.QueryRow(ctx, query, username).Scan(
		&m.ID, &m.UserID, &m.Username, &m.FirstName, &m.LastName,
		&m.Role, &m.IsAdmin, &m.IsBanned,
		&m.JoinedAt, &m.CreatedAt, &m.UpdatedAt,
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
	query := `SELECT EXISTS(SELECT 1 FROM members WHERE user_id = $1)`
	var exists bool
	if err := r.db.QueryRow(ctx, query, userID).Scan(&exists); err != nil {
		return false, fmt.Errorf("ошибка проверки существования: %w", err)
	}
	return exists, nil
}

func (r *Repository) UpdateInfo(ctx context.Context, userID int64, info UpdateInfo) error {
	query := `
		UPDATE members
		SET username = $2, first_name = $3, last_name = $4, updated_at = NOW()
		WHERE user_id = $1
	`
	if _, err := r.db.Exec(ctx, query, userID, info.Username, info.FirstName, info.LastName); err != nil {
		return fmt.Errorf("ошибка обновления данных участника: %w", err)
	}
	return nil
}

func (r *Repository) UpdateRole(ctx context.Context, userID int64, role string) error {
	query := `UPDATE members SET role = $2, updated_at = NOW() WHERE user_id = $1`
	if _, err := r.db.Exec(ctx, query, userID, role); err != nil {
		return fmt.Errorf("ошибка обновления роли: %w", err)
	}
	return nil
}

func (r *Repository) GetUsersWithoutRole(ctx context.Context) ([]*Member, error) {
	query := `
		SELECT id, user_id, username, first_name, last_name, role, is_admin, is_banned,
		       joined_at, created_at, updated_at
		FROM members
		WHERE role IS NULL AND is_banned = FALSE
		ORDER BY first_name
	`
	return r.queryMembers(ctx, query)
}

func (r *Repository) GetUsersWithRole(ctx context.Context) ([]*Member, error) {
	query := `
		SELECT id, user_id, username, first_name, last_name, role, is_admin, is_banned,
		       joined_at, created_at, updated_at
		FROM members
		WHERE role IS NOT NULL AND is_banned = FALSE
		ORDER BY first_name
	`
	return r.queryMembers(ctx, query)
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
			&m.JoinedAt, &m.CreatedAt, &m.UpdatedAt,
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