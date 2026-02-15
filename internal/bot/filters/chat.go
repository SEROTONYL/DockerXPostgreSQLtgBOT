// Package filters содержит фильтры для ограничения доступа к боту.
// chat.go реализует основное ограничение: бот работает только в FLOOD_CHAT_ID и DM участников.
package filters

import (
	"context"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	log "github.com/sirupsen/logrus"

	"telegram-bot/internal/features/members"
)

// ChatFilter проверяет, разрешено ли боту обрабатывать сообщение.
type ChatFilter struct {
	floodChatID   int64            // ID разрешённого группового чата
	memberService *members.Service // Сервис для проверки членства
	bot           *tgbotapi.BotAPI // API для отправки ответов
}

// NewChatFilter создаёт фильтр чата.
func NewChatFilter(floodChatID int64, memberService *members.Service, bot *tgbotapi.BotAPI) *ChatFilter {
	return &ChatFilter{
		floodChatID:   floodChatID,
		memberService: memberService,
		bot:           bot,
	}
}

// CheckAccess проверяет, имеет ли сообщение право быть обработанным.
//
// Возвращает:
//   - true: сообщение разрешено (из FLOOD_CHAT_ID или DM от участника)
//   - false: сообщение запрещено (другой чат или DM от неучастника)
func (f *ChatFilter) CheckAccess(ctx context.Context, message *tgbotapi.Message) bool {
	if message == nil {
		return false
	}

	chatID := message.Chat.ID
	userID := message.From.ID

	// Случай 1: Сообщение из FLOOD_CHAT_ID — всегда разрешаем
	if chatID == f.floodChatID {
		return true
	}

	// Случай 2: Личное сообщение — проверяем членство
	if message.Chat.IsPrivate() {
		isMember, err := f.memberService.IsMember(ctx, userID)
		if err != nil {
			log.WithError(err).WithField("user_id", userID).Error("Ошибка проверки членства")
			return false
		}

		if !isMember {
			// Отправляем сообщение об ограничении
			msg := tgbotapi.NewMessage(chatID, "❌ Бот работает только для участников основного чата")
			f.bot.Send(msg)
			return false
		}

		return true
	}

	// Случай 3: Любой другой чат — игнорируем полностью (без ответа)
	return false
}
