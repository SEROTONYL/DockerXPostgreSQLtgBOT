// Package postgres — вспомогательные функции для работы с БД.
// queries.go содержит общие утилиты для выполнения запросов.
package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ExecMigrationSQL выполняет один SQL-запрос миграции в транзакции.
// Если запрос упадёт — транзакция откатится автоматически.
//
// Параметры:
//   - ctx: контекст
//   - pool: пул соединений
//   - version: номер миграции (для записи в schema_migrations)
//   - sql: SQL-код миграции
func ExecMigrationSQL(ctx context.Context, pool *pgxpool.Pool, version int, sql string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("ошибка начала транзакции: %w", err)
	}
	// Откатываем транзакцию, если что-то пошло не так
	defer tx.Rollback(ctx)

	// Проверяем, не была ли эта миграция уже применена
	var exists bool
	err = tx.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)", version,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("ошибка проверки миграции: %w", err)
	}
	if exists {
		// Миграция уже применена — пропускаем
		return nil
	}

	// Выполняем SQL миграции
	if _, err := tx.Exec(ctx, sql); err != nil {
		return fmt.Errorf("ошибка выполнения миграции %d: %w", version, err)
	}

	// Записываем версию миграции
	if _, err := tx.Exec(ctx,
		"INSERT INTO schema_migrations (version) VALUES ($1)", version,
	); err != nil {
		return fmt.Errorf("ошибка записи версии миграции: %w", err)
	}

	// Фиксируем транзакцию
	return tx.Commit(ctx)
}
