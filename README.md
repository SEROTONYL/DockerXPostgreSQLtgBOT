# DockerXPostgreSQLtgBOT

Telegram-бот на Go с PostgreSQL для чата сообщества (экономика, стрики, карма, казино, админ-функции). Проект построен вокруг слоистой архитектуры (`cmd -> internal/app -> internal/bot|internal/commands|internal/features|internal/telegram`) и проверяет архитектурные границы отдельным скриптом импортов.

## Features

Фичи в `internal/features/*`:

- `admin` — админ-авторизация и сервисные команды (`members_status`).
- `economy` — баланс/переводы/транзакции.
- `karma` — механика благодарностей и лимитов.
- `streak` — учёт дневной активности и наград.
- `casino` — слот-механика.
- `members`, `debts`, `core` — есть как feature-слой/контракты, но сейчас без регистрации пользовательских команд в runtime (пустой `RegisterCommands`).

## Architecture

Подробные правила: [ARCHITECTURE.md](./ARCHITECTURE.md).

Кратко по слоям:

- `cmd/*` — entrypoint.
- `internal/app` — composition root (wiring модулей и зависимостей).
- `internal/bot` + `internal/commands` — runtime обработки апдейтов и роутинг команд.
- `internal/features/*` — бизнес-фичи.
- `internal/telegram` — шлюз к Telegram API.

Проверка архитектурных импортов запускается командой:

```bash
scripts/check_arch_imports.sh
```

или через `make arch-check` / `make ci`.

## Requirements

- Go `1.25.7` (см. `go.mod`, требуется библиотекой Telegram `github.com/mymmrac/telego`)
- PostgreSQL `16` (образ в `deploy/docker-compose.yml`)
- Docker + Docker Compose (для запуска через compose)

## Quickstart (Local)

1) Подготовь `.env`:

```bash
cp .env.example .env
```

2) Сгенерируй hash пароля админа и подставь в `.env` (`ADMIN_PASSWORD_HASH`):

```bash
make hash
```

3) Запуск без Docker (локальные Go + Postgres):

```bash
make run
```

4) Запуск через Docker Compose:

```bash
cp .env.example deploy/.env
# для compose выставь DB_HOST=db (имя сервиса БД в deploy/docker-compose.yml)
make docker-up
```


## Telegram library

- Проект использует `github.com/mymmrac/telego` (Bot API v9.5+).
- Режим получения апдейтов остаётся прежним: long polling через `internal/telegram` runtime-адаптер.
- Переменные окружения и базовая настройка запуска не менялись (тот же `TELEGRAM_BOT_TOKEN`).

## Useful commands

- `make build` — сборка `./bot`
- `make run` — сборка и запуск
- `make test` — `go test ./...`
- `make vet` — `go vet ./...`
- `make arch-check` — проверка архитектурных импортов
- `make ci` — `arch-check + vet + test`
- `make migrate` — применить SQL-миграции из `migrations/`
- `make docker-up` / `make docker-down` / `make docker-logs`

## Development workflow

1. Добавь/измени фичу в `internal/features/<name>`.
2. Подключи модуль и регистрацию команд в `internal/app/app.go` (wiring через `commands.Router`).
3. Проверь границы импортов и тесты:

```bash
make ci
```

Архитектурные ограничения (кратко):

- `internal/bot` не импортирует `internal/features/*`.
- Репозиторные слои (`internal/repo|storage|db`) не импортируют `internal/bot`/`internal/telegram`.
- Telegram-взаимодействие идёт через `internal/telegram`.

См. детали в [ARCHITECTURE.md](./ARCHITECTURE.md).

## Deployment

В репозитории есть Docker-артефакты:

- `deploy/docker-compose.yml`
- `deploy/Dockerfile`

Отдельной production-инструкции (systemd/k8s/terraform) в репозитории сейчас нет.

## Contributing

Перед PR прогоняй:

```bash
make ci
```

Это же используется в GitHub Actions (`.github/workflows/ci.yml`).
