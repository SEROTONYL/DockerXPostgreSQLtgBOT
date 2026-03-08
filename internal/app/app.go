// Package app инициализирует все компоненты приложения.
package app

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"serotonyl.ru/telegram-bot/internal/app/modules"
	"serotonyl.ru/telegram-bot/internal/bot"
	"serotonyl.ru/telegram-bot/internal/commands"
	"serotonyl.ru/telegram-bot/internal/config"
	"serotonyl.ru/telegram-bot/internal/features/admin"
	"serotonyl.ru/telegram-bot/internal/features/casino"
	"serotonyl.ru/telegram-bot/internal/features/economy"
	"serotonyl.ru/telegram-bot/internal/features/karma"
	"serotonyl.ru/telegram-bot/internal/features/members"
	"serotonyl.ru/telegram-bot/internal/features/streak"
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
	adminModule, err := admin.NewModule(admin.Deps{
		Cfg:            cfg,
		Ops:            tg.Ops,
		Service:        infra.AdminService,
		MemberService:  infra.MemberService,
		EconomyService: infra.EconomyService,
		PurgeMetrics: func() jobs.PurgeMetrics {
			if scheduler == nil {
				return jobs.PurgeMetrics{}
			}
			return scheduler.GetPurgeMetrics()
		},
	})
	if err != nil {
		return nil, err
	}

	economyModule, err := economy.NewModule(economy.Deps{Cfg: cfg, Ops: tg.Ops, Service: infra.EconomyService, MemberService: infra.MemberService})
	if err != nil {
		return nil, err
	}

	streakModule, err := streak.NewModule(streak.Deps{Cfg: cfg, Ops: tg.Ops, Service: infra.StreakService})
	if err != nil {
		return nil, err
	}

	karmaModule, err := karma.NewModule(karma.Deps{Cfg: cfg, Ops: tg.Ops, Service: infra.KarmaService})
	if err != nil {
		return nil, err
	}

	membersModule, err := members.NewModule(members.Deps{Cfg: cfg, Ops: tg.Ops, Service: infra.MemberService, Economy: infra.EconomyService})
	if err != nil {
		return nil, err
	}

	casinoModule, err := casino.NewModule(casino.Deps{Cfg: cfg, Ops: tg.Ops, Service: infra.CasinoService})
	if err != nil {
		return nil, err
	}

	cmdRouter := commands.NewRouter()
	economy.RegisterCommands(cmdRouter, economyModule.Handler, cfg)
	karma.RegisterCommands(cmdRouter, karmaModule.Handler, cfg)
	streak.RegisterCommands(cmdRouter, streakModule.Handler, cfg)
	casino.RegisterCommands(cmdRouter, casinoModule.Handler, cfg)
	membersModule.Feature.RegisterCommands(cmdRouter)

	chatFilter := modules.BuildChatFilter(cfg, infra, tg)
	b := modules.BuildBot(cfg, infra, tg, cmdRouter, chatFilter, modules.BotHandlers{
		Admin:   adminModule.Handler,
		Members: membersModule.Handler,
		Karma:   karmaModule.Handler,
	}, modules.KarmaClassifier{Match: karma.IsThankYou})

	scheduler = modules.BuildScheduler(cfg, infra, tg, b)
	b.SetPurgeMetricsProvider(scheduler)

	return &App{Bot: b, Scheduler: scheduler, DB: infra.DB}, nil
}
