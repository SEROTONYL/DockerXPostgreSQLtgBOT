// Package admin реализует админ-панель с парольной аутентификацией.
// models.go описывает структуры сессий и попыток входа.
package admin

import "time"

// AdminSession — активная сессия администратора.
type AdminSession struct {
	ID              int64     `db:"id"`
	UserID          int64     `db:"user_id"`
	SessionToken    string    `db:"session_token"`
	AuthenticatedAt time.Time `db:"authenticated_at"`
	ExpiresAt       time.Time `db:"expires_at"`
	LastActivity    time.Time `db:"last_activity"`
	IsActive        bool      `db:"is_active"`
}

// LoginAttempt — попытка входа (для защиты от brute-force).
type LoginAttempt struct {
	ID          int64     `db:"id"`
	UserID      int64     `db:"user_id"`
	AttemptTime time.Time `db:"attempt_time"`
	Success     bool      `db:"success"`
}

// AdminState — состояние диалога с админом (конечный автомат).
// Админ-панель работает по шагам: выбор действия → выбор пользователя → ввод роли.
type AdminState struct {
	State      string      // Текущее состояние ("", "awaiting_password", "assign_role_select", ...)
	Data       interface{} // Данные контекста (список пользователей, выбранный пользователь)
	ExpiresAt  time.Time   // Когда состояние истекает (5 минут)
}

// Возможные состояния админ-диалога
const (
	StateNone              = ""                    // Нет активного состояния
	StateAwaitingPassword  = "awaiting_password"   // Ждём пароль
	StateAssignRoleSelect  = "assign_role_select"  // Ждём выбор пользователя (без роли)
	StateAssignRoleText    = "assign_role_text"    // Ждём текст новой роли
	StateChangeRoleSelect  = "change_role_select"  // Ждём выбор пользователя (с ролью)
	StateChangeRoleText    = "change_role_text"    // Ждём новую роль
)
