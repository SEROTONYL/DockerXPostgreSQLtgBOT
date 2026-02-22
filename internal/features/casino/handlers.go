// Package casino вЂ” handlers.go РѕР±СЂР°Р±Р°С‚С‹РІР°РµС‚ РєРѕРјР°РЅРґС‹ !СЃР»РѕС‚С‹ Рё !СЃС‚Р°С‚СЃР»РѕС‚С‹.
package casino

import (
	"context"
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	log "github.com/sirupsen/logrus"

	"serotonyl.ru/telegram-bot/internal/common"
)

// Handler РѕР±СЂР°Р±Р°С‚С‹РІР°РµС‚ РєРѕРјР°РЅРґС‹ РєР°Р·РёРЅРѕ.
type Handler struct {
	service *Service
	bot     *tgbotapi.BotAPI
}

// NewHandler СЃРѕР·РґР°С‘С‚ РѕР±СЂР°Р±РѕС‚С‡РёРє РєР°Р·РёРЅРѕ.
func NewHandler(service *Service, bot *tgbotapi.BotAPI) *Handler {
	return &Handler{service: service, bot: bot}
}

// HandleSlots РѕР±СЂР°Р±Р°С‚С‹РІР°РµС‚ РєРѕРјР°РЅРґСѓ !СЃР»РѕС‚С‹ вЂ” СЃРїРёРЅ СЃР»РѕС‚-РјР°С€РёРЅС‹.
//
// Р¤РѕСЂРјР°С‚ РѕС‚РІРµС‚Р°:
//
//	рџЋ° РЎР›РћРўР« рџЋ°
//
//	рџЌ’ рџЌ‹ рџ’Ћ рџЌЉ рџЌ‡
//	рџЌ‹ рџЌ’ в­ђ рџЌ‹ рџЌ‰
//	рџЌЉ рџ’Ћ рџЌ’ рџЌ’ рџЌ’  в†ђ Р’Р«РР“Р Р«РЁ! 3x рџЌ’
//	...
//
//	рџ’° Р’С‹РїР»Р°С‚Р°: 100 РїР»РµРЅРѕРє (2x)
//	рџ“Љ Р‘Р°Р»Р°РЅСЃ: 150 РїР»РµРЅРѕРє
func (h *Handler) HandleSlots(ctx context.Context, chatID int64, userID int64) {
	// Р’С‹РїРѕР»РЅСЏРµРј СЃРїРёРЅ
	result, err := h.service.PlaySlots(ctx, userID)
	if err != nil {
		// РџСЂРѕРІРµСЂСЏРµРј С‚РёРї РѕС€РёР±РєРё РґР»СЏ РїРѕРЅСЏС‚РЅРѕРіРѕ СЃРѕРѕР±С‰РµРЅРёСЏ
		if strings.Contains(err.Error(), "РЅРµРґРѕСЃС‚Р°С‚РѕС‡РЅРѕ") {
			h.sendMessage(chatID, fmt.Sprintf("вќЊ РќРµРґРѕСЃС‚Р°С‚РѕС‡РЅРѕ РїР»РµРЅРѕРє! РЎС‚Р°РІРєР°: %s",
				common.FormatBalance(h.service.cfg.CasinoSlotsBet)))
		} else {
			log.WithError(err).Error("РћС€РёР±РєР° СЃРїРёРЅР° СЃР»РѕС‚РѕРІ")
			h.sendMessage(chatID, "вќЊ РћС€РёР±РєР° РїСЂРё РёРіСЂРµ РІ СЃР»РѕС‚С‹")
		}
		return
	}

	// Р¤РѕСЂРјРёСЂСѓРµРј РѕС‚РІРµС‚
	var sb strings.Builder
	sb.WriteString("рџЋ° РЎР›РћРўР« рџЋ°\n\n")

	// РЎРµС‚РєР°
	sb.WriteString(FormatGrid(result.Grid))

	// Р’С‹РёРіСЂС‹С€РЅС‹Рµ Р»РёРЅРёРё
	if result.IsWin {
		sb.WriteString("\n")
		for _, win := range result.WinLines {
			sb.WriteString(fmt.Sprintf("вњ… Р›РёРЅРёСЏ %d: %dx %s в†’ %s\n",
				win.LineIndex+1, win.Count, win.Symbol,
				common.FormatBalance(win.Payout)))
		}
	}

	// РЎРєР°С‚С‚РµСЂ-Р±РѕРЅСѓСЃ
	if result.ScatterCount >= 3 {
		sb.WriteString(fmt.Sprintf("\nрџЋ° РЎРєР°С‚С‚РµСЂ Р±РѕРЅСѓСЃ! %d СЃРєР°С‚С‚РµСЂРѕРІ в†’ +%s",
			result.ScatterCount, common.FormatBalance(result.ScatterWin)))
		if result.FreeSpins > 0 {
			sb.WriteString(fmt.Sprintf(" + %d С„СЂРёСЃРїРёРЅРѕРІ!", result.FreeSpins))
		}
		sb.WriteString("\n")
	}

	// РС‚РѕРі
	sb.WriteString("\n")
	if result.IsWin {
		sb.WriteString(fmt.Sprintf("рџ’° Р’С‹РїР»Р°С‚Р°: %s\n", common.FormatBalance(result.TotalPayout)))
	} else {
		sb.WriteString("рџ’ё РќРµС‚ РІС‹РёРіСЂС‹С€Р°\n")
	}

	// РўРµРєСѓС‰РёР№ Р±Р°Р»Р°РЅСЃ
	balance, _ := h.service.economyService.GetBalance(ctx, userID)
	sb.WriteString(fmt.Sprintf("рџ“Љ Р‘Р°Р»Р°РЅСЃ: %s", common.FormatBalance(balance)))

	h.sendMessage(chatID, sb.String())
}

// HandleSlotStats РѕР±СЂР°Р±Р°С‚С‹РІР°РµС‚ РєРѕРјР°РЅРґСѓ !СЃС‚Р°С‚СЃР»РѕС‚С‹ вЂ” СЃС‚Р°С‚РёСЃС‚РёРєР°.
//
// Р¤РѕСЂРјР°С‚ РѕС‚РІРµС‚Р°:
//
//	рџ“Љ РЎРўРђРўРРЎРўРРљРђ РЎР›РћРўРћР’
//	Р’СЃРµРіРѕ СЃРїРёРЅРѕРІ: 47
//	РџРѕСЃС‚Р°РІР»РµРЅРѕ: 2 350 РїР»РµРЅРѕРє
//	Р’С‹РёРіСЂР°РЅРѕ: 2 120 РїР»РµРЅРѕРє
//	Р§РёСЃС‚Р°СЏ РїСЂРёР±С‹Р»СЊ: -230 РїР»РµРЅРѕРє
//	рџ’Ћ Р›СѓС‡С€РёР№ РІС‹РёРіСЂС‹С€: 1 500 РїР»РµРЅРѕРє
//	рџ“€ РўРІРѕР№ RTP: 90.21%
func (h *Handler) HandleSlotStats(ctx context.Context, chatID int64, userID int64) {
	stats, err := h.service.GetStats(ctx, userID)
	if err != nil {
		h.sendMessage(chatID, "рџ“Љ РЈ С‚РµР±СЏ РїРѕРєР° РЅРµС‚ СЃС‚Р°С‚РёСЃС‚РёРєРё СЃР»РѕС‚РѕРІ. РЎС‹РіСЂР°Р№ РїРµСЂРІС‹Р№ СЃРїРёРЅ!")
		return
	}

	netProfit := stats.TotalWon - stats.TotalWagered
	profitSign := ""
	if netProfit > 0 {
		profitSign = "+"
	}

	text := fmt.Sprintf(
		"рџ“Љ РЎРўРђРўРРЎРўРРљРђ РЎР›РћРўРћР’\n\n"+
			"Р’СЃРµРіРѕ СЃРїРёРЅРѕРІ: %d\n"+
			"РџРѕСЃС‚Р°РІР»РµРЅРѕ: %s %s\n"+
			"Р’С‹РёРіСЂР°РЅРѕ: %s %s\n"+
			"Р§РёСЃС‚Р°СЏ РїСЂРёР±С‹Р»СЊ: %s%s %s\n\n"+
			"рџ’Ћ Р›СѓС‡С€РёР№ РІС‹РёРіСЂС‹С€: %s %s\n"+
			"рџ“€ РўРІРѕР№ RTP: %.2f%%",
		stats.TotalSpins,
		common.FormatNumber(stats.TotalWagered), common.PluralizeFilms(stats.TotalWagered),
		common.FormatNumber(stats.TotalWon), common.PluralizeFilms(stats.TotalWon),
		profitSign, common.FormatNumber(netProfit), common.PluralizeFilms(netProfit),
		common.FormatNumber(stats.BiggestWin), common.PluralizeFilms(stats.BiggestWin),
		stats.CurrentRTP,
	)

	h.sendMessage(chatID, text)
}

func (h *Handler) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := h.bot.Send(msg); err != nil {
		log.WithError(err).Error("РћС€РёР±РєР° РѕС‚РїСЂР°РІРєРё СЃРѕРѕР±С‰РµРЅРёСЏ")
	}
}
