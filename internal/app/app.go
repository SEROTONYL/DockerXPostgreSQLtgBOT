// Package app инициализирует все компоненты приложения.
// app.go — точка сборки: создаёт БД-пул, репозитории, сервисы, обработчики,
// фильтры и собирает всё в один объект Bot.
package app

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	log "github.com/sirupsen/logrus"

	"serotonyl.ru/telegram-bot/internal/bot"
	"serotonyl.ru/telegram-bot/internal/bot/filters"
	"serotonyl.ru/telegram-bot/internal/commands"
	"serotonyl.ru/telegram-bot/internal/config"
	"serotonyl.ru/telegram-bot/internal/db/migrations"
	"serotonyl.ru/telegram-bot/internal/db/postgres"
	"serotonyl.ru/telegram-bot/internal/feature"
	"serotonyl.ru/telegram-bot/internal/features/admin"
	"serotonyl.ru/telegram-bot/internal/features/casino"
	"serotonyl.ru/telegram-bot/internal/features/core"
	"serotonyl.ru/telegram-bot/internal/features/debts"
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
	botAPI, err := telegram.NewRawBot(cfg.TelegramBotToken)
	if err != nil {
		return nil, fmt.Errorf("ошибка создания Telegram API: %w", err)
	}

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

	// === 5. Telegram adapter/ops ===
	tgClient := telegram.NewBotClient(botAPI)
	tgOps := telegram.NewOpsWithLogger(tgClient, log.NewEntry(log.StandardLogger()))
	me, err := tgOps.GetMe(ctx)
	if err != nil {
		return nil, fmt.Errorf("ошибка getMe Telegram API: %w", err)
	}
	log.Infof("Авторизован как @%s", me.Username)

	// === 6. Обработчики ===
	economyHandler := economy.NewHandler(economyService, memberService, tgOps)
	streakHandler := streak.NewHandler(streakService, tgOps, cfg)
	karmaHandler := karma.NewHandler(karmaService, tgOps)
	casinoHandler := casino.NewHandler(casinoService, tgOps)
	adminHandler := admin.NewHandler(adminService, memberService, economyService, tgOps)

	// === 6. Фильтры ===
	chatFilter := filters.NewChatFilter(cfg.FloodChatID, cfg.AdminChatID, memberService, tgOps)

	// === 7. Роутер и регистрация команд через фичи ===
	cmdRouter := commands.NewRouter()
	var scheduler *jobs.Scheduler
	features := []feature.Feature{
		core.NewFeature(tgOps),
		admin.NewFeature(cfg, tgOps, adminHandler, memberService, func() jobs.PurgeMetrics {
			if scheduler == nil {
				return jobs.PurgeMetrics{}
			}
			return scheduler.GetPurgeMetrics()
		}),
		economy.NewFeature(economyHandler, cfg),
		karma.NewFeature(karmaHandler, cfg),
		streak.NewFeature(streakHandler, cfg),
		casino.NewFeature(casinoHandler, cfg),
		members.NewFeature(),
		debts.NewFeature(),
	}
	for _, f := range features {
		f.RegisterCommands(cmdRouter)
	}

	// === 8. Собираем бота ===
	b := bot.New(bot.Deps{
		Ops:            tgOps,
		CmdRouter:      cmdRouter,
		Cfg:            cfg,
		MemberService:  memberService,
		EconomyService: economyService,
		EconomyHandler: economyHandler,
		StreakService:  streakService,
		StreakHandler:  streakHandler,
		KarmaService:   karmaService,
		KarmaHandler:   karmaHandler,
		CasinoHandler:  casinoHandler,
		AdminHandler:   adminHandler,
		ChatFilter:     chatFilter,
		IsThankYou:     karma.IsThankYou,
	})

	// === 9. Планировщик задач ===
	scheduler = jobs.NewScheduler(streakService, memberService, b.SendMessageToUser)
	b.SetPurgeMetricsProvider(scheduler)

	return &App{
		Bot:       b,
		Scheduler: scheduler,
		DB:        pool,
	}, nil
}

// runMigrations выполняет все SQL-миграции.
func runMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	return migrations.Apply(ctx, pool)
}
