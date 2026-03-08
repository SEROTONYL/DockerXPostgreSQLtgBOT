package telegram

import (
	"context"
	"strings"

	models "github.com/mymmrac/telego"
)

type editErrorKind string

const (
	editErrNone         editErrorKind = "none"
	editErrNotModified  editErrorKind = "not_modified"
	editErrNotFound     editErrorKind = "not_found"
	editErrCantBeEdited editErrorKind = "cant_be_edited"
	editErrForbidden    editErrorKind = "forbidden"
	editErrOther        editErrorKind = "other"
)

var editNeedlesNotModified = []string{"message is not modified", "message not modified"}
var editNeedlesNotFound = []string{"message to edit not found", "message not found", "message_id_invalid", "message_id invalid"}
var editNeedlesCantBeEdited = []string{"message can't be edited", "message can’t be edited"}
var editNeedlesForbidden = []string{"bot was blocked by the user", "chat not found", "forbidden", "not enough rights", "user is deactivated"}

type Screen struct {
	ChatID      int64
	MessageID   int
	Text        string
	ReplyMarkup any
	ParseMode   *string
}

// RenderScreen is the SINGLE source of truth for edit-or-send behavior.
// All Telegram UI rendering must go through this function.
func RenderScreen(ctx context.Context, ops *Ops, s Screen) (msgID int, usedEdit bool, err error) {
	mk := inlineMarkup(s.ReplyMarkup)

	if s.MessageID > 0 {
		err = ops.EditWithParseMode(ctx, s.ChatID, s.MessageID, s.Text, mk, s.ParseMode)
		if err == nil {
			return s.MessageID, true, nil
		}

		if IsEditNotModified(err) {
			return s.MessageID, true, nil
		}

		if ShouldFallbackToSendOnEdit(err) {
			sentID, sendErr := ops.SendWithParseMode(ctx, s.ChatID, s.Text, mk, s.ParseMode)
			if sendErr != nil {
				return 0, false, sendErr
			}
			return sentID, false, nil
		}
		return 0, true, err
	}

	sentID, sendErr := ops.SendWithParseMode(ctx, s.ChatID, s.Text, mk, s.ParseMode)
	if sendErr != nil {
		return 0, false, sendErr
	}
	return sentID, false, nil
}

func ShouldFallbackToSendOnEdit(err error) bool {
	kind := classifyEditError(err)
	return kind == editErrNotFound || kind == editErrCantBeEdited || kind == editErrForbidden
}

func IsEditNotModified(err error) bool {
	return classifyEditError(err) == editErrNotModified
}

func classifyEditError(err error) editErrorKind {
	if err == nil {
		return editErrNone
	}
	d := strings.ToLower(err.Error())
	switch {
	case containsAny(d, editNeedlesNotModified):
		return editErrNotModified
	case containsAny(d, editNeedlesNotFound):
		return editErrNotFound
	case containsAny(d, editNeedlesCantBeEdited):
		return editErrCantBeEdited
	case containsAny(d, editNeedlesForbidden):
		return editErrForbidden
	default:
		return editErrOther
	}
}

func containsAny(s string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
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
