# =======================
# Этап 1: Сборка (Builder)
# =======================
FROM golang:1.22-alpine AS builder

# Установливаем зависимости для сборки
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# Копируем go.mod и go.sum для кеширования зависимостей
COPY go.mod go.sum ./

# Скачиваем зависимости
RUN go mod download

# Копируем весь исходный код
COPY . .

# Собираем бинарник
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bot ./cmd/bot

# =======================
# Этап 2: Финальный образ
# =======================
FROM alpine:3.19

# Устанавливаем CA-сертификаты и часовой пояс
RUN apk add --no-cache ca-certificates tzdata

# Устанавливаем московский часовой пояс
ENV TZ=Europe/Moscow

WORKDIR /app

# Копируем бинарник из builder
COPY --from=builder /bot .

# Копируем миграции
COPY migrations/ ./migrations/

# Запускаем бота
CMD ["sh", "-c", "echo '=== Environment ===' && env && echo '=== Starting bot ===' && ./bot 2>&1 || echo 'Exit code:' $?"]