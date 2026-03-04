// Package bot содержит главный модуль бота — инициализацию, запуск и остановку.
// bot.go создаёт все сервисы, подключает обработчики и запускает polling.
package bot

import (
	"context"
	"fmt"
	"time"

	"github.com/go-telegram/bot/models"

	"serotonyl.ru/telegram-bot/internal/commands"
	"serotonyl.ru/telegram-bot/internal/jobs"
	"serotonyl.ru/telegram-bot/internal/telegram"

	log "github.com/sirupsen/logrus"

	"serotonyl.ru/telegram-bot/internal/bot/filters"
	"serotonyl.ru/telegram-bot/internal/bot/middleware"
	"serotonyl.ru/telegram-bot/internal/config"
)

type purgeMetricsProvider interface {
	GetPurgeMetrics() jobs.PurgeMetrics
}

type memberService interface {
	EnsureActiveMemberSeen(ctx context.Context, userID int64, username, fullName string, now time.Time) error
	CountMembersByStatus(ctx context.Context) (active int, left int, err error)
	CountPendingPurge(ctx context.Context, now time.Time) (int, error)
	UpsertActiveMember(ctx context.Context, userID int64, username, fullName string, now time.Time) error
	MarkMemberLeft(ctx context.Context, userID int64, leftAt, purgeAfter time.Time) error
	HandleNewMember(ctx context.Context, userID int64, username, firstName, lastName string) error
}

type economyService interface {
	CreateBalance(ctx context.Context, userID int64) error
}

type streakService interface {
	CountMessage(ctx context.Context, userID int64, text string) error
	CreateStreak(ctx context.Context, userID int64) error
}

type karmaService interface {
	CreateKarma(ctx context.Context, userID int64) error
}

type adminHandler interface {
	HandleAdminMessage(ctx context.Context, chatID int64, userID int64, text string) bool
	HandleAdminCallback(ctx context.Context, q *models.CallbackQuery) bool
}

type economyHandler interface{}
type streakHandler interface{}
type karmaHandler interface {
	HandleThankYou(ctx context.Context, chatID int64, fromUserID int64, toUserID int64)
}
type casinoHandler interface{}

// Deps содержит зависимости для создания Bot.
type Deps struct {
	Ops            *telegram.Ops
	CmdRouter      *commands.Router
	Cfg            *config.Config
	MemberService  memberService
	EconomyService economyService
	EconomyHandler economyHandler
	StreakService  streakService
	StreakHandler  streakHandler
	KarmaService   karmaService
	KarmaHandler   karmaHandler
	CasinoHandler  casinoHandler
	AdminHandler   adminHandler
	ChatFilter     *filters.ChatFilter
	IsThankYou     func(text string) bool
}

// Validate проверяет обязательные зависимости для Bot.
func (d Deps) Validate() error {
	if d.Ops == nil {
		return fmt.Errorf("bot deps: ops is nil")
	}
	if d.Cfg == nil {
		return fmt.Errorf("bot deps: cfg is nil")
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

	chatFilter  *filters.ChatFilter
	rateLimiter *middleware.RateLimiter

	economyHandler economyHandler
	streakHandler  streakHandler
	karmaHandler   karmaHandler
	casinoHandler  casinoHandler
	adminHandler   adminHandler

	memberService  memberService
	economyService economyService
	streakService  streakService
	karmaService   karmaService
	isThankYou     func(text string) bool

	parser    *CommandParser
	cmdRouter *commands.Router

	purgeMetricsProvider purgeMetricsProvider
}

// New создаёт новый экземпляр бота со всеми зависимостями.
func New(d Deps) *Bot {
	if err := d.Validate(); err != nil {
		panic(err)
	}

	b := &Bot{
		ops:            d.Ops,
		cfg:            d.Cfg,
		chatFilter:     d.ChatFilter,
		rateLimiter:    middleware.NewRateLimiter(d.Cfg.RateLimitRequests, d.Cfg.RateLimitWindow),
		economyHandler: d.EconomyHandler,
		streakHandler:  d.StreakHandler,
		karmaHandler:   d.KarmaHandler,
		casinoHandler:  d.CasinoHandler,
		adminHandler:   d.AdminHandler,
		memberService:  d.MemberService,
		economyService: d.EconomyService,
		streakService:  d.StreakService,
		karmaService:   d.KarmaService,
		parser:         NewCommandParser(),
		cmdRouter:      d.CmdRouter,
		isThankYou:     d.IsThankYou,
	}
	if b.cmdRouter == nil {
		b.cmdRouter = commands.NewRouter()
	}
	if b.isThankYou == nil {
		b.isThankYou = func(string) bool { return false }
	}
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
