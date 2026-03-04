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

func RenderScreen(ctx context.Context, ops *Ops, s Screen) error {
	if s.MessageID != 0 {
		err := ops.Edit(ctx, s.ChatID, s.MessageID, s.Text, inlineMarkup(s.ReplyMarkup))
		if err == nil {
			return nil
		}

		switch classifyEditError(err) {
		case editErrNotModified:
			return nil
		case editErrNotFound:
			_, sendErr := ops.Send(ctx, s.ChatID, s.Text, inlineMarkup(s.ReplyMarkup))
			return sendErr
		default:
			return err
		}
	}

	_, err := ops.Send(ctx, s.ChatID, s.Text, inlineMarkup(s.ReplyMarkup))
	return err
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
