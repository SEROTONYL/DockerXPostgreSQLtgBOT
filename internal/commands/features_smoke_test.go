package commands_test

import (
	"context"
	"testing"

	"serotonyl.ru/telegram-bot/internal/commands"
	"serotonyl.ru/telegram-bot/internal/config"
	"serotonyl.ru/telegram-bot/internal/feature"
	"serotonyl.ru/telegram-bot/internal/features/admin"
	"serotonyl.ru/telegram-bot/internal/features/casino"
	"serotonyl.ru/telegram-bot/internal/features/core"
	"serotonyl.ru/telegram-bot/internal/features/debts"
	"serotonyl.ru/telegram-bot/internal/features/economy"
	"serotonyl.ru/telegram-bot/internal/features/karma"
	"serotonyl.ru/telegram-bot/internal/features/members"
	"serotonyl.ru/telegram-bot/internal/features/streak"
)

func TestRegisterAllFeaturesSmoke(t *testing.T) {
	cfg := &config.Config{
		FeatureCasinoEnabled:  true,
		FeatureKarmaEnabled:   true,
		FeatureStreaksEnabled: true,
	}

	r := commands.NewRouter()
	features := []feature.Feature{
		core.NewFeature(nil),
		admin.NewFeature(cfg, nil, nil, nil, nil),
		economy.NewFeature(nil, cfg),
		karma.NewFeature(nil, cfg),
		streak.NewFeature(nil, cfg),
		casino.NewFeature(nil, cfg),
		members.NewFeature(),
		debts.NewFeature(),
	}

	var current string
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("expected registration without panic, got: %v (feature=%s)", rec, current)
		}
	}()

	for _, f := range features {
		current = f.Name()
		f.RegisterCommands(r)
	}

	if ok := r.Dispatch(context.Background(), commands.Context{IsAdminChat: true}, "members_status", nil); !ok {
		t.Fatal("expected known command to be registered")
	}
}
