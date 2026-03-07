package admin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	models "github.com/mymmrac/telego"

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
}

type fakeTG struct {
	calls   []tgCall
	editErr error
}

func (f *fakeTG) SendMessage(chatID int64, text string, markup *models.InlineKeyboardMarkup) (int, error) {
	f.calls = append(f.calls, tgCall{kind: "send", chatID: chatID, text: text, markup: markup})
	return 100 + len(f.calls), nil
}

func (f *fakeTG) EditMessage(chatID int64, messageID int, text string, markup *models.InlineKeyboardMarkup) error {
	f.calls = append(f.calls, tgCall{kind: "edit", chatID: chatID, messageID: messageID, text: text, markup: markup})
	return f.editErr
}

func (f *fakeTG) AnswerCallbackQuery(callbackID string, text string, showAlert bool) error {
	f.calls = append(f.calls, tgCall{kind: "ack", callbackID: callbackID})
	return nil
}

func (f *fakeTG) GetChatMember(chatID int64, userID int64) (models.ChatMember, error) {
	return &models.ChatMemberMember{User: models.User{ID: userID}}, nil
}

func (f *fakeTG) EditReplyMarkup(chatID int64, messageID int, markup *models.InlineKeyboardMarkup) error {
	return nil
}

func (f *fakeTG) DeleteMessage(chatID int64, messageID int) error {
	f.calls = append(f.calls, tgCall{kind: "delete", chatID: chatID, messageID: messageID})
	return nil
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
}

func (r fakeAdminRepoHandlers) CreateSession(ctx context.Context, session *AdminSession) error {
	return nil
}
func (r fakeAdminRepoHandlers) GetActiveSession(ctx context.Context, userID int64) (*AdminSession, error) {
	if !r.hasSession {
		return nil, nil
	}
	return &AdminSession{UserID: userID}, nil
}
func (r fakeAdminRepoHandlers) DeactivateSession(ctx context.Context, userID int64) error { return nil }
func (r fakeAdminRepoHandlers) UpdateActivity(ctx context.Context, userID int64) error    { return nil }
func (r fakeAdminRepoHandlers) LogAttempt(ctx context.Context, userID int64, success bool) error {
	return nil
}
func (r fakeAdminRepoHandlers) GetRecentAttempts(ctx context.Context, userID int64, period time.Duration) (int, error) {
	return 0, nil
}
func (r fakeAdminRepoHandlers) ListBalanceDeltas(ctx context.Context, chatID int64) ([]*BalanceDelta, error) {
	if r.deltaStore == nil {
		return nil, nil
	}
	return r.deltaStore[chatID], nil
}
func (r fakeAdminRepoHandlers) CreateBalanceDelta(ctx context.Context, chatID int64, name string, amount int64, createdBy int64) error {
	if r.deltaStore == nil {
		r.deltaStore = map[int64][]*BalanceDelta{}
	}
	r.nextDeltaID++
	r.deltaStore[chatID] = append(r.deltaStore[chatID], &BalanceDelta{ID: r.nextDeltaID, Name: name, Amount: amount, ChatID: chatID, CreatedBy: createdBy, CreatedAt: time.Now()})
	return nil
}
func (r fakeAdminRepoHandlers) DeleteBalanceDelta(ctx context.Context, chatID int64, deltaID int64) error {
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
}

func (r *fakeMemberSyncRepo) UpsertActiveMember(ctx context.Context, userID int64, username, name string, joinedAt time.Time) error {
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
func (r *fakeMemberSyncRepo) EnsureMemberSeen(ctx context.Context, userID int64, username, name string, seenAt time.Time) error {
	return nil
}
func (r *fakeMemberSyncRepo) EnsureActiveMemberSeen(ctx context.Context, userID int64, username, name string, seenAt time.Time) error {
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
	return r.without, nil
}
func (r *fakeMemberRepoHandlers) GetUsersWithRole(ctx context.Context) ([]*members.Member, error) {
	return r.with, nil
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
	svc := NewService(fakeAdminRepoHandlers{hasSession: true, deltaStore: map[int64][]*BalanceDelta{77: {&BalanceDelta{Name: "Test", Amount: 10, ChatID: 77}}}}, memberRepo, &config.Config{AdminIDs: []int64{77}})
	return NewHandler(svc, nil, &fakeEconomy{}, telegram.NewOps(tg), 0)
}

func newAdminHandlerWithEconomy(t *testing.T, memberRepo *fakeMemberRepoHandlers, tg *fakeTG, econ *fakeEconomy) *Handler {
	t.Helper()
	svc := NewService(fakeAdminRepoHandlers{hasSession: true, deltaStore: map[int64][]*BalanceDelta{77: {&BalanceDelta{Name: "Test", Amount: 10, ChatID: 77}}}}, memberRepo, &config.Config{AdminIDs: []int64{77}})
	return NewHandler(svc, nil, econ, telegram.NewOps(tg), 0)
}

func newAdminHandlerWithRefresh(t *testing.T, memberRepo *fakeMemberRepoHandlers, syncRepo *fakeMemberSyncRepo, tg *fakeTG) *Handler {
	t.Helper()
	svc := NewService(fakeAdminRepoHandlers{hasSession: true, deltaStore: map[int64][]*BalanceDelta{77: {&BalanceDelta{Name: "Test", Amount: 10, ChatID: 77}}}}, memberRepo, &config.Config{AdminIDs: []int64{77}})
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
			if b.Text == text && (dataContains == "" || strings.Contains(b.CallbackData, dataContains)) {
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

func TestFormatMemberIdentityCompact(t *testing.T) {
	tag := "Мур"
	lastKnown := "Known Nick"
	tests := []struct {
		name string
		user *members.Member
		want string
	}{
		{
			name: "tag + username",
			user: &members.Member{UserID: 1, Tag: &tag, Username: "@kysxDDD"},
			want: "Мур • @kysxDDD",
		},
		{
			name: "tag + no username + nick",
			user: &members.Member{UserID: 2, Tag: &tag, FirstName: "Nick", LastName: "Name"},
			want: "Мур • Nick Name • id:2",
		},
		{
			name: "tag + no username + no nick",
			user: &members.Member{UserID: 3, Tag: &tag},
			want: "Мур • id:3",
		},
		{
			name: "no tag + username",
			user: &members.Member{UserID: 4, Username: "user"},
			want: "@user",
		},
		{
			name: "no tag + no username + nick",
			user: &members.Member{UserID: 5, FirstName: "OnlyNick"},
			want: "OnlyNick • id:5",
		},
		{
			name: "nil user",
			user: nil,
			want: "id:0",
		},
		{
			name: "fallback to last known name",
			user: &members.Member{UserID: 6, LastKnownName: &lastKnown},
			want: "Known Nick • id:6",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatMemberIdentityCompact(tt.user); got != tt.want {
				t.Fatalf("formatMemberIdentityCompact() = %q, want %q", got, tt.want)
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
	if !hasButton(s.markup, "👤 Назначить роль", cbAdminAssignRole) || !hasButton(s.markup, "🔄 Сменить роль", cbAdminChangeRole) || !hasButton(s.markup, "💸 Баланс", cbAdminBalanceAdjust) || !hasButton(s.markup, "💳 Управление кредитами", cbAdminCreditMenu) {
		t.Fatalf("expected reduced admin panel buttons")
	}
	if hasButton(s.markup, "💳 Выдать кредит", cbAdminCreditIssue) || hasButton(s.markup, "🚫 Отменить кредит", cbAdminCreditCancel) || hasButton(s.markup, "✂️ Создать сокращ.", "admin:stub") || hasButton(s.markup, "🗑 Удалить сокращ.", "admin:stub") {
		t.Fatalf("did not expect old top-level shortcuts")
	}
}

func TestHandleAdminMessage_DeniedLogin_SendsSingleMessage(t *testing.T) {
	tg := &fakeTG{}
	repo := &fakeMemberRepoHandlers{members: map[int64]*members.Member{}}
	svc := NewService(fakeAdminRepoHandlers{hasSession: false}, repo, &config.Config{AdminIDs: []int64{}})
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
	if b := buttonByText(e.markup, formatUserPickerButton(repo.with[0], UserPickerChangeWithRole)); b == nil || b.Style != "primary" {
		t.Fatalf("expected first user button style primary, got %#v", b)
	}
	if b := buttonByText(e.markup, formatUserPickerButton(repo.with[1], UserPickerChangeWithRole)); b == nil || b.Style != "success" {
		t.Fatalf("expected second user button style success, got %#v", b)
	}
	if b := buttonByText(e.markup, formatUserPickerButton(repo.with[2], UserPickerChangeWithRole)); b == nil || b.Style != "primary" {
		t.Fatalf("expected third user button style primary, got %#v", b)
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
	if b := buttonByText(e.markup, "id:1001"); b == nil {
		t.Fatalf("expected id fallback button for normalized NULL identity fields")
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
	if b := buttonByText(e.markup, "id:1001"); b == nil {
		t.Fatalf("expected id fallback button after refresh")
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
	firstPageSecondBtn := buttonByText(e.markup, formatUserPickerButton(users[8], UserPickerChangeWithRole))
	if firstPageSecondBtn == nil || firstPageSecondBtn.Style != "primary" {
		t.Fatalf("expected first button on second page to restart with primary, got %#v", firstPageSecondBtn)
	}
	if b := buttonByText(e.markup, formatUserPickerButton(users[9], UserPickerChangeWithRole)); b == nil || b.Style != "success" {
		t.Fatalf("expected second button on second page style success, got %#v", b)
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
	if b := buttonByText(e.markup, "operator • id:1001"); b == nil {
		t.Fatalf("expected change-role picker button with role and id fallback")
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
	if b := buttonByText(e.markup, "operator • Ghost User • id:1002"); b == nil {
		t.Fatalf("expected change-role picker button with last_known_name fallback")
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

func TestFormatMemberForAssignPicker(t *testing.T) {
	tag := "TEAM-A"
	withTag := &members.Member{UserID: 1001, Tag: &tag, Username: "user", FirstName: "Ivan"}
	if got := formatMemberForAssignPicker(withTag); got != "TEAM-A • id:1001" {
		t.Fatalf("unexpected assign format with tag: %q", got)
	}

	withUsername := &members.Member{UserID: 1002, Username: "user"}
	if got := formatMemberForAssignPicker(withUsername); got != "@user • id:1002" {
		t.Fatalf("unexpected assign format with username: %q", got)
	}

	withName := &members.Member{UserID: 1003, FirstName: "Ivan"}
	if got := formatMemberForAssignPicker(withName); got != "id:1003 • Ivan" {
		t.Fatalf("unexpected assign format with first name: %q", got)
	}

	idOnly := &members.Member{UserID: 1004}
	if got := formatMemberForAssignPicker(idOnly); got != "id:1004" {
		t.Fatalf("unexpected assign format id-only: %q", got)
	}
}

func TestFormatMemberForRolePicker(t *testing.T) {
	role := "мяу"
	withUsername := &members.Member{UserID: 1001, Username: "u", Role: &role}
	if got := formatMemberForRolePicker(withUsername); got != "мяу • @u" {
		t.Fatalf("unexpected role format with username: %q", got)
	}

	withoutUsername := &members.Member{UserID: 1002, Role: &role}
	if got := formatMemberForRolePicker(withoutUsername); got != "мяу • id:1002" {
		t.Fatalf("unexpected role format without username: %q", got)
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
	svc := NewService(fakeAdminRepoHandlers{hasSession: true}, repo, &config.Config{})
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

func TestAdminPanel_HasBalanceAdjustButton(t *testing.T) {
	tg := &fakeTG{}
	repo := &fakeMemberRepoHandlers{members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}}}
	h := newAdminHandlerForFlow(t, repo, tg)

	_ = h.HandleAdminMessage(context.Background(), 77, 77, 0, "Панель")
	s := tg.last("send")
	if s == nil || !hasButton(s.markup, "💸 Баланс", cbAdminBalanceAdjust) {
		t.Fatalf("expected balance adjust button")
	}
}

func TestAdminCreditMenu_OpenAndBack(t *testing.T) {
	tg := &fakeTG{}
	repo := &fakeMemberRepoHandlers{members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}}}
	h := newAdminHandlerForFlow(t, repo, tg)

	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminCreditMenu))
	e := tg.last("edit")
	if e == nil || !strings.Contains(e.text, "Управление кредитами") {
		t.Fatalf("expected credit submenu")
	}
	if !hasButton(e.markup, "💳 Выдать кредит", cbAdminCreditIssue) || !hasButton(e.markup, "🚫 Отменить кредит", cbAdminCreditCancel) || !hasButton(e.markup, userPickerBackButton, cbAdminReturnPanel) {
		t.Fatalf("expected credit submenu buttons")
	}

	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminReturnPanel))
	back := tg.last("edit")
	if back == nil || !strings.Contains(back.text, "Админ-панель") {
		t.Fatalf("expected return to panel")
	}
	if !hasButton(back.markup, "💳 Управление кредитами", cbAdminCreditMenu) {
		t.Fatalf("expected main panel after back")
	}
}

func TestBalanceAdjust_HasDeleteDeltaButton(t *testing.T) {
	tg := &fakeTG{}
	role := "role"
	repo := &fakeMemberRepoHandlers{members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}}, with: []*members.Member{{UserID: 1001, Username: "u", Role: &role}}}
	h := newAdminHandlerForFlow(t, repo, tg)

	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminBalanceAdjust))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, "admin:balmode:add"))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, fmt.Sprintf("%s:%d", cbBalPickToggle, int64(1001))))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbBalPickDone))

	e := tg.last("edit")
	if e == nil || !hasButton(e.markup, "🗑 Удалить дельту", cbBalAmtDeleteDelta) {
		t.Fatalf("expected delete delta button in balance flow")
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
	repo := &fakeMemberRepoHandlers{members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}}, with: []*members.Member{{UserID: 1001, Username: "u", Role: &role}}}
	h := newAdminHandlerForFlow(t, repo, tg)

	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminBalanceAdjust))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, "admin:balmode:add"))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, fmt.Sprintf("%s:%d", cbBalPickToggle, int64(1001))))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbBalPickDone))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbBalAmtManual))

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
func (r *fakeAdminRepoAuth) ListBalanceDeltas(ctx context.Context, chatID int64) ([]*BalanceDelta, error) {
	return nil, nil
}
func (r *fakeAdminRepoAuth) CreateBalanceDelta(ctx context.Context, chatID int64, name string, amount int64, createdBy int64) error {
	return nil
}
func (r *fakeAdminRepoAuth) DeleteBalanceDelta(ctx context.Context, chatID int64, deltaID int64) error {
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
