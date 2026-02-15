// Package economy — repository.go выполняет все операции с таблицами balances и transactions.
// Все денежные операции выполняются в транзакциях БД для целостности данных.
package economy

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository предоставляет методы для работы с балансами и транзакциями.
type Repository struct {
	db *pgxpool.Pool
}

// NewRepository создаёт новый репозиторий экономики.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// CreateBalance создаёт начальный баланс для нового участника.
// Начальный баланс всегда 0 пленок.
func (r *Repository) CreateBalance(ctx context.Context, userID int64) error {
	query := `
		INSERT INTO balances (user_id, balance, total_earned, total_spent)
		VALUES ($1, 0, 0, 0)
		ON CONFLICT (user_id) DO NOTHING
	`
	_, err := r.db.Exec(ctx, query, userID)
	if err != nil {
		return fmt.Errorf("ошибка создания баланса: %w", err)
	}
	return nil
}

// GetBalance возвращает текущий баланс пользователя.
func (r *Repository) GetBalance(ctx context.Context, userID int64) (int64, error) {
	query := `SELECT balance FROM balances WHERE user_id = $1`
	var balance int64
	err := r.db.QueryRow(ctx, query, userID).Scan(&balance)
	if err != nil {
		return 0, fmt.Errorf("ошибка получения баланса: %w", err)
	}
	return balance, nil
}

// AddBalance добавляет пленки на счёт пользователя.
// Используется для начислений: бонусы стриков, выигрыши казино, переводы.
//
// Параметры:
//   - userID: кому начислить
//   - amount: сколько (положительное число)
//   - txType: тип транзакции (streak_bonus, casino_win, transfer, ...)
//   - description: описание для истории транзакций
func (r *Repository) AddBalance(ctx context.Context, userID int64, amount int64, txType, description string) error {
	// Начинаем транзакцию БД, чтобы обновление баланса и запись транзакции
	// были атомарными (либо оба произойдут, либо ни одного)
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("ошибка начала транзакции: %w", err)
	}
	defer tx.Rollback(ctx)

	// Обновляем баланс и total_earned
	_, err = tx.Exec(ctx, `
		UPDATE balances
		SET balance = balance + $2, total_earned = total_earned + $2, updated_at = NOW()
		WHERE user_id = $1
	`, userID, amount)
	if err != nil {
		return fmt.Errorf("ошибка начисления: %w", err)
	}

	// Записываем транзакцию в историю
	_, err = tx.Exec(ctx, `
		INSERT INTO transactions (to_user_id, amount, transaction_type, description)
		VALUES ($1, $2, $3, $4)
	`, userID, amount, txType, description)
	if err != nil {
		return fmt.Errorf("ошибка записи транзакции: %w", err)
	}

	return tx.Commit(ctx)
}

// DeductBalance списывает пленки со счёта пользователя.
// Проверяет, что баланс не станет отрицательным.
func (r *Repository) DeductBalance(ctx context.Context, userID int64, amount int64, txType, description string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("ошибка начала транзакции: %w", err)
	}
	defer tx.Rollback(ctx)

	// Проверяем баланс перед списанием (с блокировкой строки FOR UPDATE)
	var currentBalance int64
	err = tx.QueryRow(ctx, `
		SELECT balance FROM balances WHERE user_id = $1 FOR UPDATE
	`, userID).Scan(&currentBalance)
	if err != nil {
		return fmt.Errorf("ошибка получения баланса: %w", err)
	}

	if currentBalance < amount {
		return fmt.Errorf("недостаточно пленок: нужно %d, есть %d", amount, currentBalance)
	}

	// Списываем
	_, err = tx.Exec(ctx, `
		UPDATE balances
		SET balance = balance - $2, total_spent = total_spent + $2, updated_at = NOW()
		WHERE user_id = $1
	`, userID, amount)
	if err != nil {
		return fmt.Errorf("ошибка списания: %w", err)
	}

	// Записываем транзакцию
	_, err = tx.Exec(ctx, `
		INSERT INTO transactions (from_user_id, amount, transaction_type, description)
		VALUES ($1, $2, $3, $4)
	`, userID, amount, txType, description)
	if err != nil {
		return fmt.Errorf("ошибка записи транзакции: %w", err)
	}

	return tx.Commit(ctx)
}

// Transfer переводит пленки от одного пользователя к другому.
// Атомарная операция: либо оба баланса обновятся, либо ни одного.
func (r *Repository) Transfer(ctx context.Context, fromUserID, toUserID, amount int64) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("ошибка начала транзакции: %w", err)
	}
	defer tx.Rollback(ctx)

	// Блокируем строку отправителя и проверяем баланс
	var senderBalance int64
	err = tx.QueryRow(ctx, `
		SELECT balance FROM balances WHERE user_id = $1 FOR UPDATE
	`, fromUserID).Scan(&senderBalance)
	if err != nil {
		return fmt.Errorf("отправитель не найден: %w", err)
	}

	if senderBalance < amount {
		return fmt.Errorf("недостаточно пленок: нужно %d, есть %d", amount, senderBalance)
	}

	// Списываем у отправителя
	_, err = tx.Exec(ctx, `
		UPDATE balances
		SET balance = balance - $2, total_spent = total_spent + $2, updated_at = NOW()
		WHERE user_id = $1
	`, fromUserID, amount)
	if err != nil {
		return fmt.Errorf("ошибка списания у отправителя: %w", err)
	}

	// Начисляем получателю
	_, err = tx.Exec(ctx, `
		UPDATE balances
		SET balance = balance + $2, total_earned = total_earned + $2, updated_at = NOW()
		WHERE user_id = $1
	`, toUserID, amount)
	if err != nil {
		return fmt.Errorf("ошибка начисления получателю: %w", err)
	}

	// Записываем транзакцию
	_, err = tx.Exec(ctx, `
		INSERT INTO transactions (from_user_id, to_user_id, amount, transaction_type, description)
		VALUES ($1, $2, $3, 'transfer', $4)
	`, fromUserID, toUserID, amount, fmt.Sprintf("Перевод %d пленок", amount))
	if err != nil {
		return fmt.Errorf("ошибка записи транзакции: %w", err)
	}

	return tx.Commit(ctx)
}

// GetTransactions возвращает последние N транзакций пользователя.
// Включает как входящие, так и исходящие операции.
func (r *Repository) GetTransactions(ctx context.Context, userID int64, limit int) ([]*Transaction, error) {
	query := `
		SELECT id, from_user_id, to_user_id, amount, transaction_type, description, created_at
		FROM transactions
		WHERE from_user_id = $1 OR to_user_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`
	rows, err := r.db.Query(ctx, query, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения транзакций: %w", err)
	}
	defer rows.Close()

	var transactions []*Transaction
	for rows.Next() {
		var t Transaction
		err := rows.Scan(
			&t.ID, &t.FromUserID, &t.ToUserID,
			&t.Amount, &t.TransactionType, &t.Description, &t.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("ошибка сканирования транзакции: %w", err)
		}
		transactions = append(transactions, &t)
	}
	return transactions, nil
}

// GetTotalStats возвращает общую статистику баланса пользователя.
func (r *Repository) GetTotalStats(ctx context.Context, userID int64) (*Balance, error) {
	query := `
		SELECT id, user_id, balance, total_earned, total_spent, created_at, updated_at
		FROM balances
		WHERE user_id = $1
	`
	var b Balance
	err := r.db.QueryRow(ctx, query, userID).Scan(
		&b.ID, &b.UserID, &b.Balance, &b.TotalEarned, &b.TotalSpent,
		&b.CreatedAt, &b.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("ошибка получения статистики: %w", err)
	}
	return &b, nil
}

// EnsureBalance гарантирует, что у пользователя есть запись баланса.
// Если нет — создаёт с нулевым балансом. Вызывается при регистрации.
func (r *Repository) EnsureBalance(ctx context.Context, userID int64) error {
	return r.CreateBalance(ctx, userID)
}

// GetBalanceCreatedAt возвращает время создания баланса.
// Нужно для проверки, зарегистрирован ли пользователь.
func (r *Repository) BalanceExists(ctx context.Context, userID int64) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM balances WHERE user_id = $1)`
	var exists bool
	err := r.db.QueryRow(ctx, query, userID).Scan(&exists)
	return exists, err
}

// GetTransactionsByPeriod возвращает транзакции за указанный период.
func (r *Repository) GetTransactionsByPeriod(ctx context.Context, userID int64, since time.Time) ([]*Transaction, error) {
	query := `
		SELECT id, from_user_id, to_user_id, amount, transaction_type, description, created_at
		FROM transactions
		WHERE (from_user_id = $1 OR to_user_id = $1) AND created_at >= $2
		ORDER BY created_at DESC
	`
	rows, err := r.db.Query(ctx, query, userID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var txs []*Transaction
	for rows.Next() {
		var t Transaction
		if err := rows.Scan(&t.ID, &t.FromUserID, &t.ToUserID, &t.Amount, &t.TransactionType, &t.Description, &t.CreatedAt); err != nil {
			return nil, err
		}
		txs = append(txs, &t)
	}
	return txs, nil
}
