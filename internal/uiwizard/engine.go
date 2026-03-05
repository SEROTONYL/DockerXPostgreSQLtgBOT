package uiwizard

import models "github.com/mymmrac/telego"

type Renderer interface {
	EditMessageText(chatID int64, messageID int, text string, markup *models.InlineKeyboardMarkup) error
	SendMessage(chatID int64, text string, markup *models.InlineKeyboardMarkup) (newMessageID int, err error)
}

func Render(r Renderer, st *WizardState, out Output, canFallback func(error) bool, isNotModified func(error) bool) error {
	if st == nil {
		return nil
	}
	if st.MessageID > 0 {
		err := r.EditMessageText(st.ChatID, st.MessageID, out.Text, out.Markup)
		if err == nil || (isNotModified != nil && isNotModified(err)) {
			return nil
		}
		if canFallback == nil || !canFallback(err) {
			return err
		}
	}
	msgID, err := r.SendMessage(st.ChatID, out.Text, out.Markup)
	if err != nil {
		return err
	}
	st.MessageID = msgID
	return nil
}
