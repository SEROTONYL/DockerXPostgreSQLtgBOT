package migrations

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	log "github.com/sirupsen/logrus"
)

const migrationsGlob = "migrations/*.sql"

var upMigrationPattern = regexp.MustCompile(`^\d{4}_.+\.sql$`)

type migrationFile struct {
	version  int
	filename string
	path     string
}

func Apply(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TIMESTAMP NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	files, err := collectMigrationFiles(migrationsGlob)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		log.Info("no up migrations found")
		return nil
	}

	applied, err := loadAppliedVersions(ctx, pool)
	if err != nil {
		return err
	}

	pending := filterPending(files, applied)
	for _, m := range pending {
		log.Infof("apply migration %s", m.filename)

		sqlBytes, err := os.ReadFile(m.path)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", m.filename, err)
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("apply migration %04d (%s): %w", m.version, m.filename, err)
		}

		if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply migration %04d (%s): %w", m.version, m.filename, err)
		}

		if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations(version) VALUES ($1)", m.version); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply migration %04d (%s): %w", m.version, m.filename, err)
		}

		if err := tx.Commit(ctx); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("apply migration %04d (%s): %w", m.version, m.filename, err)
		}
	}

	return nil
}

func collectMigrationFiles(pattern string) ([]migrationFile, error) {
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("read migration files: %w", err)
	}

	names := make([]string, 0, len(paths))
	pathByName := make(map[string]string, len(paths))
	for _, path := range paths {
		name := filepath.Base(path)
		names = append(names, name)
		pathByName[name] = path
	}

	files, err := buildMigrationFiles(names)
	if err != nil {
		return nil, err
	}

	for i := range files {
		files[i].path = pathByName[files[i].filename]
	}

	return files, nil
}

func buildMigrationFiles(filenames []string) ([]migrationFile, error) {
	files := make([]migrationFile, 0, len(filenames))
	versions := make(map[int]string)

	for _, name := range filenames {
		version, ok := isUpMigrationFile(name)
		if !ok {
			continue
		}

		if existing, dup := versions[version]; dup {
			return nil, fmt.Errorf("duplicate migration version %04d: %s and %s", version, existing, name)
		}

		versions[version] = name
		files = append(files, migrationFile{version: version, filename: name})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].version < files[j].version
	})

	return files, nil
}

func loadAppliedVersions(ctx context.Context, pool *pgxpool.Pool) (map[int]struct{}, error) {
	rows, err := pool.Query(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("select applied migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[int]struct{})
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, fmt.Errorf("scan applied migration: %w", err)
		}
		applied[version] = struct{}{}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate applied migrations: %w", err)
	}

	return applied, nil
}

func filterPending(files []migrationFile, applied map[int]struct{}) []migrationFile {
	pending := make([]migrationFile, 0, len(files))
	for _, f := range files {
		if _, ok := applied[f.version]; ok {
			continue
		}
		pending = append(pending, f)
	}
	return pending
}

func isUpMigrationFile(name string) (int, bool) {
	if strings.Contains(name, string(filepath.Separator)) || strings.Contains(name, "/") {
		return 0, false
	}
	if strings.Contains(name, ".down.") {
		return 0, false
	}
	if !upMigrationPattern.MatchString(name) {
		return 0, false
	}

	version, err := strconv.Atoi(name[:4])
	if err != nil || version <= 0 {
		return 0, false
	}

	return version, true
}
