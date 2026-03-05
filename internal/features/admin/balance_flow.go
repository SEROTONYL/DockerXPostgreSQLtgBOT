package admin

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	models "github.com/mymmrac/telego"

	"serotonyl.ru/telegram-bot/internal/features/members"
	"serotonyl.ru/telegram-bot/internal/telegram"
	"serotonyl.ru/telegram-bot/internal/uiwizard"
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
	cbBalAmtAddDelta    = "admin:balamt:add_delta"

	cbBalConfirmApply  = "admin:balconfirm:apply"
	cbBalConfirmBack   = "admin:balconfirm:back"
	cbBalConfirmCancel = "admin:balconfirm:cancel"

	cbBalUndo = "admin:balundo"
)

const (
	maxBalanceAdjustAmount = int64(1_000_000)
	undoTTL                = 30 * time.Minute
)

func (h *Handler) startBalanceAdjustMode(ctx context.Context, chatID, userID int64, panelMsgID int) {
	data := &BalanceAdjustData{PageSize: userPickerPageSize, SelectedUserIDs: map[int64]bool{}, Wizard: &uiwizard.WizardState{ChatID: chatID, MessageID: panelMsgID, StartedAt: time.Now(), Step: StateBalanceAdjustMode}}
	h.service.SetState(userID, StateBalanceAdjustMode, data)
	h.renderWizard(ctx, chatID, userID, data, "balance_adjust_mode", "Выберите режим изменения баланса:", newInlineKeyboardMarkup(
		newInlineKeyboardRow(
			newInlineKeyboardButtonData("➕ Прибавить", "admin:balmode:add"),
			newInlineKeyboardButtonData("➖ Отнять", "admin:balmode:deduct"),
		),
		newInlineKeyboardRow(newInlineKeyboardButtonDataStyled(userPickerBackButton, cbAdminReturnPanel, "danger")),
	))
}

func (h *Handler) renderWizard(ctx context.Context, chatID, userID int64, data *BalanceAdjustData, screenName, text string, markup models.InlineKeyboardMarkup) {
	if data == nil {
		if st := h.service.GetState(userID); st != nil {
			data, _ = st.Data.(*BalanceAdjustData)
		}
	}
	if data == nil {
		return
	}
	w := h.balanceWizardState(data)
	if w.ChatID == 0 {
		w.ChatID = chatID
	}
	h.setWizardCtx(ctx)
	ui := uiwizard.Output{Text: text, Markup: &markup}
	err := uiwizard.Render(h, w, ui,
		telegram.ShouldFallbackToSendOnEdit,
		telegram.IsEditNotModified,
	)
	if err != nil {
		h.logAdminUIError(userID, chatID, w.MessageID, screenName, "wizard_render", 0, "", err)
		h.sendUIErrorHint(ctx, chatID, err)
		return
	}
	h.attachPanelMessageID(userID, w.MessageID)
}

func (h *Handler) handleBalanceAdjustCallback(ctx context.Context, chatID, userID int64, panelMsgID int, data string) {
	s := h.service.GetState(userID)
	if s != nil {
		if d, ok := s.Data.(*BalanceAdjustData); ok && d != nil {
			if d.FlowChatID == 0 {
				d.FlowChatID = chatID
			}
			d.FlowMessageID = panelMsgID
		}
	}
	switch {
	case strings.HasPrefix(data, "admin:balmode:"):
		h.handleBalanceAdjustMode(ctx, chatID, userID, strings.TrimPrefix(data, "admin:balmode:"))
	case strings.HasPrefix(data, "admin:balpick:"):
		h.handleBalancePicker(chatID, userID, data)
	case strings.HasPrefix(data, "admin:balamt:"):
		h.handleBalanceAmount(chatID, userID, data)
	case strings.HasPrefix(data, "admin:balconfirm:"):
		h.handleBalanceConfirm(ctx, chatID, userID, data)
	case data == cbBalUndo:
		h.handleBalanceUndo(ctx, chatID, userID)
	}
}

func (h *Handler) handleBalanceAdjustMode(ctx context.Context, chatID, userID int64, modeRaw string) {
	if !h.ensureBalanceState(chatID, userID, StateBalanceAdjustMode) {
		return
	}
	state := h.service.GetState(userID)
	data, _ := state.Data.(*BalanceAdjustData)
	if modeRaw == "add" {
		data.Mode = BalanceAdjustModeAdd
	} else if modeRaw == "deduct" {
		data.Mode = BalanceAdjustModeDeduct
	} else {
		h.resetBalanceFlow(chatID, userID, data)
		return
	}
	users, err := h.service.GetUsersWithRole(ctx)
	if err != nil || len(users) == 0 {
		h.renderWizardError(chatID, userID, data, "balance_adjust_mode", "Выберите режим изменения баланса:", "Нет пользователей с назначенными ролями", newInlineKeyboardMarkup(newInlineKeyboardRow(newInlineKeyboardButtonDataStyled(userPickerBackButton, cbAdminReturnPanel, "danger"))))
		return
	}
	data.UsersSnapshot = users
	if data.SelectedUserIDs == nil {
		data.SelectedUserIDs = map[int64]bool{}
	}
	uiwizard.Transition(h.balanceWizardState(data), StateBalanceAdjustPicker)
	h.service.SetState(userID, StateBalanceAdjustPicker, data)
	h.renderBalancePicker(chatID, userID)
}

func (h *Handler) renderBalancePicker(chatID, userID int64) {
	if !h.ensureBalanceState(chatID, userID, StateBalanceAdjustPicker) {
		return
	}
	state := h.service.GetState(userID)
	data, _ := state.Data.(*BalanceAdjustData)
	if data == nil || len(data.UsersSnapshot) == 0 {
		h.resetBalanceFlow(chatID, userID, data)
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
	rows = append(rows, newInlineKeyboardRow(newInlineKeyboardButtonData("♻️ Сбросить", cbBalPickClear)))
	if len(data.SelectedUserIDs) > 0 {
		rows = append(rows, newInlineKeyboardRow(newInlineKeyboardButtonDataStyled("➡️ Далее", cbBalPickDone, "success")))
	} else {
		rows = append(rows, newInlineKeyboardRow(newInlineKeyboardButtonData("Выберите хотя бы одного", "admin:balpick:noop")))
	}
	rows = append(rows, newInlineKeyboardRow(newInlineKeyboardButtonDataStyled(userPickerBackButton, cbBalPickBack, "danger")))
	text := fmt.Sprintf("Выберите пользователей (только с ролью).\nВыбрано: %d", len(data.SelectedUserIDs))
	h.renderWizard(h.currentWizardCtx(), chatID, userID, data, "balance_adjust_picker", text, newInlineKeyboardMarkup(rows...))
}

func (h *Handler) handleBalancePicker(chatID, userID int64, cb string) {
	if !h.ensureBalanceState(chatID, userID, StateBalanceAdjustPicker) {
		return
	}
	state := h.service.GetState(userID)
	data, _ := state.Data.(*BalanceAdjustData)
	if data == nil {
		h.resetBalanceFlow(chatID, userID, data)
		return
	}
	switch {
	case cb == cbBalPickPrev:
		if data.PageIndex > 0 {
			data.PageIndex--
		}
		h.renderBalancePicker(chatID, userID)
	case cb == cbBalPickNext:
		totalPages := (len(data.UsersSnapshot) + data.PageSize - 1) / data.PageSize
		if data.PageIndex < totalPages-1 {
			data.PageIndex++
		}
		h.renderBalancePicker(chatID, userID)
	case cb == cbBalPickClear:
		data.SelectedUserIDs = map[int64]bool{}
		h.renderBalancePicker(chatID, userID)
	case cb == cbBalPickDone:
		if !h.isBalanceSelectionReady(data) {
			h.renderBalancePicker(chatID, userID)
			return
		}
		uiwizard.Transition(h.balanceWizardState(data), StateBalanceAdjustAmount)
		w := h.balanceWizardState(data)
		w.AwaitTextFor = ""
		uiwizard.Transition(w, StateBalanceAdjustAmount)
		h.service.SetState(userID, StateBalanceAdjustAmount, data)
		h.renderBalanceAmount(chatID, userID)
	case cb == cbBalPickBack:
		h.startBalanceAdjustMode(h.currentWizardCtx(), chatID, userID, h.balanceWizardState(data).MessageID)
	case strings.HasPrefix(cb, cbBalPickToggle+":"):
		id, err := strconv.ParseInt(strings.TrimPrefix(cb, cbBalPickToggle+":"), 10, 64)
		if err != nil || id <= 0 {
			h.resetBalanceFlow(chatID, userID, data)
			return
		}
		if data.SelectedUserIDs[id] {
			delete(data.SelectedUserIDs, id)
		} else {
			data.SelectedUserIDs[id] = true
		}
		h.renderBalancePicker(chatID, userID)
	}
}

func (h *Handler) renderBalanceAmount(chatID, userID int64) {
	if !h.ensureBalanceState(chatID, userID, StateBalanceAdjustAmount) {
		return
	}
	state := h.service.GetState(userID)
	data, _ := state.Data.(*BalanceAdjustData)
	if data == nil || !h.isBalanceSelectionReady(data) {
		h.resetBalanceFlow(chatID, userID, data)
		return
	}
	deltas, err := h.service.repo.ListBalanceDeltas(h.currentWizardCtx(), chatID)
	if err != nil {
		h.renderWizardError(chatID, userID, data, "balance_adjust_amount", "Выберите сумму изменения:", "Не удалось загрузить дельты", newInlineKeyboardMarkup(newInlineKeyboardRow(newInlineKeyboardButtonDataStyled(userPickerBackButton, cbBalAmtBack, "danger"))))
		return
	}
	rows := make([][]models.InlineKeyboardButton, 0)
	sign := "+"
	if data.Mode == BalanceAdjustModeDeduct {
		sign = "-"
	}
	for _, d := range deltas {
		rows = append(rows, newInlineKeyboardRow(newInlineKeyboardButtonData(fmt.Sprintf("%s %s%d", strings.TrimSpace(d.Name), sign, d.Amount), cbBalAmtDeltaPrefix+strconv.FormatInt(d.Amount, 10))))
	}
	rows = append(rows, newInlineKeyboardRow(newInlineKeyboardButtonData("➕ Добавить дельту", cbBalAmtAddDelta)))
	rows = append(rows, newInlineKeyboardRow(newInlineKeyboardButtonData("⌨️ Ввести вручную", cbBalAmtManual)))
	rows = append(rows, newInlineKeyboardRow(newInlineKeyboardButtonDataStyled(userPickerBackButton, cbBalAmtBack, "danger")))
	h.renderWizard(h.currentWizardCtx(), chatID, userID, data, "balance_adjust_amount", "Выберите сумму изменения:", newInlineKeyboardMarkup(rows...))
}

func (h *Handler) handleBalanceAmount(chatID, userID int64, cb string) {
	state := h.service.GetState(userID)
	if state == nil {
		return
	}
	data, _ := state.Data.(*BalanceAdjustData)
	switch {
	case state.State == StateBalanceAdjustAmount && strings.HasPrefix(cb, cbBalAmtDeltaPrefix):
		amount, err := strconv.ParseInt(strings.TrimPrefix(cb, cbBalAmtDeltaPrefix), 10, 64)
		if err != nil || validateBalanceAmount(amount) != nil {
			h.renderWizardError(chatID, userID, data, "balance_adjust_amount", "Выберите сумму изменения:", "Некорректная сумма", newInlineKeyboardMarkup(newInlineKeyboardRow(newInlineKeyboardButtonDataStyled(userPickerBackButton, cbBalAmtBack, "danger"))))
			return
		}
		data.Amount = amount
		data.AmountSource = "delta"
		data.AwaitingManual = false
		h.balanceWizardState(data).AwaitTextFor = ""
		uiwizard.Transition(h.balanceWizardState(data), StateBalanceAdjustConfirm)
		h.service.SetState(userID, StateBalanceAdjustConfirm, data)
		h.renderBalanceConfirm(chatID, userID)
	case state.State == StateBalanceAdjustAmount && cb == cbBalAmtManual:
		data.AwaitingManual = true
		h.balanceWizardState(data).AwaitTextFor = "amount"
		h.renderWizard(h.currentWizardCtx(), chatID, userID, data, "balance_adjust_amount_manual", fmt.Sprintf("Отправьте сумму (целое число > 0 и <= %d)", maxBalanceAdjustAmount), newInlineKeyboardMarkup(
			newInlineKeyboardRow(newInlineKeyboardButtonDataStyled(userPickerBackButton, cbBalAmtBack, "danger")),
		))
	case state.State == StateBalanceAdjustAmount && cb == cbBalAmtAddDelta:
		data.PendingDeltaName = ""
		w := h.balanceWizardState(data)
		uiwizard.Transition(w, StateBalanceDeltaName)
		w.AwaitTextFor = "delta_name"
		h.service.SetState(userID, StateBalanceDeltaName, data)
		h.renderWizard(h.currentWizardCtx(), chatID, userID, data, "balance_delta_name", "Введите название дельты (1..32)", newInlineKeyboardMarkup(newInlineKeyboardRow(newInlineKeyboardButtonDataStyled(userPickerBackButton, cbBalAmtBack, "danger"))))
	case cb == cbBalAmtBack && state.State == StateBalanceAdjustAmount:
		uiwizard.Transition(h.balanceWizardState(data), StateBalanceAdjustPicker)
		h.service.SetState(userID, StateBalanceAdjustPicker, data)
		h.renderBalancePicker(chatID, userID)
	case cb == cbBalAmtBack && (state.State == StateBalanceDeltaName || state.State == StateBalanceDeltaAmount):
		uiwizard.Transition(h.balanceWizardState(data), StateBalanceAdjustAmount)
		w := h.balanceWizardState(data)
		w.AwaitTextFor = ""
		uiwizard.Transition(w, StateBalanceAdjustAmount)
		h.service.SetState(userID, StateBalanceAdjustAmount, data)
		h.renderBalanceAmount(chatID, userID)
	}
}

func (h *Handler) handleBalanceAdjustManualAmount(ctx context.Context, chatID, userID int64, text string) bool {
	_ = ctx
	state := h.service.GetState(userID)
	if state == nil {
		return false
	}
	data, _ := state.Data.(*BalanceAdjustData)
	w := h.balanceWizardState(data)
	field, _ := uiwizard.ConsumeText(w, text)
	if state.State == StateBalanceAdjustAmount {
		if data == nil || !data.AwaitingManual || !h.isBalanceSelectionReady(data) {
			h.resetBalanceFlow(chatID, userID, data)
			return true
		}
		rawText := text
		if field == "amount" {
			if v, ok := w.Data[field].(string); ok {
				rawText = v
			}
		}
		normalized := strings.ReplaceAll(strings.TrimSpace(rawText), " ", "")
		amount, err := strconv.ParseInt(normalized, 10, 64)
		if err != nil || validateBalanceAmount(amount) != nil {
			h.renderWizard(h.currentWizardCtx(), chatID, userID, data, "balance_adjust_amount_manual", fmt.Sprintf("Отправьте сумму (целое число > 0 и <= %d)\n❌ Некорректная сумма", maxBalanceAdjustAmount), newInlineKeyboardMarkup(
				newInlineKeyboardRow(newInlineKeyboardButtonDataStyled(userPickerBackButton, cbBalAmtBack, "danger")),
			))
			return true
		}
		data.Amount = amount
		data.AmountSource = "manual"
		data.AwaitingManual = false
		h.balanceWizardState(data).AwaitTextFor = ""
		uiwizard.Transition(h.balanceWizardState(data), StateBalanceAdjustConfirm)
		h.service.SetState(userID, StateBalanceAdjustConfirm, data)
		h.renderBalanceConfirm(chatID, userID)
		return true
	}
	if state.State == StateBalanceDeltaName {
		name := strings.TrimSpace(text)
		if field == "delta_name" {
			if v, ok := w.Data[field].(string); ok {
				name = strings.TrimSpace(v)
			}
		}
		if len([]rune(name)) == 0 || len([]rune(name)) > 32 {
			h.renderWizard(h.currentWizardCtx(), chatID, userID, data, "balance_delta_name", "Введите название дельты (1..32)\n❌ Некорректное название", newInlineKeyboardMarkup(newInlineKeyboardRow(newInlineKeyboardButtonDataStyled(userPickerBackButton, cbBalAmtBack, "danger"))))
			return true
		}
		data.PendingDeltaName = name
		uiwizard.Transition(h.balanceWizardState(data), StateBalanceDeltaAmount)
		h.service.SetState(userID, StateBalanceDeltaAmount, data)
		h.renderWizard(ctx, chatID, userID, data, "balance_delta_amount", fmt.Sprintf("Введите сумму дельты (целое > 0 и <= %d)", maxBalanceAdjustAmount), newInlineKeyboardMarkup(newInlineKeyboardRow(newInlineKeyboardButtonDataStyled(userPickerBackButton, cbBalAmtBack, "danger"))))
		return true
	}
	if state.State == StateBalanceDeltaAmount {
		rawText := text
		if field == "delta_amount" {
			if v, ok := w.Data[field].(string); ok {
				rawText = v
			}
		}
		normalized := strings.ReplaceAll(strings.TrimSpace(rawText), " ", "")
		amount, err := strconv.ParseInt(normalized, 10, 64)
		if err != nil || validateBalanceAmount(amount) != nil {
			h.renderWizard(ctx, chatID, userID, data, "balance_delta_amount", fmt.Sprintf("Введите сумму дельты (целое > 0 и <= %d)\n❌ Некорректная сумма", maxBalanceAdjustAmount), newInlineKeyboardMarkup(newInlineKeyboardRow(newInlineKeyboardButtonDataStyled(userPickerBackButton, cbBalAmtBack, "danger"))))
			return true
		}
		if err := h.service.repo.CreateBalanceDelta(ctx, chatID, data.PendingDeltaName, amount, userID); err != nil {
			h.renderWizard(ctx, chatID, userID, data, "balance_delta_amount", "❌ Не удалось создать дельту", newInlineKeyboardMarkup(newInlineKeyboardRow(newInlineKeyboardButtonDataStyled(userPickerBackButton, cbBalAmtBack, "danger"))))
			return true
		}
		data.PendingDeltaName = ""
		uiwizard.Transition(h.balanceWizardState(data), StateBalanceAdjustAmount)
		w := h.balanceWizardState(data)
		w.AwaitTextFor = ""
		uiwizard.Transition(w, StateBalanceAdjustAmount)
		h.service.SetState(userID, StateBalanceAdjustAmount, data)
		h.renderBalanceAmount(chatID, userID)
		return true
	}
	return false
}

func (h *Handler) renderBalanceConfirm(chatID, userID int64) {
	if !h.ensureBalanceState(chatID, userID, StateBalanceAdjustConfirm) {
		return
	}
	state := h.service.GetState(userID)
	data, _ := state.Data.(*BalanceAdjustData)
	if data == nil || !h.isBalanceReadyToConfirm(data) {
		h.resetBalanceFlow(chatID, userID, data)
		return
	}
	ids := selectedIDs(data.SelectedUserIDs)
	summary := summarizeUsers(data.UsersSnapshot, data.SelectedUserIDs)
	sign := "+"
	if data.Mode == BalanceAdjustModeDeduct {
		sign = "-"
	}
	text := fmt.Sprintf("Подтверждение:\nРежим: %s\nПользователи: %s\nСумма: %s%d\nБудет применено к %d пользователям", map[BalanceAdjustMode]string{BalanceAdjustModeAdd: "➕", BalanceAdjustModeDeduct: "➖"}[data.Mode], summary, sign, data.Amount, len(ids))
	h.renderWizard(h.currentWizardCtx(), chatID, userID, data, "balance_adjust_confirm", text, newInlineKeyboardMarkup(
		newInlineKeyboardRow(newInlineKeyboardButtonDataStyled("✅ Применить", cbBalConfirmApply, "success")),
		newInlineKeyboardRow(newInlineKeyboardButtonData("↩️ Назад", cbBalConfirmBack), newInlineKeyboardButtonDataStyled("❌ Отмена", cbBalConfirmCancel, "danger")),
	))
}

func (h *Handler) handleBalanceConfirm(ctx context.Context, chatID, userID int64, cb string) {
	if !h.ensureBalanceState(chatID, userID, StateBalanceAdjustConfirm) {
		return
	}
	state := h.service.GetState(userID)
	data, _ := state.Data.(*BalanceAdjustData)
	if data == nil || !h.isBalanceReadyToConfirm(data) {
		h.resetBalanceFlow(chatID, userID, data)
		return
	}
	switch cb {
	case cbBalConfirmBack:
		uiwizard.Transition(h.balanceWizardState(data), StateBalanceAdjustAmount)
		w := h.balanceWizardState(data)
		w.AwaitTextFor = ""
		uiwizard.Transition(w, StateBalanceAdjustAmount)
		h.service.SetState(userID, StateBalanceAdjustAmount, data)
		h.renderBalanceAmount(chatID, userID)
	case cbBalConfirmCancel:
		h.service.ClearState(userID)
		h.showKeyboardSafe(ctx, chatID, userID, h.balanceWizardState(data).MessageID)
	case cbBalConfirmApply:
		ids := selectedIDs(data.SelectedUserIDs)
		invalidIDs := h.findInvalidUsersForAdjust(ctx, ids)
		if len(invalidIDs) > 0 {
			h.renderWizardError(chatID, userID, data, "balance_adjust_confirm", "Подтверждение", fmt.Sprintf("Нельзя применить: у пользователей нет роли или они отсутствуют: %s", joinInt64(invalidIDs)), newInlineKeyboardMarkup(newInlineKeyboardRow(newInlineKeyboardButtonData("⬅️ Назад", cbBalConfirmBack))))
			uiwizard.Transition(h.balanceWizardState(data), StateBalanceAdjustPicker)
			h.service.SetState(userID, StateBalanceAdjustPicker, data)
			h.renderBalancePicker(chatID, userID)
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
				errText := err.Error()
				if strings.Contains(err.Error(), "недостаточно") {
					errText = fmt.Sprintf("Недостаточно средств у userID=%d", id)
				}
				if len(rollbackErrs) > 0 {
					h.renderWizard(ctx, chatID, userID, data, "balance_adjust_error", fmt.Sprintf("❌ Ошибка применения: %s\nОткат: %s", errText, strings.Join(rollbackErrs, "; ")), newInlineKeyboardMarkup(newInlineKeyboardRow(newInlineKeyboardButtonDataStyled("🏠 Админка", cbAdminReturnPanel, "success"))))
				} else {
					h.renderWizard(ctx, chatID, userID, data, "balance_adjust_error", fmt.Sprintf("❌ Ошибка применения: %s", errText), newInlineKeyboardMarkup(newInlineKeyboardRow(newInlineKeyboardButtonDataStyled("🏠 Админка", cbAdminReturnPanel, "success"))))
				}
				return
			}
			applied = append(applied, BalanceAdjustOperation{UserID: id, Mode: data.Mode, Amount: data.Amount})
		}
		data.LastOperation = applied
		data.LastOperationID = fmt.Sprintf("%d", time.Now().UnixNano())
		data.LastOperationAt = time.Now()
		data.Undone = false
		uiwizard.Transition(h.balanceWizardState(data), StateBalanceAdjustConfirm)
		h.service.SetState(userID, StateBalanceAdjustConfirm, data)
		h.renderBalanceSuccess(chatID, userID, data, false)
	}
}

func (h *Handler) renderBalanceSuccess(chatID, userID int64, data *BalanceAdjustData, undone bool) {
	if data == nil {
		state := h.service.GetState(userID)
		if state == nil {
			return
		}
		data, _ = state.Data.(*BalanceAdjustData)
	}
	text := fmt.Sprintf("✅ Готово\nИзменено %d пользователей\nСумма: %d", len(data.LastOperation), data.Amount)
	if undone {
		text = "↩️ Откат выполнен"
	}
	rows := [][]models.InlineKeyboardButton{}
	if !undone {
		rows = append(rows, newInlineKeyboardRow(newInlineKeyboardButtonDataStyled("↩️ Отменить", cbBalUndo, "danger")))
	}
	rows = append(rows, newInlineKeyboardRow(newInlineKeyboardButtonDataStyled("🏠 Админка", cbAdminReturnPanel, "success")))
	h.renderWizard(h.currentWizardCtx(), chatID, userID, data, "balance_adjust_success", text, newInlineKeyboardMarkup(rows...))
}

func (h *Handler) handleBalanceUndo(ctx context.Context, chatID, userID int64) {
	state := h.service.GetState(userID)
	if state == nil {
		return
	}
	data, _ := state.Data.(*BalanceAdjustData)
	if data == nil || len(data.LastOperation) == 0 || data.Undone {
		h.renderWizard(h.currentWizardCtx(), chatID, userID, data, "balance_adjust_undo_empty", "Нечего отменять", newInlineKeyboardMarkup(newInlineKeyboardRow(newInlineKeyboardButtonDataStyled("🏠 Админка", cbAdminReturnPanel, "success"))))
		return
	}
	if time.Since(data.LastOperationAt) > undoTTL {
		data.LastOperation = nil
		data.Undone = true
		h.renderWizard(h.currentWizardCtx(), chatID, userID, data, "balance_adjust_undo_expired", "Операция устарела, откат недоступен", newInlineKeyboardMarkup(newInlineKeyboardRow(newInlineKeyboardButtonDataStyled("🏠 Админка", cbAdminReturnPanel, "success"))))
		return
	}
	for i := len(data.LastOperation) - 1; i >= 0; i-- {
		op := data.LastOperation[i]
		if op.Mode == BalanceAdjustModeAdd {
			if err := h.economyService.DeductBalance(ctx, op.UserID, op.Amount, "admin_adjust_undo", "undo"); err != nil {
				h.renderWizard(h.currentWizardCtx(), chatID, userID, data, "balance_adjust_undo_err", "❌ Ошибка отката", newInlineKeyboardMarkup(newInlineKeyboardRow(newInlineKeyboardButtonDataStyled("🏠 Админка", cbAdminReturnPanel, "success"))))
				return
			}
		} else {
			if err := h.economyService.AddBalance(ctx, op.UserID, op.Amount, "admin_adjust_undo", "undo"); err != nil {
				h.renderWizard(h.currentWizardCtx(), chatID, userID, data, "balance_adjust_undo_err", "❌ Ошибка отката", newInlineKeyboardMarkup(newInlineKeyboardRow(newInlineKeyboardButtonDataStyled("🏠 Админка", cbAdminReturnPanel, "success"))))
				return
			}
		}
	}
	data.Undone = true
	data.LastOperation = nil
	h.service.ClearState(userID)
	h.renderAdminScreen(h.currentWizardCtx(), chatID, userID, h.balanceWizardState(data).MessageID, "balance_adjust_undo_done", "↩️ Откат выполнен", newInlineKeyboardMarkup(newInlineKeyboardRow(newInlineKeyboardButtonDataStyled("🏠 Админка", cbAdminReturnPanel, "success"))))
}

func (h *Handler) ensureBalanceState(chatID, userID int64, expected string) bool {
	state := h.service.GetState(userID)
	if state == nil {
		h.resetBalanceFlow(chatID, userID, nil)
		return false
	}
	data, _ := state.Data.(*BalanceAdjustData)
	if state.State != expected || data == nil || !uiwizard.EnsureStep(h.balanceWizardState(data), expected) {
		h.resetBalanceFlow(chatID, userID, data)
		return false
	}
	return true
}

func (h *Handler) renderWizardError(chatID, userID int64, data *BalanceAdjustData, screenName, base, errText string, markup models.InlineKeyboardMarkup) {
	h.renderWizard(h.currentWizardCtx(), chatID, userID, data, screenName, fmt.Sprintf("%s\n❌ %s", base, errText), markup)
}

func (h *Handler) resetBalanceFlow(chatID, userID int64, data *BalanceAdjustData) {
	flowMsgID := 0
	if data != nil {
		w := h.balanceWizardState(data)
		flowMsgID = w.MessageID
		out := uiwizard.FailAndReset(w, "reset")
		_ = out
	}
	h.service.ClearState(userID)
	if flowMsgID > 0 {
		h.renderAdminScreen(h.currentWizardCtx(), chatID, userID, flowMsgID, "balance_adjust_reset", "⚠️ Сессия сбилась/устарела. Возврат в админ-панель.", newInlineKeyboardMarkup(newInlineKeyboardRow(newInlineKeyboardButtonDataStyled("🏠 Админка", cbAdminReturnPanel, "success"))))
		return
	}
	h.showKeyboardSafe(h.currentWizardCtx(), chatID, userID, 0)
}

func (h *Handler) balanceWizardState(data *BalanceAdjustData) *uiwizard.WizardState {
	if data.Wizard == nil {
		data.Wizard = &uiwizard.WizardState{}
	}
	if data.Wizard.ChatID == 0 && data.FlowChatID != 0 {
		data.Wizard.ChatID = data.FlowChatID
	}
	if data.Wizard.MessageID == 0 && data.FlowMessageID != 0 {
		data.Wizard.MessageID = data.FlowMessageID
	}
	return data.Wizard
}

func (h *Handler) EditMessageText(chatID int64, messageID int, text string, markup *models.InlineKeyboardMarkup) error {
	return h.ops.Edit(h.currentWizardCtx(), chatID, messageID, text, markup)
}

func (h *Handler) SendMessage(chatID int64, text string, markup *models.InlineKeyboardMarkup) (int, error) {
	return h.ops.Send(h.currentWizardCtx(), chatID, text, markup)
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
