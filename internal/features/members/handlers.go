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

type RankedMember struct {
	Member  *Member
	Balance int64
}

type Handler struct {
	service *Service
	economy balanceProvider
	tgOps   *telegram.Ops
	cfg     *config.Config
}

func NewHandler(service *Service, economy balanceProvider, tgOps *telegram.Ops, cfg *config.Config) *Handler {
	return &Handler{service: service, economy: economy, tgOps: tgOps, cfg: cfg}
}

func (h *Handler) HandleNewChatMembers(ctx context.Context, newMembers []models.User) {
	for _, user := range newMembers {
		err := h.service.HandleNewMember(ctx, user.ID, user.Username, user.FirstName, user.LastName, user.IsBot)
		if err != nil {
			log.WithError(err).WithField("user_id", user.ID).Error("member registration failed")
		}
	}
}

func (h *Handler) HandleMembersList(ctx context.Context, chatID int64, ownerUserID int64, limit int) {
	if !h.canRenderMembersList(chatID) {
		return
	}
	if err := h.renderMembersPage(ctx, chatID, 0, ownerUserID, 0, limit); err != nil {
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
		h.answerCallback(ctx, cb.ID, "Этот список может листать только тот, кто его открыл")
		return true
	}
	if err := h.renderMembersPage(ctx, msg.Chat.ID, msg.MessageID, ownerUserID, page, 0); err != nil {
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

func (h *Handler) renderMembersPage(ctx context.Context, chatID int64, messageID int, ownerUserID int64, page int, limit int) error {
	withRole, err := h.service.GetUsersWithRole(ctx)
	if err != nil {
		return err
	}
	withoutRole, err := h.service.GetUsersWithoutRole(ctx)
	if err != nil {
		return err
	}
	ranked, err := RankMembersByBalance(ctx, withRole, withoutRole, h.economy, limit)
	if err != nil {
		return err
	}

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
		rows = append(rows, fmt.Sprintf("%s - %s", roleAnchor(rm.Member), common.FormatBalance(rm.Balance)))
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

func RankMembersByBalance(ctx context.Context, withRole, withoutRole []*Member, economy balanceProvider, limit int) ([]RankedMember, error) {
	all := make([]*Member, 0, len(withRole)+len(withoutRole))
	all = append(all, withRole...)
	all = append(all, withoutRole...)

	ranked := make([]RankedMember, 0, len(all))
	for _, m := range all {
		balance, err := economy.GetBalance(ctx, m.UserID)
		if err != nil {
			return nil, err
		}
		ranked = append(ranked, RankedMember{Member: m, Balance: balance})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].Balance == ranked[j].Balance {
			return ranked[i].Member.UserID < ranked[j].Member.UserID
		}
		return ranked[i].Balance > ranked[j].Balance
	})
	if limit > 0 && limit < len(ranked) {
		ranked = ranked[:limit]
	}
	return ranked, nil
}

func DisplayLabel(m *Member) string {
	if m != nil && m.Role != nil {
		if role := strings.TrimSpace(*m.Role); role != "" {
			return role
		}
	}
	if m != nil && m.Tag != nil {
		if tag := strings.TrimSpace(*m.Tag); tag != "" {
			return tag
		}
	}
	if m != nil {
		username := strings.TrimSpace(strings.TrimPrefix(m.Username, "@"))
		if username != "" {
			return "@" + username
		}
		displayName := strings.TrimSpace(strings.Join([]string{
			strings.TrimSpace(m.FirstName),
			strings.TrimSpace(m.LastName),
		}, " "))
		if displayName != "" {
			return displayName
		}
		if m.LastKnownName != nil {
			if lastKnownName := strings.TrimSpace(*m.LastKnownName); lastKnownName != "" {
				return lastKnownName
			}
		}
		return fmt.Sprintf("id:%d", m.UserID)
	}
	return "id:0"
}

func roleAnchor(m *Member) string {
	return FormatParticipantHTML(m)
}

func FormatParticipantHTML(m *Member) string {
	label := html.EscapeString(participantLabel(m))
	if m == nil {
		return label
	}
	username := strings.TrimPrefix(strings.TrimSpace(m.Username), "@")
	if username != "" {
		return fmt.Sprintf("<a href=\"https://t.me/%s\">%s</a>", html.EscapeString(username), label)
	}
	return label
}

func participantLabel(m *Member) string {
	if m == nil {
		return "id:0"
	}
	displayName := strings.TrimSpace(strings.Join([]string{
		strings.TrimSpace(m.FirstName),
		strings.TrimSpace(m.LastName),
	}, " "))
	if displayName != "" {
		return displayName
	}
	if m.LastKnownName != nil {
		if lastKnownName := strings.TrimSpace(*m.LastKnownName); lastKnownName != "" {
			return lastKnownName
		}
	}
	username := strings.TrimSpace(strings.TrimPrefix(m.Username, "@"))
	if username != "" {
		return "@" + username
	}
	return fmt.Sprintf("id:%d", m.UserID)
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
		{Text: "⬅", CallbackData: membersListCallbackData(ownerUserID, prevPage)},
		{Text: fmt.Sprintf("Стр %d/%d", page+1, totalPages), CallbackData: membersListCallbackData(ownerUserID, page)},
		{Text: "➡", CallbackData: membersListCallbackData(ownerUserID, nextPage)},
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
		log.WithError(err).Debug("callback answer failed")
	}
}

func (h *Handler) sendMembersListValidationError(ctx context.Context, chatID int64, replyToMessageID int) {
	if h == nil || h.tgOps == nil {
		return
	}
	_, _ = h.tgOps.SendWithOptions(ctx, telegram.SendOptions{
		ChatID:           chatID,
		Text:             "❌ Укажите положительное целое число больше нуля: `!список <число>`.",
		ReplyToMessageID: replyToMessageID,
	})
}
