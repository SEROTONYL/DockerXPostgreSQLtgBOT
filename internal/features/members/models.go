// Package members управляет участниками чата: регистрацией, ролями, флагами.
// models.go описывает структуры данных для работы с таблицей members.
package members

import "time"

// Member представляет участника чата в базе данных.
// Каждый пользователь, вступивший в FLOOD_CHAT_ID, автоматически
// создаётся в этой таблице.
type Member struct {
	ID        int64     `db:"id"`         // Автоинкрементный ID записи в БД
	UserID    int64     `db:"user_id"`    // Telegram user ID (уникальный)
	Username  string    `db:"username"`   // @username (может быть пустым)
	FirstName string    `db:"first_name"` // Имя пользователя
	LastName  string    `db:"last_name"`  // Фамилия (может быть пустой)
	Role      *string   `db:"role"`       // Роль, назначенная админом (до 64 символов, может быть nil)
	IsAdmin   bool      `db:"is_admin"`   // Флаг администратора
	IsBanned  bool      `db:"is_banned"`  // Флаг бана
	JoinedAt  time.Time `db:"joined_at"`  // Когда вступил в чат
	CreatedAt time.Time `db:"created_at"` // Когда запись создана в БД
	UpdatedAt time.Time `db:"updated_at"` // Последнее обновление записи
}

// UpdateInfo содержит данные для обновления информации о пользователе.
// Используется, когда пользователь возвращается в чат и его имя/username могли измениться.
type UpdateInfo struct {
	Username  string // Новый @username
	FirstName string // Новое имя
	LastName  string // Новая фамилия
}

// DisplayName возвращает отображаемое имя пользователя.
// Если есть @username — возвращает его, иначе — имя + фамилию.
func (m *Member) DisplayName() string {
	if m.Username != "" {
		return "@" + m.Username
	}
	name := m.FirstName
	if m.LastName != "" {
		name += " " + m.LastName
	}
	return name
}
