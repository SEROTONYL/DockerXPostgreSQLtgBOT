// Package app инициализирует все компоненты приложения.
package app

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"serotonyl.ru/telegram-bot/internal/app/modules"
	"serotonyl.ru/telegram-bot/internal/bot"
	"serotonyl.ru/telegram-bot/internal/commands"
	"serotonyl.ru/telegram-bot/internal/config"
	"serotonyl.ru/telegram-bot/internal/feature"
	"serotonyl.ru/telegram-bot/internal/jobs"
)

// App содержит все компоненты приложения.
type App struct {
	Bot       *bot.Bot
	Scheduler *jobs.Scheduler
	DB        *pgxpool.Pool
}

// New создаёт и инициализирует приложение.
func New(ctx context.Context, cfg *config.Config) (*App, error) {
	infra, err := modules.BuildInfra(ctx, cfg)
	if err != nil {
		return nil, err
	}

	tg, err := modules.BuildTelegram(ctx, cfg)
	if err != nil {
		return nil, err
	}

	var scheduler *jobs.Scheduler
	adminModule := modules.BuildAdminModule(cfg, infra, tg, func() jobs.PurgeMetrics {
		if scheduler == nil {
			return jobs.PurgeMetrics{}
		}
		return scheduler.GetPurgeMetrics()
	})
	memberModule := modules.BuildMemberModule(cfg, infra, tg)
	streakModule := modules.BuildStreakModule(cfg, infra, tg)
	karmaModule := modules.BuildKarmaModule(cfg, infra, tg)
	casinoModule := modules.BuildCasinoModule(cfg, infra, tg)
	coreModule := modules.BuildCoreModule(tg)

	features := []feature.Feature{
		coreModule.CoreFeature,
		adminModule.Feature,
		memberModule.EconomyFeature,
		karmaModule.Feature,
		streakModule.Feature,
		casinoModule.Feature,
		memberModule.MemberFeature,
		coreModule.DebtsFeature,
	}

	cmdRouter := commands.NewRouter()
	for _, f := range features {
		f.RegisterCommands(cmdRouter)
	}

	chatFilter := modules.BuildChatFilter(cfg, infra, tg)
	b := modules.BuildBot(cfg, infra, tg, cmdRouter, chatFilter, adminModule, memberModule, streakModule, karmaModule, casinoModule)

	scheduler = modules.BuildScheduler(infra, b)
	b.SetPurgeMetricsProvider(scheduler)

	return &App{Bot: b, Scheduler: scheduler, DB: infra.DB}, nil
}
