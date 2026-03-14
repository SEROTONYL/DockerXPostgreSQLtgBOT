package admin

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	models "github.com/mymmrac/telego"
	log "github.com/sirupsen/logrus"
)

const (
	cbAdminRiddlesMenu  = "admin:riddles"
	cbRiddleCreate      = "admin:riddle:create"
	cbRiddleStop        = "admin:riddle:stop"
	cbRiddlePublish     = "admin:riddle:publish"
	cbRiddleCancelDraft = "admin:riddle:cancel"
)

func (h *Handler) handleRiddleMessageInput(ctx context.Context, chatID, userID int64, messageID int, text string) bool {
	state := h.service.GetState(userID)
	if state == nil {
		return false
	}
	switch state.State {
	case StateRiddleText:
		h.handleRiddleTextStep(ctx, chatID, userID, strings.TrimRight(text, "\r\n"))
		h.deleteAdminInputMessage(ctx, chatID, messageID)
		return true
	case StateRiddleAnswers:
		h.handleRiddleAnswersStep(ctx, chatID, userID, text)
		h.deleteAdminInputMessage(ctx, chatID, messageID)
		return true
	case StateRiddleReward:
		h.handleRiddleRewardStep(ctx, chatID, userID, text)
		h.deleteAdminInputMessage(ctx, chatID, messageID)
		return true
	}
	return false
}

func (h *Handler) showRiddlesMenu(ctx context.Context, chatID, userID int64, panelMsgID int) {
	if !h.service.CanManageRiddles(ctx, userID) {
		h.denyInsufficientPermissions(ctx, chatID)
		return
	}
	if err := h.renderAdminScreen(ctx, chatID, userID, panelMsgID, "riddles_menu", "Загадки", newInlineKeyboardMarkup(
		newInlineKeyboardRow(newInlineKeyboardButtonData("Создать загадку", cbRiddleCreate)),
		newInlineKeyboardRow(newInlineKeyboardButtonData("Остановить загадку", cbRiddleStop)),
		newInlineKeyboardRow(newInlineKeyboardButtonDataStyled("Назад", cbAdminReturnPanel, "danger")),
	)); err != nil {
		h.sendUIErrorHint(ctx, chatID, err)
	}
}

func (h *Handler) startRiddleCreate(ctx context.Context, chatID, userID int64, panelMsgID int) {
	if !h.service.CanManageRiddles(ctx, userID) {
		h.denyInsufficientPermissions(ctx, chatID)
		return
	}
	h.service.SetState(userID, StateRiddleText, &RiddleDraftData{})
	if err := h.renderAdminScreen(ctx, chatID, userID, panelMsgID, "riddle_text", "Отправьте текст поста для загадки.", newInlineKeyboardMarkup(
		newInlineKeyboardRow(newInlineKeyboardButtonDataStyled("Отмена", cbRiddleCancelDraft, "danger")),
	)); err != nil {
		h.sendUIErrorHint(ctx, chatID, err)
	}
}

func (h *Handler) handleRiddleTextStep(ctx context.Context, chatID, userID int64, text string) {
	if strings.TrimSpace(text) == "" {
		h.sendMessage(ctx, chatID, "Текст загадки не должен быть пустым.")
		return
	}
	draft := h.riddleDraftFromState(userID)
	if draft == nil {
		draft = &RiddleDraftData{}
	}
	draft.PostText = text
	h.service.SetState(userID, StateRiddleAnswers, draft)
	h.renderRiddlePrompt(ctx, chatID, userID, "Отправьте правильные ответы, по одному на строке.")
}

func (h *Handler) handleRiddleAnswersStep(ctx context.Context, chatID, userID int64, text string) {
	rawLines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	answers := make([]RiddleDraftAnswer, 0, len(rawLines))
	seen := map[string]bool{}
	for _, line := range rawLines {
		raw := strings.TrimSpace(line)
		if raw == "" {
			continue
		}
		normalized := normalizeRiddleText(raw)
		if normalized == "" || seen[normalized] {
			continue
		}
		seen[normalized] = true
		answers = append(answers, RiddleDraftAnswer{Raw: raw, Normalized: normalized})
	}
	if len(answers) == 0 {
		h.sendMessage(ctx, chatID, "Нужен хотя бы один корректный ответ.")
		return
	}
	draft := h.riddleDraftFromState(userID)
	if draft == nil {
		draft = &RiddleDraftData{}
	}
	draft.Answers = answers
	h.service.SetState(userID, StateRiddleReward, draft)
	h.renderRiddlePrompt(ctx, chatID, userID, "Укажите награду в плёнках: положительное целое число.")
}

func (h *Handler) handleRiddleRewardStep(ctx context.Context, chatID, userID int64, text string) {
	value, err := strconv.ParseInt(strings.TrimSpace(text), 10, 64)
	if err != nil || value <= 0 {
		h.sendMessage(ctx, chatID, "Награда должна быть положительным целым числом.")
		return
	}
	draft := h.riddleDraftFromState(userID)
	if draft == nil {
		draft = &RiddleDraftData{}
	}
	draft.RewardAmount = value
	h.service.SetState(userID, StateRiddleConfirm, draft)
	h.renderRiddleConfirm(ctx, chatID, userID)
}

func (h *Handler) renderRiddlePrompt(ctx context.Context, chatID, userID int64, text string) {
	panelMsgID := h.panelMessageIDFromState(userID)
	if err := h.renderAdminScreen(ctx, chatID, userID, panelMsgID, "riddle_prompt", text, newInlineKeyboardMarkup(
		newInlineKeyboardRow(newInlineKeyboardButtonDataStyled("Отмена", cbRiddleCancelDraft, "danger")),
	)); err != nil {
		h.sendUIErrorHint(ctx, chatID, err)
	}
}

func (h *Handler) renderRiddleConfirm(ctx context.Context, chatID, userID int64) {
	draft := h.riddleDraftFromState(userID)
	if draft == nil {
		h.service.ClearState(userID)
		h.showRiddlesMenu(ctx, chatID, userID, h.panelMessageIDFromState(userID))
		return
	}
	text := fmt.Sprintf("Подтверждение загадки\n\nТекст:\n%s\n\nОтветов: %d\nНаграда: %d", draft.PostText, len(draft.Answers), draft.RewardAmount)
	if err := h.renderAdminScreen(ctx, chatID, userID, h.panelMessageIDFromState(userID), "riddle_confirm", text, newInlineKeyboardMarkup(
		newInlineKeyboardRow(newInlineKeyboardButtonDataStyled("Опубликовать", cbRiddlePublish, "success")),
		newInlineKeyboardRow(newInlineKeyboardButtonDataStyled("Отмена", cbRiddleCancelDraft, "danger")),
	)); err != nil {
		h.sendUIErrorHint(ctx, chatID, err)
	}
}

func (h *Handler) handleRiddlePublish(ctx context.Context, chatID, userID int64) {
	if !h.service.CanManageRiddles(ctx, userID) {
		h.denyInsufficientPermissions(ctx, chatID)
		return
	}
	if h.riddleService == nil {
		h.sendMessage(ctx, chatID, "Функция загадок сейчас недоступна.")
		return
	}
	draft := h.riddleDraftFromState(userID)
	if draft == nil || strings.TrimSpace(draft.PostText) == "" || len(draft.Answers) == 0 || draft.RewardAmount <= 0 {
		h.sendMessage(ctx, chatID, "Черновик загадки поврежден. Начните заново.")
		h.service.ClearState(userID)
		h.showRiddlesMenu(ctx, chatID, userID, h.panelMessageIDFromState(userID))
		return
	}

	pub, err := h.riddleService.CreatePublishing(ctx, userID, draft)
	if err != nil {
		if err == ErrRiddleAlreadyActive {
			h.sendMessage(ctx, chatID, "Сейчас уже есть активная загадка.")
			return
		}
		log.WithError(err).Warn("riddle publish create failed")
		h.sendMessage(ctx, chatID, "Не удалось подготовить публикацию загадки.")
		return
	}

	msgID, sendErr := h.ops.Send(ctx, h.memberSourceChatID, pub.Riddle.PostText, nil)
	if sendErr != nil {
		h.abortRiddlePublication(ctx, pub.Riddle.ID)
		h.sendMessage(ctx, chatID, "Не удалось опубликовать загадку в основном чате.")
		return
	}
	if err := h.ops.PinChatMessage(ctx, h.memberSourceChatID, msgID, true); err != nil {
		_ = h.ops.DeleteMessage(ctx, h.memberSourceChatID, msgID)
		h.abortRiddlePublication(ctx, pub.Riddle.ID)
		h.sendMessage(ctx, chatID, "Не удалось закрепить загадку. Публикация отменена.")
		return
	}
	if err := h.riddleService.ActivatePublished(ctx, pub.Riddle.ID, h.memberSourceChatID, int64(msgID)); err != nil {
		_ = h.ops.UnpinChatMessage(ctx, h.memberSourceChatID, msgID)
		_ = h.ops.DeleteMessage(ctx, h.memberSourceChatID, msgID)
		h.abortRiddlePublication(ctx, pub.Riddle.ID)
		h.sendMessage(ctx, chatID, "Не удалось завершить публикацию загадки.")
		return
	}
	h.service.ClearState(userID)
	if err := h.renderAdminScreen(ctx, chatID, userID, h.panelMessageIDFromState(userID), "riddle_published", "Загадка опубликована.", newInlineKeyboardMarkup(
		newInlineKeyboardRow(newInlineKeyboardButtonDataStyled("Назад", cbAdminRiddlesMenu, "success")),
	)); err != nil {
		h.sendUIErrorHint(ctx, chatID, err)
	}
}

func (h *Handler) handleRiddleCancel(ctx context.Context, chatID, userID int64) {
	if !h.service.CanManageRiddles(ctx, userID) {
		h.denyInsufficientPermissions(ctx, chatID)
		return
	}
	h.service.ClearState(userID)
	h.showRiddlesMenu(ctx, chatID, userID, h.panelMessageIDFromState(userID))
}

func (h *Handler) handleRiddleStop(ctx context.Context, chatID, userID int64) {
	if !h.service.CanManageRiddles(ctx, userID) {
		h.denyInsufficientPermissions(ctx, chatID)
		return
	}
	if h.riddleService == nil {
		h.sendMessage(ctx, chatID, "Функция загадок сейчас недоступна.")
		return
	}
	result, err := h.riddleService.StopActive(ctx)
	if err != nil {
		log.WithError(err).Warn("riddle stop failed")
		h.sendMessage(ctx, chatID, "Не удалось остановить загадку.")
		return
	}
	if result == nil || result.Riddle == nil {
		h.sendMessage(ctx, chatID, "Сейчас нет активной загадки.")
		return
	}
	if result.Riddle.GroupChatID != nil && result.Riddle.MessageID != nil {
		h.cleanupPublishedRiddleMessage(ctx, *result.Riddle.GroupChatID, int(*result.Riddle.MessageID))
		summary := "Загадка остановлена."
		winners := summarizeRiddleWinners(result.Answers)
		if len(winners) > 0 {
			summary += "\nПобедители: " + strings.Join(winners, ", ")
		}
		_, _ = h.ops.Send(ctx, *result.Riddle.GroupChatID, summary, nil)
	}
	h.sendMessage(ctx, chatID, "Активная загадка остановлена.")
}

func (h *Handler) HandleRiddleMessage(ctx context.Context, message *models.Message) bool {
	if h.riddleService == nil || message == nil || message.Chat.ID != h.memberSourceChatID || strings.TrimSpace(message.Text) == "" {
		return false
	}
	result, matched, err := h.riddleService.ProcessGuess(ctx, message)
	if err != nil {
		log.WithError(err).WithFields(log.Fields{"chat_id": message.Chat.ID, "message_id": message.MessageID}).Error("riddle guess processing failed")
		return false
	}
	if !matched || result == nil || result.Riddle == nil {
		return false
	}
	if result.Riddle.GroupChatID != nil && result.Riddle.MessageID != nil {
		_ = h.ops.UnpinChatMessage(ctx, *result.Riddle.GroupChatID, int(*result.Riddle.MessageID))
	}
	winners := summarizeRiddleWinners(result.Answers)
	text := fmt.Sprintf("Загадка завершена.\nПобедители: %s\nНаграда: %d \U0001F39E\uFE0F", strings.Join(winners, ", "), result.Riddle.RewardAmount)
	_, _ = h.ops.Send(ctx, message.Chat.ID, text, nil)
	return false
}

func (h *Handler) riddleDraftFromState(userID int64) *RiddleDraftData {
	state := h.service.GetState(userID)
	if state == nil {
		return nil
	}
	data, _ := state.Data.(*RiddleDraftData)
	return data
}

func pluralizeRiddleReward(amount int64) string {
	switch {
	case amount%10 == 1 && amount%100 != 11:
		return "плёнка"
	case amount%10 >= 2 && amount%10 <= 4 && (amount%100 < 12 || amount%100 > 14):
		return "плёнки"
	default:
		return "плёнок"
	}
}

func (h *Handler) abortRiddlePublication(ctx context.Context, riddleID int64) {
	if err := h.riddleService.AbortPublication(ctx, riddleID); err != nil {
		log.WithError(err).WithField("riddle_id", riddleID).Warn("riddle publication abort failed")
	}
}

func (h *Handler) cleanupPublishedRiddleMessage(ctx context.Context, chatID int64, messageID int) {
	_ = h.ops.UnpinChatMessage(ctx, chatID, messageID)
	if err := h.ops.DeleteMessage(ctx, chatID, messageID); err != nil {
		log.WithError(err).WithFields(log.Fields{"chat_id": chatID, "message_id": messageID}).Warn("published riddle delete failed")
	}
}
