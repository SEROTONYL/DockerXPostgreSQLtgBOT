// Package streak вЂ” handlers.go РѕР±СЂР°Р±Р°С‚С‹РІР°РµС‚ РєРѕРјР°РЅРґСѓ !РѕРіРѕРЅРµРє.
// РџРѕРєР°Р·С‹РІР°РµС‚ РїСЂРѕРіСЂРµСЃСЃ СЃС‚СЂРёРєР°: С‚РµРєСѓС‰СѓСЋ СЃРµСЂРёСЋ, СЂРµРєРѕСЂРґ Рё РїСЂРѕРіСЂРµСЃСЃ Р·Р° СЃРµРіРѕРґРЅСЏ.
package streak

import (
	"context"
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	log "github.com/sirupsen/logrus"

	"serotonyl.ru/telegram-bot/internal/common"
	"serotonyl.ru/telegram-bot/internal/config"
)

// Handler РѕР±СЂР°Р±Р°С‚С‹РІР°РµС‚ РєРѕРјР°РЅРґС‹ СЃС‚СЂРёРє-СЃРёСЃС‚РµРјС‹.
type Handler struct {
	service *Service
	bot     *tgbotapi.BotAPI
	cfg     *config.Config
}

// NewHandler СЃРѕР·РґР°С‘С‚ РЅРѕРІС‹Р№ РѕР±СЂР°Р±РѕС‚С‡РёРє СЃС‚СЂРёРє-РєРѕРјР°РЅРґ.
func NewHandler(service *Service, bot *tgbotapi.BotAPI, cfg *config.Config) *Handler {
	return &Handler{service: service, bot: bot, cfg: cfg}
}

// HandleOgonek РѕР±СЂР°Р±Р°С‚С‹РІР°РµС‚ РєРѕРјР°РЅРґСѓ !РѕРіРѕРЅРµРє вЂ” РїРѕРєР°Р·С‹РІР°РµС‚ РїСЂРѕРіСЂРµСЃСЃ СЃС‚СЂРёРєР°.
//
// Р¤РѕСЂРјР°С‚ РѕС‚РІРµС‚Р° (РЅРѕСЂРјР° РЅРµ РІС‹РїРѕР»РЅРµРЅР°):
//   рџ”Ґ РўРІРѕР№ РѕРіРѕРЅРµРє
//   РўРµРєСѓС‰Р°СЏ СЃРµСЂРёСЏ: 8 РґРЅРµР№
//   Р›СѓС‡С€Р°СЏ СЃРµСЂРёСЏ: 12 РґРЅРµР№
//   рџ“Љ РЎРµРіРѕРґРЅСЏ: 35/50 СЃРѕРѕР±С‰РµРЅРёР№
//   РЎС‚Р°С‚СѓСЃ: Р’ РїСЂРѕС†РµСЃСЃРµ (РѕСЃС‚Р°Р»РѕСЃСЊ 15)
//   РќР°РіСЂР°РґР°: 70 РїР»РµРЅРѕРє
//
// Р¤РѕСЂРјР°С‚ РѕС‚РІРµС‚Р° (РЅРѕСЂРјР° РІС‹РїРѕР»РЅРµРЅР°):
//   рџ”Ґ РўРІРѕР№ РѕРіРѕРЅРµРє
//   РўРµРєСѓС‰Р°СЏ СЃРµСЂРёСЏ: 8 РґРЅРµР№
//   Р›СѓС‡С€Р°СЏ СЃРµСЂРёСЏ: 12 РґРЅРµР№
//   вњ… РќРѕСЂРјР° РІС‹РїРѕР»РЅРµРЅР°! +70 РїР»РµРЅРѕРє
func (h *Handler) HandleOgonek(ctx context.Context, chatID int64, userID int64) {
	streak, err := h.service.GetStreak(ctx, userID)
	if err != nil {
		log.WithError(err).Error("РћС€РёР±РєР° РїРѕР»СѓС‡РµРЅРёСЏ СЃС‚СЂРёРєР°")
		h.sendMessage(chatID, "вќЊ РћС€РёР±РєР° РїРѕР»СѓС‡РµРЅРёСЏ РґР°РЅРЅС‹С… СЃС‚СЂРёРєР°")
		return
	}

	var text string
	if streak.QuotaCompletedToday {
		// РќРѕСЂРјР° СѓР¶Рµ РІС‹РїРѕР»РЅРµРЅР° СЃРµРіРѕРґРЅСЏ
		bonus := CalculateReward(streak.CurrentStreak - 1) // -1 С‚.Рє. СѓР¶Рµ СѓРІРµР»РёС‡РµРЅ
		text = fmt.Sprintf(
			"рџ”Ґ РўРІРѕР№ РѕРіРѕРЅРµРє\n\n"+
				"РўРµРєСѓС‰Р°СЏ СЃРµСЂРёСЏ: %d %s\n"+
				"Р›СѓС‡С€Р°СЏ СЃРµСЂРёСЏ: %d %s\n\n"+
				"вњ… РќРѕСЂРјР° РІС‹РїРѕР»РЅРµРЅР°! +%s",
			streak.CurrentStreak, common.PluralizeDays(streak.CurrentStreak),
			streak.LongestStreak, common.PluralizeDays(streak.LongestStreak),
			common.FormatBalance(bonus),
		)
	} else {
		// РќРѕСЂРјР° РµС‰С‘ РЅРµ РІС‹РїРѕР»РЅРµРЅР°
		remaining := h.cfg.StreakMessagesNeed - streak.MessagesToday
		if remaining < 0 {
			remaining = 0
		}
		nextBonus := CalculateReward(streak.CurrentStreak)

		text = fmt.Sprintf(
			"рџ”Ґ РўРІРѕР№ РѕРіРѕРЅРµРє\n\n"+
				"РўРµРєСѓС‰Р°СЏ СЃРµСЂРёСЏ: %d %s\n"+
				"Р›СѓС‡С€Р°СЏ СЃРµСЂРёСЏ: %d %s\n\n"+
				"рџ“Љ РЎРµРіРѕРґРЅСЏ: %d/%d %s\n"+
				"РЎС‚Р°С‚СѓСЃ: Р’ РїСЂРѕС†РµСЃСЃРµ (РѕСЃС‚Р°Р»РѕСЃСЊ %d)\n"+
				"РќР°РіСЂР°РґР°: %s",
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

// sendMessage вЂ” РІСЃРїРѕРјРѕРіР°С‚РµР»СЊРЅС‹Р№ РјРµС‚РѕРґ РґР»СЏ РѕС‚РїСЂР°РІРєРё С‚РµРєСЃС‚РѕРІС‹С… СЃРѕРѕР±С‰РµРЅРёР№.
func (h *Handler) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := h.bot.Send(msg); err != nil {
		log.WithError(err).Error("РћС€РёР±РєР° РѕС‚РїСЂР°РІРєРё СЃРѕРѕР±С‰РµРЅРёСЏ")
	}
}
