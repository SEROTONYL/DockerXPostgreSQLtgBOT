package modules

import (
	"serotonyl.ru/telegram-bot/internal/feature"
	"serotonyl.ru/telegram-bot/internal/features/core"
	"serotonyl.ru/telegram-bot/internal/features/debts"
)

type CoreModule struct {
	CoreFeature  feature.Feature
	DebtsFeature feature.Feature
}

func BuildCoreModule(tg *Telegram) *CoreModule {
	return &CoreModule{
		CoreFeature:  core.NewFeature(tg.Ops),
		DebtsFeature: debts.NewFeature(),
	}
}
