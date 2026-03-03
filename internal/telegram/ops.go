package telegram

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-telegram/bot/models"
	"github.com/sirupsen/logrus"
)

type Ops struct {
	c   Client
	log *logrus.Entry
}

type callbackClient interface {
	AnswerCallback(callbackID string) error
}

type legacyCallbackClient interface {
	AnswerCallbackQuery(callbackID string, text string, showAlert bool) error
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
var editNeedlesCantBeEdited = []string{"message can't be edited", "message can’t be edited"}
var editNeedlesForbidden = []string{"bot was blocked by the user", "chat not found", "forbidden", "not enough rights", "user is deactivated"}

func NewOps(c Client) *Ops {
	return NewOpsWithLogger(c, logrus.NewEntry(logrus.StandardLogger()))
}

func NewOpsWithLogger(c Client, l *logrus.Entry) *Ops {
	if l == nil {
		l = logrus.NewEntry(logrus.StandardLogger())
	}
	if c == nil {
		panic("telegram.NewOpsWithLogger: nil client")
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

func (o *Ops) Edit(ctx context.Context, chatID int64, messageID int, text string, markup *models.InlineKeyboardMarkup) error {
	err := o.c.EditMessage(chatID, messageID, text, markup)
	if err == nil {
		return nil
	}

	kind := classifyEditError(err)
	entry := o.log.WithContext(ctx).WithError(err).WithFields(logrus.Fields{"chat_id": chatID, "message_id": messageID, "kind": kind})
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

func (o *Ops) AnswerCallback(ctx context.Context, callbackID string, _ ...any) error {
	if callbackID == "" {
		return nil
	}

	var err error
	switch c := any(o.c).(type) {
	case callbackClient:
		err = c.AnswerCallback(callbackID)
	case legacyCallbackClient:
		err = c.AnswerCallbackQuery(callbackID, "", false)
	default:
		err = fmt.Errorf("client does not support answer callback")
	}

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

	switch {
	case IsEditNotModified(err):
		return messageID, true, nil
	case ShouldFallbackToSendOnEdit(err):
		msgID, sendErr := o.Send(ctx, chatID, text, &keyboard)
		if sendErr != nil {
			return 0, false, sendErr
		}
		return msgID, false, nil
	default:
		return 0, false, err
	}
}

func ShouldFallbackToSendOnEdit(err error) bool {
	kind := classifyEditError(err)
	return kind == editErrNotFound || kind == editErrCantBeEdited
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
