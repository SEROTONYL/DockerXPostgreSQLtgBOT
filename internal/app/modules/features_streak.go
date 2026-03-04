package modules

import (
	"serotonyl.ru/telegram-bot/internal/config"
	"serotonyl.ru/telegram-bot/internal/feature"
	"serotonyl.ru/telegram-bot/internal/features/streak"
)

type StreakModule struct {
	Handler *streak.Handler
	Feature feature.Feature
}

func BuildStreakModule(cfg *config.Config, infra *Infra, tg *Telegram) *StreakModule {
	h := streak.NewHandler(infra.StreakService, tg.Ops, cfg)
	f := streak.NewFeature(h, cfg)
	return &StreakModule{Handler: h, Feature: f}
}
