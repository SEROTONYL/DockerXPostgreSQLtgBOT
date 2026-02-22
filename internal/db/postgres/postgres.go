// Package postgres управляет подключением к базе данных PostgreSQL.
// Используется пул соединений pgxpool для работы с несколькими горутинами одновременно.
package postgres

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	log "github.com/sirupsen/logrus"

	"serotonyl.ru/telegram-bot/internal/config"
)

const (
	defaultConnectTimeout = 5 * time.Second
	defaultPingTimeout    = 5 * time.Second
	defaultMigTimeout     = 30 * time.Second
)

// NewPool создаёт новый пул соединений к PostgreSQL.
func NewPool(ctx context.Context, cfg *config.Config) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseDSN())
	if err != nil {
		return nil, fmt.Errorf("ошибка парсинга DSN: %w", err)
	}

	poolConfig.MaxConns = cfg.DBMaxConns
	poolConfig.MinConns = cfg.DBMinConns
	poolConfig.MaxConnLifetime = 1 * time.Hour
	poolConfig.MaxConnIdleTime = 30 * time.Minute
	poolConfig.HealthCheckPeriod = 1 * time.Minute

	// Таймаут на установку соединения (важно при сетевых проблемах)
	if poolConfig.ConnConfig != nil {
		poolConfig.ConnConfig.ConnectTimeout = defaultConnectTimeout
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания пула: %w", err)
	}

	// Ping с таймаутом
	pingCtx, cancel := context.WithTimeout(ctx, defaultPingTimeout)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("база данных недоступна: %w", err)
	}

	log.Info("Подключение к PostgreSQL установлено")
	return pool, nil
}

// RunMigrations применяет .sql миграции из папки migrationsPath.
// Формат имени: <версия>_<описание>.sql, например: 1_init.sql, 2_add_members.sql
func RunMigrations(ctx context.Context, pool *pgxpool.Pool, migrationsPath string) error {
	// 1) гарантируем таблицу schema_migrations
	migCtx, cancel := context.WithTimeout(ctx, defaultMigTimeout)
	defer cancel()

	if _, err := pool.Exec(migCtx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TIMESTAMP DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("ошибка создания таблицы миграций: %w", err)
	}

	entries, err := os.ReadDir(migrationsPath)
	if err != nil {
		return fmt.Errorf("не удалось прочитать папку миграций %q: %w", migrationsPath, err)
	}

	type mig struct {
		version int
		path    string
	}
	migrations := make([]mig, 0, len(entries))

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".sql") {
			continue
		}

		v, ok := parseMigrationVersion(name)
		if !ok {
			return fmt.Errorf("не удалось извлечь версию миграции из имени файла: %s", name)
		}
		migrations = append(migrations, mig{
			version: v,
			path:    filepath.Join(migrationsPath, name),
		})
	}

	sort.Slice(migrations, func(i, j int) bool { return migrations[i].version < migrations[j].version })

	for _, m := range migrations {
		sqlBytes, err := os.ReadFile(m.path)
		if err != nil {
			return fmt.Errorf("не удалось прочитать миграцию %s: %w", m.path, err)
		}

		oneCtx, cancelOne := context.WithTimeout(ctx, defaultMigTimeout)
		err = ExecMigrationSQL(oneCtx, pool, m.version, string(sqlBytes))
		cancelOne()
		if err != nil {
			return fmt.Errorf("ошибка применения миграции v%d (%s): %w", m.version, m.path, err)
		}
	}

	log.WithField("count", len(migrations)).Info("Миграции применены")
	return nil
}

func parseMigrationVersion(filename string) (int, bool) {
	base := filename
	if i := strings.IndexByte(base, '_'); i >= 0 {
		base = base[:i]
	} else if i := strings.IndexByte(base, '.'); i >= 0 {
		base = base[:i]
	}
	if base == "" {
		return 0, false
	}
	v, err := strconv.Atoi(base)
	if err != nil || v <= 0 {
		return 0, false
	}
	return v, true
}