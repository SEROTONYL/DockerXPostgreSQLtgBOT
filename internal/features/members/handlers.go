// Package members — handlers.go обрабатывает Telegram-события, связанные с участниками.
package members

import (
	"context"
	"fmt"
	"html"
	"sort"
	"strconv"
	"strings"

	models "github.com/mymmrac/telego"
	log "github.com/sirupsen/logrus"

	"serotonyl.ru/telegram-bot/internal/common"
	"serotonyl.ru/telegram-bot/internal/config"
	"serotonyl.ru/telegram-bot/internal/telegram"
)

const (
	membersListCallbackPrefix = "members:list:"
	membersListPageSize       = 8
)

type balanceProvider interface {
	GetBalance(ctx context.Context, userID int64) (int64, error)
}

// Handler обрабатывает события участников.
type Handler struct {
	service *Service
	economy balanceProvider
	tgOps   *telegram.Ops
	cfg     *config.Config
}

// NewHandler создаёт новый обработчик событий участников.
func NewHandler(service *Service, economy balanceProvider, tgOps *telegram.Ops, cfg *config.Config) *Handler {
	return &Handler{service: service, economy: economy, tgOps: tgOps, cfg: cfg}
}

// HandleNewChatMembers обрабатывает событие вступления новых пользователей.
func (h *Handler) HandleNewChatMembers(ctx context.Context, newMembers []models.User) {
	for _, user := range newMembers {
		err := h.service.HandleNewMember(ctx, user.ID, user.Username, user.FirstName, user.LastName, user.IsBot)
		if err != nil {
			log.WithError(err).WithField("user_id", user.ID).Error("Ошибка регистрации нового участника")
		}
	}
}

func (h *Handler) HandleMembersList(ctx context.Context, chatID int64, ownerUserID int64) {
	if !h.canRenderMembersList(chatID) {
		return
	}
	if err := h.renderMembersPage(ctx, chatID, 0, ownerUserID, 0); err != nil {
		log.WithError(err).Warn("members list render failed")
	}
}

func (h *Handler) HandleMembersCallback(ctx context.Context, cb *models.CallbackQuery) bool {
	if cb == nil || !strings.HasPrefix(cb.Data, membersListCallbackPrefix) {
		return false
	}
	if !h.canRenderMembersList(callbackChatID(cb)) {
		h.answerCallback(ctx, cb.ID, "")
		return true
	}
	msg := callbackMessage(cb)
	if msg == nil {
		h.answerCallback(ctx, cb.ID, "")
		return true
	}

	ownerUserID, page, ok := parseMembersListCallback(cb.Data)
	if !ok {
		h.answerCallback(ctx, cb.ID, "")
		return true
	}
	if cb.From.ID != ownerUserID {
		h.answerCallback(ctx, cb.ID, "Листать список может только тот, кто его открыл")
		return true
	}
	if err := h.renderMembersPage(ctx, msg.Chat.ID, msg.MessageID, ownerUserID, page); err != nil {
		log.WithError(err).Warn("members list callback render failed")
	}
	h.answerCallback(ctx, cb.ID, "")
	return true
}

func (h *Handler) canRenderMembersList(chatID int64) bool {
	if h == nil || h.cfg == nil || h.service == nil || h.economy == nil || h.tgOps == nil {
		return false
	}
	return h.cfg.MemberSourceChatID != 0 && chatID == h.cfg.MemberSourceChatID
}

func (h *Handler) renderMembersPage(ctx context.Context, chatID int64, messageID int, ownerUserID int64, page int) error {
	withRole, err := h.service.GetUsersWithRole(ctx)
	if err != nil {
		return err
	}
	withoutRole, err := h.service.GetUsersWithoutRole(ctx)
	if err != nil {
		return err
	}
	all := make([]*Member, 0, len(withRole)+len(withoutRole))
	all = append(all, withRole...)
	all = append(all, withoutRole...)

	type rankedMember struct {
		member  *Member
		balance int64
	}
	ranked := make([]rankedMember, 0, len(all))
	for _, m := range all {
		balance, balErr := h.economy.GetBalance(ctx, m.UserID)
		if balErr != nil {
			return balErr
		}
		ranked = append(ranked, rankedMember{member: m, balance: balance})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].balance == ranked[j].balance {
			return ranked[i].member.UserID < ranked[j].member.UserID
		}
		return ranked[i].balance > ranked[j].balance
	})

	totalPages := maxPageCount(len(ranked), membersListPageSize)
	if page < 0 {
		page = 0
	}
	if page >= totalPages {
		page = totalPages - 1
	}
	if page < 0 {
		page = 0
	}

	start := page * membersListPageSize
	end := start + membersListPageSize
	if end > len(ranked) {
		end = len(ranked)
	}
	pageMembers := ranked[start:end]

	rows := make([]string, 0, len(pageMembers))
	for _, rm := range pageMembers {
		rows = append(rows, fmt.Sprintf("%s - %s", roleAnchor(rm.member), common.FormatBalance(rm.balance)))
	}

	text := "🏆 Топ участников пуст"
	if len(rows) > 0 {
		text = "🏆 Топ участников\n\n" + strings.Join(rows, "\n")
	}

	keyboard := membersListKeyboard(ownerUserID, page, totalPages)
	_, _, err = telegram.RenderScreen(ctx, h.tgOps, telegram.Screen{
		ChatID:                chatID,
		MessageID:             messageID,
		Text:                  text,
		ReplyMarkup:           keyboard,
		ParseMode:             telegram.ParseModeHTML,
		DisableWebPagePreview: true,
	})
	return err
}

func roleAnchor(m *Member) string {
	label := "Без роли"
	if m.Role != nil && strings.TrimSpace(*m.Role) != "" {
		label = strings.TrimSpace(*m.Role)
	}
	label = html.EscapeString(label)

	username := strings.TrimPrefix(strings.TrimSpace(m.Username), "@")
	if username != "" {
		return fmt.Sprintf("<a href=\"https://t.me/%s\">%s</a>", html.EscapeString(username), label)
	}
	return fmt.Sprintf("<a href=\"tg://openmessage?user_id=%d\">%s</a>", m.UserID, label)
}

func membersListKeyboard(ownerUserID int64, page int, totalPages int) models.InlineKeyboardMarkup {
	prevPage := page - 1
	if prevPage < 0 {
		prevPage = 0
	}
	nextPage := page + 1
	if nextPage >= totalPages {
		nextPage = totalPages - 1
	}
	return models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{
		{Text: "◀", CallbackData: membersListCallbackData(ownerUserID, prevPage)},
		{Text: fmt.Sprintf("Стр %d/%d", page+1, totalPages), CallbackData: membersListCallbackData(ownerUserID, page)},
		{Text: "▶", CallbackData: membersListCallbackData(ownerUserID, nextPage)},
	}}}
}

func membersListCallbackData(ownerUserID int64, page int) string {
	return fmt.Sprintf("%s%d:%d", membersListCallbackPrefix, ownerUserID, page)
}

func parseMembersListCallback(data string) (ownerUserID int64, page int, ok bool) {
	payload := strings.TrimPrefix(data, membersListCallbackPrefix)
	parts := strings.Split(payload, ":")
	if len(parts) != 2 {
		return 0, 0, false
	}
	uid, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, 0, false
	}
	p, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, false
	}
	return uid, p, true
}

func maxPageCount(total int, pageSize int) int {
	if total <= 0 {
		return 1
	}
	pages := total / pageSize
	if total%pageSize != 0 {
		pages++
	}
	if pages < 1 {
		return 1
	}
	return pages
}

func callbackMessage(q *models.CallbackQuery) *models.Message {
	if q == nil || q.Message == nil {
		return nil
	}
	return q.Message.Message()
}

func callbackChatID(q *models.CallbackQuery) int64 {
	msg := callbackMessage(q)
	if msg == nil {
		return 0
	}
	return msg.Chat.ID
}

func (h *Handler) answerCallback(ctx context.Context, callbackID, text string) {
	if h == nil || h.tgOps == nil || callbackID == "" {
		return
	}
	if err := h.tgOps.AnswerCallback(ctx, callbackID, text, false); err != nil {
		log.WithError(err).Debug("ошибка ответа на callback")
	}
}
