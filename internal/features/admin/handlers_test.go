package admin

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/go-telegram/bot/models"

	"serotonyl.ru/telegram-bot/internal/config"
	"serotonyl.ru/telegram-bot/internal/features/members"
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

func (f *fakeTG) AnswerCallback(callbackID string) error {
	f.calls = append(f.calls, tgCall{kind: "ack", callbackID: callbackID})
	return nil
}

func (f *fakeTG) GetChatMember(chatID int64, userID int64) (models.ChatMember, error) {
	return models.ChatMember{}, nil
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
	hasSession bool
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

type fakeMemberRepoHandlers struct {
	members map[int64]*members.Member
	without []*members.Member
	with    []*members.Member
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
	svc := NewService(fakeAdminRepoHandlers{hasSession: true}, memberRepo, &config.Config{AdminIDs: []int64{77}})
	return NewHandler(svc, nil, tg)
}

func callback(chatID int64, msgID int, userID int64, data string) *models.CallbackQuery {
	return &models.CallbackQuery{
		ID:   "cb-1",
		From: models.User{ID: userID},
		Data: data,
		Message: models.MaybeInaccessibleMessage{
			Type:    models.MaybeInaccessibleMessageTypeMessage,
			Message: &models.Message{ID: msgID, Chat: models.Chat{ID: chatID}},
		},
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

func TestOpenAdminPanel_ShowsKeyboard(t *testing.T) {
	tg := &fakeTG{}
	repo := &fakeMemberRepoHandlers{members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}}}
	h := newAdminHandlerForFlow(t, repo, tg)

	handled := h.HandleAdminMessage(context.Background(), 77, 77, "Панель")
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
	if !hasButton(s.markup, "Назначить роль", cbAdminAssignRole) || !hasButton(s.markup, "Сменить роль", cbAdminChangeRole) {
		t.Fatalf("expected admin panel buttons")
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
	if !strings.Contains(e.text, "Выбери участника с ролью") {
		t.Fatalf("unexpected picker text: %q", e.text)
	}
	if !hasButton(e.markup, userPickerBackButton, cbPickerBack) {
		t.Fatalf("expected back button")
	}
	if b := buttonByText(e.markup, formatUserPickerButton(repo.with[0])); b == nil || b.Style != "primary" {
		t.Fatalf("expected first user button style primary, got %#v", b)
	}
	if b := buttonByText(e.markup, formatUserPickerButton(repo.with[1])); b == nil || b.Style != "success" {
		t.Fatalf("expected second user button style success, got %#v", b)
	}
	if b := buttonByText(e.markup, formatUserPickerButton(repo.with[2])); b == nil || b.Style != "primary" {
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
	firstPageSecondBtn := buttonByText(e.markup, formatUserPickerButton(users[8]))
	if firstPageSecondBtn == nil || firstPageSecondBtn.Style != "primary" {
		t.Fatalf("expected first button on second page to restart with primary, got %#v", firstPageSecondBtn)
	}
	if b := buttonByText(e.markup, formatUserPickerButton(users[9])); b == nil || b.Style != "success" {
		t.Fatalf("expected second button on second page style success, got %#v", b)
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
}

func TestChangeRole_SubmitRole_ShowsSuccess_AndPanel(t *testing.T) {
	tg := &fakeTG{}
	role := "old_role"
	repo := &fakeMemberRepoHandlers{
		members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: true}},
		with:    []*members.Member{{UserID: 1001, Username: "u1", Role: &role}},
	}
	h := newAdminHandlerForFlow(t, repo, tg)

	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, cbAdminChangeRole))
	_ = h.HandleAdminCallback(context.Background(), callback(77, 42, 77, pickerCallbackData(UserPickerChangeWithRole, cbPickerSelect, 1001)))
	handled := h.HandleAdminMessage(context.Background(), 77, 77, "new_role")
	if !handled {
		t.Fatalf("expected handled=true")
	}

	foundSuccess := false
	foundPanel := false
	for _, c := range tg.calls {
		if c.kind == "send" && strings.Contains(c.text, "✅ Роль изменена") {
			foundSuccess = true
		}
		if c.kind == "send" && strings.Contains(c.text, "Админ-панель") {
			foundPanel = true
		}
	}
	if !foundSuccess {
		t.Fatalf("expected success message send")
	}
	if !foundPanel {
		t.Fatalf("expected panel to be shown after success")
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
	if !strings.Contains(e.text, "Выбери участника с ролью") {
		t.Fatalf("expected return to picker, got: %q", e.text)
	}
}

func TestUnauthorizedUser_CannotOpenAdminPanel(t *testing.T) {
	tg := &fakeTG{}
	repo := &fakeMemberRepoHandlers{members: map[int64]*members.Member{77: {UserID: 77, IsAdmin: false}}}
	svc := NewService(fakeAdminRepoHandlers{hasSession: true}, repo, &config.Config{})
	h := NewHandler(svc, nil, tg)

	handled := h.HandleAdminMessage(context.Background(), 77, 77, "/login")
	if !handled {
		t.Fatalf("expected handled=true")
	}

	s := tg.last("send")
	if s == nil || !strings.Contains(s.text, "Доступ запрещён") {
		t.Fatalf("expected deny message, got %#v", s)
	}
}

func TestClassifyEditError_ByMessage(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want editErrorKind
	}{
		{name: "not modified", err: errors.New("Bad Request: message is not modified"), want: editErrNotModified},
		{name: "not found", err: errors.New("Bad Request: message to edit not found"), want: editErrNotFound},
		{name: "cant be edited", err: errors.New("Bad Request: message can't be edited"), want: editErrCantBeEdited},
		{name: "forbidden", err: errors.New("Forbidden: bot was blocked by the user"), want: editErrForbidden},
		{name: "other", err: errors.New("some other error"), want: editErrOther},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, _ := classifyEditError(tt.err)
			if got != tt.want {
				t.Fatalf("classifyEditError(%v) = %q, want %q", tt.err, got, tt.want)
			}
		})
	}
}

func TestShouldFallbackToSend(t *testing.T) {
	tests := []struct {
		kind editErrorKind
		want bool
	}{
		{kind: editErrNotFound, want: true},
		{kind: editErrCantBeEdited, want: true},
		{kind: editErrNotModified, want: false},
		{kind: editErrForbidden, want: false},
	}

	for _, tt := range tests {
		if got := shouldFallbackToSend(tt.kind); got != tt.want {
			t.Fatalf("shouldFallbackToSend(%q) = %v, want %v", tt.kind, got, tt.want)
		}
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
