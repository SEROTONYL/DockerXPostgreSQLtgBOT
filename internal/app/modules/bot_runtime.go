package modules

import (
	"serotonyl.ru/telegram-bot/internal/bot"
	"serotonyl.ru/telegram-bot/internal/bot/filters"
	"serotonyl.ru/telegram-bot/internal/commands"
	"serotonyl.ru/telegram-bot/internal/config"
	"serotonyl.ru/telegram-bot/internal/features/karma"
	"serotonyl.ru/telegram-bot/internal/jobs"
)

func BuildChatFilter(cfg *config.Config, infra *Infra, tg *Telegram) *filters.ChatFilter {
	return filters.NewChatFilter(cfg.FloodChatID, cfg.AdminChatID, infra.MemberService, tg.Ops)
}

func BuildBot(cfg *config.Config, infra *Infra, tg *Telegram, cmdRouter *commands.Router, chatFilter *filters.ChatFilter, adminModule *AdminModule, memberModule *MemberModule, streakModule *StreakModule, karmaModule *KarmaModule, casinoModule *CasinoModule) *bot.Bot {
	return bot.New(bot.Deps{
		Ops:            tg.Ops,
		CmdRouter:      cmdRouter,
		Cfg:            cfg,
		MemberService:  infra.MemberService,
		EconomyService: infra.EconomyService,
		EconomyHandler: memberModule.EconomyHandler,
		StreakService:  infra.StreakService,
		StreakHandler:  streakModule.Handler,
		KarmaService:   infra.KarmaService,
		KarmaHandler:   karmaModule.Handler,
		CasinoHandler:  casinoModule.Handler,
		AdminHandler:   adminModule.Handler,
		ChatFilter:     chatFilter,
		IsThankYou:     karma.IsThankYou,
	})
}

func BuildScheduler(infra *Infra, b *bot.Bot) *jobs.Scheduler {
	return jobs.NewScheduler(infra.StreakService, infra.MemberService, b.SendMessageToUser)
}
