// Package economy вЂ” handlers.go РѕР±СЂР°Р±Р°С‚С‹РІР°РµС‚ РєРѕРјР°РЅРґС‹:
// !РїР»РµРЅРєРё (Р±Р°Р»Р°РЅСЃ), !РѕС‚СЃС‹РїР°С‚СЊ (РїРµСЂРµРІРѕРґ), !С‚СЂР°РЅР·Р°РєС†РёРё (РёСЃС‚РѕСЂРёСЏ).
package economy

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"

	"serotonyl.ru/telegram-bot/internal/common"
	"serotonyl.ru/telegram-bot/internal/features/members"
	"serotonyl.ru/telegram-bot/internal/telegram"
)

// Handler РѕР±СЂР°Р±Р°С‚С‹РІР°РµС‚ РєРѕРјР°РЅРґС‹ СЌРєРѕРЅРѕРјРёРєРё.
type Handler struct {
	service       *Service         // РЎРµСЂРІРёСЃ СЌРєРѕРЅРѕРјРёРєРё
	memberService *members.Service // РЎРµСЂРІРёСЃ СѓС‡Р°СЃС‚РЅРёРєРѕРІ (РґР»СЏ РїРѕРёСЃРєР° РїРѕР»СѓС‡Р°С‚РµР»СЏ)
	bot           telegram.Client  // API Telegram РґР»СЏ РѕС‚РїСЂР°РІРєРё РѕС‚РІРµС‚РѕРІ
}

// NewHandler СЃРѕР·РґР°С‘С‚ РЅРѕРІС‹Р№ РѕР±СЂР°Р±РѕС‚С‡РёРє СЌРєРѕРЅРѕРјРёС‡РµСЃРєРёС… РєРѕРјР°РЅРґ.
func NewHandler(service *Service, memberService *members.Service, bot telegram.Client) *Handler {
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
		h.sendMessage(chatID, "❌ Ошибка получения баланса")
		return
	}

	text := fmt.Sprintf("💰 Баланс: %s", common.FormatBalance(balance))
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
		h.sendMessage(chatID, "❌ Формат: !отсыпать @username сумма")
		return
	}

	// РџР°СЂСЃРёРј username (СѓР±РёСЂР°РµРј @ РµСЃР»Рё РµСЃС‚СЊ)
	username := strings.TrimPrefix(args[0], "@")
	if username == "" {
		h.sendMessage(chatID, "❌ Укажите @username получателя")
		return
	}

	// РџР°СЂСЃРёРј СЃСѓРјРјСѓ
	amount, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil || amount <= 0 {
		h.sendMessage(chatID, "❌ Сумма должна быть положительным числом")
		return
	}

	// РќР°С…РѕРґРёРј РїРѕР»СѓС‡Р°С‚РµР»СЏ РїРѕ username
	recipient, err := h.memberService.GetByUsername(ctx, username)
	if err != nil {
		h.sendMessage(chatID, "❌ Пользователь не найден")
		return
	}

	// Р’С‹РїРѕР»РЅСЏРµРј РїРµСЂРµРІРѕРґ
	err = h.service.Transfer(ctx, fromUserID, recipient.UserID, amount)
	if err != nil {
		switch err {
		case common.ErrSelfTransfer:
			h.sendMessage(chatID, "❌ Нельзя переводить плёнки самому себе")
		case common.ErrInsufficientBalance:
			h.sendMessage(chatID, "❌ Недостаточно плёнок на счёте")
		case common.ErrInvalidAmount:
			h.sendMessage(chatID, "❌ Сумма должна быть положительной")
		default:
			log.WithError(err).Error("РћС€РёР±РєР° РїРµСЂРµРІРѕРґР°")
			h.sendMessage(chatID, "❌ Ошибка выполнения перевода")
		}
		return
	}

	// РџРѕР»СѓС‡Р°РµРј РЅРѕРІС‹Р№ Р±Р°Р»Р°РЅСЃ РѕС‚РїСЂР°РІРёС‚РµР»СЏ
	newBalance, _ := h.service.GetBalance(ctx, fromUserID)

	text := fmt.Sprintf("✅ Переведено %s @%s\nТвой баланс: %s",
		common.FormatBalance(amount), username, common.FormatBalance(newBalance))
	h.sendMessage(chatID, text)
}

// HandleTransactions РѕР±СЂР°Р±Р°С‚С‹РІР°РµС‚ РєРѕРјР°РЅРґСѓ !С‚СЂР°РЅР·Р°РєС†РёРё вЂ” РїРѕРєР°Р·С‹РІР°РµС‚ РёСЃС‚РѕСЂРёСЋ.
func (h *Handler) HandleTransactions(ctx context.Context, chatID int64, userID int64) {
	history, err := h.service.GetTransactionHistory(ctx, userID)
	if err != nil {
		log.WithError(err).Error("РћС€РёР±РєР° РїРѕР»СѓС‡РµРЅРёСЏ С‚СЂР°РЅР·Р°РєС†РёР№")
		h.sendMessage(chatID, "❌ Ошибка получения истории транзакций")
		return
	}

	// РћС‚РїСЂР°РІР»СЏРµРј СЃ MarkdownV2 РґР»СЏ РїРѕРґРґРµСЂР¶РєРё СЃРїРѕР№Р»РµСЂРѕРІ
	h.sendMessage(chatID, history)
}

// sendMessage вЂ” РІСЃРїРѕРјРѕРіР°С‚РµР»СЊРЅС‹Р№ РјРµС‚РѕРґ РґР»СЏ РѕС‚РїСЂР°РІРєРё С‚РµРєСЃС‚РѕРІС‹С… СЃРѕРѕР±С‰РµРЅРёР№.
func (h *Handler) sendMessage(chatID int64, text string) {
	if _, err := h.bot.SendMessage(chatID, text, nil); err != nil {
		log.WithError(err).Error("РћС€РёР±РєР° РѕС‚РїСЂР°РІРєРё СЃРѕРѕР±С‰РµРЅРёСЏ")
	}
}
