.PHONY: build run test clean docker-up docker-down migrate hash deps arch-check vet ci

# Сборка бинарника
build:
	@echo "=== Сборка ==="
	CGO_ENABLED=0 go build -ldflags="-s -w" -o bot ./cmd/bot

# Запуск бота (из .env)
run: build
	@echo "=== Запуск ==="
	./bot

# Тесты
test:
	go test ./...

# Очистка
clean:
	rm -f bot
	go clean

# Docker
docker-up:
	cd deploy && docker-compose up -d --build

docker-down:
	cd deploy && docker-compose down

docker-logs:
	cd deploy && docker-compose logs -f bot

# Миграции вручную
migrate:
	bash scripts/migrate.sh

# Генерация хеша пароля
hash:
	@read -p "Введите пароль: " pwd; \
	go run scripts/generate_hash.go "$$pwd"

# Загрузка зависимостей
deps:
	go mod download
	go mod tidy

# Проверка архитектурных импортов
arch-check:
	./scripts/check_arch_imports.sh

# Локальный CI прогон
ci: arch-check vet test

# Статический анализ
vet:
	go vet ./...
