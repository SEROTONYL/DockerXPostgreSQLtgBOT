// Package members — repository.go отвечает за все операции с таблицей members в БД.
// Каждая функция выполняет один SQL-запрос и возвращает результат или ошибку.
package members

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository предоставляет методы для работы с таблицей members.
// Все методы принимают context для возможности отмены долгих запросов.
type Repository struct {
	db *pgxpool.Pool // Пул соединений к PostgreSQL
}

// NewRepository создаёт новый репозиторий участников.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// Create добавляет нового участника в таблицу members.
// Если пользователь с таким user_id уже существует — ничего не делает (ON CONFLICT DO NOTHING).
func (r *Repository) Create(ctx context.Context, m *Member) error {
	query := `
		INSERT INTO members (user_id, username, first_name, last_name, role, is_admin, is_banned, joined_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (user_id) DO NOTHING
	`
	_, err := r.db.Exec(ctx, query,
		m.UserID, m.Username, m.FirstName, m.LastName,
		m.Role, m.IsAdmin, m.IsBanned, time.Now(),
	)
	if err != nil {
		return fmt.Errorf("ошибка создания участника: %w", err)
	}
	return nil
}

// GetByUserID находит участника по его Telegram user ID.
// Возвращает nil, если участник не найден.
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
		return nil, fmt.Errorf("участник не найден (user_id=%d): %w", userID, err)
	}
	return &m, nil
}

// GetByUsername находит участника по @username.
// username передаётся БЕЗ символа @.
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
		return nil, fmt.Errorf("участник не найден (username=%s): %w", username, err)
	}
	return &m, nil
}

// Exists проверяет, существует ли участник с данным user_id.
// Используется для проверки доступа к DM.
func (r *Repository) Exists(ctx context.Context, userID int64) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM members WHERE user_id = $1)`
	var exists bool
	err := r.db.QueryRow(ctx, query, userID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("ошибка проверки существования: %w", err)
	}
	return exists, nil
}

// UpdateInfo обновляет имя и username участника.
// Вызывается при повторном вступлении в чат, когда данные могли измениться.
func (r *Repository) UpdateInfo(ctx context.Context, userID int64, info UpdateInfo) error {
	query := `
		UPDATE members
		SET username = $2, first_name = $3, last_name = $4, updated_at = NOW()
		WHERE user_id = $1
	`
	_, err := r.db.Exec(ctx, query, userID, info.Username, info.FirstName, info.LastName)
	if err != nil {
		return fmt.Errorf("ошибка обновления данных участника: %w", err)
	}
	return nil
}

// UpdateRole устанавливает или обновляет роль участника.
// Роль — строка до 64 символов, назначаемая администратором.
func (r *Repository) UpdateRole(ctx context.Context, userID int64, role string) error {
	query := `UPDATE members SET role = $2, updated_at = NOW() WHERE user_id = $1`
	_, err := r.db.Exec(ctx, query, userID, role)
	if err != nil {
		return fmt.Errorf("ошибка обновления роли: %w", err)
	}
	return nil
}

// GetUsersWithoutRole возвращает список участников, у которых нет роли.
// Используется в админ-панели для команды «Назначить роль».
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

// GetUsersWithRole возвращает участников, которым уже назначена роль.
// Используется в админ-панели для команды «Сменить роль».
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

// queryMembers — внутренний метод для выполнения запросов, возвращающих список участников.
func (r *Repository) queryMembers(ctx context.Context, query string, args ...interface{}) ([]*Member, error) {
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("ошибка запроса участников: %w", err)
	}
	defer rows.Close()

	var members []*Member
	for rows.Next() {
		var m Member
		err := rows.Scan(
			&m.ID, &m.UserID, &m.Username, &m.FirstName, &m.LastName,
			&m.Role, &m.IsAdmin, &m.IsBanned,
			&m.JoinedAt, &m.CreatedAt, &m.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("ошибка сканирования строки: %w", err)
		}
		members = append(members, &m)
	}
	return members, nil
}
