package admin

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"

	"serotonyl.ru/telegram-bot/internal/features/members"
)

const (
	cbBalPickToggle = "admin:balpick:toggle"
	cbBalPickPrev   = "admin:balpick:prev"
	cbBalPickNext   = "admin:balpick:next"
	cbBalPickClear  = "admin:balpick:clear"
	cbBalPickDone   = "admin:balpick:done"
	cbBalPickBack   = "admin:balpick:back"

	cbBalAmtDeltaPrefix = "admin:balamt:delta:"
	cbBalAmtManual      = "admin:balamt:manual"
	cbBalAmtBack        = "admin:balamt:back"

	cbBalConfirmApply  = "admin:balconfirm:apply"
	cbBalConfirmBack   = "admin:balconfirm:back"
	cbBalConfirmCancel = "admin:balconfirm:cancel"

	cbBalUndo = "admin:balundo"

	maxBalanceAdjustAmount = int64(1_000_000)
	undoTTL                = 30 * time.Minute
)

func (h *Handler) startBalanceAdjustMode(chatID, userID int64, panelMsgID int) {
	data := &BalanceAdjustData{PageSize: userPickerPageSize, SelectedUserIDs: map[int64]bool{}}
	h.service.SetState(userID, StateBalanceAdjustMode, data)
	if err := h.renderAdminScreen(chatID, userID, panelMsgID, "balance_adjust_mode", "Выберите режим изменения баланса:", newInlineKeyboardMarkup(
		newInlineKeyboardRow(
			newInlineKeyboardButtonData("➕ Прибавить", "admin:balmode:add"),
			newInlineKeyboardButtonData("➖ Отнять", "admin:balmode:deduct"),
		),
		newInlineKeyboardRow(newInlineKeyboardButtonDataStyled(userPickerBackButton, cbAdminReturnPanel, "danger")),
	)); err != nil {
		h.sendUIErrorHint(chatID, err)
	}
}

func (h *Handler) handleBalanceAdjustCallback(ctx context.Context, chatID, userID int64, panelMsgID int, data string) {
	switch {
	case strings.HasPrefix(data, "admin:balmode:"):
		h.handleBalanceAdjustMode(ctx, chatID, userID, panelMsgID, strings.TrimPrefix(data, "admin:balmode:"))
	case strings.HasPrefix(data, "admin:balpick:"):
		h.handleBalancePicker(chatID, userID, panelMsgID, data)
	case strings.HasPrefix(data, "admin:balamt:"):
		h.handleBalanceAmount(chatID, userID, panelMsgID, data)
	case strings.HasPrefix(data, "admin:balconfirm:"):
		h.handleBalanceConfirm(ctx, chatID, userID, panelMsgID, data)
	case data == cbBalUndo:
		h.handleBalanceUndo(ctx, chatID, userID, panelMsgID)
	}
}

func (h *Handler) handleBalanceAdjustMode(ctx context.Context, chatID, userID int64, panelMsgID int, modeRaw string) {
	if !h.ensureBalanceState(chatID, userID, panelMsgID, StateBalanceAdjustMode) {
		return
	}
	state := h.service.GetState(userID)
	data, _ := state.Data.(*BalanceAdjustData)
	if data == nil {
		data = &BalanceAdjustData{PageSize: userPickerPageSize, SelectedUserIDs: map[int64]bool{}}
	}
	if modeRaw == "add" {
		data.Mode = BalanceAdjustModeAdd
	} else if modeRaw == "deduct" {
		data.Mode = BalanceAdjustModeDeduct
	} else {
		h.resetBalanceFlow(chatID, userID, panelMsgID)
		return
	}
	users, err := h.service.GetUsersWithRole(ctx)
	if err != nil || len(users) == 0 {
		h.sendMessage(chatID, "❌ Нет пользователей с назначенными ролями")
		return
	}
	data.UsersSnapshot = users
	if data.SelectedUserIDs == nil {
		data.SelectedUserIDs = map[int64]bool{}
	}
	h.service.SetState(userID, StateBalanceAdjustPicker, data)
	h.renderBalancePicker(chatID, userID, panelMsgID)
}

func (h *Handler) renderBalancePicker(chatID, userID int64, panelMsgID int) {
	if !h.ensureBalanceState(chatID, userID, panelMsgID, StateBalanceAdjustPicker) {
		return
	}
	state := h.service.GetState(userID)
	data, _ := state.Data.(*BalanceAdjustData)
	if data == nil || len(data.UsersSnapshot) == 0 {
		h.resetBalanceFlow(chatID, userID, panelMsgID)
		return
	}
	if data.PageSize <= 0 {
		data.PageSize = userPickerPageSize
	}
	totalPages := (len(data.UsersSnapshot) + data.PageSize - 1) / data.PageSize
	if totalPages == 0 {
		totalPages = 1
	}
	if data.PageIndex < 0 {
		data.PageIndex = 0
	}
	if data.PageIndex >= totalPages {
		data.PageIndex = totalPages - 1
	}
	start := data.PageIndex * data.PageSize
	end := start + data.PageSize
	if end > len(data.UsersSnapshot) {
		end = len(data.UsersSnapshot)
	}
	rows := make([][]models.InlineKeyboardButton, 0)
	for _, u := range data.UsersSnapshot[start:end] {
		mark := "☐"
		if data.SelectedUserIDs[u.UserID] {
			mark = "☑"
		}
		rows = append(rows, newInlineKeyboardRow(newInlineKeyboardButtonData(fmt.Sprintf("%s %s", mark, formatMemberForAssignPicker(u)), fmt.Sprintf("%s:%d", cbBalPickToggle, u.UserID))))
	}
	rows = append(rows, newInlineKeyboardRow(
		newInlineKeyboardButtonData(userPickerPrevButton, cbBalPickPrev),
		newInlineKeyboardButtonData(fmt.Sprintf("Стр %d/%d", data.PageIndex+1, totalPages), "admin:balpick:page"),
		newInlineKeyboardButtonData(userPickerNextButton, cbBalPickNext),
	))
	rows = append(rows, newInlineKeyboardRow(newInlineKeyboardButtonData("Сбросить выбор", cbBalPickClear)))
	if len(data.SelectedUserIDs) > 0 {
		rows = append(rows, newInlineKeyboardRow(newInlineKeyboardButtonDataStyled("Далее", cbBalPickDone, "success")))
	} else {
		rows = append(rows, newInlineKeyboardRow(newInlineKeyboardButtonData("Выберите хотя бы одного", "admin:balpick:noop")))
	}
	rows = append(rows, newInlineKeyboardRow(newInlineKeyboardButtonDataStyled(userPickerBackButton, cbBalPickBack, "danger")))
	text := fmt.Sprintf("Выберите пользователей (только с ролью).\nВыбрано: %d", len(data.SelectedUserIDs))
	if err := h.renderAdminScreen(chatID, userID, panelMsgID, "balance_adjust_picker", text, newInlineKeyboardMarkup(rows...)); err != nil {
		h.sendUIErrorHint(chatID, err)
	}
}

func (h *Handler) handleBalancePicker(chatID, userID int64, panelMsgID int, cb string) {
	if !h.ensureBalanceState(chatID, userID, panelMsgID, StateBalanceAdjustPicker) {
		return
	}
	state := h.service.GetState(userID)
	data, _ := state.Data.(*BalanceAdjustData)
	if data == nil {
		h.resetBalanceFlow(chatID, userID, panelMsgID)
		return
	}
	switch {
	case cb == cbBalPickPrev:
		if data.PageIndex > 0 {
			data.PageIndex--
		}
		h.renderBalancePicker(chatID, userID, panelMsgID)
	case cb == cbBalPickNext:
		totalPages := (len(data.UsersSnapshot) + data.PageSize - 1) / data.PageSize
		if data.PageIndex < totalPages-1 {
			data.PageIndex++
		}
		h.renderBalancePicker(chatID, userID, panelMsgID)
	case cb == cbBalPickClear:
		data.SelectedUserIDs = map[int64]bool{}
		h.renderBalancePicker(chatID, userID, panelMsgID)
	case cb == cbBalPickDone:
		if !h.isBalanceSelectionReady(data) {
			h.renderBalancePicker(chatID, userID, panelMsgID)
			return
		}
		h.service.SetState(userID, StateBalanceAdjustAmount, data)
		h.renderBalanceAmount(chatID, userID, panelMsgID)
	case cb == cbBalPickBack:
		h.startBalanceAdjustMode(chatID, userID, panelMsgID)
	case strings.HasPrefix(cb, cbBalPickToggle+":"):
		id, err := strconv.ParseInt(strings.TrimPrefix(cb, cbBalPickToggle+":"), 10, 64)
		if err != nil || id <= 0 {
			h.resetBalanceFlow(chatID, userID, panelMsgID)
			return
		}
		if data.SelectedUserIDs[id] {
			delete(data.SelectedUserIDs, id)
		} else {
			data.SelectedUserIDs[id] = true
		}
		h.renderBalancePicker(chatID, userID, panelMsgID)
	}
}

func (h *Handler) renderBalanceAmount(chatID, userID int64, panelMsgID int) {
	if !h.ensureBalanceState(chatID, userID, panelMsgID, StateBalanceAdjustAmount) {
		return
	}
	state := h.service.GetState(userID)
	data, _ := state.Data.(*BalanceAdjustData)
	if data == nil || !h.isBalanceSelectionReady(data) {
		h.resetBalanceFlow(chatID, userID, panelMsgID)
		return
	}
	sign := "+"
	if data.Mode == BalanceAdjustModeDeduct {
		sign = "-"
	}
	if err := h.renderAdminScreen(chatID, userID, panelMsgID, "balance_adjust_amount", "Выберите сумму изменения:", newInlineKeyboardMarkup(
		newInlineKeyboardRow(
			newInlineKeyboardButtonData(fmt.Sprintf("Δ %s10", sign), cbBalAmtDeltaPrefix+"10"),
			newInlineKeyboardButtonData(fmt.Sprintf("Δ %s50", sign), cbBalAmtDeltaPrefix+"50"),
			newInlineKeyboardButtonData(fmt.Sprintf("Δ %s100", sign), cbBalAmtDeltaPrefix+"100"),
		),
		newInlineKeyboardRow(newInlineKeyboardButtonData("Ввести сумму вручную", cbBalAmtManual)),
		newInlineKeyboardRow(newInlineKeyboardButtonDataStyled(userPickerBackButton, cbBalAmtBack, "danger")),
	)); err != nil {
		h.sendUIErrorHint(chatID, err)
	}
}

func (h *Handler) handleBalanceAmount(chatID, userID int64, panelMsgID int, cb string) {
	if !h.ensureBalanceState(chatID, userID, panelMsgID, StateBalanceAdjustAmount) {
		return
	}
	state := h.service.GetState(userID)
	data, _ := state.Data.(*BalanceAdjustData)
	if data == nil || !h.isBalanceSelectionReady(data) {
		h.resetBalanceFlow(chatID, userID, panelMsgID)
		return
	}
	switch {
	case strings.HasPrefix(cb, cbBalAmtDeltaPrefix):
		amount, err := strconv.ParseInt(strings.TrimPrefix(cb, cbBalAmtDeltaPrefix), 10, 64)
		if err != nil || validateBalanceAmount(amount) != nil {
			h.sendMessage(chatID, "❌ Некорректная сумма")
			return
		}
		data.Amount = amount
		data.AmountSource = "delta"
		data.AwaitingManual = false
		h.service.SetState(userID, StateBalanceAdjustConfirm, data)
		h.renderBalanceConfirm(chatID, userID, panelMsgID)
	case cb == cbBalAmtManual:
		data.AwaitingManual = true
		h.sendMessage(chatID, fmt.Sprintf("Отправьте сумму (целое число > 0 и <= %d)", maxBalanceAdjustAmount))
	case cb == cbBalAmtBack:
		h.service.SetState(userID, StateBalanceAdjustPicker, data)
		h.renderBalancePicker(chatID, userID, panelMsgID)
	}
}

func (h *Handler) handleBalanceAdjustManualAmount(ctx context.Context, chatID, userID int64, text string) bool {
	_ = ctx
	state := h.service.GetState(userID)
	if state == nil || state.State != StateBalanceAdjustAmount {
		return false
	}
	data, _ := state.Data.(*BalanceAdjustData)
	if data == nil || !data.AwaitingManual || !h.isBalanceSelectionReady(data) {
		h.resetBalanceFlow(chatID, userID, h.panelMessageIDFromState(userID))
		return true
	}
	normalized := strings.ReplaceAll(strings.TrimSpace(text), " ", "")
	amount, err := strconv.ParseInt(normalized, 10, 64)
	if err != nil || validateBalanceAmount(amount) != nil {
		h.sendMessage(chatID, fmt.Sprintf("❌ Некорректная сумма. Введите целое число от 1 до %d", maxBalanceAdjustAmount))
		return true
	}
	data.Amount = amount
	data.AmountSource = "manual"
	data.AwaitingManual = false
	h.service.SetState(userID, StateBalanceAdjustConfirm, data)
	h.renderBalanceConfirm(chatID, userID, h.panelMessageIDFromState(userID))
	return true
}

func (h *Handler) renderBalanceConfirm(chatID, userID int64, panelMsgID int) {
	if !h.ensureBalanceState(chatID, userID, panelMsgID, StateBalanceAdjustConfirm) {
		return
	}
	state := h.service.GetState(userID)
	data, _ := state.Data.(*BalanceAdjustData)
	if data == nil || !h.isBalanceReadyToConfirm(data) {
		h.resetBalanceFlow(chatID, userID, panelMsgID)
		return
	}
	ids := selectedIDs(data.SelectedUserIDs)
	summary := summarizeUsers(data.UsersSnapshot, data.SelectedUserIDs)
	sign := "+"
	if data.Mode == BalanceAdjustModeDeduct {
		sign = "-"
	}
	text := fmt.Sprintf("Подтверждение:\nРежим: %s\nПользователи: %s\nСумма: %s%d\nБудет применено к %d пользователям", map[BalanceAdjustMode]string{BalanceAdjustModeAdd: "➕", BalanceAdjustModeDeduct: "➖"}[data.Mode], summary, sign, data.Amount, len(ids))
	if err := h.renderAdminScreen(chatID, userID, panelMsgID, "balance_adjust_confirm", text, newInlineKeyboardMarkup(
		newInlineKeyboardRow(newInlineKeyboardButtonDataStyled("✅ Применить", cbBalConfirmApply, "success")),
		newInlineKeyboardRow(newInlineKeyboardButtonData("↩️ Назад", cbBalConfirmBack), newInlineKeyboardButtonDataStyled("❌ Отмена", cbBalConfirmCancel, "danger")),
	)); err != nil {
		h.sendUIErrorHint(chatID, err)
	}
}

func (h *Handler) handleBalanceConfirm(ctx context.Context, chatID, userID int64, panelMsgID int, cb string) {
	if !h.ensureBalanceState(chatID, userID, panelMsgID, StateBalanceAdjustConfirm) {
		return
	}
	state := h.service.GetState(userID)
	data, _ := state.Data.(*BalanceAdjustData)
	if data == nil || !h.isBalanceReadyToConfirm(data) {
		h.resetBalanceFlow(chatID, userID, panelMsgID)
		return
	}
	switch cb {
	case cbBalConfirmBack:
		h.service.SetState(userID, StateBalanceAdjustAmount, data)
		h.renderBalanceAmount(chatID, userID, panelMsgID)
	case cbBalConfirmCancel:
		h.service.ClearState(userID)
		h.showKeyboardSafe(chatID, userID, panelMsgID)
	case cbBalConfirmApply:
		ids := selectedIDs(data.SelectedUserIDs)
		invalidIDs := h.findInvalidUsersForAdjust(ctx, ids)
		if len(invalidIDs) > 0 {
			h.sendMessage(chatID, fmt.Sprintf("❌ Нельзя применить: у пользователей нет роли или они отсутствуют: %s", joinInt64(invalidIDs)))
			h.service.SetState(userID, StateBalanceAdjustPicker, data)
			h.renderBalancePicker(chatID, userID, panelMsgID)
			return
		}

		desc := fmt.Sprintf("admin %d: %s%d", userID, map[BalanceAdjustMode]string{BalanceAdjustModeAdd: "+", BalanceAdjustModeDeduct: "-"}[data.Mode], data.Amount)
		applied := make([]BalanceAdjustOperation, 0, len(ids))
		for _, id := range ids {
			var err error
			if data.Mode == BalanceAdjustModeAdd {
				err = h.economyService.AddBalance(ctx, id, data.Amount, "admin_adjust", desc)
			} else {
				err = h.economyService.DeductBalance(ctx, id, data.Amount, "admin_adjust", desc)
			}
			if err != nil {
				rollbackErrs := h.rollbackAppliedBalanceOps(ctx, applied)
				if len(rollbackErrs) > 0 {
					h.sendMessage(chatID, fmt.Sprintf("❌ Ошибка применения: %s. Ошибка отката: %s", err.Error(), strings.Join(rollbackErrs, "; ")))
				} else {
					h.sendMessage(chatID, fmt.Sprintf("❌ Ошибка применения: %s", err.Error()))
				}
				return
			}
			applied = append(applied, BalanceAdjustOperation{UserID: id, Mode: data.Mode, Amount: data.Amount})
		}
		data.LastOperation = applied
		data.LastOperationID = fmt.Sprintf("%d", time.Now().UnixNano())
		data.LastOperationAt = time.Now()
		data.Undone = false
		h.service.SetState(userID, StateBalanceAdjustConfirm, data)
		h.renderBalanceSuccess(chatID, userID, panelMsgID, false)
	}
}

func (h *Handler) renderBalanceSuccess(chatID, userID int64, panelMsgID int, undone bool) {
	state := h.service.GetState(userID)
	if state == nil {
		return
	}
	data, _ := state.Data.(*BalanceAdjustData)
	if data == nil {
		return
	}
	text := fmt.Sprintf("✅ Готово\nИзменено %d пользователей\nСумма: %d", len(data.LastOperation), data.Amount)
	if undone {
		text = "↩️ Откат выполнен"
	}
	rows := [][]models.InlineKeyboardButton{}
	if !undone {
		rows = append(rows, newInlineKeyboardRow(newInlineKeyboardButtonDataStyled("↩️ Undo", cbBalUndo, "danger")))
	}
	rows = append(rows, newInlineKeyboardRow(newInlineKeyboardButtonDataStyled("✅ Вернуться в админ-панель", cbAdminReturnPanel, "success")))
	if err := h.renderAdminScreen(chatID, userID, panelMsgID, "balance_adjust_success", text, newInlineKeyboardMarkup(rows...)); err != nil {
		h.sendUIErrorHint(chatID, err)
	}
}

func (h *Handler) handleBalanceUndo(ctx context.Context, chatID, userID int64, panelMsgID int) {
	state := h.service.GetState(userID)
	if state == nil {
		return
	}
	data, _ := state.Data.(*BalanceAdjustData)
	if data == nil || len(data.LastOperation) == 0 || data.Undone {
		h.sendMessage(chatID, "Нечего откатывать")
		return
	}
	if time.Since(data.LastOperationAt) > undoTTL {
		data.LastOperation = nil
		data.Undone = true
		h.sendMessage(chatID, "Операция устарела, откат недоступен")
		return
	}
	for i := len(data.LastOperation) - 1; i >= 0; i-- {
		op := data.LastOperation[i]
		if op.Mode == BalanceAdjustModeAdd {
			if err := h.economyService.DeductBalance(ctx, op.UserID, op.Amount, "admin_adjust_undo", "undo"); err != nil {
				h.sendMessage(chatID, "❌ Ошибка отката")
				return
			}
		} else {
			if err := h.economyService.AddBalance(ctx, op.UserID, op.Amount, "admin_adjust_undo", "undo"); err != nil {
				h.sendMessage(chatID, "❌ Ошибка отката")
				return
			}
		}
	}
	data.Undone = true
	data.LastOperation = nil
	h.renderBalanceSuccess(chatID, userID, panelMsgID, true)
}

func (h *Handler) ensureBalanceState(chatID, userID int64, panelMsgID int, expected string) bool {
	state := h.service.GetState(userID)
	if state == nil || state.State != expected {
		h.resetBalanceFlow(chatID, userID, panelMsgID)
		return false
	}
	return true
}

func (h *Handler) resetBalanceFlow(chatID, userID int64, panelMsgID int) {
	h.service.ClearState(userID)
	h.sendMessage(chatID, "⚠️ Сессия изменения баланса устарела или сбилась. Начните заново.")
	h.showKeyboardSafe(chatID, userID, panelMsgID)
}

func validateBalanceAmount(amount int64) error {
	if amount <= 0 || amount > maxBalanceAdjustAmount {
		return fmt.Errorf("invalid amount")
	}
	return nil
}

func (h *Handler) isBalanceSelectionReady(data *BalanceAdjustData) bool {
	return data != nil && (data.Mode == BalanceAdjustModeAdd || data.Mode == BalanceAdjustModeDeduct) && len(data.SelectedUserIDs) > 0
}

func (h *Handler) isBalanceReadyToConfirm(data *BalanceAdjustData) bool {
	return h.isBalanceSelectionReady(data) && validateBalanceAmount(data.Amount) == nil
}

// Пользователь считается "с ролью", если запись существует и role != NULL/пустая строка после TrimSpace.
func (h *Handler) findInvalidUsersForAdjust(ctx context.Context, userIDs []int64) []int64 {
	invalid := make([]int64, 0)
	for _, userID := range userIDs {
		m, err := h.service.memberRepo.GetByUserID(ctx, userID)
		if err != nil || m == nil || m.Role == nil || strings.TrimSpace(*m.Role) == "" {
			invalid = append(invalid, userID)
		}
	}
	return invalid
}

// Atomicity strategy: best-effort rollback for already-applied users if one call fails.
// Ограничение: rollback тоже может упасть, тогда нужен ручной разбор по user_id из сообщения.
func (h *Handler) rollbackAppliedBalanceOps(ctx context.Context, applied []BalanceAdjustOperation) []string {
	errTexts := make([]string, 0)
	for i := len(applied) - 1; i >= 0; i-- {
		op := applied[i]
		if op.Mode == BalanceAdjustModeAdd {
			if err := h.economyService.DeductBalance(ctx, op.UserID, op.Amount, "admin_adjust_rollback", "rollback"); err != nil {
				errTexts = append(errTexts, fmt.Sprintf("user_id=%d: %s", op.UserID, err.Error()))
			}
		} else {
			if err := h.economyService.AddBalance(ctx, op.UserID, op.Amount, "admin_adjust_rollback", "rollback"); err != nil {
				errTexts = append(errTexts, fmt.Sprintf("user_id=%d: %s", op.UserID, err.Error()))
			}
		}
	}
	return errTexts
}

func selectedIDs(m map[int64]bool) []int64 {
	ids := make([]int64, 0, len(m))
	for id, ok := range m {
		if ok {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func summarizeUsers(users []*members.Member, selected map[int64]bool) string {
	names := make([]string, 0)
	for _, u := range users {
		if selected[u.UserID] {
			names = append(names, u.DisplayName())
		}
	}
	if len(names) <= 5 {
		return strings.Join(names, ", ")
	}
	return fmt.Sprintf("%s и еще %d", strings.Join(names[:5], ", "), len(names)-5)
}

func joinInt64(ids []int64) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, strconv.FormatInt(id, 10))
	}
	return strings.Join(parts, ",")
}
