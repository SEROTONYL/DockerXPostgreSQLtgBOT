package modules

import (
	"serotonyl.ru/telegram-bot/internal/config"
	"serotonyl.ru/telegram-bot/internal/feature"
	"serotonyl.ru/telegram-bot/internal/features/casino"
)

type CasinoModule struct {
	Handler *casino.Handler
	Feature feature.Feature
}

func BuildCasinoModule(cfg *config.Config, infra *Infra, tg *Telegram) *CasinoModule {
	h := casino.NewHandler(infra.CasinoService, tg.Ops)
	f := casino.NewFeature(h, cfg)
	return &CasinoModule{Handler: h, Feature: f}
}
