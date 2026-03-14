package admin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	models "github.com/mymmrac/telego"

	"serotonyl.ru/telegram-bot/internal/audit"
	"serotonyl.ru/telegram-bot/internal/config"
	"serotonyl.ru/telegram-bot/internal/features/members"
	"serotonyl.ru/telegram-bot/internal/telegram"
)

type tgCall struct {
	kind       string
	chatID     int64
	messageID  int
	text       string
	markup     *models.InlineKeyboardMarkup
	callbackID string
	parseMode  *string
	noPreview  bool
}

type fakeTG struct {
	calls            []tgCall
	sendErr          error
	sendErrByChat    map[int64]error
	editErr          error
	deleteErr        error
	pinErr           error
	unpinErr         error
	chatMemberByUser map[int64]models.User
}

func (f *fakeTG) SendMessage(chatID int64, text string, markup *models.InlineKeyboardMarkup) (int, error) {
	f.calls = append(f.calls, tgCall{kind: "send", chatID: chatID, text: text, markup: markup})
	if f.sendErrByChat != nil {
		if err := f.sendErrByChat[chatID]; err != nil {
			return 100 + len(f.calls), err
		}
	}
	return 100 + len(f.calls), f.sendErr
}

func (f *fakeTG) EditMessage(chatID int64, messageID int, text string, markup *models.InlineKeyboardMarkup) error {
	f.calls = append(f.calls, tgCall{kind: "edit", chatID: chatID, messageID: messageID, text: text, markup: markup})
	return f.editErr
}

func (f *fakeTG) SendMessageWithOptions(opts telegram.SendOptions) (int, error) {
	f.calls = append(f.calls, tgCall{
		kind:      "send",
		chatID:    opts.ChatID,
		text:      opts.Text,
		markup:    opts.ReplyMarkup,
		parseMode: opts.ParseMode,
		noPreview: opts.DisableWebPagePreview,
	})
	if f.sendErrByChat != nil {
		if err := f.sendErrByChat[opts.ChatID]; err != nil {
			return 100 + len(f.calls), err
		}
	}
	return 100 + len(f.calls), f.sendErr
}

func (f *fakeTG) EditMessageWithOptions(opts telegram.EditOptions) error {
	f.calls = append(f.calls, tgCall{
		kind:      "edit",
		chatID:    opts.ChatID,
		messageID: opts.MessageID,
		text:      opts.Text,
		markup:    opts.ReplyMarkup,
		parseMode: opts.ParseMode,
		noPreview: opts.DisableWebPagePreview,
	})
	return f.editErr
}

func (f *fakeTG) AnswerCallbackQuery(callbackID string, text string, showAlert bool) error {
	f.calls = append(f.calls, tgCall{kind: "ack", callbackID: callbackID})
	return nil
}

func (f *fakeTG) GetChatMember(chatID int64, userID int64) (models.ChatMember, error) {
	if u, ok := f.chatMemberByUser[userID]; ok {
		return &models.ChatMemberMember{User: u}, nil
	}
	return &models.ChatMemberMember{User: models.User{ID: userID}}, nil
}

func (f *fakeTG) EditReplyMarkup(chatID int64, messageID int, markup *models.InlineKeyboardMarkup) error {
	return nil
}

func (f *fakeTG) DeleteMessage(chatID int64, messageID int) error {
	f.calls = append(f.calls, tgCall{kind: "delete", chatID: chatID, messageID: messageID})
	return f.deleteErr
}

func (f *fakeTG) count(kind string) int {
	n := 0
	for _, c := range f.calls {
		if c.kind == kind {
			n++
		}
	}
	return n
}

func (f *fakeTG) last(kind string) *tgCall {
	for i := len(f.calls) - 1; i >= 0; i-- {
		if f.calls[i].kind == kind {
			return &f.calls[i]
		}
	}
	return nil
}

type fakeAdminRepoHandlers struct {
	hasSession     bool
	deltaStore     map[int64][]*BalanceDelta
	nextDeltaID    int64
	deleteDeltaErr error
	states         map[int64]*AdminState
	panels         map[int64]AdminPanelMessage
	roundTripState bool
}

func (r *fakeAdminRepoHandlers) CreateSession(ctx context.Context, session *AdminSession) error {
	return nil
}
func (r *fakeAdminRepoHandlers) GetActiveSession(ctx context.Context, userID int64) (*AdminSession, error) {
	if !r.hasSession {
		return nil, nil
	}
	return &AdminSession{UserID: userID}, nil
}
func (r *fakeAdminRepoHandlers) DeactivateSession(ctx context.Context, userID int64) error {
	return nil
}
func (r *fakeAdminRepoHandlers) UpdateActivity(ctx context.Context, userID int64) error { return nil }
func (r *fakeAdminRepoHandlers) LogAttempt(ctx context.Context, userID int64, success bool) error {
	return nil
}
func (r *fakeAdminRepoHandlers) GetRecentAttempts(ctx context.Context, userID int64, period time.Duration) (int, error) {
	return 0, nil
}
func (r *fakeAdminRepoHandlers) CleanupStaleAuthState(ctx context.Context, now time.Time) (CleanupResult, error) {
	return CleanupResult{}, nil
}
func (r *fakeAdminRepoHandlers) ListBalanceDeltas(ctx context.Context, chatID int64) ([]*BalanceDelta, error) {
	if r.deltaStore == nil {
		return nil, nil
	}
	return r.deltaStore[chatID], nil
}
func (r *fakeAdminRepoHandlers) CreateBalanceDelta(ctx context.Context, chatID int64, name string, amount int64, createdBy int64) error {
	if r.deltaStore == nil {
		r.deltaStore = map[int64][]*BalanceDelta{}
	}
	r.nextDeltaID++
	r.deltaStore[chatID] = append(r.deltaStore[chatID], &BalanceDelta{ID: r.nextDeltaID, Name: name, Amount: amount, ChatID: chatID, CreatedBy: createdBy, CreatedAt: time.Now()})
	return nil
}
func (r *fakeAdminRepoHandlers) DeleteBalanceDelta(ctx context.Context, chatID int64, deltaID int64) error {
	if r.deleteDeltaErr != nil {
		return r.deleteDeltaErr
	}
	deltas := r.deltaStore[chatID]
	for i, d := range deltas {
		if d.ID == deltaID {
			r.deltaStore[chatID] = append(deltas[:i], deltas[i+1:]...)
			return nil
		}
	}
	return errors.New("not found")
}
func (r *fakeAdminRepoHandlers) SaveFlowState(ctx context.Context, userID int64, state *AdminState) error {
	if r.states == nil {
		r.states = map[int64]*AdminState{}
	}
	if state != nil && r.roundTripState {
		payload, err := marshalAdminStateData(state.State, state.Data)
		if err != nil {
			return err
		}
		state = &AdminState{State: state.State, Data: payload, ExpiresAt: state.ExpiresAt}
	}
	r.states[userID] = state
	return nil
}
func (r *fakeAdminRepoHandlers) GetFlowState(ctx context.Context, userID int64) (*AdminState, error) {
	if r.states == nil {
		return nil, nil
	}
	state := r.states[userID]
	if state == nil {
		return nil, nil
	}
	if !r.roundTripState {
		return state, nil
	}
	payload, _ := state.Data.([]byte)
	data, err := unmarshalAdminStateData(state.State, payload)
	if err != nil {
		return nil, err
	}
	return &AdminState{State: state.State, Data: data, ExpiresAt: state.ExpiresAt}, nil
}
func (r *fakeAdminRepoHandlers) ClearFlowState(ctx context.Context, userID int64) error {
	delete(r.states, userID)
	delete(r.panels, userID)
	return nil
}
func (r *fakeAdminRepoHandlers) SetPanelMessage(ctx context.Context, userID int64, panel AdminPanelMessage) error {
	if r.panels == nil {
		r.panels = map[int64]AdminPanelMessage{}
	}
	r.panels[userID] = panel
	return nil
}
func (r *fakeAdminRepoHandlers) GetPanelMessage(ctx context.Context, userID int64) (AdminPanelMessage, error) {
	if r.panels == nil {
		return AdminPanelMessage{}, nil
	}
	return r.panels[userID], nil
}
func (r *fakeAdminRepoHandlers) ClearPanelMessage(ctx context.Context, userID int64) error {
	delete(r.panels, userID)
	return nil
}

type econCall struct {
	method string
	userID int64
	amount int64
	txType string
}

type fakeEconomy struct {
	addCalls         int
	deductCalls      int
	addErr           error
	deductErr        error
	failOnAddCall    int
	failOnDeductCall int
	calls            []econCall
	balances         map[int64]int64
}

func (f *fakeEconomy) AddBalance(ctx context.Context, userID int64, amount int64, txType, description string) error {
	f.addCalls++
	f.calls = append(f.calls, econCall{method: "add", userID: userID, amount: amount, txType: txType})
	if f.failOnAddCall > 0 && f.addCalls == f.failOnAddCall {
		return errors.New("forced add error")
	}
	return f.addErr
}

func (f *fakeEconomy) DeductBalance(ctx context.Context, userID int64, amount int64, txType, description string) error {
	f.deductCalls++
	f.calls = append(f.calls, econCall{method: "deduct", userID: userID, amount: amount, txType: txType})
	if f.failOnDeductCall > 0 && f.deductCalls == f.failOnDeductCall {
		return errors.New("forced deduct error")
	}
	return f.deductErr
}

func (f *fakeEconomy) GetBalance(ctx context.Context, userID int64) (int64, error) {
	return f.balances[userID], nil
}

type fakeMemberRepoHandlers struct {
	members map[int64]*members.Member
	without []*members.Member
	with    []*members.Member
	deltas  []*BalanceDelta
}

type fakeMemberSyncRepo struct {
	activeIDs           []int64
	refreshCandidateIDs []int64
	updateTagErr        error
	listErr             error
	updateTagBlocked    bool
	onUpsertActive      func(userID int64, username, name string, isBot bool)
}

func (r *fakeMemberSyncRepo) UpsertActiveMember(ctx context.Context, userID int64, username, name string, isBot bool, joinedAt time.Time) error {
	if r.onUpsertActive != nil {
		r.onUpsertActive(userID, username, name, isBot)
	}
	return nil
}
func (r *fakeMemberSyncRepo) MarkMemberLeft(ctx context.Context, userID int64, leftAt, deleteAfter time.Time) error {
	return nil
}
func (r *fakeMemberSyncRepo) IsActiveMember(ctx context.Context, userID int64) (bool, error) {
	return true, nil
}
func (r *fakeMemberSyncRepo) PurgeExpiredLeftMembers(ctx context.Context, now time.Time, limit int) (int, error) {
	return 0, nil
}
func (r *fakeMemberSyncRepo) GetByUserID(ctx context.Context, userID int64) (*members.Member, error) {
	return nil, nil
}
func (r *fakeMemberSyncRepo) GetByUsername(ctx context.Context, username string) (*members.Member, error) {
	return nil, nil
}
func (r *fakeMemberSyncRepo) FindByNickname(ctx context.Context, nickname string) (*members.Member, error) {
	return nil, nil
}
func (r *fakeMemberSyncRepo) EnsureMemberSeen(ctx context.Context, userID int64, username, name string, isBot bool, seenAt time.Time) error {
	return nil
}
func (r *fakeMemberSyncRepo) EnsureActiveMemberSeen(ctx context.Context, userID int64, username, name string, isBot bool, seenAt time.Time) error {
	return nil
}
func (r *fakeMemberSyncRepo) TouchLastSeen(ctx context.Context, userID int64, seenAt time.Time) error {
	return nil
}
func (r *fakeMemberSyncRepo) ListActiveUserIDs(ctx context.Context) ([]int64, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	return r.activeIDs, nil
}
func (r *fakeMemberSyncRepo) ListRefreshCandidateUserIDs(ctx context.Context) ([]int64, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	if r.refreshCandidateIDs != nil {
		return r.refreshCandidateIDs, nil
	}
	return r.activeIDs, nil
}
func (r *fakeMemberSyncRepo) UpdateMemberTag(ctx context.Context, userID int64, tag *string, updatedAt time.Time) error {
	if r.updateTagBlocked {
		<-ctx.Done()
		return ctx.Err()
	}
	if r.updateTagErr != nil {
		return r.updateTagErr
	}
	return nil
}
func (r *fakeMemberSyncRepo) CountMembersByStatus(ctx context.Context) (active int, left int, err error) {
	return 0, 0, nil
}
func (r *fakeMemberSyncRepo) CountPendingPurge(ctx context.Context, now time.Time) (int, error) {
	return 0, nil
}

func (r *fakeMemberRepoHandlers) GetByUserID(ctx context.Context, userID int64) (*members.Member, error) {
	if m, ok := r.members[userID]; ok {
		return m, nil
	}
	return nil, nil
}
func (r *fakeMemberRepoHandlers) GetUsersWithoutRole(ctx context.Context) ([]*members.Member, error) {
	out := make([]*members.Member, 0, len(r.without))
	for _, m := range r.without {
		if m != nil && !m.IsBot {
			out = append(out, m)
		}
	}
	return out, nil
}
func (r *fakeMemberRepoHandlers) GetUsersWithRole(ctx context.Context) ([]*members.Member, error) {
	out := make([]*members.Member, 0, len(r.with))
	for _, m := range r.with {
		if m != nil && !m.IsBot {
			out = append(out, m)
		}
	}
	return out, nil
}
func (r *fakeMemberRepoHandlers) UpdateRole(ctx context.Context, userID int64, role string) error {
	for _, m := range r.with {
		if m.UserID == userID {
			m.Role = &role
		}
	}
	for _, m := range r.without {
		if m.UserID == userID {
			m.Role = &role
		}
	}
	if r.members[userID] != nil {
		r.members[userID].Role = &role
	}
	return nil
}
func (r *fakeMemberRepoHandlers) ListBalanceDeltas(ctx context.Context, chatID int64) ([]*BalanceDelta, error) {
	return r.deltas, nil
}
func (r *fakeMemberRepoHandlers) CreateBalanceDelta(ctx context.Context, chatID int64, name string, amount int64, createdBy int64) error {
	r.deltas = append(r.deltas, &BalanceDelta{ID: int64(len(r.deltas) + 1), ChatID: chatID, Name: name, Amount: amount, CreatedBy: createdBy, CreatedAt: time.Now()})
	return nil
}
func (r *fakeMemberRepoHandlers) UpdateAdminFlag(ctx context.Context, userID int64, isAdmin bool) error {
	m := r.members[userID]
	if m == nil {
		m = &members.Member{UserID: userID}
		r.members[userID] = m
	}
	m.IsAdmin = isAdmin
	return nil
}

func newAdminHandlerForFlow(t *testing.T, memberRepo *fakeMemberRepoHandlers, tg *fakeTG) *Handler {
	t.Helper()
	svc := NewService(&fakeAdminRepoHandlers{hasSession: true, deltaStore: map[int64][]*BalanceDelta{77: {&BalanceDelta{Name: "Test", Amount: 10, ChatID: 77}}}}, memberRepo, &config.Config{AdminIDs: []int64{77}})
	return NewHandler(svc, nil, &fakeEconomy{}, telegram.NewOps(tg), 0)
}

func newAdminHandlerForFlowWithRepo(t *testing.T, stateRepo *fakeAdminRepoHandlers, memberRepo *fakeMemberRepoHandlers, tg *fakeTG) *Handler {
	t.Helper()
	if stateRepo.deltaStore == nil {
		stateRepo.deltaStore = map[int64][]*BalanceDelta{77: {&BalanceDelta{Name: "Test", Amount: 10, ChatID: 77}}}
	}
	svc := NewService(stateRepo, memberRepo, &config.Config{AdminIDs: []int64{77}})
	return NewHandler(svc, nil, &fakeEconomy{}, telegram.NewOps(tg), 0)
}

func newModeratorHandlerForFlow(t *testing.T, memberRepo *fakeMemberRepoHandlers, tg *fakeTG) *Handler {
	t.Helper()
	svc := NewService(&fakeAdminRepoHandlers{hasSession: true}, memberRepo, &config.Config{ModeratorIDs: []int64{77}})
	return NewHandler(svc, nil, &fakeEconomy{}, telegram.NewOps(tg), -1001)
}

func newModeratorHandlerWithRiddles(t *testing.T, tg *fakeTG, repo *fakeRiddleRepo) *Handler {
	t.Helper()
	svc := NewService(&fakeAdminRepoHandlers{hasSession: true}, &fakeMemberRepoHandlers{members: map[int64]*members.Member{}}, &config.Config{ModeratorIDs: []int64{77}})
	riddleSvc := NewRiddleService(repo, &fakeRiddleEconomy{})
	svc.SetRiddleService(riddleSvc)
	h := NewHandler(svc, nil, &fakeEconomy{}, telegram.NewOps(tg), -1001)
	h.riddleService = riddleSvc
	return h
}

func newAdminHandlerWithEconomy(t *testing.T, memberRepo *fakeMemberRepoHandlers, tg *fakeTG, econ *fakeEconomy) *Handler {
	t.Helper()
	svc := NewService(&fakeAdminRepoHandlers{hasSession: true, deltaStore: map[int64][]*BalanceDelta{77: {&BalanceDelta{Name: "Test", Amount: 10, ChatID: 77}}}}, memberRepo, &config.Config{AdminIDs: []int64{77}})
	return NewHandler(svc, nil, econ, telegram.NewOps(tg), 0)
}

func newAdminHandlerWithRefresh(t *testing.T, memberRepo *fakeMemberRepoHandlers, syncRepo *fakeMemberSyncRepo, tg *fakeTG) *Handler {
	t.Helper()
	svc := NewService(&fakeAdminRepoHandlers{hasSession: true, deltaStore: map[int64][]*BalanceDelta{77: {&BalanceDelta{Name: "Test", Amount: 10, ChatID: 77}}}}, memberRepo, &config.Config{AdminIDs: []int64{77}})
	h := NewHandler(svc, members.NewService(syncRepo), &fakeEconomy{}, telegram.NewOps(tg), 123)
	h.refreshTimeout = 20 * time.Millisecond
	return h
}

func callback(chatID int64, msgID int, userID int64, data string) *models.CallbackQuery {
	return &models.CallbackQuery{
		ID:      "cb-1",
		From:    models.User{ID: userID},
		Data:    data,
		Message: &models.Message{MessageID: msgID, Chat: models.Chat{ID: chatID}},
	}
}

func hasButton(markup *models.InlineKeyboardMarkup, text, dataContains string) bool {
	if markup == nil {
		return false
	}
	for _, row := range markup.InlineKeyboard {
		for _, b := range row {
			if (text == "" || b.Text == text) && (dataContains == "" || strings.Contains(b.CallbackData, dataContains)) {
				return true
			}
		}
	}
	return false
}

func hasCallText(calls []tgCall, kind, needle string) bool {
	for _, c := range calls {
		if c.kind == kind && strings.Contains(c.text, needle) {
			return true
		}
	}
	return false
}

func buttonByText(markup *models.InlineKeyboardMarkup, text string) *models.InlineKeyboardButton {
	if markup == nil {
		return nil
	}
	for _, row := range markup.InlineKeyboard {
		for _, b := range row {
			if b.Text == text {
				btn := b
				return &btn
			}
		}
	}
	return nil
}

func buttonByCallbackData(markup *models.InlineKeyboardMarkup, data string) *models.InlineKeyboardButton {
	if markup == nil {
		return nil
	}
	for _, row := range markup.InlineKeyboard {
		for _, b := range row {
			if b.CallbackData == data {
				btn := b
				return &btn
			}
		}
	}
	return nil
}

func ptrString(s string) *string {
	return &s
}

func TestFormatMemberTagOnly(t *testing.T) {
	tag := "TEAM-A"
	tests := []struct {
		name string
		user *members.Member
		want string
	}{
		{
			name: "with tag",
			user: &members.Member{UserID: 1, Tag: &tag, Username: "user", FirstName: "Name"},
			want: "TEAM-A",
		},
		{
			name: "empty tag",
			user: &members.Member{UserID: 2, Tag: ptrString("  ")},
			want: "Без тега",
		},
		{
			name: "no tag",
			user: &members.Member{UserID: 3, Username: "user", FirstName: "Name"},
			want: "Без тега",
		},
		{
			name: "nil user",
			user: nil,
			want: "Без тега",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatMemberTagOnly(tt.user); got != tt.want {
				t.Fatalf("formatMemberTagOnly() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatMemberRoleOnly(t *testing.T) {
	role := "Модератор"
	tests := []struct {
		name string
		user *members.Member
		want string
	}{
		{
			name: "with role",
			user: &members.Member{UserID: 1, Role: &role, Username: "user", FirstName: "Name"},
			want: "Модератор",
		},
		{
			name: "empty role",
			user: &members.Member{UserID: 2, Role: ptrString("  ")},
			want: "Без роли",
		},
		{
			name: "no role",
			user: &members.Member{UserID: 3, Username: "user", FirstName: "Name"},
			want: "Без роли",
		},
		{
			name: "nil user",
			user: nil,
			want: "Без роли",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatMemberRoleOnly(tt.user); got != tt.want {
				t.Fatalf("formatMemberRoleOnly() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOpenAdminPanel_ShowsKeyboard(t *testing.T) {
	tg := &fakeTG{}
	repo := &fakeMemberRepoHandlers{members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}}}
	h := newAdminHandlerForFlow(t, repo, tg)

	handled := h.HandleAdminMessage(context.Background(), 77, 77, 0, "Панель")
	if !handled {
		t.Fatalf("expected handled=true")
	}

	s := tg.last("send")
	if s == nil {
		t.Fatalf("expected SendMessage")
	}
	if !strings.Contains(s.text, "Админ-панель") {
		t.Fatalf("unexpected panel text: %q", s.text)
	}
	if !hasButton(s.markup, "👤 Назначить роль", cbAdminAssignRole) || !hasButton(s.markup, "🔄 Сменить роль", cbAdminChangeRole) || !hasButton(s.markup, "🎞️ Валюта", cbAdminBalanceAdjust) || !hasButton(s.markup, "👥 Участники", cbAdminParticipants) || !hasButton(s.markup, "❓ Загадки", cbAdminRiddlesMenu) || !hasButton(s.markup, "➕ Дельты", cbAdminDeltasMenu) {
		t.Fatalf("expected reduced admin panel buttons")
	}
	if hasButton(s.markup, "💸 Баланс", cbAdminBalanceAdjust) {
		t.Fatalf("old balance label must be removed")
	}
	if hasButton(s.markup, "💳 Управление кредитами", "admin:credit_menu") || hasButton(s.markup, "💳 Выдать кредит", "admin:credit_issue") || hasButton(s.markup, "🚫 Отменить кредит", "admin:credit_cancel") || hasButton(s.markup, "✂️ Создать сокращ.", "admin:stub") || hasButton(s.markup, "🗑 Удалить сокращ.", "admin:stub") {
		t.Fatalf("did not expect old top-level shortcuts")
	}
}

func TestOpenAdminPanel_ModeratorSeesOnlyRiddleControls(t *testing.T) {
	tg := &fakeTG{}
	h := newModeratorHandlerForFlow(t, &fakeMemberRepoHandlers{members: map[int64]*members.Member{}}, tg)

	handled := h.HandleAdminMessage(context.Background(), 77, 77, 0, "/login")
	if !handled {
		t.Fatalf("expected handled=true")
	}

	s := tg.last("send")
	if s == nil {
		t.Fatalf("expected SendMessage")
	}
	if !hasButton(s.markup, "Создать загадку", cbRiddleCreate) || !hasButton(s.markup, "Остановить загадку", cbRiddleStop) {
		t.Fatalf("expected moderator riddle buttons, got %#v", s.markup)
	}
	if hasButton(s.markup, "", cbAdminAssignRole) || hasButton(s.markup, "", cbAdminBalanceAdjust) || hasButton(s.markup, "", cbAdminParticipants) || hasButton(s.markup, "", cbAdminDeltasMenu) {
		t.Fatalf("moderator panel must hide admin-only controls")
	}
}

func TestModeratorCannotAccessBalanceOrCreditsOrRoleManagement(t *testing.T) {
	tg := &fakeTG{}
	h := newModeratorHandlerForFlow(t, &fakeMemberRepoHandlers{members: map[int64]*members.Member{}}, tg)

	for _, data := range []string{cbAdminAssignRole, cbAdminBalanceAdjust, cbAdminParticipants, cbAdminDeltasMenu, cbAdminDeltaAdd, cbAdminDeltaDelete} {
		if !h.HandleAdminCallback(context.Background(), callback(77, 42, 77, data)) {
			t.Fatalf("expected callback %q handled", data)
		}
	}

	if !hasCallText(tg.calls, "send", "Недостаточно прав") {
		t.Fatalf("expected permission denial message")
	}
	if st := h.service.GetState(77); st != nil {
		t.Fatalf("forbidden callbacks must not enter protected flows, got %+v", st)
	}
}

func TestModeratorForbiddenCraftedCallbackIsRejected(t *testing.T) {
	tg := &fakeTG{}
	role := "role"
	repo := &fakeMemberRepoHandlers{
		members: map[int64]*members.Member{
			1001: {UserID: 1001, Username: "u1", Role: &role},
		},
	}
	econ := &fakeEconomy{}
	svc := NewService(&fakeAdminRepoHandlers{hasSession: true}, repo, &config.Config{ModeratorIDs: []int64{77}})
	h := NewHandler(svc, nil, econ, telegram.NewOps(tg), -1001)

	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbBalConfirmApply))

	if econ.addCalls != 0 || econ.deductCalls != 0 {
		t.Fatalf("forbidden crafted callback must not touch economy")
	}
	if !hasCallText(tg.calls, "send", "Недостаточно прав") {
		t.Fatalf("expected denial for forged admin callback")
	}
}

func TestModeratorCanCreateAndPublishRiddle(t *testing.T) {
	tg := &fakeTG{}
	riddleRepo := &fakeRiddleRepo{}
	h := newModeratorHandlerWithRiddles(t, tg, riddleRepo)

	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbRiddleCreate))
	_ = h.HandleAdminMessage(context.Background(), 77, 77, 1, "Текст загадки")
	_ = h.HandleAdminMessage(context.Background(), 77, 77, 2, "apple\npear")
	_ = h.HandleAdminMessage(context.Background(), 77, 77, 3, "15")
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbRiddlePublish))

	if riddleRepo.riddle == nil || riddleRepo.riddle.State != riddleStateActive {
		t.Fatalf("expected moderator-published active riddle, got %+v", riddleRepo.riddle)
	}
}

func TestModeratorCanStopRiddle(t *testing.T) {
	tg := &fakeTG{}
	riddleRepo := &fakeRiddleRepo{}
	h := newModeratorHandlerWithRiddles(t, tg, riddleRepo)

	pub, err := h.riddleService.CreatePublishing(context.Background(), 77, &RiddleDraftData{
		PostText:     "riddle",
		RewardAmount: 5,
		Answers:      []RiddleDraftAnswer{{Raw: "Apple", Normalized: "apple"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.riddleService.ActivatePublished(context.Background(), pub.Riddle.ID, -1001, 10); err != nil {
		t.Fatal(err)
	}

	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbRiddleStop))

	if riddleRepo.riddle == nil || riddleRepo.riddle.State != riddleStateStopped {
		t.Fatalf("expected stopped riddle, got %+v", riddleRepo.riddle)
	}
	del := tg.last("delete")
	if del == nil || del.chatID != -1001 || del.messageID != 10 {
		t.Fatalf("expected published riddle message delete, got %#v", del)
	}
}

func TestModeratorCanStopRiddleWhenDeleteFails(t *testing.T) {
	tg := &fakeTG{deleteErr: errors.New("already deleted")}
	riddleRepo := &fakeRiddleRepo{}
	h := newModeratorHandlerWithRiddles(t, tg, riddleRepo)

	pub, err := h.riddleService.CreatePublishing(context.Background(), 77, &RiddleDraftData{
		PostText:     "riddle",
		RewardAmount: 5,
		Answers:      []RiddleDraftAnswer{{Raw: "Apple", Normalized: "apple"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.riddleService.ActivatePublished(context.Background(), pub.Riddle.ID, -1001, 10); err != nil {
		t.Fatal(err)
	}

	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbRiddleStop))

	if riddleRepo.riddle == nil || riddleRepo.riddle.State != riddleStateStopped {
		t.Fatalf("expected stopped riddle after delete failure, got %+v", riddleRepo.riddle)
	}
	if tg.count("delete") != 1 {
		t.Fatalf("expected one delete attempt, got %d", tg.count("delete"))
	}
}

func TestHandleAdminMessage_DeniedLogin_SendsSingleMessage(t *testing.T) {
	tg := &fakeTG{}
	repo := &fakeMemberRepoHandlers{members: map[int64]*members.Member{}}
	svc := NewService(&fakeAdminRepoHandlers{hasSession: false}, repo, &config.Config{AdminIDs: []int64{}})
	h := NewHandler(svc, nil, &fakeEconomy{}, telegram.NewOps(tg), 0)

	handled := h.HandleAdminMessage(context.Background(), 77, 77, 0, "/login")
	if !handled {
		t.Fatal("expected /login to be handled")
	}
	if tg.count("send") != 1 {
		t.Fatalf("send calls = %d, want 1", tg.count("send"))
	}
	if last := tg.last("send"); last == nil || !strings.Contains(last.text, "Доступ запрещён") {
		t.Fatalf("expected denied message, got %#v", last)
	}
}

func TestPickerFlow_OpenPicker_ShowsUserList(t *testing.T) {
	tg := &fakeTG{}
	role := "old"
	repo := &fakeMemberRepoHandlers{
		members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}},
		with: []*members.Member{
			{UserID: 1001, Username: "u1", Role: &role},
			{UserID: 1002, Username: "u2", Role: &role},
			{UserID: 1003, Username: "u3", Role: &role},
		},
	}
	h := newAdminHandlerForFlow(t, repo, tg)

	ok := h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminChangeRole))
	if !ok {
		t.Fatalf("expected callback handled")
	}
	if tg.count("ack") == 0 {
		t.Fatalf("expected callback ack")
	}

	e := tg.last("edit")
	if e == nil {
		t.Fatalf("expected EditMessage")
	}
	if !strings.Contains(e.text, "Выбери участника") {
		t.Fatalf("unexpected picker text: %q", e.text)
	}
	if !hasButton(e.markup, userPickerBackButton, cbPickerBack) {
		t.Fatalf("expected back button")
	}
	if hasButton(e.markup, "Отменить действие", cbAdminCancelAction) {
		t.Fatalf("did not expect cancel action button")
	}
	if b := buttonByCallbackData(e.markup, pickerCallbackData(UserPickerChangeWithRole, cbPickerSelect, repo.with[0].UserID)); b == nil || b.Style != "" {
		t.Fatalf("expected first user button without style, got %#v", b)
	}
	if b := buttonByCallbackData(e.markup, pickerCallbackData(UserPickerChangeWithRole, cbPickerSelect, repo.with[1].UserID)); b == nil || b.Style != "" {
		t.Fatalf("expected second user button without style, got %#v", b)
	}
	if b := buttonByCallbackData(e.markup, pickerCallbackData(UserPickerChangeWithRole, cbPickerSelect, repo.with[2].UserID)); b == nil || b.Style != "" {
		t.Fatalf("expected third user button without style, got %#v", b)
	}
	if b := buttonByText(e.markup, userPickerBackButton); b == nil || b.Style != "danger" {
		t.Fatalf("expected back button style danger, got %#v", b)
	}
	if b := buttonByText(e.markup, userPickerPrevButton); b != nil && b.Style != "" {
		t.Fatalf("expected prev pagination button without style, got %q", b.Style)
	}
	if b := buttonByText(e.markup, userPickerNextButton); b != nil && b.Style != "" {
		t.Fatalf("expected next pagination button without style, got %q", b.Style)
	}
}

func TestAssignRolePicker_UserButtonsUseDefaultStyle(t *testing.T) {
	tg := &fakeTG{}
	repo := &fakeMemberRepoHandlers{
		members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}},
		without: []*members.Member{
			{UserID: 2001, Username: "u1"},
			{UserID: 2002, Username: "u2"},
			{UserID: 2003, Username: "u3"},
		},
	}
	h := newAdminHandlerForFlow(t, repo, tg)

	if !h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminAssignRole)) {
		t.Fatalf("expected callback handled")
	}

	e := tg.last("edit")
	if e == nil {
		t.Fatalf("expected EditMessage")
	}
	for _, user := range repo.without {
		if b := buttonByCallbackData(e.markup, pickerCallbackData(UserPickerAssignWithoutRole, cbPickerSelect, user.UserID)); b == nil || b.Style != "" {
			t.Fatalf("expected assign-role user button without style, got %#v", b)
		}
	}
	if b := buttonByText(e.markup, userPickerBackButton); b == nil || b.Style != "danger" {
		t.Fatalf("expected back button style danger, got %#v", b)
	}
}

func TestPickerFlow_NoCandidates_RendersPanelEdit_NoSend(t *testing.T) {
	tg := &fakeTG{}
	repo := &fakeMemberRepoHandlers{
		members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}},
		without: []*members.Member{},
	}
	h := newAdminHandlerForFlow(t, repo, tg)

	ok := h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminAssignRole))
	if !ok {
		t.Fatalf("expected callback handled")
	}

	if hasCallText(tg.calls, "send", "Все пользователи уже имеют роли") {
		t.Fatalf("must not send standalone no-candidates message")
	}

	e := tg.last("edit")
	if e == nil {
		t.Fatalf("expected panel edit")
	}
	if !strings.Contains(e.text, "Все пользователи уже имеют роли") {
		t.Fatalf("unexpected no-candidates text: %q", e.text)
	}
	if e.messageID != 42 {
		t.Fatalf("expected edit of panel message id=42, got %d", e.messageID)
	}
	if e.markup == nil {
		t.Fatalf("expected markup")
	}
	if len(e.markup.InlineKeyboard) != 2 || len(e.markup.InlineKeyboard[0]) != 1 || len(e.markup.InlineKeyboard[1]) != 1 {
		t.Fatalf("expected refresh + return rows, got %#v", e.markup.InlineKeyboard)
	}
	refreshBtn := e.markup.InlineKeyboard[0][0]
	if refreshBtn.Text != "🔄 Обновить список" || refreshBtn.CallbackData != cbAssignRefresh {
		t.Fatalf("unexpected refresh button: %#v", refreshBtn)
	}
	btn := e.markup.InlineKeyboard[1][0]
	if btn.Text != "✅ Вернуться в админку" || btn.CallbackData != cbAdminReturnPanel {
		t.Fatalf("unexpected return button: %#v", btn)
	}
	if hasButton(e.markup, "👤 Назначить роль", cbAdminAssignRole) || hasButton(e.markup, "🔄 Сменить роль", cbAdminChangeRole) {
		t.Fatalf("no-candidates screen must not contain admin panel action buttons")
	}
	if st := h.service.GetState(77); st != nil {
		t.Fatalf("expected flow state cleared, got %q", st.State)
	}
}

func TestAssignRole_PickerRenders_WhenMemberHasOnlyIDAfterNullNormalization(t *testing.T) {
	tg := &fakeTG{}
	repo := &fakeMemberRepoHandlers{
		members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}},
		without: []*members.Member{{UserID: 1001}},
	}
	h := newAdminHandlerForFlow(t, repo, tg)

	ok := h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminAssignRole))
	if !ok {
		t.Fatalf("expected callback handled")
	}
	if hasCallText(tg.calls, "send", "Ошибка получения списка пользователей") {
		t.Fatalf("did not expect list error message")
	}

	e := tg.last("edit")
	if e == nil || !strings.Contains(e.text, "Выбери участника") {
		t.Fatalf("expected picker render, got %#v", e)
	}
	if b := buttonByText(e.markup, "Без тега"); b == nil {
		t.Fatalf("expected neutral fallback button for users without tag")
	}
}

func TestAssignRolePicker_DoesNotRenderBots(t *testing.T) {
	tg := &fakeTG{}
	repo := &fakeMemberRepoHandlers{
		members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}},
		without: []*members.Member{{UserID: 1001, FirstName: "Human"}, {UserID: 2002, FirstName: "Bot", IsBot: true}},
	}
	h := newAdminHandlerForFlow(t, repo, tg)

	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminAssignRole))
	e := tg.last("edit")
	if e == nil || e.markup == nil {
		t.Fatalf("expected picker edit")
	}
	if buttonByText(e.markup, "Bot • id:2002") != nil || buttonByText(e.markup, "id:2002") != nil {
		t.Fatalf("bot candidate must not be rendered in assign picker")
	}
}

func TestChangeRolePicker_DoesNotRenderBots(t *testing.T) {
	tg := &fakeTG{}
	role := "member"
	repo := &fakeMemberRepoHandlers{
		members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}},
		with:    []*members.Member{{UserID: 1001, FirstName: "Human", Role: &role}, {UserID: 2002, FirstName: "Bot", Role: &role, IsBot: true}},
	}
	h := newAdminHandlerForFlow(t, repo, tg)

	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminChangeRole))
	e := tg.last("edit")
	if e == nil || e.markup == nil {
		t.Fatalf("expected picker edit")
	}
	if buttonByCallbackData(e.markup, pickerCallbackData(UserPickerChangeWithRole, cbPickerSelect, 2002)) != nil {
		t.Fatalf("bot candidate must not be rendered in change-role picker")
	}
}

func TestAssignRefresh_UsesFreshRepositorySnapshot(t *testing.T) {
	tg := &fakeTG{}
	memberRepo := &fakeMemberRepoHandlers{
		members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}},
		without: []*members.Member{{UserID: 1001, FirstName: "Old"}},
	}
	syncRepo := &fakeMemberSyncRepo{activeIDs: []int64{1001, 2002}}
	h := newAdminHandlerWithRefresh(t, memberRepo, syncRepo, tg)

	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminAssignRole))
	memberRepo.without = []*members.Member{{UserID: 2002, FirstName: "Fresh"}}

	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAssignRefresh))
	e := tg.last("edit")
	if e == nil || e.markup == nil {
		t.Fatalf("expected picker rerender after refresh")
	}
	if !hasButton(e.markup, "", pickerCallbackData(UserPickerAssignWithoutRole, cbPickerSelect, 2002)) {
		t.Fatalf("expected refreshed candidate from fresh repo snapshot")
	}
	if hasButton(e.markup, "", pickerCallbackData(UserPickerAssignWithoutRole, cbPickerSelect, 1001)) {
		t.Fatalf("stale snapshot candidate must not be rendered after refresh")
	}
}

func TestAssignRefresh_CorrectedBotIsHiddenFromPicker(t *testing.T) {
	memberRepo := &fakeMemberRepoHandlers{
		members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}},
		without: []*members.Member{{UserID: 2002, FirstName: "LegacyBot", IsBot: false}},
	}
	tg := &fakeTG{chatMemberByUser: map[int64]models.User{2002: {ID: 2002, Username: "legacy_helper_bot", FirstName: "Legacy", IsBot: true}}}
	syncRepo := &fakeMemberSyncRepo{activeIDs: []int64{2002}, refreshCandidateIDs: []int64{2002}}
	syncRepo.onUpsertActive = func(userID int64, username, name string, isBot bool) {
		if userID == 2002 && isBot {
			memberRepo.without = []*members.Member{}
		}
	}
	h := newAdminHandlerWithRefresh(t, memberRepo, syncRepo, tg)

	ok := h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAssignRefresh))
	if !ok {
		t.Fatalf("expected callback handled")
	}
	e := tg.last("edit")
	if e == nil {
		t.Fatalf("expected panel edit")
	}
	if !strings.Contains(e.text, "Все пользователи уже имеют роли") {
		t.Fatalf("expected no-candidates screen after bot correction, got %q", e.text)
	}
}

func TestAssignRefresh_NotModifiedEdit_DoesNotShowUIFailure(t *testing.T) {
	tg := &fakeTG{editErr: errors.New("editMessageText: api: 400 Bad Request: message is not modified")}
	memberRepo := &fakeMemberRepoHandlers{
		members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}},
		without: []*members.Member{{UserID: 1001, FirstName: "Same"}},
	}
	syncRepo := &fakeMemberSyncRepo{activeIDs: []int64{1001}}
	h := newAdminHandlerWithRefresh(t, memberRepo, syncRepo, tg)

	ok := h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAssignRefresh))
	if !ok {
		t.Fatalf("expected callback handled")
	}
	if hasCallText(tg.calls, "send", "Не удалось обновить панель") || hasCallText(tg.calls, "send", "Панель устарела") {
		t.Fatalf("not-modified must be treated as benign and not produce UI error hints")
	}
}
func TestAssignRefresh_Success_ReopensPicker_WithNullSafeIdentityFallback(t *testing.T) {
	tg := &fakeTG{}
	memberRepo := &fakeMemberRepoHandlers{
		members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}},
		without: []*members.Member{{UserID: 1001}},
	}
	syncRepo := &fakeMemberSyncRepo{activeIDs: []int64{1001}}
	h := newAdminHandlerWithRefresh(t, memberRepo, syncRepo, tg)

	ok := h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAssignRefresh))
	if !ok {
		t.Fatalf("expected callback handled")
	}
	if hasCallText(tg.calls, "send", assignRefreshFailureHint) {
		t.Fatalf("did not expect refresh failure hint on successful refresh")
	}

	e := tg.last("edit")
	if e == nil || !strings.Contains(e.text, "Выбери участника") {
		t.Fatalf("expected picker rerender after refresh, got %#v", e)
	}
	if b := buttonByText(e.markup, "Без тега"); b == nil {
		t.Fatalf("expected neutral fallback button after refresh")
	}
}

func TestNoCandidates_ReturnButton_GoesBackToPanel(t *testing.T) {
	tg := &fakeTG{}
	repo := &fakeMemberRepoHandlers{
		members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}},
		without: []*members.Member{},
	}
	h := newAdminHandlerForFlow(t, repo, tg)

	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminAssignRole))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminReturnPanel))

	e := tg.last("edit")
	if e == nil {
		t.Fatalf("expected panel edit")
	}
	if !strings.Contains(e.text, "✅ Админ-панель открыта") {
		t.Fatalf("expected return to panel text, got: %q", e.text)
	}
	if !hasButton(e.markup, "👤 Назначить роль", cbAdminAssignRole) || !hasButton(e.markup, "🔄 Сменить роль", cbAdminChangeRole) {
		t.Fatalf("expected main admin panel buttons after return")
	}
}

func TestAssignRefresh_ManualTimeout_ShowsHintAndRerendersPicker(t *testing.T) {
	tg := &fakeTG{}
	role := "old"
	memberRepo := &fakeMemberRepoHandlers{
		members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}},
		without: []*members.Member{{UserID: 1001, Username: "u1", Role: &role}},
	}
	syncRepo := &fakeMemberSyncRepo{activeIDs: []int64{1001}, updateTagBlocked: true}
	h := newAdminHandlerWithRefresh(t, memberRepo, syncRepo, tg)
	h.refreshTimeout = 5 * time.Millisecond

	ok := h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAssignRefresh))
	if !ok {
		t.Fatalf("expected callback handled")
	}
	if !hasCallText(tg.calls, "send", assignRefreshFailureHint) {
		t.Fatalf("expected user-facing refresh timeout hint")
	}
	e := tg.last("edit")
	if e == nil || !strings.Contains(e.text, "Выбери участника") {
		t.Fatalf("expected picker rerender after timeout, got %#v", e)
	}
}

func TestAssignRefresh_SyncFailure_ShowsHintAndRerendersNoCandidates(t *testing.T) {
	tg := &fakeTG{}
	memberRepo := &fakeMemberRepoHandlers{members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}}, without: []*members.Member{}}
	syncRepo := &fakeMemberSyncRepo{listErr: errors.New("list active failed")}
	h := newAdminHandlerWithRefresh(t, memberRepo, syncRepo, tg)

	ok := h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAssignRefresh))
	if !ok {
		t.Fatalf("expected callback handled")
	}
	if tg.count("ack") == 0 {
		t.Fatalf("expected callback ack")
	}
	if !hasCallText(tg.calls, "send", assignRefreshFailureHint) {
		t.Fatalf("expected user-facing refresh failure hint")
	}
	e := tg.last("edit")
	if e == nil || !strings.Contains(e.text, "Все пользователи уже имеют роли") {
		t.Fatalf("expected no-candidates screen rerender, got %#v", e)
	}
}

func TestAssignRefresh_CanceledContext_NoHintButRerender(t *testing.T) {
	tg := &fakeTG{}
	role := "old"
	memberRepo := &fakeMemberRepoHandlers{
		members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}},
		without: []*members.Member{{UserID: 1001, Username: "u1", Role: &role}},
	}
	syncRepo := &fakeMemberSyncRepo{activeIDs: []int64{1001}}
	h := newAdminHandlerWithRefresh(t, memberRepo, syncRepo, tg)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ok := h.HandleAdminCallback(ctx, callback(77, 42, 77, cbAssignRefresh))
	if !ok {
		t.Fatalf("expected callback handled")
	}
	if hasCallText(tg.calls, "send", assignRefreshFailureHint) {
		t.Fatalf("did not expect hint when request context is canceled")
	}
	e := tg.last("edit")
	if e == nil || !strings.Contains(e.text, "Выбери участника") {
		t.Fatalf("expected picker rerender after cancellation, got %#v", e)
	}
}

func TestPickerFlow_Pagination_StyleRestartsOnNewPage(t *testing.T) {
	tg := &fakeTG{}
	role := "old"
	users := make([]*members.Member, 0, 10)
	for i := 0; i < 10; i++ {
		id := int64(2000 + i)
		users = append(users, &members.Member{UserID: id, Username: "user" + string(rune('0'+i)), Role: &role})
	}
	repo := &fakeMemberRepoHandlers{members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}}, with: users}
	h := newAdminHandlerForFlow(t, repo, tg)

	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminChangeRole))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, pickerCallbackData(UserPickerChangeWithRole, cbPickerNext, 0)))

	e := tg.last("edit")
	if e == nil || e.markup == nil {
		t.Fatalf("expected edit with picker markup")
	}
	firstPageSecondBtn := buttonByCallbackData(e.markup, pickerCallbackData(UserPickerChangeWithRole, cbPickerSelect, users[8].UserID))
	if firstPageSecondBtn == nil || firstPageSecondBtn.Style != "" {
		t.Fatalf("expected first button on second page without style, got %#v", firstPageSecondBtn)
	}
	if b := buttonByCallbackData(e.markup, pickerCallbackData(UserPickerChangeWithRole, cbPickerSelect, users[9].UserID)); b == nil || b.Style != "" {
		t.Fatalf("expected second button on second page without style, got %#v", b)
	}
}

func TestChangeRole_PickerRenders_WithIDFallback_WhenIdentityFieldsEmpty(t *testing.T) {
	tg := &fakeTG{}
	role := "operator"
	repo := &fakeMemberRepoHandlers{
		members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}},
		with:    []*members.Member{{UserID: 1001, Role: &role}},
	}
	h := newAdminHandlerForFlow(t, repo, tg)

	ok := h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminChangeRole))
	if !ok {
		t.Fatalf("expected callback handled")
	}
	if hasCallText(tg.calls, "send", "Ошибка получения списка пользователей") {
		t.Fatalf("did not expect list error message")
	}

	e := tg.last("edit")
	if e == nil || !strings.Contains(e.text, "Выбери участника") {
		t.Fatalf("expected picker render, got %#v", e)
	}
	if b := buttonByText(e.markup, "operator"); b == nil {
		t.Fatalf("expected change-role picker button with stored role")
	}
}

func TestChangeRole_PickerRenders_WithLastKnownNameFallback_WhenIdentityFieldsEmpty(t *testing.T) {
	tg := &fakeTG{}
	role := "operator"
	lastKnownName := "Ghost User"
	repo := &fakeMemberRepoHandlers{
		members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}},
		with: []*members.Member{{
			UserID:        1002,
			Role:          &role,
			LastKnownName: &lastKnownName,
		}},
	}
	h := newAdminHandlerForFlow(t, repo, tg)

	ok := h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminChangeRole))
	if !ok {
		t.Fatalf("expected callback handled")
	}

	e := tg.last("edit")
	if e == nil || !strings.Contains(e.text, "Выбери участника") {
		t.Fatalf("expected picker render, got %#v", e)
	}
	if b := buttonByText(e.markup, "operator"); b == nil {
		t.Fatalf("expected change-role picker to ignore last known name and show only role")
	}
}

func TestPickerFlow_SelectUser_ShowsRolePrompt(t *testing.T) {
	tg := &fakeTG{}
	role := "old_role"
	repo := &fakeMemberRepoHandlers{
		members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}},
		with:    []*members.Member{{UserID: 1001, Username: "u1", Role: &role}},
	}
	h := newAdminHandlerForFlow(t, repo, tg)

	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminChangeRole))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, pickerCallbackData(UserPickerChangeWithRole, cbPickerSelect, 1001)))

	e := tg.last("edit")
	if e == nil {
		t.Fatalf("expected EditMessage")
	}
	if !strings.Contains(e.text, "Текущая роль") || !strings.Contains(e.text, "Введите новую роль") {
		t.Fatalf("unexpected role prompt: %q", e.text)
	}
	if !hasButton(e.markup, userPickerBackButton, cbRoleInputBack) {
		t.Fatalf("expected role-input back button")
	}
	if hasButton(e.markup, "Отменить действие", cbAdminCancelAction) {
		t.Fatalf("did not expect cancel action button on role input")
	}
}

func TestCancelAction_FromPicker_ReturnsToPanel(t *testing.T) {
	tg := &fakeTG{}
	role := "old"
	repo := &fakeMemberRepoHandlers{
		members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}},
		with:    []*members.Member{{UserID: 1001, Username: "u1", Role: &role}},
	}
	h := newAdminHandlerForFlow(t, repo, tg)

	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminChangeRole))
	ok := h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminCancelAction))
	if !ok {
		t.Fatalf("expected callback handled")
	}
	if tg.count("ack") < 2 {
		t.Fatalf("expected callback ack for picker open and cancel")
	}

	e := tg.last("edit")
	if e == nil || !strings.Contains(e.text, "Админ-панель") {
		t.Fatalf("expected return to panel by edit, got %#v", e)
	}
	if st := h.service.GetState(77); st != nil {
		t.Fatalf("expected admin flow state cleared, got %q", st.State)
	}
}

func TestCancelAction_FromRoleInput_ReturnsToPanel(t *testing.T) {
	tg := &fakeTG{}
	role := "old"
	repo := &fakeMemberRepoHandlers{
		members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}},
		with:    []*members.Member{{UserID: 1001, Username: "u1", Role: &role}},
	}
	h := newAdminHandlerForFlow(t, repo, tg)

	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminChangeRole))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, pickerCallbackData(UserPickerChangeWithRole, cbPickerSelect, 1001)))
	ok := h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminCancelAction))
	if !ok {
		t.Fatalf("expected callback handled")
	}

	e := tg.last("edit")
	if e == nil || !strings.Contains(e.text, "Админ-панель") {
		t.Fatalf("expected return to panel by edit, got %#v", e)
	}
	if st := h.service.GetState(77); st != nil {
		t.Fatalf("expected admin flow state cleared, got %q", st.State)
	}
}

func TestFormatUserPickerButton_UsesRoleOnlyForChangePicker(t *testing.T) {
	role := "Администратор"
	tag := "TEAM-A"
	userWithRole := &members.Member{UserID: 1001, Tag: &tag, Role: &role, Username: "u1"}
	if got := formatUserPickerButton(userWithRole, UserPickerChangeWithRole); got != "Администратор" {
		t.Fatalf("unexpected picker format with role: %q", got)
	}

	userWithoutRole := &members.Member{UserID: 1002, Tag: &tag, Username: "u2", FirstName: "Ivan"}
	if got := formatUserPickerButton(userWithoutRole, UserPickerChangeWithRole); got != "Без роли" {
		t.Fatalf("unexpected picker format without role: %q", got)
	}
}

func TestFormatUserPickerButton_KeepsAssignPickerFormat(t *testing.T) {
	tag := "TEAM-A"
	user := &members.Member{UserID: 1001, Tag: &tag, Role: ptrString("role"), Username: "u1"}
	if got := formatUserPickerButton(user, UserPickerAssignWithoutRole); got != "TEAM-A" {
		t.Fatalf("unexpected assign picker format: %q", got)
	}
}

func TestFormatBalancePickerLabel(t *testing.T) {
	role := "Администратор"
	tag := "TEAM-A"

	if got := formatBalancePickerLabel(&members.Member{UserID: 1001, Role: &role, Tag: &tag, Username: "u1"}); got != role {
		t.Fatalf("expected role-only label, got %q", got)
	}
	if got := formatBalancePickerLabel(&members.Member{UserID: 1002, Tag: &tag, Username: "u2", FirstName: "Ivan"}); got != tag {
		t.Fatalf("expected tag fallback label, got %q", got)
	}
	if got := formatBalancePickerLabel(&members.Member{UserID: 1003, Username: "u3", FirstName: "Ivan"}); got != "@u3" {
		t.Fatalf("expected username fallback label, got %q", got)
	}
}

func TestChangeRole_SubmitRole_ShowsSingleSuccessScreenWithActions(t *testing.T) {
	tg := &fakeTG{}
	role := "old_role"
	repo := &fakeMemberRepoHandlers{
		members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}},
		with:    []*members.Member{{UserID: 1001, Username: "u1", Role: &role}},
	}
	h := newAdminHandlerForFlow(t, repo, tg)

	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminChangeRole))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, pickerCallbackData(UserPickerChangeWithRole, cbPickerSelect, 1001)))
	handled := h.HandleAdminMessage(context.Background(), 77, 77, 0, "new_role")
	if !handled {
		t.Fatalf("expected handled=true")
	}

	success := tg.last("edit")
	if success == nil || !strings.Contains(success.text, "✅ Роль изменена") {
		t.Fatalf("expected success edit screen, got %#v", success)
	}
	if !hasButton(success.markup, "↩️ Отменить", cbAdminUndoLast) {
		t.Fatalf("expected undo button in success screen")
	}
	if !hasButton(success.markup, "🏠 Админка", cbAdminReturnPanel) {
		t.Fatalf("expected return-to-panel button in success screen")
	}
	if !strings.Contains(success.text, "old_role → new_role") {
		t.Fatalf("expected old/new role text, got %q", success.text)
	}

	for _, c := range tg.calls {
		if c.kind == "send" && strings.Contains(c.text, "Админ-панель") {
			t.Fatalf("did not expect second panel message after success")
		}
	}
}

func TestChangeRole_SubmitRole_DeletesAdminInputMessage(t *testing.T) {
	tg := &fakeTG{}
	role := "old_role"
	repo := &fakeMemberRepoHandlers{
		members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}},
		with:    []*members.Member{{UserID: 1001, Username: "u1", Role: &role}},
	}
	h := newAdminHandlerForFlow(t, repo, tg)

	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminChangeRole))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, pickerCallbackData(UserPickerChangeWithRole, cbPickerSelect, 1001)))
	_ = h.HandleAdminMessage(context.Background(), 77, 77, 555, "new_role")

	d := tg.last("delete")
	if d == nil {
		t.Fatalf("expected delete call for admin input message")
	}
	if d.chatID != 77 || d.messageID != 555 {
		t.Fatalf("unexpected delete call: %#v", d)
	}
}

func TestChangeRole_UndoLast_RestoresRole_AndShowsOnlyReturnButton(t *testing.T) {
	tg := &fakeTG{}
	adminAID := int64(77)
	oldRole := "old_role"
	repo := &fakeMemberRepoHandlers{
		members: map[int64]*members.Member{adminAID: {UserID: adminAID, IsAdmin: true}, 1001: {UserID: 1001, Username: "u1", Role: &oldRole}},
		with:    []*members.Member{{UserID: 1001, Username: "u1", Role: &oldRole}},
	}
	h := newAdminHandlerForFlow(t, repo, tg)

	_ = h.HandleAdminCallback(context.Background(), callback(adminAID, 42, adminAID, cbAdminChangeRole))
	_ = h.HandleAdminCallback(context.Background(), callback(adminAID, 42, adminAID, pickerCallbackData(UserPickerChangeWithRole, cbPickerSelect, 1001)))
	_ = h.HandleAdminMessage(context.Background(), adminAID, adminAID, 0, "new_role")
	ok := h.HandleAdminCallback(context.Background(), callback(adminAID, 42, adminAID, cbAdminUndoLast))
	if !ok {
		t.Fatalf("expected undo callback handled")
	}

	if repo.members[1001] == nil || repo.members[1001].Role == nil || *repo.members[1001].Role != oldRole {
		t.Fatalf("expected old role restored, got %#v", repo.members[1001])
	}

	undoScreen := tg.last("edit")
	if undoScreen == nil || !strings.Contains(undoScreen.text, "↩️ Откат выполнен") {
		t.Fatalf("expected undo success screen by edit, got %#v", undoScreen)
	}
	if hasButton(undoScreen.markup, "↩️ Отменить", cbAdminUndoLast) {
		t.Fatalf("did not expect undo button after rollback")
	}
	if !hasButton(undoScreen.markup, "🏠 Админка", cbAdminReturnPanel) {
		t.Fatalf("expected return-to-panel button to remain after rollback")
	}
}

func TestRoleActionKeyboard_UsesShortLabelsAndSingleColumnLayout(t *testing.T) {
	tg := &fakeTG{}
	role := "old_role"
	repo := &fakeMemberRepoHandlers{
		members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}},
		with:    []*members.Member{{UserID: 1001, Username: "u1", Role: &role}},
	}
	h := newAdminHandlerForFlow(t, repo, tg)

	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminChangeRole))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, pickerCallbackData(UserPickerChangeWithRole, cbPickerSelect, 1001)))
	_ = h.HandleAdminMessage(context.Background(), 77, 77, 0, "new_role")

	success := tg.last("edit")
	if success == nil || success.markup == nil {
		t.Fatalf("expected success screen with keyboard")
	}
	if !hasButton(success.markup, "↩️ Отменить", cbAdminUndoLast) {
		t.Fatalf("expected short undo label")
	}
	if !hasButton(success.markup, "🏠 Админка", cbAdminReturnPanel) {
		t.Fatalf("expected short return label")
	}
	for i, row := range success.markup.InlineKeyboard {
		if len(row) != 1 {
			t.Fatalf("expected single button per row, row %d has %d", i, len(row))
		}
		if strings.TrimSpace(row[0].Text) != row[0].Text {
			t.Fatalf("button text must be trimmed, got %q", row[0].Text)
		}
	}
}

func TestReturnPanelCallback_EditsMessageToPanel_AndClearsState(t *testing.T) {
	tg := &fakeTG{}
	repo := &fakeMemberRepoHandlers{members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}}}
	h := newAdminHandlerForFlow(t, repo, tg)
	h.service.SetState(77, StateChangeRoleText, &RoleInputData{})

	ok := h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminReturnPanel))
	if !ok {
		t.Fatalf("expected callback handled")
	}
	if tg.count("ack") == 0 {
		t.Fatalf("expected callback ack")
	}

	e := tg.last("edit")
	if e == nil || !strings.Contains(e.text, "✅ Админ-панель открыта") {
		t.Fatalf("expected panel edit, got %#v", e)
	}
	if !hasButton(e.markup, "👤 Назначить роль", cbAdminAssignRole) || !hasButton(e.markup, "🔄 Сменить роль", cbAdminChangeRole) {
		t.Fatalf("expected panel keyboard after return")
	}
	if st := h.service.GetState(77); st != nil {
		t.Fatalf("expected state cleared after return panel callback, got %q", st.State)
	}
}

func TestNormalizeRoleLabel(t *testing.T) {
	if got := normalizeRoleLabel("   "); got != "—" {
		t.Fatalf("expected placeholder for empty role, got %q", got)
	}
	if got := normalizeRoleLabel(" admin "); got != "admin" {
		t.Fatalf("expected trimmed role, got %q", got)
	}
}

func TestUndo_IsolatedPerAdmin(t *testing.T) {
	tg := &fakeTG{}
	adminAID := int64(77)
	adminBID := int64(88)
	oldRole := "old_role"
	repo := &fakeMemberRepoHandlers{
		members: map[int64]*members.Member{
			adminAID: {UserID: adminAID, IsAdmin: true},
			adminBID: {UserID: adminBID, IsAdmin: true},
			1001:     {UserID: 1001, Username: "u1", Role: &oldRole},
		},
		with: []*members.Member{{UserID: 1001, Username: "u1", Role: &oldRole}},
	}
	h := newAdminHandlerForFlow(t, repo, tg)

	_ = h.HandleAdminCallback(context.Background(), callback(adminAID, 42, adminAID, cbAdminChangeRole))
	_ = h.HandleAdminCallback(context.Background(), callback(adminAID, 42, adminAID, pickerCallbackData(UserPickerChangeWithRole, cbPickerSelect, 1001)))
	_ = h.HandleAdminMessage(context.Background(), adminAID, adminAID, 0, "new_role")

	_ = h.HandleAdminCallback(context.Background(), callback(adminBID, 52, adminBID, cbAdminUndoLast))
	if repo.members[1001] == nil || repo.members[1001].Role == nil || *repo.members[1001].Role != "new_role" {
		t.Fatalf("expected role to remain new_role after admin B undo attempt, got %#v", repo.members[1001])
	}

	foundNoAction := false
	for _, c := range tg.calls {
		if c.kind == "send" && strings.Contains(c.text, "Нет действия для отката") {
			foundNoAction = true
			break
		}
	}
	if !foundNoAction {
		t.Fatalf("expected neutral message for admin B without undo slot")
	}

	_ = h.HandleAdminCallback(context.Background(), callback(adminAID, 42, adminAID, cbAdminUndoLast))
	if repo.members[1001] == nil || repo.members[1001].Role == nil || *repo.members[1001].Role != oldRole {
		t.Fatalf("expected old role restored by admin A undo, got %#v", repo.members[1001])
	}
}

func TestBackButton_Works_FromRoleInput(t *testing.T) {
	tg := &fakeTG{}
	role := "old_role"
	repo := &fakeMemberRepoHandlers{
		members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}},
		with:    []*members.Member{{UserID: 1001, Username: "u1", Role: &role}},
	}
	h := newAdminHandlerForFlow(t, repo, tg)

	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminChangeRole))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, pickerCallbackData(UserPickerChangeWithRole, cbPickerSelect, 1001)))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbRoleInputBack))

	e := tg.last("edit")
	if e == nil {
		t.Fatalf("expected EditMessage")
	}
	if !strings.Contains(e.text, "Выбери участника") {
		t.Fatalf("expected return to picker, got: %q", e.text)
	}
}

func TestUnauthorizedUser_CannotOpenAdminPanel(t *testing.T) {
	tg := &fakeTG{}
	repo := &fakeMemberRepoHandlers{members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: false}}}
	svc := NewService(&fakeAdminRepoHandlers{hasSession: true}, repo, &config.Config{})
	h := NewHandler(svc, nil, &fakeEconomy{}, telegram.NewOps(tg), 0)

	handled := h.HandleAdminMessage(context.Background(), 77, 77, 0, "/login")
	if !handled {
		t.Fatalf("expected handled=true")
	}

	s := tg.last("send")
	if s == nil || !strings.Contains(s.text, "Доступ запрещён") {
		t.Fatalf("expected deny message, got %#v", s)
	}
}

func TestHandleAdminCallback_UnknownData_Acked(t *testing.T) {
	tg := &fakeTG{}
	repo := &fakeMemberRepoHandlers{members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}}}
	h := newAdminHandlerForFlow(t, repo, tg)

	ok := h.HandleAdminCallback(context.Background(), callback(1, 123, 77, "admin:unknown"))
	if !ok {
		t.Fatalf("expected callback handled")
	}
	if tg.count("ack") == 0 {
		t.Fatalf("expected callback ack to be sent")
	}
}

func TestAdminPanel_HasCurrencyParticipantsAndDeltasButtons(t *testing.T) {
	tg := &fakeTG{}
	repo := &fakeMemberRepoHandlers{members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}}}
	h := newAdminHandlerForFlow(t, repo, tg)

	_ = h.HandleAdminMessage(context.Background(), 77, 77, 0, "/login")
	s := tg.last("send")
	if s == nil || !hasButton(s.markup, "🎞️ Валюта", cbAdminBalanceAdjust) || !hasButton(s.markup, "👥 Участники", cbAdminParticipants) || !hasButton(s.markup, "❓ Загадки", cbAdminRiddlesMenu) || !hasButton(s.markup, "➕ Дельты", cbAdminDeltasMenu) {
		t.Fatalf("expected updated admin buttons")
	}
	if hasButton(s.markup, "💰 Баланс", cbAdminBalanceAdjust) {
		t.Fatalf("did not expect old label")
	}
	if hasButton(s.markup, "💳 Управление кредитами", "admin:credit_menu") {
		t.Fatalf("did not expect credit button")
	}
}

func TestAdminParticipants_OpenShowsRowsPaginationAndBack(t *testing.T) {
	tg := &fakeTG{}
	role := "Капитан"
	tag := "TEAM-A"
	lastKnown := "Fallback Name"
	htmlName := "Bob & Carol <QA>"
	repo := &fakeMemberRepoHandlers{
		members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}},
		with: []*members.Member{
			{UserID: 10, Username: "role_user", FirstName: "Role", LastName: "User", Role: &role},
		},
		without: []*members.Member{
			{UserID: 20, Tag: &tag},
			{UserID: 30, LastKnownName: &lastKnown},
			{UserID: 31, FirstName: htmlName}, {UserID: 32}, {UserID: 33}, {UserID: 34}, {UserID: 35}, {UserID: 36}, {UserID: 37},
		},
	}
	h := newAdminHandlerForFlow(t, repo, tg)
	h.economyService = &fakeEconomy{balances: map[int64]int64{
		10: 100, 20: 90, 30: 80, 31: 70, 32: 60, 33: 50, 34: 40, 35: 30, 36: 20, 37: 10,
	}}

	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminParticipants))

	edit := tg.last("edit")
	if edit == nil {
		t.Fatalf("expected participants screen edit")
	}
	rawText := edit.text
	if !strings.Contains(rawText, "<a href=\"https://t.me/role_user\">") {
		t.Fatalf("expected username-based profile link, got %q", rawText)
	}
	if !strings.Contains(rawText, "Bob &amp; Carol &lt;QA&gt;") {
		t.Fatalf("expected escaped plain name, got %q", rawText)
	}
	if strings.Contains(rawText, "tg://") {
		t.Fatalf("must not contain telegram mention links: %q", rawText)
	}
	if edit.parseMode == nil || *edit.parseMode != "HTML" {
		t.Fatalf("expected HTML parse mode, got %#v", edit.parseMode)
	}
	if !edit.noPreview {
		t.Fatal("expected web page preview disabled for participants screen")
	}
	edit.text = strings.ReplaceAll(edit.text, "<a href=\"https://t.me/role_user\">", "")
	edit.text = strings.ReplaceAll(edit.text, "</a>", "")
	edit.text = strings.ReplaceAll(edit.text, "Role User", role)
	edit.text = strings.ReplaceAll(edit.text, "id:20", tag)
	if !strings.Contains(edit.text, "Капитан – 100🎞️") || !strings.Contains(edit.text, "TEAM-A – 90🎞️") || !strings.Contains(edit.text, "Fallback Name – 80🎞️") {
		t.Fatalf("unexpected participants text: %q", edit.text)
	}
	if !hasButton(edit.markup, userPickerPrevButton, cbAdminParticipantsPage+"0") || !hasButton(edit.markup, userPickerNextButton, cbAdminParticipantsPage+"1") {
		t.Fatalf("expected pagination buttons")
	}
	if !hasButton(edit.markup, "Назад", cbAdminReturnPanel) {
		t.Fatalf("expected back button")
	}

	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminParticipantsPage+"1"))
	edit = tg.last("edit")
	if edit == nil || !strings.Contains(edit.text, "id:37 – 10🎞️") {
		t.Fatalf("expected second page, got %#v", edit)
	}
}

func TestAdminDeltasMenu_OpenAddDeleteAndBack(t *testing.T) {
	tg := &fakeTG{}
	repo := &fakeMemberRepoHandlers{members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}}}
	h := newAdminHandlerForFlow(t, repo, tg)

	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminDeltasMenu))

	edit := tg.last("edit")
	if edit == nil || edit.text != "Дельты" {
		t.Fatalf("expected delta menu, got %#v", edit)
	}
	if !hasButton(edit.markup, "Добавить дельту", cbAdminDeltaAdd) || !hasButton(edit.markup, "Удалить дельту", cbAdminDeltaDelete) || !hasButton(edit.markup, "Назад", cbAdminReturnPanel) {
		t.Fatalf("expected delta menu buttons")
	}

	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminDeltaAdd))
	state := h.service.GetState(77)
	if state == nil || state.State != StateBalanceDeltaName {
		t.Fatalf("expected delta create flow, got %+v", state)
	}

	_ = h.HandleAdminMessage(context.Background(), 77, 77, 1, "Новая дельта")
	_ = h.HandleAdminMessage(context.Background(), 77, 77, 2, "15")
	edit = tg.last("edit")
	if edit == nil || edit.text != "Дельты" {
		t.Fatalf("expected return to delta menu after create")
	}

	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminDeltaDelete))
	if state = h.service.GetState(77); state == nil || state.State != StateBalanceAdjustAmount {
		t.Fatalf("expected delta delete flow, got %+v", state)
	}
	if len(h.service.repo.(*fakeAdminRepoHandlers).deltaStore[77]) < 2 {
		t.Fatalf("expected created delta in store")
	}
	deltas := h.service.repo.(*fakeAdminRepoHandlers).deltaStore[77]
	deltaID := deltas[len(deltas)-1].ID
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbBalAmtDeleteID+fmt.Sprint(deltaID)))
	edit = tg.last("edit")
	if edit == nil || edit.text != "Дельты" {
		t.Fatalf("expected return to delta menu after delete")
	}
}

func TestAdminPanel_DoesNotExposeCreditMenu(t *testing.T) {
	tg := &fakeTG{}
	repo := &fakeMemberRepoHandlers{members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}}}
	h := newAdminHandlerForFlow(t, repo, tg)

	_ = h.HandleAdminMessage(context.Background(), 77, 77, 0, "Панель")
	e := tg.last("send")
	if e == nil {
		t.Fatalf("expected panel")
	}
	if hasButton(e.markup, "💳 Управление кредитами", "admin:credit_menu") || hasButton(e.markup, "💳 Выдать кредит", "admin:credit_issue") || hasButton(e.markup, "🚫 Отменить кредит", "admin:credit_cancel") {
		t.Fatalf("credit controls must be absent from admin panel")
	}
}

func TestBalanceAdjust_NoLongerHasDeltaManagementButtons(t *testing.T) {
	tg := &fakeTG{}
	role := "role"
	repo := &fakeMemberRepoHandlers{members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}}, with: []*members.Member{{UserID: 1001, Username: "u", Role: &role}}}
	h := newAdminHandlerForFlow(t, repo, tg)

	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminBalanceAdjust))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, "admin:balmode:add"))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, fmt.Sprintf("%s:%d", cbBalPickToggle, int64(1001))))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbBalPickDone))

	e := tg.last("edit")
	if e == nil {
		t.Fatalf("expected amount screen")
	}
	if hasButton(e.markup, "➕ Добавить дельту", cbBalAmtAddDelta) || hasButton(e.markup, "🗑 Удалить дельту", cbBalAmtDeleteDelta) {
		t.Fatalf("did not expect delta management buttons in balance flow")
	}
}

func TestBalanceAdjust_MultiSelectPersistsAcrossPages(t *testing.T) {
	tg := &fakeTG{}
	role := "role"
	users := make([]*members.Member, 0, 10)
	for i := 0; i < 10; i++ {
		users = append(users, &members.Member{UserID: int64(5000 + i), Username: fmt.Sprintf("u%d", i), Role: &role})
	}
	repo := &fakeMemberRepoHandlers{members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}}, with: users}
	h := newAdminHandlerForFlow(t, repo, tg)

	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminBalanceAdjust))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, "admin:balmode:add"))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, fmt.Sprintf("%s:%d", cbBalPickToggle, users[0].UserID)))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbBalPickNext))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, fmt.Sprintf("%s:%d", cbBalPickToggle, users[8].UserID)))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbBalPickPrev))

	e := tg.last("edit")
	if e == nil {
		t.Fatalf("expected edit")
	}
	if !strings.Contains(e.text, "Выбрано: 2") {
		t.Fatalf("selection should persist, text=%q", e.text)
	}
}

func TestBalanceAdjust_ManualAmountValidation(t *testing.T) {
	tg := &fakeTG{}
	role := "role"
	memberRepo := &fakeMemberRepoHandlers{members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}}, with: []*members.Member{{UserID: 1001, Username: "u", Role: &role}}}
	stateRepo := &fakeAdminRepoHandlers{hasSession: true, roundTripState: true}
	h := newAdminHandlerForFlowWithRepo(t, stateRepo, memberRepo, tg)

	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminBalanceAdjust))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, "admin:balmode:add"))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, fmt.Sprintf("%s:%d", cbBalPickToggle, int64(1001))))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbBalPickDone))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbBalAmtManual))
	if state := h.service.GetState(77); state == nil || state.State != StateBalanceAdjustAmount {
		t.Fatalf("expected amount state after manual prompt, got %#v", state)
	} else if data, _ := state.Data.(*BalanceAdjustData); data == nil || !data.AwaitingManual || h.balanceWizardState(data).AwaitTextFor != "amount" {
		t.Fatalf("manual prompt must persist awaiting state, got %#v", data)
	}

	if !h.HandleAdminMessage(context.Background(), 77, 77, 0, "abc") {
		t.Fatalf("manual input should be handled")
	}
	if s := tg.last("edit"); s == nil || !strings.Contains(s.text, "Некорректная сумма") {
		t.Fatalf("expected validation error")
	}
	_ = h.HandleAdminMessage(context.Background(), 77, 77, 0, "150")
	if e := tg.last("edit"); e == nil || !strings.Contains(e.text, "Подтверждение") {
		t.Fatalf("expected confirmation screen")
	}
}

func TestBalanceAdjust_ManualAmountRejectedForDifferentOwner(t *testing.T) {
	tg := &fakeTG{}
	role := "role"
	memberRepo := &fakeMemberRepoHandlers{
		members: map[int64]*members.Member{
			77: {UserID: 77, IsAdmin: true},
			88: {UserID: 88, IsAdmin: true},
		},
		with: []*members.Member{{UserID: 1001, Username: "u", Role: &role}},
	}
	stateRepo := &fakeAdminRepoHandlers{hasSession: true, roundTripState: true}
	svc := NewService(stateRepo, memberRepo, &config.Config{AdminIDs: []int64{77, 88}})
	h := NewHandler(svc, nil, &fakeEconomy{}, telegram.NewOps(tg), 0)

	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminBalanceAdjust))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, "admin:balmode:add"))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, fmt.Sprintf("%s:%d", cbBalPickToggle, int64(1001))))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbBalPickDone))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbBalAmtManual))

	if h.HandleAdminMessage(context.Background(), 88, 88, 0, "150") {
		t.Fatalf("different admin must not consume another admin's manual amount step")
	}
	if state := h.service.GetState(77); state == nil || state.State != StateBalanceAdjustAmount {
		t.Fatalf("owner state must remain intact, got %#v", state)
	}
	if state := h.service.GetState(88); state != nil {
		t.Fatalf("different admin must not get flow state, got %#v", state)
	}
}

func TestBalanceAdjust_FilterOnlyUsersWithRole(t *testing.T) {
	tg := &fakeTG{}
	role := "role"
	repo := &fakeMemberRepoHandlers{
		members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}},
		with:    []*members.Member{{UserID: 1001, Username: "withrole", Role: &role}},
		without: []*members.Member{{UserID: 1002, Username: "norole"}},
	}
	h := newAdminHandlerForFlow(t, repo, tg)

	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminBalanceAdjust))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, "admin:balmode:add"))
	e := tg.last("edit")
	if e == nil {
		t.Fatalf("expected picker")
	}
	if strings.Contains(e.text, "norole") {
		t.Fatalf("user without role must not be present")
	}
}

func TestBalanceAdjust_PickerUsesMinimalHeaderAndSingleLabelButtons(t *testing.T) {
	tg := &fakeTG{}
	role := "Модератор"
	tag := "TEAM-A"
	repo := &fakeMemberRepoHandlers{
		members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}},
		with: []*members.Member{
			{UserID: 1001, Role: &role, Tag: &tag, Username: "with_role"},
			{UserID: 1002, Tag: &tag, Username: "tag_only", FirstName: "Ivan"},
		},
	}
	h := newAdminHandlerForFlow(t, repo, tg)

	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminBalanceAdjust))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, "admin:balmode:add"))

	e := tg.last("edit")
	if e == nil {
		t.Fatalf("expected picker")
	}
	if e.text != "Выберите пользователей\n\nВыбрано: 0" {
		t.Fatalf("unexpected picker header: %q", e.text)
	}
	if strings.Contains(e.text, "(только с ролью)") || strings.Contains(e.text, "Формат:") {
		t.Fatalf("old explanatory text must be removed: %q", e.text)
	}

	first := buttonByCallbackData(e.markup, fmt.Sprintf("%s:%d", cbBalPickToggle, int64(1001)))
	if first == nil || strings.HasPrefix(first.Text, "\u25a1") || strings.HasPrefix(first.Text, "\u25a2") || strings.HasPrefix(first.Text, "\u2610") || strings.HasPrefix(first.Text, "\u2611") {
		t.Fatalf("expected role-only button label, got %#v", first)
	}

	second := buttonByCallbackData(e.markup, fmt.Sprintf("%s:%d", cbBalPickToggle, int64(1002)))
	if second == nil || second.Text != "TEAM-A" {
		t.Fatalf("expected tag-only button label, got %#v", second)
	}
	if second != nil && (strings.Contains(second.Text, "@") || strings.Contains(second.Text, "\u25a1") || strings.Contains(second.Text, "\u25a2") || strings.Contains(second.Text, "\u2610") || strings.Contains(second.Text, "\u2611") || strings.Contains(second.Text, "id:")) {
		t.Fatalf("expected single primary label without mixed formatting, got %q", second.Text)
	}
}

func TestBalanceAdjust_PickerSelectedLabelUsesCheckmarkOnly(t *testing.T) {
	tg := &fakeTG{}
	role := "role"
	repo := &fakeMemberRepoHandlers{
		members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}},
		with:    []*members.Member{{UserID: 1001, Username: "picked", Role: &role}},
	}
	h := newAdminHandlerForFlow(t, repo, tg)

	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminBalanceAdjust))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, "admin:balmode:add"))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, fmt.Sprintf("%s:%d", cbBalPickToggle, int64(1001))))

	e := tg.last("edit")
	if e == nil {
		t.Fatalf("expected picker rerender")
	}
	btn := buttonByCallbackData(e.markup, fmt.Sprintf("%s:%d", cbBalPickToggle, int64(1001)))
	if btn == nil || !strings.HasPrefix(btn.Text, "\u2705 ") {
		t.Fatalf("expected selected label with checkmark, got %#v", btn)
	}
	if strings.Contains(btn.Text, "\u25a1") || strings.Contains(btn.Text, "\u25a2") || strings.Contains(btn.Text, "\u2610") || strings.Contains(btn.Text, "\u2611") {
		t.Fatalf("selected label must not use square markers: %q", btn.Text)
	}
}

func TestBalanceAdjust_ApplyValidatesRoleOnServerSide(t *testing.T) {
	tg := &fakeTG{}
	role := "role"
	repo := &fakeMemberRepoHandlers{
		members: map[int64]*members.Member{
			77:   {UserID: 77, IsAdmin: true},
			1001: {UserID: 1001, Username: "u1", Role: &role},
		},
		with: []*members.Member{{UserID: 1001, Username: "u1", Role: &role}},
	}
	econ := &fakeEconomy{}
	h := newAdminHandlerWithEconomy(t, repo, tg, econ)

	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminBalanceAdjust))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, "admin:balmode:add"))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, fmt.Sprintf("%s:%d", cbBalPickToggle, int64(1001))))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbBalPickDone))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbBalAmtDeltaPrefix+"10"))

	empty := ""
	repo.members[1001].Role = &empty // role dropped after picker, before apply
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbBalConfirmApply))

	if econ.addCalls != 0 || econ.deductCalls != 0 {
		t.Fatalf("should not touch economy when server-side role validation fails")
	}
	if !hasCallText(tg.calls, "edit", "Нельзя применить") {
		t.Fatalf("expected server-side validation error")
	}
}

func TestBalanceAdjust_RollbackOnMiddleFailure(t *testing.T) {
	tg := &fakeTG{}
	role := "role"
	repo := &fakeMemberRepoHandlers{
		members: map[int64]*members.Member{
			77:   {UserID: 77, IsAdmin: true},
			1001: {UserID: 1001, Username: "u1", Role: &role},
			1002: {UserID: 1002, Username: "u2", Role: &role},
			1003: {UserID: 1003, Username: "u3", Role: &role},
		},
		with: []*members.Member{
			{UserID: 1001, Username: "u1", Role: &role},
			{UserID: 1002, Username: "u2", Role: &role},
			{UserID: 1003, Username: "u3", Role: &role},
		},
	}
	econ := &fakeEconomy{failOnAddCall: 2}
	h := newAdminHandlerWithEconomy(t, repo, tg, econ)

	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminBalanceAdjust))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, "admin:balmode:add"))
	for _, id := range []int64{1001, 1002, 1003} {
		_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, fmt.Sprintf("%s:%d", cbBalPickToggle, id)))
	}
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbBalPickDone))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbBalAmtDeltaPrefix+"10"))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbBalConfirmApply))

	if econ.addCalls != 2 {
		t.Fatalf("expected 2 add calls before failure, got %d", econ.addCalls)
	}
	if econ.deductCalls != 1 {
		t.Fatalf("expected 1 rollback deduct call, got %d", econ.deductCalls)
	}
	if s := tg.last("edit"); s == nil || !strings.Contains(s.text, "Ошибка применения") {
		t.Fatalf("expected apply error message")
	}
}

func TestBalanceAdjust_UndoTwice(t *testing.T) {
	tg := &fakeTG{}
	role := "role"
	repo := &fakeMemberRepoHandlers{
		members: map[int64]*members.Member{
			77:   {UserID: 77, IsAdmin: true},
			1001: {UserID: 1001, Username: "u1", Role: &role},
		},
		with: []*members.Member{{UserID: 1001, Username: "u1", Role: &role}},
	}
	econ := &fakeEconomy{}
	h := newAdminHandlerWithEconomy(t, repo, tg, econ)

	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminBalanceAdjust))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, "admin:balmode:add"))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, fmt.Sprintf("%s:%d", cbBalPickToggle, int64(1001))))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbBalPickDone))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbBalAmtDeltaPrefix+"10"))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbBalConfirmApply))

	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbBalUndo))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbBalUndo))

	if econ.deductCalls == 0 {
		t.Fatalf("expected undo deduct call")
	}
	before := econ.deductCalls
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbBalUndo))
	if econ.deductCalls != before {
		t.Fatalf("second undo must not execute economy operations")
	}
}

func TestBalanceManualAmount_UsesEditNotSend(t *testing.T) {
	tg := &fakeTG{}
	role := "role"
	repo := &fakeMemberRepoHandlers{members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}}, with: []*members.Member{{UserID: 1001, Username: "u1", Role: &role}}}
	h := newAdminHandlerForFlow(t, repo, tg)

	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminBalanceAdjust))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, "admin:balmode:add"))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbBalPickToggle+":1001"))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbBalPickDone))
	sendBefore := tg.count("send")
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbBalAmtManual))
	if tg.count("send") != sendBefore {
		t.Fatalf("manual step must not send new message")
	}
	if e := tg.last("edit"); e == nil || !strings.Contains(e.text, "Отправьте сумму") {
		t.Fatalf("expected edit with manual prompt")
	}
}

func TestBalanceStateCleared_AfterReturnPanel(t *testing.T) {
	tg := &fakeTG{}
	role := "role"
	repo := &fakeMemberRepoHandlers{members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}}, with: []*members.Member{{UserID: 1001, Username: "u1", Role: &role}}}
	h := newAdminHandlerForFlow(t, repo, tg)

	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminBalanceAdjust))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, "admin:balmode:add"))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbBalPickToggle+":1001"))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbBalPickDone))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbBalAmtManual))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminReturnPanel))

	if st := h.service.GetState(77); st != nil {
		t.Fatalf("state should be cleared")
	}
	if h.HandleAdminMessage(context.Background(), 77, 77, 0, "250") {
		t.Fatalf("plain amount must not be handled after flow exit")
	}
}

func TestBalanceUndo_ClearsOperation_SecondUndoShowsEmpty(t *testing.T) {
	tg := &fakeTG{}
	repo := &fakeMemberRepoHandlers{members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}}}
	econ := &fakeEconomy{}
	h := newAdminHandlerWithEconomy(t, repo, tg, econ)

	h.service.SetState(77, StateBalanceAdjustConfirm, &BalanceAdjustData{
		FlowChatID:      77,
		FlowMessageID:   42,
		LastOperation:   []BalanceAdjustOperation{{UserID: 1001, Mode: BalanceAdjustModeAdd, Amount: 10}},
		LastOperationAt: time.Now(),
	})
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbBalUndo))
	if econ.deductCalls != 1 {
		t.Fatalf("expected undo to execute inverse operation")
	}

	h.service.SetState(77, StateBalanceAdjustConfirm, &BalanceAdjustData{FlowChatID: 77, FlowMessageID: 42, Undone: true})
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbBalUndo))
	if !hasCallText(tg.calls, "edit", "Нечего отменять") {
		t.Fatalf("expected empty undo message")
	}
}

type fakeAdminRepoAuth struct {
	hasSession bool
	attempts   int
	state      *AdminState
	panel      AdminPanelMessage
}

func (r *fakeAdminRepoAuth) CreateSession(ctx context.Context, session *AdminSession) error {
	r.hasSession = true
	return nil
}
func (r *fakeAdminRepoAuth) GetActiveSession(ctx context.Context, userID int64) (*AdminSession, error) {
	if !r.hasSession {
		return nil, nil
	}
	return &AdminSession{UserID: userID}, nil
}
func (r *fakeAdminRepoAuth) DeactivateSession(ctx context.Context, userID int64) error { return nil }
func (r *fakeAdminRepoAuth) UpdateActivity(ctx context.Context, userID int64) error    { return nil }
func (r *fakeAdminRepoAuth) LogAttempt(ctx context.Context, userID int64, success bool) error {
	if !success {
		r.attempts++
	}
	return nil
}
func (r *fakeAdminRepoAuth) GetRecentAttempts(ctx context.Context, userID int64, period time.Duration) (int, error) {
	return r.attempts, nil
}
func (r *fakeAdminRepoAuth) CleanupStaleAuthState(ctx context.Context, now time.Time) (CleanupResult, error) {
	return CleanupResult{}, nil
}
func (r *fakeAdminRepoAuth) ListBalanceDeltas(ctx context.Context, chatID int64) ([]*BalanceDelta, error) {
	return nil, nil
}
func (r *fakeAdminRepoAuth) CreateBalanceDelta(ctx context.Context, chatID int64, name string, amount int64, createdBy int64) error {
	return nil
}
func (r *fakeAdminRepoAuth) DeleteBalanceDelta(ctx context.Context, chatID int64, deltaID int64) error {
	return nil
}
func (r *fakeAdminRepoAuth) SaveFlowState(ctx context.Context, userID int64, state *AdminState) error {
	r.state = state
	return nil
}
func (r *fakeAdminRepoAuth) GetFlowState(ctx context.Context, userID int64) (*AdminState, error) {
	return r.state, nil
}
func (r *fakeAdminRepoAuth) ClearFlowState(ctx context.Context, userID int64) error {
	r.state = nil
	r.panel = AdminPanelMessage{}
	return nil
}
func (r *fakeAdminRepoAuth) SetPanelMessage(ctx context.Context, userID int64, panel AdminPanelMessage) error {
	r.panel = panel
	return nil
}
func (r *fakeAdminRepoAuth) GetPanelMessage(ctx context.Context, userID int64) (AdminPanelMessage, error) {
	return r.panel, nil
}
func (r *fakeAdminRepoAuth) ClearPanelMessage(ctx context.Context, userID int64) error {
	r.panel = AdminPanelMessage{}
	return nil
}

func TestHandleAdminMessage_LoginWithActiveSession_ShowsPanelWithoutAlreadyLoggedMessage(t *testing.T) {
	tg := &fakeTG{}
	repo := &fakeMemberRepoHandlers{members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}}}
	h := newAdminHandlerForFlow(t, repo, tg)

	handled := h.HandleAdminMessage(context.Background(), 77, 77, 0, "/login")
	if !handled {
		t.Fatalf("expected handled=true")
	}
	if !hasCallText(tg.calls, "send", "Админ-панель") {
		t.Fatalf("expected admin panel render")
	}
	if hasCallText(tg.calls, "send", "уже вош") {
		t.Fatalf("did not expect already-logged-in message")
	}
}

func TestHandleAdminMessage_LoginAuthFlowSuccess_ShowsPanelAndClearsState(t *testing.T) {
	tg := &fakeTG{}
	repo := &fakeMemberRepoHandlers{members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}}}
	authRepo := &fakeAdminRepoAuth{}
	svc := NewService(authRepo, repo, &config.Config{
		AdminIDs:          []int64{77},
		AdminPasswordHash: "$argon2id$v=19$m=65536,t=3,p=2$VHfCcsoxysCkOC6xwArT0A$XbpCLks/kLUE2rUgd7m9gqEIft8M+LQf+2ibCRLitAU",
	})
	h := NewHandler(svc, nil, &fakeEconomy{}, telegram.NewOps(tg), 0)

	handled := h.HandleAdminMessage(context.Background(), 77, 77, 0, "/login")
	if !handled {
		t.Fatalf("expected login prompt handled")
	}
	if state := svc.GetState(77); state == nil || state.State != StateAwaitingPassword {
		t.Fatalf("expected awaiting password state")
	}

	handled = h.HandleAdminMessage(context.Background(), 77, 77, 0, "secret")
	if !handled {
		t.Fatalf("expected password handled")
	}
	if !authRepo.hasSession {
		t.Fatalf("expected active admin session")
	}
	if svc.GetState(77) != nil {
		t.Fatalf("expected state to be cleared after successful auth")
	}
	if !hasCallText(tg.calls, "send", "Админ-панель") {
		t.Fatalf("expected admin panel render after successful auth")
	}
	if hasCallText(tg.calls, "send", "Аутентификация успешна") {
		t.Fatalf("expected no extra success message spam")
	}
}

func (f *fakeMemberSyncRepo) GetUsersWithRole(ctx context.Context) ([]*members.Member, error) {
	return nil, nil
}

func (f *fakeMemberSyncRepo) GetUsersWithoutRole(ctx context.Context) ([]*members.Member, error) {
	return nil, nil
}

func TestBalanceAdjust_TogglePersistsAcrossReloadAndRerender(t *testing.T) {
	tg := &fakeTG{}
	role := "role"
	repo := &fakeMemberRepoHandlers{
		members: map[int64]*members.Member{
			77:         {UserID: 77, IsAdmin: true},
			6899309136: {UserID: 6899309136, Username: "picked", Role: &role},
		},
		with: []*members.Member{{UserID: 6899309136, Username: "picked", Role: &role}},
	}
	stateRepo := &fakeAdminRepoHandlers{hasSession: true, roundTripState: true}
	h := newAdminHandlerForFlowWithRepo(t, stateRepo, repo, tg)

	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminBalanceAdjust))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, "admin:balmode:add"))

	initial := tg.last("edit")
	if initial == nil {
		t.Fatalf("expected initial picker render")
	}
	initialEdit := *initial
	initialButton := buttonByCallbackData(initialEdit.markup, cbBalPickToggle+":6899309136")
	if initialButton == nil {
		t.Fatalf("expected picker toggle button")
	}

	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbBalPickToggle+":6899309136"))

	stateOn := h.service.GetState(77)
	if stateOn == nil || stateOn.State != StateBalanceAdjustPicker {
		t.Fatalf("expected picker state after toggle on, got %+v", stateOn)
	}
	dataOn, _ := stateOn.Data.(*BalanceAdjustData)
	if dataOn == nil || len(dataOn.SelectedUserIDs) != 1 || !dataOn.SelectedUserIDs[6899309136] {
		t.Fatalf("expected persisted selected user after toggle on, got %+v", dataOn)
	}

	afterOn := tg.last("edit")
	if afterOn == nil {
		t.Fatalf("expected rerender after toggle on")
	}
	if !strings.HasSuffix(afterOn.text, "1") {
		t.Fatalf("expected selected count 1 after toggle on, text=%q", afterOn.text)
	}
	if afterOn.text == initialEdit.text {
		t.Fatalf("expected picker text to change after toggle on")
	}
	if !hasButton(afterOn.markup, "", cbBalPickDone) {
		t.Fatalf("expected done button after toggle on")
	}
	afterOnButton := buttonByCallbackData(afterOn.markup, cbBalPickToggle+":6899309136")
	if afterOnButton == nil || afterOnButton.Text == initialButton.Text {
		t.Fatalf("expected user button text to change after toggle on")
	}

	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbBalPickToggle+":6899309136"))

	stateOff := h.service.GetState(77)
	if stateOff == nil || stateOff.State != StateBalanceAdjustPicker {
		t.Fatalf("expected picker state after toggle off, got %+v", stateOff)
	}
	dataOff, _ := stateOff.Data.(*BalanceAdjustData)
	if dataOff == nil || len(dataOff.SelectedUserIDs) != 0 {
		t.Fatalf("expected persisted deselection after toggle off, got %+v", dataOff)
	}

	afterOff := tg.last("edit")
	if afterOff == nil {
		t.Fatalf("expected rerender after toggle off")
	}
	if !strings.HasSuffix(afterOff.text, "0") {
		t.Fatalf("expected selected count 0 after toggle off, text=%q", afterOff.text)
	}
	if afterOff.text == afterOn.text {
		t.Fatalf("expected picker text to change after toggle off")
	}
	if hasButton(afterOff.markup, "", cbBalPickDone) {
		t.Fatalf("did not expect done button after toggle off")
	}
	afterOffButton := buttonByCallbackData(afterOff.markup, cbBalPickToggle+":6899309136")
	if afterOffButton == nil || afterOffButton.Text != initialButton.Text {
		t.Fatalf("expected user button text to revert after toggle off")
	}
}

func TestAdminAudit_LoginUsesAdminChatAndIsNonBlocking(t *testing.T) {
	tg := &fakeTG{sendErrByChat: map[int64]error{999: errors.New("audit down")}}
	repo := &fakeMemberRepoHandlers{members: map[int64]*members.Member{77: {UserID: 77, Username: "admin_user", IsAdmin: true}}}
	h := newAdminHandlerForFlow(t, repo, tg)
	h.SetAuditLogger(audit.NewLogger(telegram.NewOps(tg), 999))

	if !h.HandleAdminMessage(context.Background(), 77, 77, 0, "/login") {
		t.Fatalf("expected handled=true")
	}
	foundPanel := false
	for _, call := range tg.calls {
		if call.kind == "send" && call.chatID == 77 {
			foundPanel = true
		}
	}
	if !foundPanel {
		t.Fatalf("expected admin panel render to still succeed")
	}
	foundAudit := false
	for _, call := range tg.calls {
		if call.kind == "send" && call.chatID == 999 && strings.Contains(call.text, "Login:") {
			foundAudit = true
		}
	}
	if !foundAudit {
		t.Fatalf("expected login audit log to admin chat")
	}
}

func TestAdminAudit_RoleAssignAndChangeEmitLogs(t *testing.T) {
	tg := &fakeTG{}
	oldRole := "Старая роль"
	target := &members.Member{UserID: 1001, Username: "target_user", Role: &oldRole}
	repo := &fakeMemberRepoHandlers{
		members: map[int64]*members.Member{
			77:   {UserID: 77, Username: "admin_user", IsAdmin: true},
			1001: target,
		},
	}
	h := newAdminHandlerForFlow(t, repo, tg)
	h.SetAuditLogger(audit.NewLogger(telegram.NewOps(tg), 999))

	h.service.SetState(77, StateAssignRoleText, &RoleInputData{SelectedUser: &members.Member{UserID: 1001, Username: "target_user"}})
	h.handleAssignRoleText(context.Background(), 77, 77, "Кокоми")

	h.service.SetState(77, StateChangeRoleText, &RoleInputData{SelectedUser: target})
	h.handleChangeRoleText(context.Background(), 77, 77, "Жуань Мэй")

	if !hasCallText(tg.calls, "send", "set_role:") {
		t.Fatalf("expected set_role audit log")
	}
	if !hasCallText(tg.calls, "send", "change_role:") {
		t.Fatalf("expected change_role audit log")
	}
}

func TestAdminAudit_BalanceGiveEmitsSingleMultilineLog(t *testing.T) {
	tg := &fakeTG{}
	role := "role"
	repo := &fakeMemberRepoHandlers{
		members: map[int64]*members.Member{
			77:   {UserID: 77, Username: "admin_user", IsAdmin: true},
			1001: {UserID: 1001, Username: "u1", Role: &role},
			1002: {UserID: 1002, Username: "u2", Role: &role},
		},
		with: []*members.Member{
			{UserID: 1001, Username: "u1", Role: &role},
			{UserID: 1002, Username: "u2", Role: &role},
		},
	}
	econ := &fakeEconomy{balances: map[int64]int64{1001: 110, 1002: 210}}
	h := newAdminHandlerWithEconomy(t, repo, tg, econ)
	h.SetAuditLogger(audit.NewLogger(telegram.NewOps(tg), 999))

	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminBalanceAdjust))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, "admin:balmode:add"))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbBalPickToggle+":1001"))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbBalPickToggle+":1002"))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbBalPickDone))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbBalAmtDeltaPrefix+"10"))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbBalConfirmApply))

	var auditMsgs []string
	for _, call := range tg.calls {
		if call.kind == "send" && call.chatID == 999 {
			auditMsgs = append(auditMsgs, call.text)
		}
	}
	if len(auditMsgs) != 1 {
		t.Fatalf("expected one audit message, got %d: %#v", len(auditMsgs), auditMsgs)
	}
	if !strings.Contains(auditMsgs[0], "give (+10) by") || !strings.Contains(auditMsgs[0], "@u1 +10") || !strings.Contains(auditMsgs[0], "@u2 +10") {
		t.Fatalf("unexpected audit text: %q", auditMsgs[0])
	}
}
