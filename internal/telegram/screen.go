package telegram

import (
	"context"

	"github.com/go-telegram/bot/models"
)

type Screen struct {
	ChatID      int64
	MessageID   int
	Text        string
	ReplyMarkup any
}

func RenderScreen(ctx context.Context, ops *Ops, s Screen) (msgID int, usedEdit bool, err error) {
	if s.MessageID > 0 {
		err = ops.Edit(ctx, s.ChatID, s.MessageID, s.Text, inlineMarkup(s.ReplyMarkup))
		if err == nil || IsEditNotModified(err) {
			return s.MessageID, true, nil
		}

		if ShouldFallbackToSendOnEdit(err) {
			msgID, sendErr := ops.Send(ctx, s.ChatID, s.Text, inlineMarkup(s.ReplyMarkup))
			if sendErr != nil {
				return 0, false, sendErr
			}
			return msgID, false, nil
		}
		return 0, false, err
	}

	msgID, err = ops.Send(ctx, s.ChatID, s.Text, inlineMarkup(s.ReplyMarkup))
	if err != nil {
		return 0, false, err
	}
	return msgID, false, nil
}

func inlineMarkup(markup any) *models.InlineKeyboardMarkup {
	if markup == nil {
		return nil
	}
	if v, ok := markup.(*models.InlineKeyboardMarkup); ok {
		return v
	}
	if v, ok := markup.(models.InlineKeyboardMarkup); ok {
		return &v
	}
	return nil
}
