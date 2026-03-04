package modules

import (
	"serotonyl.ru/telegram-bot/internal/config"
	"serotonyl.ru/telegram-bot/internal/feature"
	"serotonyl.ru/telegram-bot/internal/features/karma"
)

type KarmaModule struct {
	Handler *karma.Handler
	Feature feature.Feature
}

func BuildKarmaModule(cfg *config.Config, infra *Infra, tg *Telegram) *KarmaModule {
	h := karma.NewHandler(infra.KarmaService, tg.Ops)
	f := karma.NewFeature(h, cfg)
	return &KarmaModule{Handler: h, Feature: f}
}
