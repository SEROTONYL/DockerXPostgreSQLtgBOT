.PHONY: build run test test-race lint lint-install clean docker-up docker-down migrate hash deps arch-check vet ci

BIN := $(CURDIR)/bin
GOLANGCI_LINT := $(BIN)/golangci-lint
GOLANGCI_LINT_VERSION := v2.10.1
GOLANGCI_LINT_VERSION_NUMBER := $(patsubst v%,%,$(GOLANGCI_LINT_VERSION))

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

test-race:
	go test -race ./...

lint: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run ./...

lint-install:
	@mkdir -p $(BIN)
	@OS=$$(uname -s | tr '[:upper:]' '[:lower:]'); \
	ARCH=$$(uname -m); \
	case "$$ARCH" in \
		x86_64) ARCH=amd64 ;; \
		aarch64|arm64) ARCH=arm64 ;; \
		*) echo "Unsupported architecture: $$ARCH"; exit 1 ;; \
	esac; \
	URL="https://github.com/golangci/golangci-lint/releases/download/$(GOLANGCI_LINT_VERSION)/golangci-lint-$(GOLANGCI_LINT_VERSION_NUMBER)-$$OS-$$ARCH.tar.gz"; \
	echo "Installing golangci-lint $(GOLANGCI_LINT_VERSION) from $$URL"; \
	curl -sSfL "$$URL" | tar -xz -C $(BIN) --strip-components=1 "golangci-lint-$(GOLANGCI_LINT_VERSION_NUMBER)-$$OS-$$ARCH/golangci-lint"; \
	chmod +x $(GOLANGCI_LINT)

$(GOLANGCI_LINT):
	@$(MAKE) lint-install

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
ci: test test-race lint

# Статический анализ
vet:
	go vet ./...
