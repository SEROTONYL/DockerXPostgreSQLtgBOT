package economy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	models "github.com/mymmrac/telego"
	log "github.com/sirupsen/logrus"

	"serotonyl.ru/telegram-bot/internal/commands"
	"serotonyl.ru/telegram-bot/internal/common"
	"serotonyl.ru/telegram-bot/internal/features/members"
	"serotonyl.ru/telegram-bot/internal/telegram"
)

const (
	transferCallbackPrefix   = "economy:transfer:"
	transferConfirmYes       = "yes"
	transferConfirmNo        = "no"
	transferStateAwaitFirst  = "await_first"
	transferStateAwaitSecond = "await_second"
	transferStateExecuting   = "executing"
	transferStateCompleted   = "completed"
	transferStateCanceled    = "canceled"
	transferStateExpired     = "expired"
	transferStateFailed      = "failed"
	transferTTL              = 15 * time.Minute
	confirmEmojiID           = "5210952531676504517"
	cancelEmojiID            = "5206607081334906820"
	buttonYesText            = "Да"
	buttonNoText             = "Нет"
)

type handlerService interface {
	GetBalance(ctx context.Context, userID int64) (int64, error)
	Transfer(ctx context.Context, fromUserID, toUserID, amount int64) error
	GetTransactionHistory(ctx context.Context, userID int64) (string, error)
}

type memberLookup interface {
	GetByUsername(ctx context.Context, username string) (*members.Member, error)
}

type transferConfirmation struct {
	Token            string
	ChatID           int64
	MessageID        int
	OwnerUserID      int64
	FromUserID       int64
	ToUserID         int64
	Amount           int64
	RecipientDisplay string
	State            string
	ExpiresAt        time.Time
}

type transferTarget struct {
	UserID  int64
	Display string
}

type transferRequest struct {
	Amount           int64
	ExplicitUsername string
	Target           transferTarget
}

type Handler struct {
	service       handlerService
	memberService memberLookup
	tgOps         *telegram.Ops

	mu        sync.Mutex
	confirmBy map[string]*transferConfirmation
	now       func() time.Time
}

func NewHandler(service *Service, memberService memberLookup, tgOps *telegram.Ops) *Handler {
	return &Handler{
		service:       service,
		memberService: memberService,
		tgOps:         tgOps,
		confirmBy:     make(map[string]*transferConfirmation),
		now:           func() time.Time { return time.Now().UTC() },
	}
}

func (h *Handler) HandleBalance(ctx context.Context, chatID int64, userID int64, replyToMessageID int) {
	balance, err := h.service.GetBalance(ctx, userID)
	if err != nil {
		log.WithError(err).Error("ошибка получения баланса")
		h.sendMessage(ctx, chatID, "❌ Ошибка получения баланса", replyToMessageID)
		return
	}

	h.sendMessage(ctx, chatID, fmt.Sprintf("У вас: %d🎞️", balance), replyToMessageID)
}

func (h *Handler) HandleTransferCommand(ctx context.Context, c commands.Context, args []string) {
	if c.Message == nil {
		h.sendMessage(ctx, c.ChatID, "❌ Не удалось прочитать сообщение для перевода", c.MessageID)
		return
	}
	req, err := h.parseTransferCommand(ctx, c.Message, args)
	if err != nil {
		h.sendMessage(ctx, c.ChatID, userFacingTransferError(err), c.MessageID)
		return
	}
	h.startTransferConfirmation(ctx, c.ChatID, c.UserID, c.MessageID, req)
}

func (h *Handler) HandleEconomyMessage(ctx context.Context, message *models.Message) bool {
	if message == nil || strings.TrimSpace(message.Text) == "" || message.From == nil {
		return false
	}

	req, handled, err := h.parseTransferPhrase(ctx, message)
	if !handled {
		return false
	}
	if err != nil {
		h.sendMessage(ctx, message.Chat.ID, userFacingTransferError(err), message.MessageID)
		return true
	}
	h.startTransferConfirmation(ctx, message.Chat.ID, message.From.ID, message.MessageID, req)
	return true
}

func (h *Handler) HandleEconomyCallback(ctx context.Context, q *models.CallbackQuery) bool {
	if q == nil || !strings.HasPrefix(q.Data, transferCallbackPrefix) {
		return false
	}

	token, action, ok := parseTransferCallbackData(q.Data)
	if !ok {
		h.answerCallback(ctx, q.ID, "")
		return true
	}

	msg := callbackMessage(q)
	if msg == nil {
		h.answerCallback(ctx, q.ID, "")
		return true
	}

	now := h.now()
	entry, state, allowed, alertText := h.prepareTransferCallback(token, q.From.ID, msg.Chat.ID, msg.MessageID, now)
	if alertText != "" {
		h.answerCallback(ctx, q.ID, alertText)
	} else {
		h.answerCallback(ctx, q.ID, "")
	}
	if !allowed {
		if state == transferStateExpired && entry != nil {
			h.finishTransferMessage(ctx, entry, "Перевод устарел. Начните заново.", transferStateExpired)
		}
		return true
	}

	switch action {
	case transferConfirmNo:
		h.finishTransferMessage(ctx, entry, "Передача плёнок отменена.", transferStateCanceled)
		return true
	case transferConfirmYes:
		return h.handleTransferConfirmYes(ctx, entry)
	default:
		return true
	}
}

func (h *Handler) HandleTransactions(ctx context.Context, chatID int64, userID int64) {
	history, err := h.service.GetTransactionHistory(ctx, userID)
	if err != nil {
		log.WithError(err).Error("ошибка получения транзакций")
		h.sendMessage(ctx, chatID, "❌ Ошибка получения истории транзакций", 0)
		return
	}

	h.sendMessage(ctx, chatID, history, 0)
}

func (h *Handler) parseTransferCommand(ctx context.Context, message *models.Message, args []string) (*transferRequest, error) {
	if len(args) == 0 {
		return nil, errTransferAmountMissing
	}

	var (
		username string
		amount   int64
		amountOK bool
	)
	for _, arg := range args {
		token := strings.TrimSpace(arg)
		if token == "" {
			continue
		}
		if isUsernameToken(token) {
			username = normalizeUsernameToken(token)
			continue
		}
		if amountOK {
			return nil, errTransferMalformed
		}
		parsed, err := parsePositiveInteger(token)
		if err != nil {
			return nil, err
		}
		amount = parsed
		amountOK = true
	}
	if !amountOK {
		return nil, errTransferAmountMissing
	}
	return h.buildTransferRequest(ctx, message, amount, username)
}

func (h *Handler) parseTransferPhrase(ctx context.Context, message *models.Message) (*transferRequest, bool, error) {
	parts := strings.Fields(strings.TrimSpace(message.Text))
	if len(parts) == 0 {
		return nil, false, nil
	}

	head0 := normalizeWord(parts[0])
	head1 := ""
	if len(parts) > 1 {
		head1 = normalizeWord(parts[1])
	}
	if head0 != "передать" || head1 != "пленки" {
		return nil, false, nil
	}
	if len(parts) < 3 {
		return nil, true, errTransferAmountMissing
	}

	amount, err := parsePositiveInteger(parts[2])
	if err != nil {
		return nil, true, err
	}
	if len(parts) < 4 {
		return nil, true, errTransferRecipientMissing
	}

	username := normalizeUsernameToken(parts[3])
	if username == "" {
		return nil, true, errTransferMalformed
	}
	if len(parts) > 4 {
		return nil, true, errTransferMalformed
	}
	req, buildErr := h.buildTransferRequest(ctx, message, amount, username)
	return req, true, buildErr
}

func (h *Handler) buildTransferRequest(ctx context.Context, message *models.Message, amount int64, explicitUsername string) (*transferRequest, error) {
	if message == nil || message.From == nil {
		return nil, errTransferMalformed
	}
	if amount <= 0 {
		return nil, common.ErrInvalidAmount
	}

	target, err := h.resolveTransferTarget(ctx, message, explicitUsername)
	if err != nil {
		return nil, err
	}
	if message.From.ID == target.UserID {
		return nil, common.ErrSelfTransfer
	}

	balance, err := h.service.GetBalance(ctx, message.From.ID)
	if err != nil {
		return nil, fmt.Errorf("get balance: %w", err)
	}
	if balance < amount {
		return nil, common.ErrInsufficientBalance
	}

	return &transferRequest{
		Amount:           amount,
		ExplicitUsername: explicitUsername,
		Target:           target,
	}, nil
}

func (h *Handler) resolveTransferTarget(ctx context.Context, message *models.Message, explicitUsername string) (transferTarget, error) {
	if explicitUsername != "" {
		if h.memberService == nil {
			return transferTarget{}, common.ErrUserNotFound
		}
		member, err := h.memberService.GetByUsername(ctx, explicitUsername)
		if err != nil || member == nil {
			return transferTarget{}, common.ErrUserNotFound
		}
		return transferTarget{UserID: member.UserID, Display: "@" + strings.TrimPrefix(member.Username, "@")}, nil
	}

	if message.ReplyToMessage != nil && message.ReplyToMessage.From != nil {
		user := message.ReplyToMessage.From
		return transferTarget{UserID: user.ID, Display: visibleUserName(*user)}, nil
	}
	return transferTarget{}, errTransferRecipientMissing
}

func (h *Handler) startTransferConfirmation(ctx context.Context, chatID, ownerUserID int64, replyToMessageID int, req *transferRequest) {
	if req == nil {
		h.sendMessage(ctx, chatID, "❌ Не удалось подготовить перевод", replyToMessageID)
		return
	}

	token, err := randomToken()
	if err != nil {
		log.WithError(err).Warn("transfer token generation failed")
		h.sendMessage(ctx, chatID, "❌ Не удалось создать подтверждение перевода", replyToMessageID)
		return
	}

	confirm := &transferConfirmation{
		Token:            token,
		ChatID:           chatID,
		OwnerUserID:      ownerUserID,
		FromUserID:       ownerUserID,
		ToUserID:         req.Target.UserID,
		Amount:           req.Amount,
		RecipientDisplay: req.Target.Display,
		State:            transferStateAwaitFirst,
		ExpiresAt:        h.now().Add(transferTTL),
	}

	messageID, err := h.tgOps.SendWithOptions(ctx, telegram.SendOptions{
		ChatID:           chatID,
		Text:             firstTransferConfirmationText(req.Amount, req.Target.Display),
		ReplyMarkup:      transferConfirmationMarkup(token),
		ReplyToMessageID: replyToMessageID,
	})
	if err != nil {
		log.WithError(err).Warn("transfer confirmation send failed")
		h.sendMessage(ctx, chatID, "❌ Не удалось отправить подтверждение перевода", replyToMessageID)
		return
	}

	confirm.MessageID = messageID
	h.storeTransferConfirmation(confirm)
}

func (h *Handler) handleTransferConfirmYes(ctx context.Context, entry *transferConfirmation) bool {
	switch entry.State {
	case transferStateAwaitFirst:
		h.setTransferState(entry.Token, transferStateAwaitSecond)
		if err := h.tgOps.Edit(ctx, entry.ChatID, entry.MessageID, secondTransferConfirmationText(entry.Amount, entry.RecipientDisplay), transferConfirmationMarkup(entry.Token)); err != nil && !telegram.IsEditNotModified(err) {
			log.WithError(err).Warn("transfer first confirm edit failed")
		}
		return true
	case transferStateAwaitSecond:
		if !h.tryStartTransferExecution(entry.Token) {
			return true
		}
		err := h.service.Transfer(ctx, entry.FromUserID, entry.ToUserID, entry.Amount)
		if err != nil {
			finalText := userFacingTransferExecutionError(err)
			h.finishTransferMessage(ctx, entry, finalText, transferStateFailed)
			return true
		}
		h.finishTransferMessage(ctx, entry, successTransferText(entry.Amount, entry.RecipientDisplay), transferStateCompleted)
		return true
	default:
		return true
	}
}

func (h *Handler) prepareTransferCallback(token string, actorUserID, chatID int64, messageID int, now time.Time) (*transferConfirmation, string, bool, string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.cleanupExpiredLocked(now)
	entry := h.confirmBy[token]
	if entry == nil {
		return nil, transferStateExpired, false, "Подтверждение устарело."
	}
	if entry.ChatID != chatID || entry.MessageID != messageID {
		return entry, transferStateExpired, false, "Подтверждение устарело."
	}
	if actorUserID != entry.OwnerUserID {
		return entry, entry.State, false, "Подтверждать или отменять перевод может только отправитель."
	}
	if now.After(entry.ExpiresAt) {
		entry.State = transferStateExpired
		return entry, transferStateExpired, false, "Подтверждение устарело."
	}
	switch entry.State {
	case transferStateCompleted:
		return entry, entry.State, false, "Перевод уже выполнен."
	case transferStateCanceled:
		return entry, entry.State, false, "Перевод уже отменён."
	case transferStateExpired:
		return entry, entry.State, false, "Подтверждение устарело."
	case transferStateFailed:
		return entry, entry.State, false, "Операция уже завершена."
	case transferStateExecuting:
		return entry, entry.State, false, "Операция уже обрабатывается."
	default:
		return entry, entry.State, true, ""
	}
}

func (h *Handler) tryStartTransferExecution(token string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	entry := h.confirmBy[token]
	if entry == nil || entry.State != transferStateAwaitSecond {
		return false
	}
	entry.State = transferStateExecuting
	return true
}

func (h *Handler) setTransferState(token, state string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if entry := h.confirmBy[token]; entry != nil {
		entry.State = state
	}
}

func (h *Handler) finishTransferMessage(ctx context.Context, entry *transferConfirmation, text, finalState string) {
	if entry == nil {
		return
	}
	h.setTransferState(entry.Token, finalState)
	if err := h.tgOps.Edit(ctx, entry.ChatID, entry.MessageID, text, nil); err != nil && !telegram.IsEditNotModified(err) {
		log.WithError(err).Warn("transfer final edit failed")
	}
}

func (h *Handler) storeTransferConfirmation(entry *transferConfirmation) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cleanupExpiredLocked(h.now())
	h.confirmBy[entry.Token] = entry
}

func (h *Handler) cleanupExpiredLocked(now time.Time) {
	for token, entry := range h.confirmBy {
		if entry == nil {
			delete(h.confirmBy, token)
			continue
		}
		if now.After(entry.ExpiresAt.Add(5 * time.Minute)) {
			delete(h.confirmBy, token)
		}
	}
}

func parseTransferCallbackData(data string) (token, action string, ok bool) {
	payload := strings.TrimPrefix(data, transferCallbackPrefix)
	parts := strings.Split(payload, ":")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func transferConfirmationMarkup(token string) *models.InlineKeyboardMarkup {
	return &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{
					Text:              buttonYesText,
					IconCustomEmojiID: confirmEmojiID,
					CallbackData:      transferCallbackPrefix + token + ":" + transferConfirmYes,
				},
				{
					Text:              buttonNoText,
					IconCustomEmojiID: cancelEmojiID,
					CallbackData:      transferCallbackPrefix + token + ":" + transferConfirmNo,
				},
			},
		},
	}
}

func firstTransferConfirmationText(amount int64, recipient string) string {
	return fmt.Sprintf("Вы уверены, что хотите передать %d  пользователю %s?", amount, recipient)
}

func secondTransferConfirmationText(amount int64, recipient string) string {
	return fmt.Sprintf("Вы точно уверены, что хотите передать %d  пользователю %s?", amount, recipient)
}

func successTransferText(amount int64, recipient string) string {
	return fmt.Sprintf("✅ Передано %d  пользователю %s.", amount, recipient)
}

func userFacingTransferError(err error) string {
	switch {
	case errors.Is(err, errTransferRecipientMissing):
		return "❌ Не указан получатель. Укажите @username или ответьте на сообщение пользователя."
	case errors.Is(err, errTransferAmountMissing):
		return "❌ Не указана сумма перевода."
	case errors.Is(err, errTransferMalformed):
		return "❌ Некорректная команда перевода. Используйте `!отсыпать <сумма>` в ответе или `передать плёнки <сумма> @username`."
	case errors.Is(err, common.ErrUserNotFound):
		return "❌ Не удалось найти пользователя по указанному username."
	case errors.Is(err, common.ErrSelfTransfer):
		return "❌ Нельзя переводить плёнки самому себе."
	case errors.Is(err, common.ErrInsufficientBalance):
		return "❌ Недостаточно плёнок для перевода."
	case errors.Is(err, common.ErrInvalidAmount):
		return "❌ Сумма должна быть положительным целым числом больше нуля."
	default:
		return "❌ Не удалось подготовить перевод."
	}
}

func userFacingTransferExecutionError(err error) string {
	switch {
	case errors.Is(err, common.ErrSelfTransfer):
		return "❌ Нельзя переводить плёнки самому себе."
	case errors.Is(err, common.ErrInsufficientBalance):
		return "❌ Недостаточно плёнок для перевода."
	case errors.Is(err, common.ErrInvalidAmount):
		return "❌ Сумма должна быть положительным целым числом больше нуля."
	default:
		return "❌ Не удалось выполнить перевод."
	}
}

var (
	errTransferRecipientMissing = errors.New("transfer recipient missing")
	errTransferAmountMissing    = errors.New("transfer amount missing")
	errTransferMalformed        = errors.New("transfer malformed")
)

func parsePositiveInteger(raw string) (int64, error) {
	token := strings.TrimSpace(raw)
	if token == "" {
		return 0, errTransferAmountMissing
	}
	value, err := strconv.ParseInt(token, 10, 64)
	if err != nil || value <= 0 {
		return 0, common.ErrInvalidAmount
	}
	return value, nil
}

func normalizeWord(s string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(s)), "ё", "е")
}

func isUsernameToken(s string) bool {
	return strings.HasPrefix(strings.TrimSpace(s), "@")
}

func normalizeUsernameToken(s string) string {
	return strings.TrimPrefix(strings.TrimSpace(s), "@")
}

func visibleUserName(user models.User) string {
	username := strings.TrimPrefix(strings.TrimSpace(user.Username), "@")
	if username != "" {
		return "@" + username
	}
	name := strings.TrimSpace(strings.Join([]string{strings.TrimSpace(user.FirstName), strings.TrimSpace(user.LastName)}, " "))
	if name != "" {
		return name
	}
	return fmt.Sprintf("id:%d", user.ID)
}

func randomToken() (string, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}

func callbackMessage(q *models.CallbackQuery) *models.Message {
	if q == nil || q.Message == nil {
		return nil
	}
	return q.Message.Message()
}

func (h *Handler) answerCallback(ctx context.Context, callbackID, text string) {
	if h == nil || h.tgOps == nil || callbackID == "" {
		return
	}
	if err := h.tgOps.AnswerCallback(ctx, callbackID, text, false); err != nil {
		log.WithError(err).Debug("ошибка ответа на callback перевода")
	}
}

func (h *Handler) sendMessage(ctx context.Context, chatID int64, text string, replyToMessageID int) {
	_, _ = h.tgOps.SendWithOptions(ctx, telegram.SendOptions{
		ChatID:           chatID,
		Text:             text,
		ReplyToMessageID: replyToMessageID,
	})
}
