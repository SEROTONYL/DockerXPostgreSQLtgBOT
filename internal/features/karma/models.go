// Package karma реализует систему репутации (кармы).
// models.go описывает структуры для хранения кармы и логов.
package karma

import "time"

// Karma хранит карму пользователя.
type Karma struct {
	ID               int64     `db:"id"`
	UserID           int64     `db:"user_id"`
	KarmaPoints      int       `db:"karma_points"`
	PositiveReceived int       `db:"positive_received"`
	CreatedAt        time.Time `db:"created_at"`
	UpdatedAt        time.Time `db:"updated_at"`
}

// KarmaLog — запись о выдаче кармы.
type KarmaLog struct {
	ID         int64     `db:"id"`
	FromUserID int64     `db:"from_user_id"`
	ToUserID   int64     `db:"to_user_id"`
	Points     int       `db:"points"` // Всегда +1
	CreatedAt  time.Time `db:"created_at"`
}
