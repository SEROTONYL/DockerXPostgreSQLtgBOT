// Package economy вЂ” handlers.go РѕР±СЂР°Р±Р°С‚С‹РІР°РµС‚ РєРѕРјР°РЅРґС‹:
// !РїР»РµРЅРєРё (Р±Р°Р»Р°РЅСЃ), !РѕС‚СЃС‹РїР°С‚СЊ (РїРµСЂРµРІРѕРґ), !С‚СЂР°РЅР·Р°РєС†РёРё (РёСЃС‚РѕСЂРёСЏ).
package economy

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	log "github.com/sirupsen/logrus"

	"serotonyl.ru/telegram-bot/internal/common"
	"serotonyl.ru/telegram-bot/internal/features/members"
)

// Handler РѕР±СЂР°Р±Р°С‚С‹РІР°РµС‚ РєРѕРјР°РЅРґС‹ СЌРєРѕРЅРѕРјРёРєРё.
type Handler struct {
	service       *Service          // РЎРµСЂРІРёСЃ СЌРєРѕРЅРѕРјРёРєРё
	memberService *members.Service  // РЎРµСЂРІРёСЃ СѓС‡Р°СЃС‚РЅРёРєРѕРІ (РґР»СЏ РїРѕРёСЃРєР° РїРѕР»СѓС‡Р°С‚РµР»СЏ)
	bot           *tgbotapi.BotAPI  // API Telegram РґР»СЏ РѕС‚РїСЂР°РІРєРё РѕС‚РІРµС‚РѕРІ
}

// NewHandler СЃРѕР·РґР°С‘С‚ РЅРѕРІС‹Р№ РѕР±СЂР°Р±РѕС‚С‡РёРє СЌРєРѕРЅРѕРјРёС‡РµСЃРєРёС… РєРѕРјР°РЅРґ.
func NewHandler(service *Service, memberService *members.Service, bot *tgbotapi.BotAPI) *Handler {
	return &Handler{
		service:       service,
		memberService: memberService,
		bot:           bot,
	}
}

// HandleBalance РѕР±СЂР°Р±Р°С‚С‹РІР°РµС‚ РєРѕРјР°РЅРґСѓ !РїР»РµРЅРєРё вЂ” РїРѕРєР°Р·С‹РІР°РµС‚ Р±Р°Р»Р°РЅСЃ.
//
// Р¤РѕСЂРјР°С‚ РѕС‚РІРµС‚Р°:
//
//	рџ’° Р‘Р°Р»Р°РЅСЃ: 150 РїР»РµРЅРѕРє
func (h *Handler) HandleBalance(ctx context.Context, chatID int64, userID int64) {
	balance, err := h.service.GetBalance(ctx, userID)
	if err != nil {
		log.WithError(err).Error("РћС€РёР±РєР° РїРѕР»СѓС‡РµРЅРёСЏ Р±Р°Р»Р°РЅСЃР°")
		h.sendMessage(chatID, "вќЊ РћС€РёР±РєР° РїРѕР»СѓС‡РµРЅРёСЏ Р±Р°Р»Р°РЅСЃР°")
		return
	}

	text := fmt.Sprintf("рџ’° Р‘Р°Р»Р°РЅСЃ: %s", common.FormatBalance(balance))
	h.sendMessage(chatID, text)
}

// HandleTransfer РѕР±СЂР°Р±Р°С‚С‹РІР°РµС‚ РєРѕРјР°РЅРґСѓ !РѕС‚СЃС‹РїР°С‚СЊ @username 100.
// РџРµСЂРµРІРѕРґРёС‚ СѓРєР°Р·Р°РЅРЅСѓСЋ СЃСѓРјРјСѓ РѕС‚ РѕС‚РїСЂР°РІРёС‚РµР»СЏ Рє РїРѕР»СѓС‡Р°С‚РµР»СЋ.
//
// Р¤РѕСЂРјР°С‚: !РѕС‚СЃС‹РїР°С‚СЊ @username 100
// РёР»Рё: !РѕС‚СЃС‹РїР°С‚СЊ username 100 (Р±РµР· @)
//
// РћС‚РІРµС‚ РїСЂРё СѓСЃРїРµС…Рµ:
//
//	вњ… РџРµСЂРµРІРµРґРµРЅРѕ 100 РїР»РµРЅРѕРє @username
//	РўРІРѕР№ Р±Р°Р»Р°РЅСЃ: 50 РїР»РµРЅРѕРє
func (h *Handler) HandleTransfer(ctx context.Context, chatID int64, fromUserID int64, args []string) {
	// РџСЂРѕРІРµСЂСЏРµРј Р°СЂРіСѓРјРµРЅС‚С‹: РЅСѓР¶РµРЅ @username Рё СЃСѓРјРјР°
	if len(args) < 2 {
		h.sendMessage(chatID, "вќЊ Р¤РѕСЂРјР°С‚: !РѕС‚СЃС‹РїР°С‚СЊ @username СЃСѓРјРјР°")
		return
	}

	// РџР°СЂСЃРёРј username (СѓР±РёСЂР°РµРј @ РµСЃР»Рё РµСЃС‚СЊ)
	username := strings.TrimPrefix(args[0], "@")
	if username == "" {
		h.sendMessage(chatID, "вќЊ РЈРєР°Р¶РёС‚Рµ @username РїРѕР»СѓС‡Р°С‚РµР»СЏ")
		return
	}

	// РџР°СЂСЃРёРј СЃСѓРјРјСѓ
	amount, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil || amount <= 0 {
		h.sendMessage(chatID, "вќЊ РЎСѓРјРјР° РґРѕР»Р¶РЅР° Р±С‹С‚СЊ РїРѕР»РѕР¶РёС‚РµР»СЊРЅС‹Рј С‡РёСЃР»РѕРј")
		return
	}

	// РќР°С…РѕРґРёРј РїРѕР»СѓС‡Р°С‚РµР»СЏ РїРѕ username
	recipient, err := h.memberService.GetByUsername(ctx, username)
	if err != nil {
		h.sendMessage(chatID, "вќЊ РџРѕР»СЊР·РѕРІР°С‚РµР»СЊ РЅРµ РЅР°Р№РґРµРЅ")
		return
	}

	// Р’С‹РїРѕР»РЅСЏРµРј РїРµСЂРµРІРѕРґ
	err = h.service.Transfer(ctx, fromUserID, recipient.UserID, amount)
	if err != nil {
		switch err {
		case common.ErrSelfTransfer:
			h.sendMessage(chatID, "вќЊ РќРµР»СЊР·СЏ РїРµСЂРµРІРѕРґРёС‚СЊ РїР»РµРЅРєРё СЃР°РјРѕРјСѓ СЃРµР±Рµ")
		case common.ErrInsufficientBalance:
			h.sendMessage(chatID, "вќЊ РќРµРґРѕСЃС‚Р°С‚РѕС‡РЅРѕ РїР»РµРЅРѕРє РЅР° СЃС‡С‘С‚Рµ")
		case common.ErrInvalidAmount:
			h.sendMessage(chatID, "вќЊ РЎСѓРјРјР° РґРѕР»Р¶РЅР° Р±С‹С‚СЊ РїРѕР»РѕР¶РёС‚РµР»СЊРЅРѕР№")
		default:
			log.WithError(err).Error("РћС€РёР±РєР° РїРµСЂРµРІРѕРґР°")
			h.sendMessage(chatID, "вќЊ РћС€РёР±РєР° РІС‹РїРѕР»РЅРµРЅРёСЏ РїРµСЂРµРІРѕРґР°")
		}
		return
	}

	// РџРѕР»СѓС‡Р°РµРј РЅРѕРІС‹Р№ Р±Р°Р»Р°РЅСЃ РѕС‚РїСЂР°РІРёС‚РµР»СЏ
	newBalance, _ := h.service.GetBalance(ctx, fromUserID)

	text := fmt.Sprintf("вњ… РџРµСЂРµРІРµРґРµРЅРѕ %s @%s\nРўРІРѕР№ Р±Р°Р»Р°РЅСЃ: %s",
		common.FormatBalance(amount), username, common.FormatBalance(newBalance))
	h.sendMessage(chatID, text)
}

// HandleTransactions РѕР±СЂР°Р±Р°С‚С‹РІР°РµС‚ РєРѕРјР°РЅРґСѓ !С‚СЂР°РЅР·Р°РєС†РёРё вЂ” РїРѕРєР°Р·С‹РІР°РµС‚ РёСЃС‚РѕСЂРёСЋ.
func (h *Handler) HandleTransactions(ctx context.Context, chatID int64, userID int64) {
	history, err := h.service.GetTransactionHistory(ctx, userID)
	if err != nil {
		log.WithError(err).Error("РћС€РёР±РєР° РїРѕР»СѓС‡РµРЅРёСЏ С‚СЂР°РЅР·Р°РєС†РёР№")
		h.sendMessage(chatID, "вќЊ РћС€РёР±РєР° РїРѕР»СѓС‡РµРЅРёСЏ РёСЃС‚РѕСЂРёРё С‚СЂР°РЅР·Р°РєС†РёР№")
		return
	}

	// РћС‚РїСЂР°РІР»СЏРµРј СЃ MarkdownV2 РґР»СЏ РїРѕРґРґРµСЂР¶РєРё СЃРїРѕР№Р»РµСЂРѕРІ
	msg := tgbotapi.NewMessage(chatID, history)
	msg.ParseMode = "MarkdownV2"
	if _, err := h.bot.Send(msg); err != nil {
		// Р•СЃР»Рё MarkdownV2 РЅРµ СЃСЂР°Р±РѕС‚Р°Р» вЂ” РѕС‚РїСЂР°РІР»СЏРµРј Р±РµР· С„РѕСЂРјР°С‚РёСЂРѕРІР°РЅРёСЏ
		h.sendMessage(chatID, history)
	}
}

// sendMessage вЂ” РІСЃРїРѕРјРѕРіР°С‚РµР»СЊРЅС‹Р№ РјРµС‚РѕРґ РґР»СЏ РѕС‚РїСЂР°РІРєРё С‚РµРєСЃС‚РѕРІС‹С… СЃРѕРѕР±С‰РµРЅРёР№.
func (h *Handler) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := h.bot.Send(msg); err != nil {
		log.WithError(err).Error("РћС€РёР±РєР° РѕС‚РїСЂР°РІРєРё СЃРѕРѕР±С‰РµРЅРёСЏ")
	}
}
