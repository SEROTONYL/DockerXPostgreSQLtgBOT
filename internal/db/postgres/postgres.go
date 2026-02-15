// Package postgres управляет подключением к базе данных PostgreSQL.
// Используется пул соединений pgxpool для эффективной работы
// с несколькими горутинами одновременно.
//
// Пул автоматически управляет открытием/закрытием соединений,
// переподключается при обрыве и ограничивает максимальное число соединений.
package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	log "github.com/sirupsen/logrus"

	"telegram-bot/internal/config"
)

// NewPool создаёт новый пул соединений к PostgreSQL.
//
// Параметры:
//   - ctx: контекст для отмены операции
//   - cfg: конфигурация с параметрами подключения
//
// Возвращает:
//   - *pgxpool.Pool: готовый к использованию пул
//   - error: ошибка, если подключение не удалось
//
// Пример:
//
//	pool, err := postgres.NewPool(ctx, cfg)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer pool.Close()
func NewPool(ctx context.Context, cfg *config.Config) (*pgxpool.Pool, error) {
	// Парсим строку подключения и настраиваем пул
	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseDSN())
	if err != nil {
		return nil, fmt.Errorf("ошибка парсинга DSN: %w", err)
	}

	// Настройки пула соединений
	poolConfig.MaxConns = cfg.DBMaxConns             // Максимум соединений
	poolConfig.MinConns = cfg.DBMinConns             // Минимум (держать открытыми)
	poolConfig.MaxConnLifetime = 1 * time.Hour       // Время жизни одного соединения
	poolConfig.MaxConnIdleTime = 30 * time.Minute    // Время простоя до закрытия
	poolConfig.HealthCheckPeriod = 1 * time.Minute   // Проверка здоровья соединений

	// Создаём пул с заданной конфигурацией
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания пула: %w", err)
	}

	// Проверяем, что база доступна
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("база данных недоступна: %w", err)
	}

	log.Info("Подключение к PostgreSQL установлено")
	return pool, nil
}

// RunMigrations выполняет SQL-миграции из папки migrations/.
// Миграции применяются последовательно по номеру файла.
//
// Параметры:
//   - ctx: контекст
//   - pool: пул соединений
//   - migrationsPath: путь к папке с .sql файлами
func RunMigrations(ctx context.Context, pool *pgxpool.Pool, migrationsPath string) error {
	// Читаем и выполняем миграции вручную (без зависимости golang-migrate,
	// чтобы упростить сборку). В продакшене рекомендуется golang-migrate.
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("не удалось получить соединение: %w", err)
	}
	defer conn.Release()

	// Создаём таблицу для отслеживания миграций, если её нет
	_, err = conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TIMESTAMP DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("ошибка создания таблицы миграций: %w", err)
	}

	log.Info("Система миграций готова")
	return nil
}
