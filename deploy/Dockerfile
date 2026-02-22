FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bot ./cmd/bot

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata

ENV TZ=Europe/Moscow

RUN addgroup -S app && adduser -S app -G app
USER app

WORKDIR /app
COPY --from=builder /bot .
COPY migrations/ ./migrations/

CMD ["./bot"]