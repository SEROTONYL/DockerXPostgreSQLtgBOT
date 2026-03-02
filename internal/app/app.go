// Package app инициализирует все компоненты приложения.
// app.go — точка сборки: создаёт БД-пул, репозитории, сервисы, обработчики,
// фильтры и собирает всё в один объект Bot.
package app

import (
	"context"
	"fmt"

	botapi "github.com/go-telegram/bot"
	"github.com/jackc/pgx/v5/pgxpool"
	log "github.com/sirupsen/logrus"

	"serotonyl.ru/telegram-bot/internal/bot"
	"serotonyl.ru/telegram-bot/internal/bot/filters"
	"serotonyl.ru/telegram-bot/internal/config"
	"serotonyl.ru/telegram-bot/internal/db/migrations"
	"serotonyl.ru/telegram-bot/internal/db/postgres"
	"serotonyl.ru/telegram-bot/internal/features/admin"
	"serotonyl.ru/telegram-bot/internal/features/casino"
	"serotonyl.ru/telegram-bot/internal/features/economy"
	"serotonyl.ru/telegram-bot/internal/features/karma"
	"serotonyl.ru/telegram-bot/internal/features/members"
	"serotonyl.ru/telegram-bot/internal/features/streak"
	"serotonyl.ru/telegram-bot/internal/jobs"
	"serotonyl.ru/telegram-bot/internal/telegram"
)

// App содержит все компоненты приложения.
type App struct {
	Bot       *bot.Bot
	Scheduler *jobs.Scheduler
	DB        *pgxpool.Pool
	BotAPI    *botapi.Bot
}

// New создаёт и инициализирует приложение.
// Порядок инициализации важен — компоненты зависят друг от друга.
func New(ctx context.Context, cfg *config.Config) (*App, error) {
	// === 1. База данных ===
	pool, err := postgres.NewPool(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("ошибка подключения к БД: %w", err)
	}

	// Запускаем миграции
	if err := runMigrations(ctx, pool); err != nil {
		return nil, fmt.Errorf("ошибка миграций: %w", err)
	}

	// === 2. Telegram Bot API ===
	botAPI, err := botapi.New(cfg.TelegramBotToken)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания Telegram API: %w", err)
	}
	me, err := botAPI.GetMe(ctx)
	if err != nil {
		return nil, fmt.Errorf("ошибка getMe Telegram API: %w", err)
	}
	log.Infof("Авторизован как @%s", me.Username)

	// === 3. Репозитории ===
	memberRepo := members.NewRepository(pool)
	economyRepo := economy.NewRepository(pool)
	streakRepo := streak.NewRepository(pool)
	karmaRepo := karma.NewRepository(pool)
	casinoRepo := casino.NewRepository(pool)
	adminRepo := admin.NewRepository(pool)

	// === 4. Сервисы ===
	memberService := members.NewService(memberRepo)
	economyService := economy.NewService(economyRepo)
	streakService := streak.NewService(streakRepo, economyService, cfg)
	karmaService := karma.NewService(karmaRepo, cfg)
	casinoService := casino.NewService(casinoRepo, economyService, cfg)
	adminService := admin.NewService(adminRepo, memberRepo, cfg)

	// === 5. Telegram adapter ===
	tg := telegram.NewAdapter(botAPI)

	// === 6. Обработчики ===
	memberHandler := members.NewHandler(memberService)
	economyHandler := economy.NewHandler(economyService, memberService, tg)
	streakHandler := streak.NewHandler(streakService, tg, cfg)
	karmaHandler := karma.NewHandler(karmaService, tg)
	casinoHandler := casino.NewHandler(casinoService, tg)
	adminHandler := admin.NewHandler(adminService, memberService, economyService, tg)

	// === 6. Фильтры ===
	chatFilter := filters.NewChatFilter(cfg.FloodChatID, memberService, tg)

	// === 7. Собираем бота ===
	b := bot.New(
		botAPI, tg, cfg,
		memberService, memberHandler,
		economyService, economyHandler,
		streakService, streakHandler,
		karmaService, karmaHandler,
		casinoService, casinoHandler,
		adminService, adminHandler,
		chatFilter,
	)

	// === 8. Планировщик задач ===
	scheduler := jobs.NewScheduler(streakService, b.SendMessageToUser)

	return &App{
		Bot:       b,
		Scheduler: scheduler,
		DB:        pool,
		BotAPI:    botAPI,
	}, nil
}

// runMigrations выполняет все SQL-миграции.
func runMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	return migrations.Apply(ctx, pool)
}
