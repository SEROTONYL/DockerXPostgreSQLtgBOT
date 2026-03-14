// Package casino — handlers.go обрабатывает команды !слоты и !статслоты.
package casino

import (
	"context"
	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"

	"serotonyl.ru/telegram-bot/internal/common"
	"serotonyl.ru/telegram-bot/internal/telegram"
)

// Handler обрабатывает команды казино.
type Handler struct {
	service *Service
	tgOps   *telegram.Ops
}

// NewHandler создаёт обработчик казино.
func NewHandler(service *Service, tgOps *telegram.Ops) *Handler {
	return &Handler{service: service, tgOps: tgOps}
}

// HandleSlots обрабатывает команду !слоты — спин слот-машины.
//
// Формат ответа:
//
//	🎰 СЛОТЫ 🎰
//
//	🍒 🍋 💎 🍊 🍇
//	🍋 🍒 ⭐ 🍋 🍉
//	🍊 💎 🍒 🍒 🍒  ← ВЫИГРЫШ! 3x 🍒
//	...
//
//	💰 Выплата: 100 пленок (2x)
//	📊 Баланс: 150 пленок
func (h *Handler) HandleSlots(ctx context.Context, chatID int64, userID int64) {
	// Выполняем спин
	result, err := h.service.PlaySlots(ctx, userID)
	if err != nil {
		// Проверяем тип ошибки для понятного сообщения
		if strings.Contains(err.Error(), "недостаточно") {
			h.sendMessage(ctx, chatID, fmt.Sprintf("❌ Недостаточно плёнок! Ставка: %s",
				common.FormatBalance(h.service.cfg.CasinoSlotsBet)))
		} else {
			log.WithError(err).Error("Ошибка спина слотов")
			h.sendMessage(ctx, chatID, "❌ Ошибка при игре в слоты")
		}
		return
	}

	// Формируем ответ
	var sb strings.Builder
	sb.WriteString("🎰 СЛОТЫ 🎰\n\n")

	// Сетка
	sb.WriteString(FormatGrid(result.Grid))

	// Выигрышные линии
	if result.IsWin {
		sb.WriteString("\n")
		for _, win := range result.WinLines {
			fmt.Fprintf(&sb, "✅ Линия %d: %dx %s → %s\n",
				win.LineIndex+1, win.Count, win.Symbol,
				common.FormatBalance(win.Payout))
		}
	}

	// Скаттер-бонус
	if result.ScatterCount >= 3 {
		fmt.Fprintf(&sb, "\n🎰 Скаттер бонус! %d скаттеров → +%s",
			result.ScatterCount, common.FormatBalance(result.ScatterWin))
		if result.FreeSpins > 0 {
			fmt.Fprintf(&sb, " + %d фриспинов!", result.FreeSpins)
		}
		sb.WriteString("\n")
	}

	// Итог
	sb.WriteString("\n")
	if result.IsWin {
		fmt.Fprintf(&sb, "💰 Выплата: %s\n", common.FormatBalance(result.TotalPayout))
	} else {
		sb.WriteString("💸 Нет выигрыша\n")
	}

	// Текущий баланс
	balance, _ := h.service.economyService.GetBalance(ctx, userID)
	fmt.Fprintf(&sb, "📊 Баланс: %s", common.FormatBalance(balance))

	h.sendMessage(ctx, chatID, sb.String())
}

// HandleSlotStats обрабатывает команду !статслоты — статистика.
//
// Формат ответа:
//
//	📊 СТАТИСТИКА СЛОТОВ
//	Всего спинов: 47
//	Поставлено: 2 350 пленок
//	Выиграно: 2 120 пленок
//	Чистая прибыль: -230 пленок
//	💎 Лучший выигрыш: 1 500 пленок
//	📈 Твой RTP: 90.21%
func (h *Handler) HandleSlotStats(ctx context.Context, chatID int64, userID int64) {
	stats, err := h.service.GetStats(ctx, userID)
	if err != nil {
		h.sendMessage(ctx, chatID, "📊 У тебя пока нет статистики слотов. Сыграй первый спин!")
		return
	}

	netProfit := stats.TotalWon - stats.TotalWagered
	profitSign := ""
	if netProfit > 0 {
		profitSign = "+"
	}

	text := fmt.Sprintf(
		"📊 СТАТИСТИКА СЛОТОВ\n\n"+
			"Всего спинов: %d\n"+
			"Поставлено: %s%s\n"+
			"Выиграно: %s%s\n"+
			"Чистая прибыль: %s%s%s\n\n"+
			"💎 Лучший выигрыш: %s%s\n"+
			"📈 Твой RTP: %.2f%%",
		stats.TotalSpins,
		common.FormatNumber(stats.TotalWagered), common.PluralizeFilms(stats.TotalWagered),
		common.FormatNumber(stats.TotalWon), common.PluralizeFilms(stats.TotalWon),
		profitSign, common.FormatNumber(netProfit), common.PluralizeFilms(netProfit),
		common.FormatNumber(stats.BiggestWin), common.PluralizeFilms(stats.BiggestWin),
		stats.CurrentRTP,
	)

	h.sendMessage(ctx, chatID, text)
}

func (h *Handler) sendMessage(ctx context.Context, chatID int64, text string) {
	_, _ = h.tgOps.Send(ctx, chatID, text, nil)
}
