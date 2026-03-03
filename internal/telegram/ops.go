package telegram

import (
	"context"
	"strings"

	"github.com/go-telegram/bot/models"
	log "github.com/sirupsen/logrus"
)

type Ops struct {
	c   Client
	log *log.Entry
}

type editErrorKind string

const (
	editErrNone         editErrorKind = "none"
	editErrNotModified  editErrorKind = "not_modified"
	editErrNotFound     editErrorKind = "not_found"
	editErrCantBeEdited editErrorKind = "cant_be_edited"
	editErrForbidden    editErrorKind = "forbidden"
	editErrOther        editErrorKind = "other"
)

var editNeedlesNotModified = []string{"message is not modified"}
var editNeedlesNotFound = []string{"message to edit not found", "message not found", "message_id_invalid", "message_id invalid"}
var editNeedlesCantBeEdited = []string{"message can't be edited", "message can\u2019t be edited"}
var editNeedlesForbidden = []string{"bot was blocked by the user", "chat not found", "forbidden", "not enough rights", "user is deactivated"}

func NewOps(c Client) *Ops {
	return NewOpsWithLogger(c, log.NewEntry(log.StandardLogger()))
}

func NewOpsWithLogger(c Client, l *log.Entry) *Ops {
	if l == nil {
		l = log.NewEntry(log.StandardLogger())
	}
	return &Ops{c: c, log: l.WithField("component", "telegram.ops")}
}

func (o *Ops) Send(ctx context.Context, chatID int64, text string, markup *models.InlineKeyboardMarkup) (int, error) {
	msgID, err := o.c.SendMessage(chatID, text, markup)
	if err != nil {
		o.log.WithContext(ctx).WithError(err).WithField("chat_id", chatID).Warn("telegram send failed")
		return 0, err
	}
	return msgID, nil
}

func (o *Ops) Edit(ctx context.Context, chatID int64, messageID int, text string, keyboard *models.InlineKeyboardMarkup) error {
	err := o.c.EditMessage(chatID, messageID, text, keyboard)
	if err == nil {
		return nil
	}

	kind := classifyEditError(err)
	entry := o.log.WithContext(ctx).WithError(err).WithFields(log.Fields{"chat_id": chatID, "message_id": messageID, "kind": kind})
	switch kind {
	case editErrNotModified:
		entry.Debug("telegram edit skipped: message is not modified")
	case editErrForbidden:
		entry.Warn("telegram edit forbidden")
	default:
		entry.Warn("telegram edit failed")
	}
	return err
}

func (o *Ops) AnswerCallback(ctx context.Context, callbackID, text string, showAlert bool) error {
	err := o.c.AnswerCallbackQuery(callbackID, text, showAlert)
	if err != nil {
		o.log.WithContext(ctx).WithError(err).WithField("callback_id", callbackID).Debug("telegram answer callback failed")
		return err
	}
	return nil
}

func (o *Ops) EditOrSend(ctx context.Context, chatID int64, messageID int, text string, keyboard models.InlineKeyboardMarkup) (int, bool, error) {
	if messageID <= 0 {
		msgID, err := o.Send(ctx, chatID, text, &keyboard)
		if err != nil {
			return 0, false, err
		}
		return msgID, false, nil
	}

	err := o.Edit(ctx, chatID, messageID, text, &keyboard)
	if err == nil {
		return messageID, true, nil
	}

	kind := classifyEditError(err)
	switch kind {
	case editErrNotModified:
		return messageID, true, nil
	case editErrNotFound, editErrCantBeEdited:
		msgID, sendErr := o.Send(ctx, chatID, text, &keyboard)
		if sendErr != nil {
			return 0, false, sendErr
		}
		return msgID, false, nil
	case editErrForbidden:
		return 0, false, err
	default:
		return 0, false, err
	}
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
