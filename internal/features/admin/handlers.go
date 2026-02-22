// Package admin вЂ” handlers.go РѕР±СЂР°Р±Р°С‚С‹РІР°РµС‚ РІР·Р°РёРјРѕРґРµР№СЃС‚РІРёРµ СЃ Р°РґРјРёРЅ-РїР°РЅРµР»СЊСЋ.
// РџР°РЅРµР»СЊ СЂР°Р±РѕС‚Р°РµС‚ С‡РµСЂРµР· Reply Keyboard РІ Р»РёС‡РЅС‹С… СЃРѕРѕР±С‰РµРЅРёСЏС….
// РџРѕС‚РѕРє: Р°СѓС‚РµРЅС‚РёС„РёРєР°С†РёСЏ в†’ РєР»Р°РІРёР°С‚СѓСЂР° в†’ РІС‹Р±РѕСЂ РґРµР№СЃС‚РІРёСЏ в†’ РїРѕС€Р°РіРѕРІС‹Р№ РґРёР°Р»РѕРі.
package admin

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	log "github.com/sirupsen/logrus"

	"serotonyl.ru/telegram-bot/internal/features/members"
)

// Handler РѕР±СЂР°Р±Р°С‚С‹РІР°РµС‚ Р°РґРјРёРЅ-РєРѕРјР°РЅРґС‹.
type Handler struct {
	service       *Service
	memberService *members.Service
	bot           *tgbotapi.BotAPI
}

// NewHandler СЃРѕР·РґР°С‘С‚ РѕР±СЂР°Р±РѕС‚С‡РёРє Р°РґРјРёРЅ-РїР°РЅРµР»Рё.
func NewHandler(service *Service, memberService *members.Service, bot *tgbotapi.BotAPI) *Handler {
	return &Handler{
		service:       service,
		memberService: memberService,
		bot:           bot,
	}
}

// HandleAdminMessage РѕР±СЂР°Р±Р°С‚С‹РІР°РµС‚ Р»СЋР±РѕРµ СЃРѕРѕР±С‰РµРЅРёРµ РѕС‚ Р°РґРјРёРЅРёСЃС‚СЂР°С‚РѕСЂР° РІ DM.
// РћРїСЂРµРґРµР»СЏРµС‚ С‚РµРєСѓС‰РµРµ СЃРѕСЃС‚РѕСЏРЅРёРµ РґРёР°Р»РѕРіР° Рё РјР°СЂС€СЂСѓС‚РёР·РёСЂСѓРµС‚ СЃРѕРѕР±С‰РµРЅРёРµ.
func (h *Handler) HandleAdminMessage(ctx context.Context, chatID int64, userID int64, text string) bool {
	// РџСЂРѕРІРµСЂСЏРµРј, СЏРІР»СЏРµС‚СЃСЏ Р»Рё РїРѕР»СЊР·РѕРІР°С‚РµР»СЊ Р°РґРјРёРЅРѕРј
	member, err := h.memberService.GetByUserID(ctx, userID)
	if err != nil || !member.IsAdmin {
		return false // РќРµ Р°РґРјРёРЅ
	}

	// РџСЂРѕРІРµСЂСЏРµРј СЃРѕСЃС‚РѕСЏРЅРёРµ РґРёР°Р»РѕРіР°
	state := h.service.GetState(userID)

	// РћР±СЂР°Р±Р°С‚С‹РІР°РµРј СЃРѕСЃС‚РѕСЏРЅРёРµ РѕР¶РёРґР°РЅРёСЏ РїР°СЂРѕР»СЏ
	if state != nil && state.State == StateAwaitingPassword {
		h.handlePasswordInput(ctx, chatID, userID, text)
		return true
	}

	// РџСЂРѕРІРµСЂСЏРµРј Р°РєС‚РёРІРЅСѓСЋ СЃРµСЃСЃРёСЋ
	if !h.service.HasActiveSession(ctx, userID) {
		// РќРµС‚ СЃРµСЃСЃРёРё вЂ” Р·Р°РїСЂР°С€РёРІР°РµРј РїР°СЂРѕР»СЊ
		h.sendMessage(chatID, "рџ”ђ Р’РІРµРґРёС‚Рµ РїР°СЂРѕР»СЊ РґР»СЏ РґРѕСЃС‚СѓРїР° Рє Р°РґРјРёРЅ-РїР°РЅРµР»Рё:")
		h.service.SetState(userID, StateAwaitingPassword, nil)
		return true
	}

	// РћР±РЅРѕРІР»СЏРµРј Р°РєС‚РёРІРЅРѕСЃС‚СЊ СЃРµСЃСЃРёРё
	h.service.repo.UpdateActivity(ctx, userID)

	// РћР±СЂР°Р±Р°С‚С‹РІР°РµРј С‚РµРєСѓС‰РµРµ СЃРѕСЃС‚РѕСЏРЅРёРµ
	if state != nil {
		switch state.State {
		case StateAssignRoleSelect:
			h.handleAssignRoleSelect(ctx, chatID, userID, text)
			return true
		case StateAssignRoleText:
			h.handleAssignRoleText(ctx, chatID, userID, text)
			return true
		case StateChangeRoleSelect:
			h.handleChangeRoleSelect(ctx, chatID, userID, text)
			return true
		case StateChangeRoleText:
			h.handleChangeRoleText(ctx, chatID, userID, text)
			return true
		}
	}

	// РћР±СЂР°Р±Р°С‚С‹РІР°РµРј РєРЅРѕРїРєРё РєР»Р°РІРёР°С‚СѓСЂС‹
	switch text {
	case "РќР°Р·РЅР°С‡РёС‚СЊ СЂРѕР»СЊ":
		h.startAssignRole(ctx, chatID, userID)
		return true
	case "РЎРјРµРЅРёС‚СЊ СЂРѕР»СЊ":
		h.startChangeRole(ctx, chatID, userID)
		return true
	case "Р’С‹РґР°С‚СЊ РїР»С‘РЅРєРё", "РћС‚РЅСЏС‚СЊ РїР»С‘РЅРєРё", "Р’С‹РґР°С‚СЊ РєСЂРµРґРёС‚",
		"РђРЅРЅСѓР»РёСЂРѕРІР°С‚СЊ РєСЂРµРґРёС‚", "РЎРѕР·РґР°С‚СЊ СЃРѕРєСЂР°С‰РµРЅРёРµ", "РЈРґР°Р»РёС‚СЊ СЃРѕРєСЂР°С‰РµРЅРёРµ":
		h.sendMessage(chatID, "рџ”§ Р¤СѓРЅРєС†РёСЏ РІ СЂР°Р·СЂР°Р±РѕС‚РєРµ")
		return true
	case "РђРґРјРёРЅ", "РџР°РЅРµР»СЊ", "Р°РґРјРёРЅ", "РїР°РЅРµР»СЊ":
		h.showKeyboard(chatID)
		return true
	}

	return false
}

// handlePasswordInput РѕР±СЂР°Р±Р°С‚С‹РІР°РµС‚ РІРІРѕРґ РїР°СЂРѕР»СЏ.
func (h *Handler) handlePasswordInput(ctx context.Context, chatID int64, userID int64, password string) {
	err := h.service.VerifyPassword(ctx, userID, password)
	if err != nil {
		h.sendMessage(chatID, fmt.Sprintf("вќЊ %s", err.Error()))
		h.service.ClearState(userID)
		return
	}

	h.service.ClearState(userID)
	h.sendMessage(chatID, "вњ… РђСѓС‚РµРЅС‚РёС„РёРєР°С†РёСЏ СѓСЃРїРµС€РЅР°!")
	h.showKeyboard(chatID)
}

// showKeyboard РѕС‚РѕР±СЂР°Р¶Р°РµС‚ РєР»Р°РІРёР°С‚СѓСЂСѓ Р°РґРјРёРЅ-РїР°РЅРµР»Рё.
func (h *Handler) showKeyboard(chatID int64) {
	keyboard := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("РќР°Р·РЅР°С‡РёС‚СЊ СЂРѕР»СЊ"),
			tgbotapi.NewKeyboardButton("РЎРјРµРЅРёС‚СЊ СЂРѕР»СЊ"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("Р’С‹РґР°С‚СЊ РїР»С‘РЅРєРё"),
			tgbotapi.NewKeyboardButton("РћС‚РЅСЏС‚СЊ РїР»С‘РЅРєРё"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("Р’С‹РґР°С‚СЊ РєСЂРµРґРёС‚"),
			tgbotapi.NewKeyboardButton("РђРЅРЅСѓР»РёСЂРѕРІР°С‚СЊ РєСЂРµРґРёС‚"),
		),
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("РЎРѕР·РґР°С‚СЊ СЃРѕРєСЂР°С‰РµРЅРёРµ"),
			tgbotapi.NewKeyboardButton("РЈРґР°Р»РёС‚СЊ СЃРѕРєСЂР°С‰РµРЅРёРµ"),
		),
	)

	msg := tgbotapi.NewMessage(chatID, "вњ… РђРґРјРёРЅ-РїР°РЅРµР»СЊ РѕС‚РєСЂС‹С‚Р°")
	msg.ReplyMarkup = keyboard
	if _, err := h.bot.Send(msg); err != nil {
		log.WithError(err).Error("РћС€РёР±РєР° РѕС‚РїСЂР°РІРєРё РєР»Р°РІРёР°С‚СѓСЂС‹")
	}
}

// --- РќР°Р·РЅР°С‡РёС‚СЊ СЂРѕР»СЊ (3 С€Р°РіР°) ---

// startAssignRole вЂ” РЁР°Рі 1: РїРѕРєР°Р·Р°С‚СЊ РїРѕР»СЊР·РѕРІР°С‚РµР»РµР№ Р‘Р•Р— СЂРѕР»Рё.
func (h *Handler) startAssignRole(ctx context.Context, chatID int64, userID int64) {
	users, err := h.service.GetUsersWithoutRole(ctx)
	if err != nil || len(users) == 0 {
		h.sendMessage(chatID, "Р’СЃРµ РїРѕР»СЊР·РѕРІР°С‚РµР»Рё СѓР¶Рµ РёРјРµСЋС‚ СЂРѕР»Рё")
		return
	}

	var sb strings.Builder
	sb.WriteString("Р’С‹Р±РµСЂРёС‚Рµ РїРѕР»СЊР·РѕРІР°С‚РµР»СЏ (РѕС‚РїСЂР°РІСЊС‚Рµ РЅРѕРјРµСЂ):\n\n")
	for i, user := range users {
		name := user.DisplayName()
		sb.WriteString(fmt.Sprintf("%d. %s (%s)\n", i+1, name, user.FirstName))
	}

	h.sendMessage(chatID, sb.String())
	h.service.SetState(userID, StateAssignRoleSelect, users)
}

// handleAssignRoleSelect вЂ” РЁР°Рі 2: РїРѕР»СЊР·РѕРІР°С‚РµР»СЊ РІС‹Р±СЂР°Р» РЅРѕРјРµСЂ.
func (h *Handler) handleAssignRoleSelect(ctx context.Context, chatID int64, userID int64, text string) {
	state := h.service.GetState(userID)
	users := state.Data.([]*members.Member)

	num, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || num < 1 || num > len(users) {
		h.sendMessage(chatID, "вќЊ РќРµРІРµСЂРЅС‹Р№ РЅРѕРјРµСЂ. РџРѕРїСЂРѕР±СѓР№С‚Рµ РµС‰С‘ СЂР°Р·.")
		return
	}

	selected := users[num-1]
	h.sendMessage(chatID, fmt.Sprintf("Р’РІРµРґРёС‚Рµ СЂРѕР»СЊ РґР»СЏ %s (РјР°РєСЃРёРјСѓРј 64 СЃРёРјРІРѕР»Р°):", selected.DisplayName()))
	h.service.SetState(userID, StateAssignRoleText, selected)
}

// handleAssignRoleText вЂ” РЁР°Рі 3: РІРІРѕРґ С‚РµРєСЃС‚Р° СЂРѕР»Рё.
func (h *Handler) handleAssignRoleText(ctx context.Context, chatID int64, userID int64, text string) {
	state := h.service.GetState(userID)
	selected := state.Data.(*members.Member)

	role := strings.TrimSpace(text)
	if len([]rune(role)) > 64 {
		h.sendMessage(chatID, "вќЊ Р РѕР»СЊ СЃР»РёС€РєРѕРј РґР»РёРЅРЅР°СЏ (РјР°РєСЃРёРјСѓРј 64 СЃРёРјРІРѕР»Р°)")
		return
	}

	if err := h.service.AssignRole(ctx, selected.UserID, role); err != nil {
		h.sendMessage(chatID, fmt.Sprintf("вќЊ РћС€РёР±РєР°: %s", err.Error()))
		h.service.ClearState(userID)
		return
	}

	h.sendMessage(chatID, fmt.Sprintf("вњ… Р РѕР»СЊ РЅР°Р·РЅР°С‡РµРЅР°: %s в†’ %s", selected.DisplayName(), role))
	h.service.ClearState(userID)
}

// --- РЎРјРµРЅРёС‚СЊ СЂРѕР»СЊ (3 С€Р°РіР°) ---

func (h *Handler) startChangeRole(ctx context.Context, chatID int64, userID int64) {
	users, err := h.service.GetUsersWithRole(ctx)
	if err != nil || len(users) == 0 {
		h.sendMessage(chatID, "РќРµС‚ РїРѕР»СЊР·РѕРІР°С‚РµР»РµР№ СЃ РЅР°Р·РЅР°С‡РµРЅРЅС‹РјРё СЂРѕР»СЏРјРё")
		return
	}

	var sb strings.Builder
	sb.WriteString("Р’С‹Р±РµСЂРёС‚Рµ РїРѕР»СЊР·РѕРІР°С‚РµР»СЏ (РѕС‚РїСЂР°РІСЊС‚Рµ РЅРѕРјРµСЂ):\n\n")
	for i, user := range users {
		role := ""
		if user.Role != nil {
			role = *user.Role
		}
		sb.WriteString(fmt.Sprintf("%d. %s - %s\n", i+1, user.DisplayName(), role))
	}

	h.sendMessage(chatID, sb.String())
	h.service.SetState(userID, StateChangeRoleSelect, users)
}

func (h *Handler) handleChangeRoleSelect(ctx context.Context, chatID int64, userID int64, text string) {
	state := h.service.GetState(userID)
	users := state.Data.([]*members.Member)

	num, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || num < 1 || num > len(users) {
		h.sendMessage(chatID, "вќЊ РќРµРІРµСЂРЅС‹Р№ РЅРѕРјРµСЂ")
		return
	}

	selected := users[num-1]
	currentRole := ""
	if selected.Role != nil {
		currentRole = *selected.Role
	}
	h.sendMessage(chatID, fmt.Sprintf("РўРµРєСѓС‰Р°СЏ СЂРѕР»СЊ: %s\nР’РІРµРґРёС‚Рµ РЅРѕРІСѓСЋ СЂРѕР»СЊ:", currentRole))
	h.service.SetState(userID, StateChangeRoleText, selected)
}

func (h *Handler) handleChangeRoleText(ctx context.Context, chatID int64, userID int64, text string) {
	state := h.service.GetState(userID)
	selected := state.Data.(*members.Member)

	role := strings.TrimSpace(text)
	if len([]rune(role)) > 64 {
		h.sendMessage(chatID, "вќЊ Р РѕР»СЊ СЃР»РёС€РєРѕРј РґР»РёРЅРЅР°СЏ (РјР°РєСЃРёРјСѓРј 64 СЃРёРјРІРѕР»Р°)")
		return
	}

	if err := h.service.AssignRole(ctx, selected.UserID, role); err != nil {
		h.sendMessage(chatID, fmt.Sprintf("вќЊ РћС€РёР±РєР°: %s", err.Error()))
		h.service.ClearState(userID)
		return
	}

	h.sendMessage(chatID, fmt.Sprintf("вњ… Р РѕР»СЊ РёР·РјРµРЅРµРЅР°: %s в†’ %s", selected.DisplayName(), role))
	h.service.ClearState(userID)
}

func (h *Handler) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := h.bot.Send(msg); err != nil {
		log.WithError(err).Error("РћС€РёР±РєР° РѕС‚РїСЂР°РІРєРё СЃРѕРѕР±С‰РµРЅРёСЏ")
	}
}
