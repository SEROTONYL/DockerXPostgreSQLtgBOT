#!/bin/bash
# build.sh — сборка бота
set -e

echo "=== Сборка Telegram Bot ==="

# Проверяем Go
if ! command -v go &> /dev/null; then
    echo "Go не найден. Установите Go 1.22+"
    exit 1
fi

# Скачиваем зависимости
echo "Скачиваем зависимости..."
go mod download

# Собираем бинарник
echo "Собираем бинарник..."
CGO_ENABLED=0 go build -ldflags="-s -w" -o bot ./cmd/bot

echo "=== Готово: ./bot ==="
