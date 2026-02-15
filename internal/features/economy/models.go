// Package economy управляет виртуальной валютой «пленки».
// models.go описывает структуры для балансов и транзакций.
package economy

import "time"

// Balance представляет баланс пользователя.
// Каждый участник имеет ровно одну запись в таблице balances.
type Balance struct {
	ID         int64     `db:"id"`          // ID записи
	UserID     int64     `db:"user_id"`     // Telegram user ID
	Balance    int64     `db:"balance"`     // Текущий баланс (начинается с 0)
	TotalEarned int64   `db:"total_earned"` // Сколько всего заработано
	TotalSpent int64    `db:"total_spent"`  // Сколько всего потрачено
	CreatedAt  time.Time `db:"created_at"`
	UpdatedAt  time.Time `db:"updated_at"`
}

// Transaction представляет одну операцию с пленками.
// Все движения пленок (переводы, бонусы, ставки) записываются сюда.
type Transaction struct {
	ID              int64     `db:"id"`               // ID транзакции
	FromUserID      *int64    `db:"from_user_id"`     // Отправитель (nil для системных начислений)
	ToUserID        *int64    `db:"to_user_id"`       // Получатель (nil для системных списаний)
	Amount          int64     `db:"amount"`            // Сумма (всегда положительная)
	TransactionType string    `db:"transaction_type"`  // Тип: 'transfer', 'casino_win', 'streak_bonus', и т.д.
	Description     string    `db:"description"`       // Описание для отображения
	CreatedAt       time.Time `db:"created_at"`        // Время транзакции
}

// TransactionTypes — допустимые типы транзакций
const (
	TxTypeTransfer    = "transfer"     // Перевод между пользователями
	TxTypeCasinoBet   = "casino_bet"   // Ставка в казино
	TxTypeCasinoWin   = "casino_win"   // Выигрыш в казино
	TxTypeCasinoRefund = "casino_refund" // Возврат ставки (ошибка)
	TxTypeStreakBonus  = "streak_bonus"  // Бонус за стрик
	TxTypeAdminGive   = "admin_give"   // Выдача админом
	TxTypeAdminTake   = "admin_take"   // Изъятие админом
)
