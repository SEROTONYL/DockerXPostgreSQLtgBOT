#!/bin/bash
# migrate.sh — выполнение миграций через psql
set -e

DB_HOST=${DB_HOST:-localhost}
DB_PORT=${DB_PORT:-5432}
DB_USER=${DB_USER:-botuser}
DB_NAME=${DB_NAME:-telegram_bot}

echo "=== Миграции БД ==="

for f in migrations/*.up.sql; do
    echo "Применяю: $f"
    PGPASSWORD=$DB_PASSWORD psql -h $DB_HOST -p $DB_PORT -U $DB_USER -d $DB_NAME -f "$f"
done

echo "=== Миграции применены ==="
