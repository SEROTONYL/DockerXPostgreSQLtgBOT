// Package bot содержит главный модуль бота — инициализацию, запуск и остановку.
// bot.go создаёт все сервисы, подключает обработчики и запускает polling.
package bot

import (
	"context"
	"fmt"

	models "github.com/mymmrac/telego"

	"serotonyl.ru/telegram-bot/internal/commands"
	"serotonyl.ru/telegram-bot/internal/jobs"
	"serotonyl.ru/telegram-bot/internal/telegram"

	log "github.com/sirupsen/logrus"

	"serotonyl.ru/telegram-bot/internal/bot/middleware"
	"serotonyl.ru/telegram-bot/internal/config"
)

type purgeMetricsProvider interface {
	GetPurgeMetrics() jobs.PurgeMetrics
}

// Deps содержит зависимости для создания Bot.
type Deps struct {
	Ops            *telegram.Ops
	CmdRouter      *commands.Router
	Cfg            *config.Config
	MemberService  MemberService
	EconomyService EconomyService
	StreakService  StreakService
	KarmaService   KarmaService
	KarmaHandler   KarmaHandler
	AdminHandler   AdminHandler
	ChatFilter     ChatAccessFilter
	ThankYou       KarmaThankYouClassifier
}

// Validate проверяет обязательные зависимости для Bot.
func (d Deps) Validate() error {
	if d.Ops == nil {
		return fmt.Errorf("bot deps: ops is nil")
	}
	if d.Cfg == nil {
		return fmt.Errorf("bot deps: cfg is nil")
	}
	if d.CmdRouter == nil {
		return fmt.Errorf("bot deps: cmd router is nil")
	}
	if d.MemberService == nil {
		return fmt.Errorf("bot deps: member service is nil")
	}
	if d.AdminHandler == nil {
		return fmt.Errorf("bot deps: admin handler is nil")
	}
	if d.KarmaHandler == nil {
		return fmt.Errorf("bot deps: karma handler is nil")
	}
	if d.ChatFilter == nil {
		return fmt.Errorf("bot deps: chat filter is nil")
	}
	return nil
}

// Bot — главная структура бота, объединяющая все компоненты.
type Bot struct {
	ops *telegram.Ops
	cfg *config.Config

	chatFilter  ChatAccessFilter
	rateLimiter *middleware.RateLimiter

	adminHandler AdminHandler
	karmaHandler KarmaHandler

	memberService  MemberService
	economyService EconomyService
	streakService  StreakService
	karmaService   KarmaService
	thankYou       KarmaThankYouClassifier

	parser    *CommandParser
	cmdRouter *commands.Router

	purgeMetricsProvider purgeMetricsProvider
}

// New создаёт новый экземпляр бота.
// Все команды фич должны быть зарегистрированы в CmdRouter снаружи (composition root в internal/app).
func New(d Deps) *Bot {
	if err := d.Validate(); err != nil {
		panic(err)
	}

	b := &Bot{
		ops:            d.Ops,
		cfg:            d.Cfg,
		chatFilter:     d.ChatFilter,
		rateLimiter:    middleware.NewRateLimiter(d.Cfg.RateLimitRequests, d.Cfg.RateLimitWindow),
		adminHandler:   d.AdminHandler,
		karmaHandler:   d.KarmaHandler,
		memberService:  d.MemberService,
		economyService: d.EconomyService,
		streakService:  d.StreakService,
		karmaService:   d.KarmaService,
		parser:         NewCommandParser(),
		cmdRouter:      d.CmdRouter,
		thankYou:       d.ThankYou,
	}
	b.registerCoreCommands()
	return b
}

// SetPurgeMetricsProvider подключает источник метрик purge для служебных команд.
func (b *Bot) SetPurgeMetricsProvider(provider purgeMetricsProvider) {
	b.purgeMetricsProvider = provider
}

// Start запускает polling обновлений от Telegram.
func (b *Bot) Start(ctx context.Context) {
	pool := newUpdatePool(b.cfg.BotWorkers, b.cfg.BotUpdateQueue, b.handleUpdate)
	pool.Start()
	defer pool.Stop()

	log.WithFields(log.Fields{
		"max_inflight": b.cfg.BotMaxInflight,
		"workers":      b.cfg.BotWorkers,
		"queue_size":   b.cfg.BotUpdateQueue,
		"timeout_sec":  b.cfg.BotUpdateTimeoutSeconds,
	}).Info("Бот запущен и ожидает сообщения...")
	log.Infof("update pool: workers=%d queue=%d", b.cfg.BotWorkers, b.cfg.BotUpdateQueue)

	err := b.ops.RegisterUpdateHandler(func(update *models.Update) bool { return true }, func(handlerCtx context.Context, update *models.Update) {
		pool.Enqueue(handlerCtx, *update)
	})
	if err != nil {
		log.WithError(err).Error("failed to register telegram update handler")
		return
	}

	if err := b.ops.Start(ctx); err != nil {
		log.WithError(err).Error("telegram bot runtime stopped with error")
	}
}

// sendMessage — утилита для отправки сообщений.
func (b *Bot) sendMessage(ctx context.Context, chatID int64, text string) {
	if b.ops == nil {
		return
	}
	_, _ = b.ops.Send(ctx, chatID, text, nil)
}

// SendMessageToUser отправляет сообщение пользователю (для напоминаний).
func (b *Bot) SendMessageToUser(userID int64, text string) {
	if b.ops == nil {
		return
	}
	_, _ = b.ops.Send(context.Background(), userID, text, nil)
}
