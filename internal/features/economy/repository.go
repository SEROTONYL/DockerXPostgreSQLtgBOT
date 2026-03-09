// Package economy: repository.go выполняет все операции с таблицами balances и transactions.
// Все денежные операции выполняются в транзакциях БД для целостности данных.
package economy

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"serotonyl.ru/telegram-bot/internal/common"
)

// Repository предоставляет методы для работы с балансами и транзакциями.
var ErrInsufficientFunds = errors.New("insufficient funds")
var ErrTransferConfirmationNotFound = errors.New("transfer confirmation not found")
var ErrTransferConfirmationStateConflict = errors.New("transfer confirmation state conflict")

type Repository struct {
	db *pgxpool.Pool
}

// NewRepository создаёт новый репозиторий экономики.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// CreateBalance создаёт начальный баланс для нового участника.
// Начальный баланс всегда 0 плёнок.
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
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, fmt.Errorf("ошибка получения баланса: %w", err)
	}
	return balance, nil
}

// AddBalance добавляет плёнки на счёт пользователя.
func (r *Repository) AddBalance(ctx context.Context, userID int64, amount int64, txType, description string) (err error) {
	return r.AddBalanceWithHook(ctx, userID, amount, txType, description, nil)
}

func (r *Repository) AddBalanceWithHook(ctx context.Context, userID int64, amount int64, txType, description string, hook func(context.Context, pgx.Tx) error) (err error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("ошибка начала транзакции: %w", err)
	}
	defer rollbackOnFailure(ctx, tx, &err)

	if err = r.addBalanceTx(ctx, tx, userID, amount, txType, description); err != nil {
		return err
	}
	if hook != nil {
		if err = hook(ctx, tx); err != nil {
			return err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("ошибка коммита транзакции: %w", err)
	}
	err = nil
	return nil
}

func (r *Repository) WithTransaction(ctx context.Context, fn func(context.Context, pgx.Tx) error) (err error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("ошибка начала транзакции: %w", err)
	}
	defer rollbackOnFailure(ctx, tx, &err)

	if err = fn(ctx, tx); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("ошибка коммита транзакции: %w", err)
	}
	err = nil
	return nil
}

// DeductBalance списывает плёнки со счёта пользователя.
func (r *Repository) DeductBalance(ctx context.Context, userID int64, amount int64, txType, description string) (err error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("ошибка начала транзакции: %w", err)
	}
	defer rollbackOnFailure(ctx, tx, &err)

	if err = r.ensureBalanceRowTx(ctx, tx, userID); err != nil {
		return err
	}

	var currentBalance int64
	err = tx.QueryRow(ctx, `
		SELECT balance FROM balances WHERE user_id = $1 FOR UPDATE
	`, userID).Scan(&currentBalance)
	if err != nil {
		return fmt.Errorf("ошибка получения баланса: %w", err)
	}
	if currentBalance < amount {
		return fmt.Errorf("%w: нужно %d, есть %d", ErrInsufficientFunds, amount, currentBalance)
	}

	_, err = tx.Exec(ctx, `
		UPDATE balances
		SET balance = balance - $2, total_spent = total_spent + $2, updated_at = NOW()
		WHERE user_id = $1
	`, userID, amount)
	if err != nil {
		return fmt.Errorf("ошибка списания: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO transactions (from_user_id, amount, transaction_type, description)
		VALUES ($1, $2, $3, $4)
	`, userID, amount, txType, description)
	if err != nil {
		return fmt.Errorf("ошибка записи транзакции: %w", err)
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("ошибка коммита транзакции: %w", err)
	}
	err = nil
	return nil
}

// Transfer переводит плёнки от одного пользователя к другому.
func (r *Repository) Transfer(ctx context.Context, fromUserID, toUserID, amount int64) (err error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("ошибка начала транзакции: %w", err)
	}
	defer rollbackOnFailure(ctx, tx, &err)

	if err = r.transferTx(ctx, tx, fromUserID, toUserID, amount); err != nil {
		return err
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("ошибка коммита транзакции: %w", err)
	}
	err = nil
	return nil
}

func (r *Repository) CreateTransferConfirmation(ctx context.Context, entry *transferConfirmation) error {
	if entry == nil {
		return fmt.Errorf("transfer confirmation is nil")
	}
	_, err := r.db.Exec(ctx, `
		INSERT INTO economy_transfer_confirmations (
			token, chat_id, message_id, owner_user_id, sender_user_id, target_user_id,
			amount, recipient_display, state, expires_at, consumed_at, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW())
	`, entry.Token, entry.ChatID, entry.MessageID, entry.OwnerUserID, entry.FromUserID, entry.ToUserID,
		entry.Amount, entry.RecipientDisplay, entry.State, entry.ExpiresAt, entry.ConsumedAt)
	if err != nil {
		return fmt.Errorf("create transfer confirmation: %w", err)
	}
	return nil
}

func (r *Repository) GetTransferConfirmation(ctx context.Context, token string) (*transferConfirmation, error) {
	return r.getTransferConfirmation(ctx, r.db, token)
}

func (r *Repository) TransitionTransferConfirmation(ctx context.Context, token string, fromStates []string, toState string) (bool, error) {
	cmd, err := r.db.Exec(ctx, `
		UPDATE economy_transfer_confirmations
		SET state = $3, updated_at = NOW()
		WHERE token = $1 AND state = ANY($2)
	`, token, fromStates, toState)
	if err != nil {
		return false, fmt.Errorf("transition transfer confirmation: %w", err)
	}
	return cmd.RowsAffected() == 1, nil
}

func (r *Repository) MarkTransferConfirmationExpired(ctx context.Context, token string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE economy_transfer_confirmations
		SET state = $2, updated_at = NOW()
		WHERE token = $1 AND state IN ($3, $4)
	`, token, transferStateExpired, transferStateAwaitFirst, transferStateAwaitSecond)
	if err != nil {
		return fmt.Errorf("expire transfer confirmation: %w", err)
	}
	return nil
}

func (r *Repository) ExecuteTransferConfirmation(ctx context.Context, token string, now time.Time) (entry *transferConfirmation, err error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transfer confirmation execution: %w", err)
	}
	committed := false
	defer func() {
		if committed || err == nil {
			return
		}
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
			err = errors.Join(err, fmt.Errorf("ошибка отката транзакции: %w", rollbackErr))
		}
	}()

	entry, err = r.getTransferConfirmationForUpdate(ctx, tx, token)
	if err != nil {
		return nil, err
	}

	if now.After(entry.ExpiresAt) {
		if _, err = tx.Exec(ctx, `
			UPDATE economy_transfer_confirmations
			SET state = $2, updated_at = NOW()
			WHERE token = $1
		`, token, transferStateExpired); err != nil {
			return nil, fmt.Errorf("expire transfer confirmation: %w", err)
		}
		entry.State = transferStateExpired
		if err = tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit expired transfer confirmation: %w", err)
		}
		committed = true
		return entry, ErrTransferConfirmationStateConflict
	}
	if entry.State != transferStateAwaitSecond || entry.ConsumedAt != nil {
		return entry, ErrTransferConfirmationStateConflict
	}

	consumedAt := now.UTC()
	if _, err = tx.Exec(ctx, `
		UPDATE economy_transfer_confirmations
		SET state = $2, consumed_at = $3, updated_at = NOW()
		WHERE token = $1
	`, token, transferStateExecuting, consumedAt); err != nil {
		return nil, fmt.Errorf("mark transfer confirmation executing: %w", err)
	}

	transferErr := r.transferTx(ctx, tx, entry.FromUserID, entry.ToUserID, entry.Amount)
	if transferErr != nil {
		if _, err = tx.Exec(ctx, `
			UPDATE economy_transfer_confirmations
			SET state = $2, updated_at = NOW()
			WHERE token = $1
		`, token, transferStateFailed); err != nil {
			return nil, fmt.Errorf("mark transfer confirmation failed: %w", err)
		}
		entry.State = transferStateFailed
		entry.ConsumedAt = &consumedAt
		if err = tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit failed transfer confirmation execution: %w", err)
		}
		committed = true
		return entry, normalizeTransferExecutionError(transferErr)
	}

	if _, err = tx.Exec(ctx, `
		UPDATE economy_transfer_confirmations
		SET state = $2, updated_at = NOW()
		WHERE token = $1
	`, token, transferStateCompleted); err != nil {
		return nil, fmt.Errorf("mark transfer confirmation completed: %w", err)
	}

	if err = tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit transfer confirmation execution: %w", err)
	}
	committed = true
	entry.State = transferStateCompleted
	entry.ConsumedAt = &consumedAt
	err = nil
	return entry, nil
}

func rollbackOnFailure(ctx context.Context, tx pgx.Tx, errp *error) {
	if errp == nil || *errp == nil {
		return
	}

	if rollbackErr := tx.Rollback(ctx); rollbackErr != nil {
		*errp = errors.Join(*errp, fmt.Errorf("ошибка отката транзакции: %w", rollbackErr))
	}
}

func (r *Repository) transferTx(ctx context.Context, tx pgx.Tx, fromUserID, toUserID, amount int64) error {
	if err := r.ensureBalanceRowTx(ctx, tx, fromUserID); err != nil {
		return err
	}
	if err := r.ensureBalanceRowTx(ctx, tx, toUserID); err != nil {
		return err
	}

	var senderBalance int64
	err := tx.QueryRow(ctx, `
		SELECT balance FROM balances WHERE user_id = $1 FOR UPDATE
	`, fromUserID).Scan(&senderBalance)
	if err != nil {
		return fmt.Errorf("отправитель не найден: %w", err)
	}
	if senderBalance < amount {
		return fmt.Errorf("%w: нужно %d, есть %d", ErrInsufficientFunds, amount, senderBalance)
	}

	if _, err = tx.Exec(ctx, `
		UPDATE balances
		SET balance = balance - $2, total_spent = total_spent + $2, updated_at = NOW()
		WHERE user_id = $1
	`, fromUserID, amount); err != nil {
		return fmt.Errorf("ошибка списания у отправителя: %w", err)
	}

	if _, err = tx.Exec(ctx, `
		UPDATE balances
		SET balance = balance + $2, total_earned = total_earned + $2, updated_at = NOW()
		WHERE user_id = $1
	`, toUserID, amount); err != nil {
		return fmt.Errorf("ошибка начисления получателю: %w", err)
	}

	if _, err = tx.Exec(ctx, `
		INSERT INTO transactions (from_user_id, to_user_id, amount, transaction_type, description)
		VALUES ($1, $2, $3, 'transfer', $4)
	`, fromUserID, toUserID, amount, fmt.Sprintf("Перевод %d плёнок", amount)); err != nil {
		return fmt.Errorf("ошибка записи транзакции: %w", err)
	}
	return nil
}

func (r *Repository) addBalanceTx(ctx context.Context, tx pgx.Tx, userID int64, amount int64, txType, description string) error {
	if err := r.ensureBalanceRowTx(ctx, tx, userID); err != nil {
		return err
	}

	_, err := tx.Exec(ctx, `
		UPDATE balances
		SET balance = balance + $2, total_earned = total_earned + $2, updated_at = NOW()
		WHERE user_id = $1
	`, userID, amount)
	if err != nil {
		return fmt.Errorf("ошибка начисления: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO transactions (to_user_id, amount, transaction_type, description)
		VALUES ($1, $2, $3, $4)
	`, userID, amount, txType, description)
	if err != nil {
		return fmt.Errorf("ошибка записи транзакции: %w", err)
	}

	return nil
}

func (r *Repository) AddBalanceTx(ctx context.Context, tx pgx.Tx, userID int64, amount int64, txType, description string) error {
	return r.addBalanceTx(ctx, tx, userID, amount, txType, description)
}

func (r *Repository) ensureBalanceRowTx(ctx context.Context, tx pgx.Tx, userID int64) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO balances (user_id, balance, total_earned, total_spent)
		VALUES ($1, 0, 0, 0)
		ON CONFLICT (user_id) DO NOTHING
	`, userID)
	if err != nil {
		return fmt.Errorf("ошибка подготовки баланса: %w", err)
	}
	return nil
}

type transferConfirmationQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (r *Repository) getTransferConfirmation(ctx context.Context, q transferConfirmationQuerier, token string) (*transferConfirmation, error) {
	var entry transferConfirmation
	err := q.QueryRow(ctx, `
		SELECT token, chat_id, message_id, owner_user_id, sender_user_id, target_user_id,
		       amount, recipient_display, state, created_at, expires_at, consumed_at
		FROM economy_transfer_confirmations
		WHERE token = $1
	`, token).Scan(
		&entry.Token, &entry.ChatID, &entry.MessageID, &entry.OwnerUserID, &entry.FromUserID, &entry.ToUserID,
		&entry.Amount, &entry.RecipientDisplay, &entry.State, &entry.CreatedAt, &entry.ExpiresAt, &entry.ConsumedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTransferConfirmationNotFound
		}
		return nil, fmt.Errorf("get transfer confirmation: %w", err)
	}
	return &entry, nil
}

func (r *Repository) getTransferConfirmationForUpdate(ctx context.Context, tx pgx.Tx, token string) (*transferConfirmation, error) {
	var entry transferConfirmation
	err := tx.QueryRow(ctx, `
		SELECT token, chat_id, message_id, owner_user_id, sender_user_id, target_user_id,
		       amount, recipient_display, state, created_at, expires_at, consumed_at
		FROM economy_transfer_confirmations
		WHERE token = $1
		FOR UPDATE
	`, token).Scan(
		&entry.Token, &entry.ChatID, &entry.MessageID, &entry.OwnerUserID, &entry.FromUserID, &entry.ToUserID,
		&entry.Amount, &entry.RecipientDisplay, &entry.State, &entry.CreatedAt, &entry.ExpiresAt, &entry.ConsumedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTransferConfirmationNotFound
		}
		return nil, fmt.Errorf("lock transfer confirmation: %w", err)
	}
	return &entry, nil
}

func normalizeTransferExecutionError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrInsufficientFunds):
		return common.ErrInsufficientBalance
	default:
		return err
	}
}

// GetTransactions возвращает последние N транзакций пользователя.
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ошибка итерации транзакций: %w", err)
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
func (r *Repository) EnsureBalance(ctx context.Context, userID int64) error {
	return r.CreateBalance(ctx, userID)
}

// GetBalanceCreatedAt возвращает время создания баланса.
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate transactions by period: %w", err)
	}
	return txs, nil
}
