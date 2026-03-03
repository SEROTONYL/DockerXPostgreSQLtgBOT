// Package bot содержит главный модуль бота — инициализацию, запуск и остановку.
// bot.go создаёт все сервисы, подключает обработчики и запускает polling.
package bot

import (
	"context"
	"strings"
	"time"

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
	"serotonyl.ru/telegram-bot/internal/telegram"
)

// Bot — главная структура бота, объединяющая все компоненты.
type Bot struct {
	api *botapi.Bot
	tg  telegram.Client
	cfg *config.Config

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

	parser *CommandParser
}

// New создаёт новый экземпляр бота со всеми зависимостями.
func New(
	api *botapi.Bot,
	tg telegram.Client,
	cfg *config.Config,
	memberService *members.Service,
	memberHandler *members.Handler,
	economyService *economy.Service,
	economyHandler *economy.Handler,
	streakService *streak.Service,
	streakHandler *streak.Handler,
	karmaService *karma.Service,
	karmaHandler *karma.Handler,
	casinoService *casino.Service,
	casinoHandler *casino.Handler,
	adminService *admin.Service,
	adminHandler *admin.Handler,
	chatFilter *filters.ChatFilter,
) *Bot {
	return &Bot{
		api:            api,
		tg:             tg,
		cfg:            cfg,
		chatFilter:     chatFilter,
		rateLimiter:    middleware.NewRateLimiter(cfg.RateLimitRequests, cfg.RateLimitWindow),
		memberHandler:  memberHandler,
		economyHandler: economyHandler,
		streakHandler:  streakHandler,
		karmaHandler:   karmaHandler,
		casinoHandler:  casinoHandler,
		adminHandler:   adminHandler,
		memberService:  memberService,
		economyService: economyService,
		streakService:  streakService,
		karmaService:   karmaService,
		casinoService:  casinoService,
		adminService:   adminService,
		parser:         NewCommandParser(),
	}
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

// handleUpdate обрабатывает одно обновление от Telegram.
func (b *Bot) handleUpdate(ctx context.Context, update models.Update) {
	defer middleware.RecoverFromPanic()

	if b.handleMembershipUpdate(ctx, update) {
		return
	}

	if update.CallbackQuery != nil {
		if b.adminHandler.HandleAdminCallback(ctx, update.CallbackQuery) {
			return
		}
	}

	if update.Message == nil || update.Message.Text == "" {
		return
	}

	message := update.Message
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

	if err := b.memberService.EnsureMember(ctx, userID,
		message.From.Username, message.From.FirstName, message.From.LastName,
	); err != nil {
		log.WithError(err).WithField("user_id", userID).Warn("EnsureMember failed")
	}

	if message.Chat.Type == models.ChatTypePrivate {
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
		b.routeCommand(ctx, chatID, userID, cmd, args)
		return
	} else if chatID == b.cfg.FloodChatID {
		if b.cfg.FeatureStreaksEnabled {
			b.streakService.CountMessage(ctx, userID, message.Text)
		}
	}
}

func (b *Bot) handleMembershipUpdate(ctx context.Context, update models.Update) bool {
	cmu := extractChatMemberUpdate(update)
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
	now := time.Now().UTC()

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
func (b *Bot) routeCommand(ctx context.Context, chatID, userID int64, cmd string, args []string) {
	log.WithFields(log.Fields{
		"cmd":  cmd,
		"args": args,
	}).Debug("routing command")
	switch cmd {
	case "start", "help":
		b.sendMessage(chatID, "Я живой. Команды: /login <пароль> (админ), !плёнки, !карма, !слоты ...")

	case "login":
		if chatID == userID {
			b.adminHandler.HandleAdminMessage(ctx, chatID, userID, "/login "+strings.Join(args, " "))
		}
	case "пленки":
		b.economyHandler.HandleBalance(ctx, chatID, userID)

	case "отсыпать":
		b.economyHandler.HandleTransfer(ctx, chatID, userID, args)

	case "транзакции":
		b.economyHandler.HandleTransactions(ctx, chatID, userID)

	case "карма":
		if b.cfg.FeatureKarmaEnabled {
			b.karmaHandler.HandleKarma(ctx, chatID, userID)
		}

	case "огонек":
		if b.cfg.FeatureStreaksEnabled {
			b.streakHandler.HandleOgonek(ctx, chatID, userID)
		}

	case "слоты":
		if b.cfg.FeatureCasinoEnabled {
			b.casinoHandler.HandleSlots(ctx, chatID, userID)
		} else {
			b.sendMessage(chatID, "🎰 Казино временно отключено")
		}

	case "статслоты":
		if b.cfg.FeatureCasinoEnabled {
			b.casinoHandler.HandleSlotStats(ctx, chatID, userID)
		}
	}
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
func (b *Bot) sendMessage(chatID int64, text string) {
	if _, err := b.tg.SendMessage(chatID, text, nil); err != nil {
		log.WithError(err).WithField("chat_id", chatID).Error("Ошибка отправки сообщения")
	}
}

// SendMessageToUser отправляет сообщение пользователю (для напоминаний).
func (b *Bot) SendMessageToUser(userID int64, text string) {
	if _, err := b.tg.SendMessage(userID, text, nil); err != nil {
		log.WithError(err).WithField("user_id", userID).Debug("Не удалось отправить сообщение")
	} else {
		log.WithField("user_id", userID).Debug("message sent")
	}
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
