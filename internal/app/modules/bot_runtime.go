package modules

import (
	"context"

	models "github.com/mymmrac/telego"

	"serotonyl.ru/telegram-bot/internal/bot"
	"serotonyl.ru/telegram-bot/internal/bot/filters"
	"serotonyl.ru/telegram-bot/internal/commands"
	"serotonyl.ru/telegram-bot/internal/config"
	"serotonyl.ru/telegram-bot/internal/jobs"
)

func BuildChatFilter(cfg *config.Config, infra *Infra, tg *Telegram) *filters.ChatFilter {
	return filters.NewChatFilter(cfg.MemberSourceChatID, cfg.AdminChatID, infra.MemberService, tg.Ops)
}

type BotHandlers struct {
	Admin interface {
		HandleAdminMessage(ctx context.Context, chatID int64, userID int64, messageID int, text string) bool
		HandleAdminCallback(ctx context.Context, q *models.CallbackQuery) bool
	}
	Members interface {
		HandleMembersCallback(ctx context.Context, q *models.CallbackQuery) bool
	}
	Karma interface {
		HandleThankYou(ctx context.Context, chatID int64, fromUserID int64, toUserID int64)
	}
}

type StreakServiceAdapter struct {
	Service interface {
		CountMessage(ctx context.Context, userID int64, text string) error
		CreateStreak(ctx context.Context, userID int64) error
	}
}

func (a StreakServiceAdapter) CountMessage(ctx context.Context, userID int64, text string) {
	if a.Service == nil {
		return
	}
	_ = a.Service.CountMessage(ctx, userID, text)
}

func (a StreakServiceAdapter) CreateStreak(ctx context.Context, userID int64) error {
	if a.Service == nil {
		return nil
	}
	return a.Service.CreateStreak(ctx, userID)
}

type KarmaClassifier struct {
	Match func(text string) bool
}

func (k KarmaClassifier) IsThankYou(text string) bool {
	if k.Match == nil {
		return false
	}
	return k.Match(text)
}

func BuildBot(cfg *config.Config, infra *Infra, tg *Telegram, cmdRouter *commands.Router, chatFilter bot.ChatAccessFilter, handlers BotHandlers, classifier bot.KarmaThankYouClassifier) *bot.Bot {
	return bot.New(bot.Deps{
		Ops:            tg.Ops,
		CmdRouter:      cmdRouter,
		Cfg:            cfg,
		MemberService:  infra.MemberService,
		EconomyService: infra.EconomyService,
		StreakService:  StreakServiceAdapter{Service: infra.StreakService},
		KarmaService:   infra.KarmaService,
		KarmaHandler:   handlers.Karma,
		AdminHandler:   handlers.Admin,
		MembersHandler: handlers.Members,
		ChatFilter:     chatFilter,
		ThankYou:       classifier,
	})
}

func BuildScheduler(cfg *config.Config, infra *Infra, tg *Telegram, b *bot.Bot) *jobs.Scheduler {
	return jobs.NewScheduler(cfg, infra.StreakService, infra.MemberService, b.SendMessageToUser, tg.Ops)
}
