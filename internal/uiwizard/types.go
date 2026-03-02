// Package uiwizard предоставляет минимальный reusable engine для wizard-flow'ов
// c одним редактируемым сообщением.
package uiwizard

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-telegram/bot/models"
)

type Output struct {
	Text   string
	Markup *models.InlineKeyboardMarkup
}

type WizardState struct {
	ChatID       int64
	MessageID    int
	StartedAt    time.Time
	Step         string
	AwaitTextFor string
	Data         map[string]any
	LastError    string
}

func Reset(st *WizardState) {
	if st == nil {
		return
	}
	*st = WizardState{}
}

func EnsureStep(st *WizardState, expected string) bool {
	return st != nil && st.Step == expected
}

func Transition(st *WizardState, next string) {
	if st == nil {
		return
	}
	st.Step = next
}

func Require(st *WizardState, keys ...string) error {
	if st == nil {
		return fmt.Errorf("wizard state is nil")
	}
	for _, k := range keys {
		if _, ok := st.Data[k]; !ok {
			return fmt.Errorf("missing required key: %s", k)
		}
	}
	return nil
}

func FailAndReset(st *WizardState, reason string) Output {
	if st != nil {
		st.LastError = reason
	}
	Reset(st)
	return Output{Text: "⚠️ Сессия сбилась/устарела. Возврат в админ-панель."}
}

func IsAwaitingText(st *WizardState) bool {
	return st != nil && st.AwaitTextFor != ""
}

func ConsumeText(st *WizardState, input string) (field string, ok bool) {
	if !IsAwaitingText(st) {
		return "", false
	}
	field = st.AwaitTextFor
	st.AwaitTextFor = ""
	if st.Data == nil {
		st.Data = map[string]any{}
	}
	st.Data[field] = strings.TrimSpace(input)
	return field, true
}

func ParseAction(callbackData string) (action string, args []string) {
	parts := strings.Split(callbackData, ":")
	if len(parts) == 0 {
		return "", nil
	}
	if len(parts) <= 2 {
		return callbackData, nil
	}
	action = strings.Join(parts[:len(parts)-1], ":")
	return action, parts[len(parts)-1:]
}
