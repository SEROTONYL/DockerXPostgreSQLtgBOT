package telegram

import (
	"context"
	"fmt"
	"strings"

	botapi "github.com/mymmrac/telego"
	"github.com/sirupsen/logrus"
)

// Client инкапсулирует минимум операций Telegram API, которые используются проектом.
type Client interface {
	SendMessage(chatID int64, text string, markup *botapi.InlineKeyboardMarkup) (messageID int, err error)
	EditMessage(chatID int64, messageID int, text string, markup *botapi.InlineKeyboardMarkup) error
	EditReplyMarkup(chatID int64, messageID int, markup *botapi.InlineKeyboardMarkup) error
	DeleteMessage(chatID int64, messageID int) error
	GetChatMember(chatID int64, userID int64) (member botapi.ChatMember, err error)
}

type parseModeSender interface {
	SendMessageWithParseMode(chatID int64, text string, markup *botapi.InlineKeyboardMarkup, parseMode *string) (messageID int, err error)
}

type optionSender interface {
	SendMessageWithOptions(opts SendOptions) (messageID int, err error)
}

type parseModeEditor interface {
	EditMessageWithParseMode(chatID int64, messageID int, text string, markup *botapi.InlineKeyboardMarkup, parseMode *string) error
}

type optionEditor interface {
	EditMessageWithOptions(opts EditOptions) error
}

var ParseModeHTML = stringPtr("HTML")

type SendOptions struct {
	ChatID                int64
	Text                  string
	ReplyMarkup           *botapi.InlineKeyboardMarkup
	ParseMode             *string
	ReplyToMessageID      int
	DisableWebPagePreview bool
}

type EditOptions struct {
	ChatID                int64
	MessageID             int
	Text                  string
	ReplyMarkup           *botapi.InlineKeyboardMarkup
	ParseMode             *string
	DisableWebPagePreview bool
}

func stringPtr(v string) *string { return &v }

type updateHandler struct {
	match   func(*botapi.Update) bool
	handler func(context.Context, *botapi.Update)
}

type botClient struct {
	bot      *botapi.Bot
	handlers []updateHandler
}

type updateRuntime interface {
	RegisterUpdateHandler(match func(*botapi.Update) bool, handler func(context.Context, *botapi.Update))
	Start(ctx context.Context)
	GetMe(ctx context.Context) (*botapi.User, error)
}

func NewBotClient(bot *botapi.Bot) Client {
	if bot == nil {
		panic("telegram.NewBotClient: nil bot")
	}
	return &botClient{bot: bot}
}

func (a *botClient) SendMessage(chatID int64, text string, markup *botapi.InlineKeyboardMarkup) (int, error) {
	msg, err := a.bot.SendMessage(context.Background(), buildSendMessageParams(SendOptions{ChatID: chatID, Text: text, ReplyMarkup: markup}))
	if err != nil {
		return 0, err
	}
	if msg == nil {
		return 0, nil
	}
	return msg.MessageID, nil
}

func (a *botClient) SendMessageWithParseMode(chatID int64, text string, markup *botapi.InlineKeyboardMarkup, parseMode *string) (int, error) {
	msg, err := a.bot.SendMessage(context.Background(), buildSendMessageParams(SendOptions{ChatID: chatID, Text: text, ReplyMarkup: markup, ParseMode: parseMode}))
	if err != nil {
		return 0, err
	}
	if msg == nil {
		return 0, nil
	}
	return msg.MessageID, nil
}

func (a *botClient) SendMessageWithOptions(opts SendOptions) (int, error) {
	msg, err := a.bot.SendMessage(context.Background(), buildSendMessageParams(opts))
	if err != nil {
		return 0, err
	}
	if msg == nil {
		return 0, nil
	}
	return msg.MessageID, nil
}

func (a *botClient) EditMessage(chatID int64, messageID int, text string, markup *botapi.InlineKeyboardMarkup) error {
	_, err := a.bot.EditMessageText(context.Background(), buildEditMessageTextParams(EditOptions{ChatID: chatID, MessageID: messageID, Text: text, ReplyMarkup: markup}))
	return err
}

func (a *botClient) EditMessageWithParseMode(chatID int64, messageID int, text string, markup *botapi.InlineKeyboardMarkup, parseMode *string) error {
	_, err := a.bot.EditMessageText(context.Background(), buildEditMessageTextParams(EditOptions{ChatID: chatID, MessageID: messageID, Text: text, ReplyMarkup: markup, ParseMode: parseMode}))
	return err
}

func (a *botClient) EditMessageWithOptions(opts EditOptions) error {
	_, err := a.bot.EditMessageText(context.Background(), buildEditMessageTextParams(opts))
	return err
}

func (a *botClient) EditReplyMarkup(chatID int64, messageID int, markup *botapi.InlineKeyboardMarkup) error {
	_, err := a.bot.EditMessageReplyMarkup(context.Background(), &botapi.EditMessageReplyMarkupParams{
		ChatID:      botapi.ChatID{ID: chatID},
		MessageID:   messageID,
		ReplyMarkup: markup,
	})
	return err
}

func (a *botClient) DeleteMessage(chatID int64, messageID int) error {
	return a.bot.DeleteMessage(context.Background(), &botapi.DeleteMessageParams{ChatID: botapi.ChatID{ID: chatID}, MessageID: messageID})
}

func (a *botClient) GetChatMember(chatID int64, userID int64) (botapi.ChatMember, error) {
	cm, err := a.bot.GetChatMember(context.Background(), &botapi.GetChatMemberParams{ChatID: botapi.ChatID{ID: chatID}, UserID: userID})
	if err != nil {
		return nil, err
	}
	return cm, nil
}

func (a *botClient) RegisterUpdateHandler(match func(*botapi.Update) bool, handler func(context.Context, *botapi.Update)) {
	a.handlers = append(a.handlers, updateHandler{match: match, handler: handler})
}

func (a *botClient) Start(ctx context.Context) {
	updates, err := a.bot.UpdatesViaLongPolling(ctx, longPollingUpdatesParams())
	if err != nil {
		logrus.WithError(err).Error("failed to start telegram long polling")
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case update, ok := <-updates:
			if !ok {
				return
			}
			u := update
			for _, h := range a.handlers {
				if h.match == nil || h.match(&u) {
					h.handler(ctx, &u)
				}
			}
		}
	}
}

func longPollingUpdatesParams() *botapi.GetUpdatesParams {
	return &botapi.GetUpdatesParams{
		Timeout: 30,
		// Явно подписываемся на типы обновлений, которые реально используем:
		// - message/callback_query для основного message-driven потока;
		// - chat_member/my_chat_member для lifecycle-событий (требуют allowed_updates и прав администратора для чужих участников).
		AllowedUpdates: []string{"message", "callback_query", "chat_member", "my_chat_member"},
	}
}

func (a *botClient) GetMe(ctx context.Context) (*botapi.User, error) {
	return a.bot.GetMe(ctx)
}

func (a *botClient) AnswerCallback(callbackID string) error {
	return a.AnswerCallbackCtx(context.Background(), callbackID)
}

func (a *botClient) AnswerCallbackCtx(ctx context.Context, callbackID string) error {
	if callbackID == "" {
		return nil
	}
	return a.bot.AnswerCallbackQuery(ctx, &botapi.AnswerCallbackQueryParams{CallbackQueryID: callbackID})
}

// AnswerCallbackQuery оставлен для обратной совместимости с существующими вызовами.
func (a *botClient) AnswerCallbackQuery(callbackID string, text string, showAlert bool) error {
	return a.AnswerCallbackQueryCtx(context.Background(), callbackID, text, showAlert)
}

func (a *botClient) AnswerCallbackQueryCtx(ctx context.Context, callbackID string, text string, showAlert bool) error {
	if callbackID == "" {
		return nil
	}
	return a.bot.AnswerCallbackQuery(ctx, &botapi.AnswerCallbackQueryParams{
		CallbackQueryID: callbackID,
		Text:            text,
		ShowAlert:       showAlert,
	})
}

func buildSendMessageParams(opts SendOptions) *botapi.SendMessageParams {
	params := &botapi.SendMessageParams{ChatID: botapi.ChatID{ID: opts.ChatID}, Text: opts.Text}
	if opts.ReplyMarkup != nil {
		params.ReplyMarkup = opts.ReplyMarkup
	}
	if opts.ParseMode != nil {
		params.ParseMode = *opts.ParseMode
	}
	if opts.ReplyToMessageID > 0 {
		params.ReplyParameters = &botapi.ReplyParameters{MessageID: opts.ReplyToMessageID}
	}
	if opts.DisableWebPagePreview {
		params.LinkPreviewOptions = &botapi.LinkPreviewOptions{IsDisabled: true}
	}
	return params
}

func buildEditMessageTextParams(opts EditOptions) *botapi.EditMessageTextParams {
	params := &botapi.EditMessageTextParams{ChatID: botapi.ChatID{ID: opts.ChatID}, MessageID: opts.MessageID, Text: opts.Text}
	if opts.ReplyMarkup != nil {
		params.ReplyMarkup = opts.ReplyMarkup
	}
	if opts.ParseMode != nil {
		params.ParseMode = *opts.ParseMode
	}
	if opts.DisableWebPagePreview {
		params.LinkPreviewOptions = &botapi.LinkPreviewOptions{IsDisabled: true}
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

func NewOps(c Client) *Ops               { return NewOpsWithLogger(c, logrus.NewEntry(logrus.StandardLogger())) }
func NewOpsFromBot(bot *botapi.Bot) *Ops { return NewOps(NewBotClient(bot)) }

func NewOpsWithLogger(c Client, l *logrus.Entry) *Ops {
	if l == nil {
		l = logrus.NewEntry(logrus.StandardLogger())
	}
	if c == nil {
		panic("telegram.NewOpsWithLogger: nil client")
	}
	return &Ops{c: c, log: l.WithField("component", "telegram.ops")}
}

func (o *Ops) Send(ctx context.Context, chatID int64, text string, markup *botapi.InlineKeyboardMarkup) (int, error) {
	return o.SendWithOptions(ctx, SendOptions{ChatID: chatID, Text: text, ReplyMarkup: markup})
}

func (o *Ops) SendWithParseMode(ctx context.Context, chatID int64, text string, markup *botapi.InlineKeyboardMarkup, parseMode *string) (int, error) {
	return o.SendWithOptions(ctx, SendOptions{ChatID: chatID, Text: text, ReplyMarkup: markup, ParseMode: parseMode})
}

func (o *Ops) SendWithOptions(ctx context.Context, opts SendOptions) (int, error) {
	msgID, err := o.sendWithOptions(opts)
	if err != nil {
		o.log.WithContext(ctx).WithError(err).WithField("chat_id", opts.ChatID).Warn("telegram send failed")
		return 0, err
	}
	return msgID, nil
}
func (o *Ops) SendText(ctx context.Context, chatID int64, text string, markup *botapi.InlineKeyboardMarkup) (int, error) {
	return o.Send(ctx, chatID, text, markup)
}
func (o *Ops) Edit(ctx context.Context, chatID int64, messageID int, text string, markup *botapi.InlineKeyboardMarkup) error {
	return o.EditWithParseMode(ctx, chatID, messageID, text, markup, nil)
}

func (o *Ops) EditWithParseMode(ctx context.Context, chatID int64, messageID int, text string, markup *botapi.InlineKeyboardMarkup, parseMode *string) error {
	return o.EditWithOptions(ctx, EditOptions{ChatID: chatID, MessageID: messageID, Text: text, ReplyMarkup: markup, ParseMode: parseMode})
}

func (o *Ops) EditWithOptions(ctx context.Context, opts EditOptions) error {
	err := o.editWithOptions(opts)
	if err == nil {
		return nil
	}
	kind := classifyEditError(err)
	entry := o.log.WithContext(ctx).WithError(err).WithFields(logrus.Fields{"chat_id": opts.ChatID, "message_id": opts.MessageID, "kind": kind})
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
func (o *Ops) EditText(ctx context.Context, chatID int64, messageID int, text string, markup *botapi.InlineKeyboardMarkup) error {
	return o.Edit(ctx, chatID, messageID, text, markup)
}
func (o *Ops) EditReplyMarkup(ctx context.Context, chatID int64, messageID int, markup *botapi.InlineKeyboardMarkup) error {
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
func (o *Ops) GetChatMember(ctx context.Context, chatID int64, userID int64) (botapi.ChatMember, error) {
	member, err := o.c.GetChatMember(chatID, userID)
	if err != nil {
		o.log.WithContext(ctx).WithError(err).WithFields(logrus.Fields{"chat_id": chatID, "user_id": userID}).Warn("telegram get chat member failed")
		return nil, err
	}
	return member, nil
}

func (o *Ops) ExtractMemberTag(member botapi.ChatMember) *string {
	switch m := member.(type) {
	case *botapi.ChatMemberMember:
		return normalizedTag(m.Tag)
	case *botapi.ChatMemberRestricted:
		return normalizedTag(m.Tag)
	default:
		return nil
	}
}

func normalizedTag(tag string) *string {
	t := strings.TrimSpace(tag)
	if len(t) == 0 {
		return nil
	}
	return &t
}

func (o *Ops) RegisterUpdateHandler(match func(*botapi.Update) bool, handler func(context.Context, *botapi.Update)) error {
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
func (o *Ops) GetMe(ctx context.Context) (*botapi.User, error) {
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
	text, showAlert := "", false
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

func (o *Ops) sendWithOptions(opts SendOptions) (int, error) {
	if c, ok := any(o.c).(optionSender); ok {
		return c.SendMessageWithOptions(opts)
	}
	if c, ok := any(o.c).(parseModeSender); ok {
		return c.SendMessageWithParseMode(opts.ChatID, opts.Text, opts.ReplyMarkup, opts.ParseMode)
	}
	return o.c.SendMessage(opts.ChatID, opts.Text, opts.ReplyMarkup)
}

func (o *Ops) editWithOptions(opts EditOptions) error {
	if c, ok := any(o.c).(optionEditor); ok {
		return c.EditMessageWithOptions(opts)
	}
	if c, ok := any(o.c).(parseModeEditor); ok {
		return c.EditMessageWithParseMode(opts.ChatID, opts.MessageID, opts.Text, opts.ReplyMarkup, opts.ParseMode)
	}
	return o.c.EditMessage(opts.ChatID, opts.MessageID, opts.Text, opts.ReplyMarkup)
}

func (o *Ops) EditOrSend(ctx context.Context, chatID int64, messageID int, text string, keyboard botapi.InlineKeyboardMarkup) (int, bool, error) {
	return RenderScreen(ctx, o, Screen{ChatID: chatID, MessageID: messageID, Text: text, ReplyMarkup: keyboard})
}
