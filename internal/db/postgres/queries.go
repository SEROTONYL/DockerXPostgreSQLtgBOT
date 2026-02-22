// Package postgres — вспомогательные функции для работы с БД.
// queries.go содержит общие утилиты для выполнения запросов.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ExecMigrationSQL выполняет один SQL-запрос миграции в транзакции.
func ExecMigrationSQL(ctx context.Context, pool *pgxpool.Pool, version int, sql string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("ошибка начала транзакции: %w", err)
	}
	defer func() {
		// Rollback после Commit вернёт ErrTxClosed — это нормально.
		if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			// логера тут нет — игнорируем
		}
	}()

	var exists bool
	err = tx.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)", version,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("ошибка проверки миграции: %w", err)
	}
	if exists {
		return nil
	}

	if _, err := tx.Exec(ctx, sql); err != nil {
		return fmt.Errorf("ошибка выполнения миграции %d: %w", version, err)
	}

	if _, err := tx.Exec(ctx,
		"INSERT INTO schema_migrations (version) VALUES ($1)", version,
	); err != nil {
		return fmt.Errorf("ошибка записи версии миграции: %w", err)
	}

	return tx.Commit(ctx)
}