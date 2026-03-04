package telegram

import (
	"context"
	"fmt"
	"strings"

	botapi "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/sirupsen/logrus"
)

// Client инкапсулирует минимум операций Telegram API, которые используются проектом.
type Client interface {
	SendMessage(chatID int64, text string, markup *models.InlineKeyboardMarkup) (messageID int, err error)
	EditMessage(chatID int64, messageID int, text string, markup *models.InlineKeyboardMarkup) error
	EditReplyMarkup(chatID int64, messageID int, markup *models.InlineKeyboardMarkup) error
	DeleteMessage(chatID int64, messageID int) error
	GetChatMember(chatID int64, userID int64) (member models.ChatMember, err error)
}

type botClient struct {
	bot *botapi.Bot
}

type updateRuntime interface {
	RegisterUpdateHandler(match func(*models.Update) bool, handler func(context.Context, *models.Update))
	Start(ctx context.Context)
	GetMe(ctx context.Context) (*models.User, error)
}

func NewBotClient(bot *botapi.Bot) Client {
	if bot == nil {
		panic("telegram.NewBotClient: nil bot")
	}
	return &botClient{bot: bot}
}

func (a *botClient) SendMessage(chatID int64, text string, markup *models.InlineKeyboardMarkup) (int, error) {
	msg, err := a.bot.SendMessage(context.Background(), buildSendMessageParams(chatID, text, markup))
	if err != nil {
		return 0, err
	}
	if msg == nil {
		return 0, nil
	}
	return msg.ID, nil
}

func (a *botClient) EditMessage(chatID int64, messageID int, text string, markup *models.InlineKeyboardMarkup) error {
	_, err := a.bot.EditMessageText(context.Background(), buildEditMessageTextParams(chatID, messageID, text, markup))
	return err
}

func (a *botClient) EditReplyMarkup(chatID int64, messageID int, markup *models.InlineKeyboardMarkup) error {
	_, err := a.bot.EditMessageReplyMarkup(context.Background(), &botapi.EditMessageReplyMarkupParams{
		ChatID:      chatID,
		MessageID:   messageID,
		ReplyMarkup: markup,
	})
	return err
}

func (a *botClient) DeleteMessage(chatID int64, messageID int) error {
	_, err := a.bot.DeleteMessage(context.Background(), &botapi.DeleteMessageParams{ChatID: chatID, MessageID: messageID})
	return err
}

func (a *botClient) GetChatMember(chatID int64, userID int64) (models.ChatMember, error) {
	cm, err := a.bot.GetChatMember(context.Background(), &botapi.GetChatMemberParams{ChatID: chatID, UserID: userID})
	if err != nil {
		return models.ChatMember{}, err
	}
	if cm == nil {
		return models.ChatMember{}, nil
	}
	return *cm, nil
}

func (a *botClient) RegisterUpdateHandler(match func(*models.Update) bool, handler func(context.Context, *models.Update)) {
	a.bot.RegisterHandlerMatchFunc(match, func(handlerCtx context.Context, _ *botapi.Bot, update *models.Update) {
		handler(handlerCtx, update)
	})
}

func (a *botClient) Start(ctx context.Context) {
	a.bot.Start(ctx)
}

func (a *botClient) GetMe(ctx context.Context) (*models.User, error) {
	return a.bot.GetMe(ctx)
}

func (a *botClient) AnswerCallback(callbackID string) error {
	return a.AnswerCallbackCtx(context.Background(), callbackID)
}

func (a *botClient) AnswerCallbackCtx(ctx context.Context, callbackID string) error {
	if callbackID == "" {
		return nil
	}
	_, err := a.bot.AnswerCallbackQuery(ctx, &botapi.AnswerCallbackQueryParams{CallbackQueryID: callbackID})
	return err
}

// AnswerCallbackQuery оставлен для обратной совместимости с существующими вызовами.
func (a *botClient) AnswerCallbackQuery(callbackID string, text string, showAlert bool) error {
	return a.AnswerCallbackQueryCtx(context.Background(), callbackID, text, showAlert)
}

func (a *botClient) AnswerCallbackQueryCtx(ctx context.Context, callbackID string, text string, showAlert bool) error {
	if callbackID == "" {
		return nil
	}
	_, err := a.bot.AnswerCallbackQuery(ctx, &botapi.AnswerCallbackQueryParams{
		CallbackQueryID: callbackID,
		Text:            text,
		ShowAlert:       showAlert,
	})
	return err
}

func buildSendMessageParams(chatID int64, text string, markup *models.InlineKeyboardMarkup) *botapi.SendMessageParams {
	params := &botapi.SendMessageParams{ChatID: chatID, Text: text}
	if markup != nil {
		params.ReplyMarkup = markup
	}
	return params
}

func buildEditMessageTextParams(chatID int64, messageID int, text string, markup *models.InlineKeyboardMarkup) *botapi.EditMessageTextParams {
	params := &botapi.EditMessageTextParams{ChatID: chatID, MessageID: messageID, Text: text}
	if markup != nil {
		params.ReplyMarkup = markup
	}
	return params
}

type Ops struct {
	c   Client
	log *logrus.Entry
}

type callbackClient interface {
	AnswerCallback(callbackID string) error
}

type callbackClientCtx interface {
	AnswerCallbackCtx(ctx context.Context, callbackID string) error
}

type legacyCallbackClient interface {
	AnswerCallbackQuery(callbackID string, text string, showAlert bool) error
}

type legacyCallbackClientCtx interface {
	AnswerCallbackQueryCtx(ctx context.Context, callbackID string, text string, showAlert bool) error
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

func NewOpsFromBot(bot *botapi.Bot) *Ops {
	return NewOps(NewBotClient(bot))
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

func (o *Ops) SendText(ctx context.Context, chatID int64, text string, markup *models.InlineKeyboardMarkup) (int, error) {
	return o.Send(ctx, chatID, text, markup)
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

func (o *Ops) EditText(ctx context.Context, chatID int64, messageID int, text string, markup *models.InlineKeyboardMarkup) error {
	return o.Edit(ctx, chatID, messageID, text, markup)
}

func (o *Ops) EditReplyMarkup(ctx context.Context, chatID int64, messageID int, markup *models.InlineKeyboardMarkup) error {
	err := o.c.EditReplyMarkup(chatID, messageID, markup)
	if err != nil {
		o.log.WithContext(ctx).WithError(err).WithFields(logrus.Fields{"chat_id": chatID, "message_id": messageID}).Warn("telegram edit reply markup failed")
	}
	return err
}

func (o *Ops) DeleteMessage(ctx context.Context, chatID int64, messageID int) error {
	err := o.c.DeleteMessage(chatID, messageID)
	if err != nil {
		o.log.WithContext(ctx).WithError(err).WithFields(logrus.Fields{"chat_id": chatID, "message_id": messageID}).Warn("telegram delete failed")
	}
	return err
}

func (o *Ops) GetChatMember(ctx context.Context, chatID int64, userID int64) (models.ChatMember, error) {
	member, err := o.c.GetChatMember(chatID, userID)
	if err != nil {
		o.log.WithContext(ctx).WithError(err).WithFields(logrus.Fields{"chat_id": chatID, "user_id": userID}).Warn("telegram get chat member failed")
		return models.ChatMember{}, err
	}
	return member, nil
}

func (o *Ops) RegisterUpdateHandler(match func(*models.Update) bool, handler func(context.Context, *models.Update)) error {
	r, ok := any(o.c).(updateRuntime)
	if !ok {
		return fmt.Errorf("client does not support update handlers")
	}
	r.RegisterUpdateHandler(match, handler)
	return nil
}

func (o *Ops) Start(ctx context.Context) error {
	r, ok := any(o.c).(updateRuntime)
	if !ok {
		return fmt.Errorf("client does not support bot runtime")
	}
	r.Start(ctx)
	return nil
}

func (o *Ops) GetMe(ctx context.Context) (*models.User, error) {
	r, ok := any(o.c).(updateRuntime)
	if !ok {
		return nil, fmt.Errorf("client does not support getMe")
	}
	return r.GetMe(ctx)
}

func (o *Ops) AnswerCallback(ctx context.Context, callbackID string, args ...any) error {
	if callbackID == "" {
		return nil
	}

	text := ""
	showAlert := false
	if len(args) > 0 {
		if v, ok := args[0].(string); ok {
			text = v
		}
	}
	if len(args) > 1 {
		if v, ok := args[1].(bool); ok {
			showAlert = v
		}
	}

	var err error
	if text != "" || showAlert {
		err = o.answerCallbackWithPayload(ctx, callbackID, text, showAlert)
	} else {
		err = o.answerCallbackAck(ctx, callbackID)
	}

	if err != nil {
		o.log.WithContext(ctx).WithError(err).WithField("callback_id", callbackID).Debug("telegram answer callback failed")
		return err
	}
	return nil
}

func (o *Ops) answerCallbackWithPayload(ctx context.Context, callbackID, text string, showAlert bool) error {
	switch c := any(o.c).(type) {
	case legacyCallbackClientCtx:
		return c.AnswerCallbackQueryCtx(ctx, callbackID, text, showAlert)
	case legacyCallbackClient:
		return c.AnswerCallbackQuery(callbackID, text, showAlert)
	case callbackClientCtx:
		return c.AnswerCallbackCtx(ctx, callbackID)
	case callbackClient:
		return c.AnswerCallback(callbackID)
	default:
		return fmt.Errorf("client does not support answer callback")
	}
}

func (o *Ops) answerCallbackAck(ctx context.Context, callbackID string) error {
	switch c := any(o.c).(type) {
	case callbackClientCtx:
		return c.AnswerCallbackCtx(ctx, callbackID)
	case callbackClient:
		return c.AnswerCallback(callbackID)
	case legacyCallbackClientCtx:
		return c.AnswerCallbackQueryCtx(ctx, callbackID, "", false)
	case legacyCallbackClient:
		return c.AnswerCallbackQuery(callbackID, "", false)
	default:
		return fmt.Errorf("client does not support answer callback")
	}
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
