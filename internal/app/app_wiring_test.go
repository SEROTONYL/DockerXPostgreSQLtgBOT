package app

import (
	"context"
	"testing"

	"serotonyl.ru/telegram-bot/internal/app/modules"
	"serotonyl.ru/telegram-bot/internal/config"
)

func TestWiringCompileTimeContracts(t *testing.T) {
	var buildInfra func(context.Context, *config.Config) (*modules.Infra, error) = modules.BuildInfra
	var buildTelegram func(context.Context, *config.Config) (*modules.Telegram, error) = modules.BuildTelegram
	var buildApp func(context.Context, *config.Config) (*App, error) = New

	if buildInfra == nil || buildTelegram == nil || buildApp == nil {
		t.Fatal("wiring functions must be linked")
	}
}
