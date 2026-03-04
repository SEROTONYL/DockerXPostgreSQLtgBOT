package modules

import (
	"serotonyl.ru/telegram-bot/internal/config"
	"serotonyl.ru/telegram-bot/internal/feature"
	"serotonyl.ru/telegram-bot/internal/features/economy"
	"serotonyl.ru/telegram-bot/internal/features/members"
)

type MemberModule struct {
	EconomyHandler *economy.Handler
	MemberFeature  feature.Feature
	EconomyFeature feature.Feature
}

func BuildMemberModule(cfg *config.Config, infra *Infra, tg *Telegram) *MemberModule {
	economyHandler := economy.NewHandler(infra.EconomyService, infra.MemberService, tg.Ops)
	memberFeature := members.NewFeature()
	economyFeature := economy.NewFeature(economyHandler, cfg)

	return &MemberModule{
		EconomyHandler: economyHandler,
		MemberFeature:  memberFeature,
		EconomyFeature: economyFeature,
	}
}
