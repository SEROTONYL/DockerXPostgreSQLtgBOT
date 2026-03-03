#!/bin/bash
# migrate.sh — выполнение up-миграций через psql
set -euo pipefail

DB_HOST=${DB_HOST:-localhost}
DB_PORT=${DB_PORT:-5432}
DB_USER=${DB_USER:-botuser}
DB_NAME=${DB_NAME:-telegram_bot}

run_psql() {
    local sql_file=$1
    echo "Применяю: $sql_file"

    if [[ -n "${DATABASE_URL:-}" ]]; then
        PGPASSWORD="${DB_PASSWORD:-}" psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f "$sql_file"
        return
    fi

    PGPASSWORD="${DB_PASSWORD:-}" psql \
        -h "$DB_HOST" \
        -p "$DB_PORT" \
        -U "$DB_USER" \
        -d "$DB_NAME" \
        -v ON_ERROR_STOP=1 \
        -f "$sql_file"
}

echo "=== Миграции БД ==="

mapfile -t migration_files < <(find migrations -maxdepth 1 -type f -name '[0-9][0-9][0-9][0-9]_*.sql' | sort)

if [[ ${#migration_files[@]} -eq 0 ]]; then
    echo "Up-миграции не найдены, пропускаю."
    exit 0
fi

for f in "${migration_files[@]}"; do
    run_psql "$f"
done

echo "=== Миграции применены ==="
