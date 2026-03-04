package modules

import (
	"context"

	"github.com/go-telegram/bot/models"

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

type BotHandlers struct {
	Admin interface {
		HandleAdminMessage(ctx context.Context, chatID int64, userID int64, text string) bool
		HandleAdminCallback(ctx context.Context, q *models.CallbackQuery) bool
	}
	Economy interface{}
	Streak  interface{}
	Karma   interface {
		HandleThankYou(ctx context.Context, chatID int64, fromUserID int64, toUserID int64)
	}
	Casino interface{}
}

func BuildBot(cfg *config.Config, infra *Infra, tg *Telegram, cmdRouter *commands.Router, chatFilter *filters.ChatFilter, handlers BotHandlers) *bot.Bot {
	return bot.New(bot.Deps{
		Ops:            tg.Ops,
		CmdRouter:      cmdRouter,
		Cfg:            cfg,
		MemberService:  infra.MemberService,
		EconomyService: infra.EconomyService,
		EconomyHandler: handlers.Economy,
		StreakService:  infra.StreakService,
		StreakHandler:  handlers.Streak,
		KarmaService:   infra.KarmaService,
		KarmaHandler:   handlers.Karma,
		CasinoHandler:  handlers.Casino,
		AdminHandler:   handlers.Admin,
		ChatFilter:     chatFilter,
		IsThankYou:     karma.IsThankYou,
	})
}

func BuildScheduler(infra *Infra, b *bot.Bot) *jobs.Scheduler {
	return jobs.NewScheduler(infra.StreakService, infra.MemberService, b.SendMessageToUser)
}
