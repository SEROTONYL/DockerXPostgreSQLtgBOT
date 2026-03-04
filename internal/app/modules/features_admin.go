package modules

import (
	"serotonyl.ru/telegram-bot/internal/config"
	"serotonyl.ru/telegram-bot/internal/feature"
	"serotonyl.ru/telegram-bot/internal/features/admin"
	"serotonyl.ru/telegram-bot/internal/jobs"
)

type AdminModule struct {
	Handler *admin.Handler
	Feature feature.Feature
}

func BuildAdminModule(cfg *config.Config, infra *Infra, tg *Telegram, purgeMetrics func() jobs.PurgeMetrics) *AdminModule {
	h := admin.NewHandler(infra.AdminService, infra.MemberService, infra.EconomyService, tg.Ops)
	f := admin.NewFeature(cfg, tg.Ops, h, infra.MemberService, purgeMetrics)
	return &AdminModule{Handler: h, Feature: f}
}
