// Package bot содержит главный модуль бота — инициализацию, запуск и остановку.
// bot.go создаёт все сервисы, подключает обработчики и запускает polling.
package bot

import (
	"context"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
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
)

// Bot — главная структура бота, объединяющая все компоненты.
type Bot struct {
	api *tgbotapi.BotAPI
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

	// ограничитель параллелизма обработки апдейтов
	inflight chan struct{}
}

// New создаёт новый экземпляр бота со всеми зависимостями.
func New(
	api *tgbotapi.BotAPI,
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
	maxInFlight := cfg.BotMaxInflight
	if maxInFlight <= 0 {
		maxInFlight = 64
	}

	return &Bot{
		api:            api,
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
		inflight:       make(chan struct{}, maxInFlight),
	}
}

// Start запускает polling обновлений от Telegram.
func (b *Bot) Start(ctx context.Context) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = b.cfg.BotUpdateTimeoutSeconds

	updates := b.api.GetUpdatesChan(u)

	log.WithFields(log.Fields{
		"max_inflight": b.cfg.BotMaxInflight,
		"timeout_sec":  b.cfg.BotUpdateTimeoutSeconds,
	}).Info("Бот запущен и ожидает сообщения...")

	for {
		select {
		case <-ctx.Done():
			log.Info("Бот останавливается (ctx done)...")
			b.api.StopReceivingUpdates()
			return

		case update, ok := <-updates:
			if !ok {
				log.Info("Канал updates закрыт, бот остановлен")
				return
			}

			// лимит параллелизма
			b.inflight <- struct{}{}
			go func(upd tgbotapi.Update) {
				defer func() { <-b.inflight }()
				b.handleUpdate(ctx, upd)
			}(update)
		}
	}
}

// handleUpdate обрабатывает одно обновление от Telegram.
func (b *Bot) handleUpdate(ctx context.Context, update tgbotapi.Update) {
	defer middleware.RecoverFromPanic()

	// Обрабатываем новых участников (событие вступления)
	if update.Message != nil && update.Message.NewChatMembers != nil {
		if update.Message.Chat != nil && update.Message.Chat.ID == b.cfg.FloodChatID {
			b.handleNewMembers(ctx, update.Message.NewChatMembers)
		}
		return
	}

	// Обрабатываем обычные сообщения
	if update.Message == nil || update.Message.Text == "" {
		return
	}

	message := update.Message

	// Логируем входящее
	middleware.LogMessage(message)

	// Проверяем доступ (FLOOD_CHAT_ID или DM участника)
	if !b.chatFilter.CheckAccess(ctx, message) {
		return
	}

	// Rate limiting
	if message.From != nil && !b.rateLimiter.Allow(message.From.ID) {
	log.WithField("user_id", message.From.ID).Debug("rate limited")
		return
	}

	chatID := message.Chat.ID
	userID := message.From.ID

	// EnsureMember — ошибки нельзя игнорировать, иначе потом будет "оно не работает"
	if err := b.memberService.EnsureMember(ctx, userID,
		message.From.UserName, message.From.FirstName, message.From.LastName,
	); err != nil {
		log.WithError(err).WithField("user_id", userID).Warn("EnsureMember failed")
	}

	// В DM проверяем админ-панель
	if message.Chat.IsPrivate() {
		handled := b.adminHandler.HandleAdminMessage(ctx, chatID, userID, message.Text)
		if handled {
			return
		}
	}

	// Проверяем «спасибо» для кармы
	if b.cfg.FeatureKarmaEnabled && message.ReplyToMessage != nil && message.ReplyToMessage.From != nil {
		if karma.IsThankYou(message.Text) {
			b.karmaHandler.HandleThankYou(ctx, chatID, userID, message.ReplyToMessage.From.ID)
			return
		}
	}

	// Парсим команду
	cmd, args, isCommand := b.parser.ParseCommand(message.Text)
	log.WithFields(log.Fields{
      "isCommand": isCommand,
      "cmd": cmd,
      "args": args,
      "text": message.Text,
    }).Debug("parsed command")

	if isCommand {
		b.routeCommand(ctx, chatID, userID, cmd, args)
		return
	} else if chatID == b.cfg.FloodChatID {
		// Не команда в основном чате — считаем для стрика
		if b.cfg.FeatureStreaksEnabled {
			// если у CountMessage есть error — логируй; если нет — оставляем как есть
			b.streakService.CountMessage(ctx, userID, message.Text)
		}
	}
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
func (b *Bot) handleNewMembers(ctx context.Context, newMembers []tgbotapi.User) {
	for _, user := range newMembers {
		if err := b.memberService.HandleNewMember(ctx, user.ID, user.UserName, user.FirstName, user.LastName); err != nil {
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

		log.WithField("user", user.UserName).Info("Новый участник обработан")
	}
}

// sendMessage — утилита для отправки сообщений.
func (b *Bot) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := b.api.Send(msg); err != nil {
		log.WithError(err).WithField("chat_id", chatID).Error("Ошибка отправки сообщения")
	}
}

// SendMessageToUser отправляет сообщение пользователю (для напоминаний).
func (b *Bot) SendMessageToUser(userID int64, text string) {
	msg := tgbotapi.NewMessage(userID, text)
	if _, err := b.api.Send(msg); err != nil {
		log.WithError(err).WithField("user_id", userID).Debug("Не удалось отправить сообщение")
	} else {
        log.WithField("user_id", userID).Debug("message sent")
    }
}

// CommandParser парсит русские команды с префиксами ! и .
type CommandParser struct {
	validPrefixes []string
}

// NewCommandParser создаёт парсер команд.
func NewCommandParser() *CommandParser {
	return &CommandParser{
		validPrefixes: []string{"!", ".","/"},
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