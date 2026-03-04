package modules

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"serotonyl.ru/telegram-bot/internal/config"
	"serotonyl.ru/telegram-bot/internal/db/migrations"
	"serotonyl.ru/telegram-bot/internal/db/postgres"
	"serotonyl.ru/telegram-bot/internal/features/admin"
	"serotonyl.ru/telegram-bot/internal/features/casino"
	"serotonyl.ru/telegram-bot/internal/features/economy"
	"serotonyl.ru/telegram-bot/internal/features/karma"
	"serotonyl.ru/telegram-bot/internal/features/members"
	"serotonyl.ru/telegram-bot/internal/features/streak"
)

type Infra struct {
	DB *pgxpool.Pool

	MemberRepo  *members.Repository
	EconomyRepo *economy.Repository
	StreakRepo  *streak.Repository
	KarmaRepo   *karma.Repository
	CasinoRepo  *casino.Repository
	AdminRepo   *admin.Repository

	MemberService  *members.Service
	EconomyService *economy.Service
	StreakService  *streak.Service
	KarmaService   *karma.Service
	CasinoService  *casino.Service
	AdminService   *admin.Service
}

func BuildInfra(ctx context.Context, cfg *config.Config) (*Infra, error) {
	pool, err := postgres.NewPool(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("ошибка подключения к БД: %w", err)
	}

	if err := migrations.Apply(ctx, pool); err != nil {
		return nil, fmt.Errorf("ошибка миграций: %w", err)
	}

	memberRepo := members.NewRepository(pool)
	economyRepo := economy.NewRepository(pool)
	streakRepo := streak.NewRepository(pool)
	karmaRepo := karma.NewRepository(pool)
	casinoRepo := casino.NewRepository(pool)
	adminRepo := admin.NewRepository(pool)

	memberService := members.NewService(memberRepo)
	economyService := economy.NewService(economyRepo)
	streakService := streak.NewService(streakRepo, economyService, cfg)
	karmaService := karma.NewService(karmaRepo, cfg)
	casinoService := casino.NewService(casinoRepo, economyService, cfg)
	adminService := admin.NewService(adminRepo, memberRepo, cfg)

	return &Infra{
		DB:             pool,
		MemberRepo:     memberRepo,
		EconomyRepo:    economyRepo,
		StreakRepo:     streakRepo,
		KarmaRepo:      karmaRepo,
		CasinoRepo:     casinoRepo,
		AdminRepo:      adminRepo,
		MemberService:  memberService,
		EconomyService: economyService,
		StreakService:  streakService,
		KarmaService:   karmaService,
		CasinoService:  casinoService,
		AdminService:   adminService,
	}, nil
}
