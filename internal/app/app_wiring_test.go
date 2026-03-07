package app

import (
	"testing"

	"serotonyl.ru/telegram-bot/internal/app/modules"
)

func TestWiringCompileTimeContracts(t *testing.T) {
	buildInfra := modules.BuildInfra
	buildTelegram := modules.BuildTelegram
	buildApp := New

	if buildInfra == nil || buildTelegram == nil || buildApp == nil {
		t.Fatal("wiring functions must be linked")
	}
}
