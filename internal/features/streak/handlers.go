// Package streak — handlers.go обрабатывает команду !огонек.
// Показывает прогресс стрика: текущую серию, рекорд и прогресс за сегодня.
package streak

import (
	"context"
	"fmt"

	log "github.com/sirupsen/logrus"

	"serotonyl.ru/telegram-bot/internal/common"
	"serotonyl.ru/telegram-bot/internal/config"
	"serotonyl.ru/telegram-bot/internal/telegram"
)

// Handler обрабатывает команды стрик-системы.
type Handler struct {
	service *Service
	bot     telegram.Client
	cfg     *config.Config
}

// NewHandler создаёт новый обработчик стрик-команд.
func NewHandler(service *Service, bot telegram.Client, cfg *config.Config) *Handler {
	return &Handler{service: service, bot: bot, cfg: cfg}
}

// HandleOgonek обрабатывает команду !огонек — показывает прогресс стрика.
//
// Формат ответа (норма не выполнена):
//
//	🔥 Твой огонек
//	Текущая серия: 8 дней
//	Лучшая серия: 12 дней
//	📊 Сегодня: 35/50 сообщений
//	Статус: В процессе (осталось 15)
//	Награда: 70 пленок
//
// Формат ответа (норма выполнена):
//
//	🔥 Твой огонек
//	Текущая серия: 8 дней
//	Лучшая серия: 12 дней
//	✅ Норма выполнена! +70 пленок
func (h *Handler) HandleOgonek(ctx context.Context, chatID int64, userID int64) {
	streak, err := h.service.GetStreak(ctx, userID)
	if err != nil {
		log.WithError(err).Error("Ошибка получения стрика")
		h.sendMessage(chatID, "❌ Ошибка получения данных стрика")
		return
	}

	var text string
	if streak.QuotaCompletedToday {
		// Норма уже выполнена сегодня
		bonus := CalculateReward(streak.CurrentStreak - 1) // -1 т.к. уже увеличен
		text = fmt.Sprintf(
			"🔥 Твой огонек\n\n"+
				"Текущая серия: %d %s\n"+
				"Лучшая серия: %d %s\n\n"+
				"✅ Норма выполнена! +%s",
			streak.CurrentStreak, common.PluralizeDays(streak.CurrentStreak),
			streak.LongestStreak, common.PluralizeDays(streak.LongestStreak),
			common.FormatBalance(bonus),
		)
	} else {
		// Норма ещё не выполнена
		remaining := h.cfg.StreakMessagesNeed - streak.MessagesToday
		if remaining < 0 {
			remaining = 0
		}
		nextBonus := CalculateReward(streak.CurrentStreak)

		text = fmt.Sprintf(
			"🔥 Твой огонек\n\n"+
				"Текущая серия: %d %s\n"+
				"Лучшая серия: %d %s\n\n"+
				"📊 Сегодня: %d/%d %s\n"+
				"Статус: В процессе (осталось %d)\n"+
				"Награда: %s",
			streak.CurrentStreak, common.PluralizeDays(streak.CurrentStreak),
			streak.LongestStreak, common.PluralizeDays(streak.LongestStreak),
			streak.MessagesToday, h.cfg.StreakMessagesNeed,
			common.PluralizeMessages(streak.MessagesToday),
			remaining,
			common.FormatBalance(nextBonus),
		)
	}

	h.sendMessage(chatID, text)
}

// sendMessage — вспомогательный метод для отправки текстовых сообщений.
func (h *Handler) sendMessage(chatID int64, text string) {
	if _, err := h.bot.SendMessage(chatID, text, nil); err != nil {
		log.WithError(err).Error("Ошибка отправки сообщения")
	}
}
