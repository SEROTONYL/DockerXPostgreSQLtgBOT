// Package bot содержит главный модуль бота — инициализацию, запуск и остановку.
// bot.go создаёт все сервисы, подключает обработчики и запускает polling.
package bot

import (
	"context"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	log "github.com/sirupsen/logrus"

	"telegram-bot/internal/bot/filters"
	"telegram-bot/internal/bot/middleware"
	"telegram-bot/internal/config"
	"telegram-bot/internal/features/admin"
	"telegram-bot/internal/features/casino"
	"telegram-bot/internal/features/economy"
	"telegram-bot/internal/features/karma"
	"telegram-bot/internal/features/members"
	"telegram-bot/internal/features/streak"
)

// Bot — главная структура бота, объединяющая все компоненты.
type Bot struct {
	api *tgbotapi.BotAPI // Telegram Bot API
	cfg *config.Config   // Конфигурация

	// Фильтры и middleware
	chatFilter  *filters.ChatFilter
	rateLimiter *middleware.RateLimiter

	// Обработчики фич
	memberHandler  *members.Handler
	economyHandler *economy.Handler
	streakHandler  *streak.Handler
	karmaHandler   *karma.Handler
	casinoHandler  *casino.Handler
	adminHandler   *admin.Handler

	// Сервисы (нужны для межмодульного взаимодействия)
	memberService  *members.Service
	economyService *economy.Service
	streakService  *streak.Service
	karmaService   *karma.Service
	casinoService  *casino.Service
	adminService   *admin.Service

	// Парсер команд
	parser *CommandParser
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
	}
}

// Start запускает polling обновлений от Telegram.
func (b *Bot) Start(ctx context.Context) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60 // Long polling: ждём до 60 секунд

	updates := b.api.GetUpdatesChan(u)

	log.Info("Бот запущен и ожидает сообщения...")

	for {
		select {
		case <-ctx.Done():
			log.Info("Бот останавливается...")
			return
		case update := <-updates:
			go b.handleUpdate(ctx, update)
		}
	}
}

// handleUpdate обрабатывает одно обновление от Telegram.
func (b *Bot) handleUpdate(ctx context.Context, update tgbotapi.Update) {
	// Защита от паник
	defer middleware.RecoverFromPanic()

	// Обрабатываем новых участников (событие вступления)
	if update.Message != nil && update.Message.NewChatMembers != nil {
		if update.Message.Chat.ID == b.cfg.FloodChatID {
			b.handleNewMembers(ctx, update.Message.NewChatMembers)
		}
		return
	}

	// Обрабатываем обычные сообщения
	if update.Message == nil || update.Message.Text == "" {
		return
	}

	message := update.Message

	// Логируем
	middleware.LogMessage(message)

	// Проверяем доступ (FLOOD_CHAT_ID или DM участника)
	if !b.chatFilter.CheckAccess(ctx, message) {
		return
	}

	// Rate limiting
	if !b.rateLimiter.Allow(message.From.ID) {
		return // Тихо игнорируем
	}

	chatID := message.Chat.ID
	userID := message.From.ID

	// Обеспечиваем регистрацию пользователя
	b.memberService.EnsureMember(ctx, userID,
		message.From.UserName, message.From.FirstName, message.From.LastName)

	// В DM проверяем админ-панель
	if message.Chat.IsPrivate() {
		handled := b.adminHandler.HandleAdminMessage(ctx, chatID, userID, message.Text)
		if handled {
			return
		}
	}

	// Проверяем «спасибо» для кармы
	if b.cfg.FeatureKarmaEnabled && message.ReplyToMessage != nil {
		if karma.IsThankYou(message.Text) {
			b.karmaHandler.HandleThankYou(ctx, chatID, userID, message.ReplyToMessage.From.ID)
			return
		}
	}

	// Парсим команду
	cmd, args, isCommand := b.parser.ParseCommand(message.Text)

	if isCommand {
		b.routeCommand(ctx, chatID, userID, cmd, args)
	} else if chatID == b.cfg.FloodChatID {
		// Не команда в основном чате — считаем для стрика
		if b.cfg.FeatureStreaksEnabled {
			b.streakService.CountMessage(ctx, userID, message.Text)
		}
	}
}

// routeCommand маршрутизирует команду к нужному обработчику.
func (b *Bot) routeCommand(ctx context.Context, chatID, userID int64, cmd string, args []string) {
	switch cmd {
	// Экономика
	case "пленки":
		b.economyHandler.HandleBalance(ctx, chatID, userID)
	case "отсыпать":
		b.economyHandler.HandleTransfer(ctx, chatID, userID, args)
	case "транзакции":
		b.economyHandler.HandleTransactions(ctx, chatID, userID)

	// Карма
	case "карма":
		if b.cfg.FeatureKarmaEnabled {
			b.karmaHandler.HandleKarma(ctx, chatID, userID)
		}

	// Стрик
	case "огонек":
		if b.cfg.FeatureStreaksEnabled {
			b.streakHandler.HandleOgonek(ctx, chatID, userID)
		}

	// Казино
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
		// Регистрируем участника
		b.memberService.HandleNewMember(ctx, user.ID, user.UserName, user.FirstName, user.LastName)

		// Создаём связанные записи
		b.economyService.CreateBalance(ctx, user.ID)
		b.streakService.CreateStreak(ctx, user.ID)
		b.karmaService.CreateKarma(ctx, user.ID)

		log.WithField("user", user.UserName).Info("Новый участник зарегистрирован")
	}
}

// sendMessage — утилита для отправки сообщений.
func (b *Bot) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := b.api.Send(msg); err != nil {
		log.WithError(err).Error("Ошибка отправки сообщения")
	}
}

// SendMessageToUser отправляет сообщение пользователю (для напоминаний).
func (b *Bot) SendMessageToUser(userID int64, text string) {
	msg := tgbotapi.NewMessage(userID, text)
	if _, err := b.api.Send(msg); err != nil {
		log.WithError(err).WithField("user_id", userID).Debug("Не удалось отправить сообщение")
	}
}

// CommandParser парсит русские команды с префиксами ! и .
type CommandParser struct {
	validPrefixes []string
}

// NewCommandParser создаёт парсер команд.
func NewCommandParser() *CommandParser {
	return &CommandParser{
		validPrefixes: []string{"!", "."},
	}
}

// ParseCommand разбирает текст на команду и аргументы.
//
// Примеры:
//
//	"!пленки"           → ("пленки", nil, true)
//	".отсыпать @ivan 100" → ("отсыпать", ["@ivan", "100"], true)
//	"! пленки"          → ("пленки", nil, true)  — пробел после префикса OK
//	"привет"            → ("", nil, false)        — не команда
func (p *CommandParser) ParseCommand(text string) (string, []string, bool) {
	text = strings.TrimSpace(text)

	// Проверяем префикс
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

	// Убираем лишние пробелы
	text = strings.TrimSpace(text)
	parts := strings.Fields(text)

	if len(parts) == 0 {
		return "", nil, false
	}

	// Команда в нижнем регистре
	command := strings.ToLower(parts[0])
	var args []string
	if len(parts) > 1 {
		args = parts[1:]
	}

	return command, args, true
}
