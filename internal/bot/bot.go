// Package bot содержит главный модуль бота — инициализацию, запуск и остановку.
// bot.go создаёт все сервисы, подключает обработчики и запускает polling.
package bot

import (
	"context"
	"fmt"
	"strings"
	"time"

	"serotonyl.ru/telegram-bot/internal/commands"

	botapi "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	log "github.com/sirupsen/logrus"

	"serotonyl.ru/telegram-bot/internal/bot/filters"
	"serotonyl.ru/telegram-bot/internal/bot/middleware"
	"serotonyl.ru/telegram-bot/internal/config"
	"serotonyl.ru/telegram-bot/internal/features/admin"
	"serotonyl.ru/telegram-bot/internal/features/casino"
	"serotonyl.ru/telegram-bot/internal/features/economy"
	"serotonyl.ru/telegram-bot/internal/features/karma"
	"serotonyl.ru/telegram-bot/internal/features/members"
	"serotonyl.ru/telegram-bot/internal/features/streak"
	"serotonyl.ru/telegram-bot/internal/jobs"
	"serotonyl.ru/telegram-bot/internal/telegram"
)

type purgeMetricsProvider interface {
	GetPurgeMetrics() jobs.PurgeMetrics
}

// Deps содержит зависимости для создания Bot.
type Deps struct {
	API            *botapi.Bot
	Ops            *telegram.Ops
	Cfg            *config.Config
	MemberService  *members.Service
	MemberHandler  *members.Handler
	EconomyService *economy.Service
	EconomyHandler *economy.Handler
	StreakService  *streak.Service
	StreakHandler  *streak.Handler
	KarmaService   *karma.Service
	KarmaHandler   *karma.Handler
	CasinoService  *casino.Service
	CasinoHandler  *casino.Handler
	AdminService   *admin.Service
	AdminHandler   *admin.Handler
	ChatFilter     *filters.ChatFilter
}

// Bot — главная структура бота, объединяющая все компоненты.
type Bot struct {
	api   *botapi.Bot
	ops   *telegram.Ops
	tgOps *telegram.Ops
	cfg   *config.Config

	chatFilter  *filters.ChatFilter
	rateLimiter *middleware.RateLimiter

	memberHandler  *members.Handler
	economyHandler *economy.Handler
	streakHandler  *streak.Handler
	karmaHandler   *karma.Handler
	casinoHandler  *casino.Handler
	adminHandler   *admin.Handler

	memberService  *members.Service
	economyService *economy.Service
	streakService  *streak.Service
	karmaService   *karma.Service
	casinoService  *casino.Service
	adminService   *admin.Service

	parser    *CommandParser
	cmdRouter *commands.Router

	purgeMetricsProvider purgeMetricsProvider
}

// New создаёт новый экземпляр бота со всеми зависимостями.
func New(d Deps) *Bot {
	b := &Bot{
		api:            d.API,
		ops:            d.Ops,
		tgOps:          d.Ops,
		cfg:            d.Cfg,
		chatFilter:     d.ChatFilter,
		rateLimiter:    middleware.NewRateLimiter(d.Cfg.RateLimitRequests, d.Cfg.RateLimitWindow),
		memberHandler:  d.MemberHandler,
		economyHandler: d.EconomyHandler,
		streakHandler:  d.StreakHandler,
		karmaHandler:   d.KarmaHandler,
		casinoHandler:  d.CasinoHandler,
		adminHandler:   d.AdminHandler,
		memberService:  d.MemberService,
		economyService: d.EconomyService,
		streakService:  d.StreakService,
		karmaService:   d.KarmaService,
		casinoService:  d.CasinoService,
		adminService:   d.AdminService,
		parser:         NewCommandParser(),
		cmdRouter:      commands.NewRouter(),
	}
	b.registerCommands()
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

	b.api.RegisterHandlerMatchFunc(func(update *models.Update) bool { return true }, func(handlerCtx context.Context, _ *botapi.Bot, update *models.Update) {
		pool.Enqueue(handlerCtx, *update)
	})

	b.api.Start(ctx)
}

func (b *Bot) shouldTouchLastSeen(uc UpdateContext) bool {
	if uc.IsAdminChat || uc.UserID == 0 || uc.ChatMember != nil {
		return false
	}
	if uc.ChatID == b.cfg.MainGroupID && (uc.Message != nil || uc.Callback != nil) {
		return true
	}
	return false
}

// handleUpdate обрабатывает одно обновление от Telegram.
func (b *Bot) handleUpdate(ctx context.Context, update models.Update) {
	defer middleware.RecoverFromPanic()

	uc := BuildUpdateContext(update, time.Now().UTC(), b.cfg)

	if uc.IsAdminChat {
		if uc.Message == nil {
			return
		}
		cmd, args, isCommand := b.parser.ParseCommand(uc.Message.Text)
		if isCommand && isAdminChatAllowedCommand(cmd) {
			b.routeCommand(ctx, uc, cmd, args)
		}
		return
	}

	if b.handleMembershipUpdate(ctx, uc) {
		return
	}

	if uc.Callback != nil {
		if uc.Callback.Message.Message == nil {
			return
		}
		middleware.LogMessage(uc.Callback.Message.Message)
		if !b.chatFilter.CheckAccess(ctx, uc.Callback.Message.Message) {
			return
		}
		if b.shouldTouchLastSeen(uc) {
			if err := b.memberService.EnsureActiveMemberSeen(ctx, uc.UserID, uc.Username, uc.FullName, uc.Now); err != nil {
				log.WithError(err).WithField("user_id", uc.UserID).Debug("EnsureActiveMemberSeen failed")
			}
		}
		if b.adminHandler.HandleAdminCallback(ctx, uc.Callback) {
			return
		}
	}

	if uc.Message == nil || uc.Message.Text == "" {
		return
	}

	message := uc.Message
	middleware.LogMessage(message)

	if !b.chatFilter.CheckAccess(ctx, message) {
		return
	}

	if message.From != nil && !b.rateLimiter.Allow(message.From.ID) {
		log.WithField("user_id", message.From.ID).Debug("rate limited")
		return
	}

	chatID := message.Chat.ID
	userID := message.From.ID

	if b.shouldTouchLastSeen(uc) {
		if err := b.memberService.EnsureActiveMemberSeen(ctx, userID, message.From.Username, buildDisplayName(message.From.FirstName, message.From.LastName), uc.Now); err != nil {
			log.WithError(err).WithField("user_id", userID).Debug("EnsureActiveMemberSeen failed")
		}
	}

	if uc.IsPrivate {
		handled := b.adminHandler.HandleAdminMessage(ctx, chatID, userID, message.Text)
		if handled {
			return
		}
	}

	if b.cfg.FeatureKarmaEnabled && message.ReplyToMessage != nil && message.ReplyToMessage.From != nil {
		if karma.IsThankYou(message.Text) {
			b.karmaHandler.HandleThankYou(ctx, chatID, userID, message.ReplyToMessage.From.ID)
			return
		}
	}

	cmd, args, isCommand := b.parser.ParseCommand(message.Text)
	log.WithFields(log.Fields{
		"isCommand": isCommand,
		"cmd":       cmd,
		"args":      args,
		"text":      message.Text,
	}).Debug("parsed command")

	if isCommand {
		b.routeCommand(ctx, uc, cmd, args)
		return
	} else if chatID == b.cfg.FloodChatID {
		if b.cfg.FeatureStreaksEnabled {
			b.streakService.CountMessage(ctx, userID, message.Text)
		}
	}
}

func (b *Bot) handleMembershipUpdate(ctx context.Context, uc UpdateContext) bool {
	cmu := uc.ChatMember
	if cmu == nil {
		return false
	}
	if cmu.Chat.ID != b.cfg.MainGroupID {
		return true
	}

	oldStatus := cmu.OldChatMember.Type
	newStatus := cmu.NewChatMember.Type
	user, ok := chatMemberUser(cmu.NewChatMember)
	if !ok {
		log.WithFields(log.Fields{"old_status": oldStatus, "new_status": newStatus, "chat_id": cmu.Chat.ID}).Warn("membership update without user payload")
		return true
	}

	name := buildDisplayName(user.FirstName, user.LastName)
	now := uc.Now

	switch classifyMemberStatus(newStatus) {
	case membershipActionActive:
		if err := b.memberService.UpsertActiveMember(ctx, user.ID, user.Username, name, now); err != nil {
			log.WithError(err).WithField("user_id", user.ID).Warn("UpsertActiveMember failed")
			return true
		}
		if classifyMemberStatus(oldStatus) != membershipActionActive {
			b.handleNewMembers(ctx, []models.User{*user})
		}
		log.WithFields(log.Fields{"user_id": user.ID, "old_status": oldStatus, "new_status": newStatus, "action": "active"}).Info("membership transition handled")
	case membershipActionLeft:
		if err := b.memberService.MarkMemberLeft(ctx, user.ID, now, now.Add(5*24*time.Hour)); err != nil {
			log.WithError(err).WithField("user_id", user.ID).Warn("MarkMemberLeft failed")
			return true
		}
		log.WithFields(log.Fields{"user_id": user.ID, "old_status": oldStatus, "new_status": newStatus, "action": "left"}).Info("membership transition handled")
	default:
		log.WithFields(log.Fields{"user_id": user.ID, "old_status": oldStatus, "new_status": newStatus, "action": "ignore"}).Debug("membership transition ignored")
	}

	return true
}

// routeCommand маршрутизирует команду к нужному обработчику.
func isAdminChatAllowedCommand(cmd string) bool {
	switch cmd {
	case "members_status", "members_stats":
		return true
	default:
		return false
	}
}

func (b *Bot) registerCommands() {
	b.cmdRouter.Register("start", func(ctx context.Context, c commands.Context, args []string) {
		b.sendMessage(ctx, c.ChatID, "Я живой. Команды: /login <пароль> (админ), !плёнки, !карма, !слоты ...")
	})
	b.cmdRouter.Register("help", func(ctx context.Context, c commands.Context, args []string) {
		b.sendMessage(ctx, c.ChatID, "Я живой. Команды: /login <пароль> (админ), !плёнки, !карма, !слоты ...")
	})
	b.cmdRouter.Register("login", func(ctx context.Context, c commands.Context, args []string) {
		if c.ChatID == c.UserID {
			b.adminHandler.HandleAdminMessage(ctx, c.ChatID, c.UserID, "/login "+strings.Join(args, " "))
		}
	})
	b.cmdRouter.Register("members_status", func(ctx context.Context, c commands.Context, args []string) {
		b.handleMembersStatusCommand(ctx, UpdateContext{ChatID: c.ChatID, UserID: c.UserID, IsPrivate: c.IsPrivate, IsAdminChat: c.IsAdminChat, Now: c.Now})
	})
	b.cmdRouter.Register("members_stats", func(ctx context.Context, c commands.Context, args []string) {
		b.handleMembersStatusCommand(ctx, UpdateContext{ChatID: c.ChatID, UserID: c.UserID, IsPrivate: c.IsPrivate, IsAdminChat: c.IsAdminChat, Now: c.Now})
	})

	economy.RegisterCommands(b.cmdRouter, b.economyHandler, b.cfg)
	karma.RegisterCommands(b.cmdRouter, b.karmaHandler, b.cfg)
	streak.RegisterCommands(b.cmdRouter, b.streakHandler, b.cfg)
	casino.RegisterCommands(b.cmdRouter, b.casinoHandler, b.cfg)
}

func (b *Bot) routeCommand(ctx context.Context, uc UpdateContext, cmd string, args []string) {
	log.WithFields(log.Fields{
		"cmd":  cmd,
		"args": args,
	}).Debug("routing command")

	if uc.IsAdminChat && !isAdminChatAllowedCommand(cmd) {
		return
	}

	c := commands.Context{
		ChatID:      uc.ChatID,
		UserID:      uc.UserID,
		IsPrivate:   uc.IsPrivate,
		IsAdminChat: uc.IsAdminChat,
		Now:         uc.Now,
	}

	if ok := b.cmdRouter.Dispatch(ctx, c, cmd, args); !ok {
		log.WithField("cmd", cmd).Debug("unknown command")
	}
}

func (b *Bot) handleMembersStatusCommand(ctx context.Context, uc UpdateContext) {
	if !uc.IsAdminChat {
		return
	}

	active, left, err := b.memberService.CountMembersByStatus(ctx)
	if err != nil {
		log.WithError(err).Warn("members status: count by status failed")
		return
	}
	pending, err := b.memberService.CountPendingPurge(ctx, uc.Now)
	if err != nil {
		log.WithError(err).Warn("members status: pending purge count failed")
		return
	}

	metrics := jobs.PurgeMetrics{}
	if b.purgeMetricsProvider != nil {
		metrics = b.purgeMetricsProvider.GetPurgeMetrics()
	}
	lastRun := "never"
	if !metrics.LastRunAt.IsZero() {
		lastRun = metrics.LastRunAt.Format(time.RFC3339)
	}
	lastError := metrics.LastError
	if strings.TrimSpace(lastError) == "" {
		lastError = "none"
	}

	text := fmt.Sprintf("Members:\n- Active: %d\n- Left (grace): %d\n- Pending purge: %d\n\nPurge:\n- Last run: %s\n- Last deleted: %d\n- Total deleted: %d\n- Last error: %s", active, left, pending, lastRun, metrics.LastRunDeleted, metrics.TotalDeleted, lastError)
	b.sendMessage(ctx, uc.ChatID, text)
}

// handleNewMembers обрабатывает вступление новых участников.
func (b *Bot) handleNewMembers(ctx context.Context, newMembers []models.User) {
	for _, user := range newMembers {
		if err := b.memberService.HandleNewMember(ctx, user.ID, user.Username, user.FirstName, user.LastName); err != nil {
			log.WithError(err).WithField("user_id", user.ID).Warn("HandleNewMember failed")
		}
		if err := b.economyService.CreateBalance(ctx, user.ID); err != nil {
			log.WithError(err).WithField("user_id", user.ID).Warn("CreateBalance failed")
		}
		if err := b.streakService.CreateStreak(ctx, user.ID); err != nil {
			log.WithError(err).WithField("user_id", user.ID).Warn("CreateStreak failed")
		}
		if err := b.karmaService.CreateKarma(ctx, user.ID); err != nil {
			log.WithError(err).WithField("user_id", user.ID).Warn("CreateKarma failed")
		}

		log.WithField("user", user.Username).Info("Новый участник обработан")
	}
}

// sendMessage — утилита для отправки сообщений.
func (b *Bot) sendMessage(ctx context.Context, chatID int64, text string) {
	ops := b.ops
	if ops == nil {
		ops = b.tgOps
	}
	if ops == nil {
		return
	}
	_, _ = ops.Send(ctx, chatID, text, nil)
}

// SendMessageToUser отправляет сообщение пользователю (для напоминаний).
func (b *Bot) SendMessageToUser(userID int64, text string) {
	ops := b.ops
	if ops == nil {
		ops = b.tgOps
	}
	if ops == nil {
		return
	}
	_, _ = ops.Send(context.Background(), userID, text, nil)
}

type membershipAction string

const (
	membershipActionIgnore membershipAction = "ignore"
	membershipActionActive membershipAction = "active"
	membershipActionLeft   membershipAction = "left"
)

func classifyMemberStatus(status models.ChatMemberType) membershipAction {
	switch status {
	case models.ChatMemberTypeOwner, models.ChatMemberTypeAdministrator, models.ChatMemberTypeMember:
		return membershipActionActive
	case models.ChatMemberTypeLeft, models.ChatMemberTypeBanned, models.ChatMemberTypeRestricted:
		return membershipActionLeft
	default:
		return membershipActionIgnore
	}
}

func extractChatMemberUpdate(update models.Update) *models.ChatMemberUpdated {
	if update.ChatMember != nil {
		return update.ChatMember
	}
	if update.MyChatMember != nil {
		return update.MyChatMember
	}
	return nil
}

func chatMemberUser(member models.ChatMember) (*models.User, bool) {
	switch member.Type {
	case models.ChatMemberTypeOwner:
		if member.Owner != nil && member.Owner.User != nil {
			return member.Owner.User, true
		}
	case models.ChatMemberTypeAdministrator:
		if member.Administrator != nil {
			u := member.Administrator.User
			return &u, true
		}
	case models.ChatMemberTypeMember:
		if member.Member != nil && member.Member.User != nil {
			return member.Member.User, true
		}
	case models.ChatMemberTypeRestricted:
		if member.Restricted != nil && member.Restricted.User != nil {
			return member.Restricted.User, true
		}
	case models.ChatMemberTypeLeft:
		if member.Left != nil && member.Left.User != nil {
			return member.Left.User, true
		}
	case models.ChatMemberTypeBanned:
		if member.Banned != nil && member.Banned.User != nil {
			return member.Banned.User, true
		}
	}
	return nil, false
}

func buildDisplayName(firstName, lastName string) string {
	name := strings.TrimSpace(firstName)
	if ln := strings.TrimSpace(lastName); ln != "" {
		if name != "" {
			name += " "
		}
		name += ln
	}
	return name
}

// CommandParser парсит русские команды с префиксами ! и .
type CommandParser struct {
	validPrefixes []string
}

// NewCommandParser создаёт парсер команд.
func NewCommandParser() *CommandParser {
	return &CommandParser{
		validPrefixes: []string{"!", ".", "/"},
	}
}

// ParseCommand разбирает текст на команду и аргументы.
func (p *CommandParser) ParseCommand(text string) (string, []string, bool) {
	text = strings.TrimSpace(text)

	hasPrefix := false
	for _, prefix := range p.validPrefixes {
		if strings.HasPrefix(text, prefix) {
			text = strings.TrimPrefix(text, prefix)
			hasPrefix = true
			break
		}
	}

	if !hasPrefix {
		return "", nil, false
	}

	text = strings.TrimSpace(text)
	parts := strings.Fields(text)

	if len(parts) == 0 {
		return "", nil, false
	}

	command := strings.ToLower(parts[0])
	var args []string
	if len(parts) > 1 {
		args = parts[1:]
	}

	return command, args, true
}
