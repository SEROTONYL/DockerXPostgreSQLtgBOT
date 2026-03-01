package admin

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	log "github.com/sirupsen/logrus"

	"serotonyl.ru/telegram-bot/internal/config"
	"serotonyl.ru/telegram-bot/internal/features/members"
)

type fakeAdminRepoFlow struct {
	hasSession bool
}

func (f *fakeAdminRepoFlow) CreateSession(ctx context.Context, session *AdminSession) error {
	return nil
}
func (f *fakeAdminRepoFlow) GetActiveSession(ctx context.Context, userID int64) (*AdminSession, error) {
	if f.hasSession {
		return &AdminSession{UserID: userID}, nil
	}
	return nil, errors.New("no session")
}
func (f *fakeAdminRepoFlow) DeactivateSession(ctx context.Context, userID int64) error { return nil }
func (f *fakeAdminRepoFlow) UpdateActivity(ctx context.Context, userID int64) error    { return nil }
func (f *fakeAdminRepoFlow) LogAttempt(ctx context.Context, userID int64, success bool) error {
	return nil
}
func (f *fakeAdminRepoFlow) GetRecentAttempts(ctx context.Context, userID int64, period time.Duration) (int, error) {
	return 0, nil
}

type fakeMemberRepoFlow struct {
	member *members.Member
}

func (f *fakeMemberRepoFlow) GetByUserID(ctx context.Context, userID int64) (*members.Member, error) {
	if f.member != nil {
		return f.member, nil
	}
	return nil, errors.New("not found")
}
func (f *fakeMemberRepoFlow) GetUsersWithoutRole(ctx context.Context) ([]*members.Member, error) {
	return nil, nil
}
func (f *fakeMemberRepoFlow) GetUsersWithRole(ctx context.Context) ([]*members.Member, error) {
	return nil, nil
}
func (f *fakeMemberRepoFlow) UpdateRole(ctx context.Context, userID int64, role string) error {
	return nil
}
func (f *fakeMemberRepoFlow) UpdateAdminFlag(ctx context.Context, userID int64, isAdmin bool) error {
	return nil
}

func TestHandleAdminMessage_LoginDeniedResponds(t *testing.T) {
	svc := NewService(&fakeAdminRepoFlow{}, &fakeMemberRepoFlow{member: &members.Member{IsAdmin: false}}, &config.Config{})
	var sent []string
	h := &Handler{
		service: svc,
		sendFn: func(c tgbotapi.Chattable) (tgbotapi.Message, error) {
			if m, ok := c.(tgbotapi.MessageConfig); ok {
				sent = append(sent, m.Text)
			}
			return tgbotapi.Message{}, nil
		},
	}

	handled := h.HandleAdminMessage(context.Background(), 1, 77, "/login")
	if !handled {
		t.Fatalf("expected handled for /login deny")
	}
	if len(sent) != 1 || sent[0] != "❌ Доступ запрещён" {
		t.Fatalf("unexpected deny response: %#v", sent)
	}
}

func TestHandleAdminMessage_LoginAllowWithoutSessionAsksPassword(t *testing.T) {
	svc := NewService(&fakeAdminRepoFlow{hasSession: false}, &fakeMemberRepoFlow{member: &members.Member{IsAdmin: true}}, &config.Config{})
	var sent []string
	h := &Handler{
		service: svc,
		sendFn: func(c tgbotapi.Chattable) (tgbotapi.Message, error) {
			if m, ok := c.(tgbotapi.MessageConfig); ok {
				sent = append(sent, m.Text)
			}
			return tgbotapi.Message{}, nil
		},
	}

	handled := h.HandleAdminMessage(context.Background(), 1, 77, "/login")
	if !handled {
		t.Fatalf("expected handled")
	}
	if len(sent) != 1 || !strings.Contains(sent[0], "Введите пароль") {
		t.Fatalf("expected password prompt, got %#v", sent)
	}
	state := svc.GetState(77)
	if state == nil || state.State != StateAwaitingPassword {
		t.Fatalf("expected awaiting_password state, got %#v", state)
	}
}

func TestHandleAdminMessage_LoginAllowWithSessionShowsMenu(t *testing.T) {
	svc := NewService(&fakeAdminRepoFlow{hasSession: true}, &fakeMemberRepoFlow{member: &members.Member{IsAdmin: true}}, &config.Config{})
	var sent []string
	h := &Handler{
		service: svc,
		sendFn: func(c tgbotapi.Chattable) (tgbotapi.Message, error) {
			if m, ok := c.(tgbotapi.MessageConfig); ok {
				sent = append(sent, m.Text)
			}
			return tgbotapi.Message{}, nil
		},
	}

	handled := h.HandleAdminMessage(context.Background(), 1, 77, "/login")
	if !handled {
		t.Fatalf("expected handled")
	}
	if len(sent) != 1 {
		t.Fatalf("expected one menu message, got %#v", sent)
	}
	if strings.TrimSpace(sent[0]) == "" {
		t.Fatalf("expected non-empty menu text")
	}
}

func TestHandleAdminMessage_LoginRegressionAlwaysResponds(t *testing.T) {
	svc := NewService(&fakeAdminRepoFlow{}, &fakeMemberRepoFlow{member: &members.Member{IsAdmin: false}}, &config.Config{})
	messages := 0
	h := &Handler{
		service: svc,
		sendFn: func(c tgbotapi.Chattable) (tgbotapi.Message, error) {
			messages++
			return tgbotapi.Message{}, nil
		},
	}

	handled := h.HandleAdminMessage(context.Background(), 1, 99, "/login secret")
	if !handled {
		t.Fatalf("expected /login to be handled")
	}
	if messages == 0 {
		t.Fatalf("expected response message for /login")
	}
}

func TestHandleAdminMessage_LoginPrefixDoesNotMatchOtherCommands(t *testing.T) {
	svc := NewService(&fakeAdminRepoFlow{}, &fakeMemberRepoFlow{member: &members.Member{IsAdmin: false}}, &config.Config{})
	messages := 0
	h := &Handler{
		service: svc,
		sendFn: func(c tgbotapi.Chattable) (tgbotapi.Message, error) {
			messages++
			return tgbotapi.Message{}, nil
		},
	}

	handled := h.HandleAdminMessage(context.Background(), 1, 99, "/login123")
	if handled {
		t.Fatalf("expected /login123 not to be treated as /login")
	}
	if messages != 0 {
		t.Fatalf("expected no response for non-login command")
	}
}

func TestHandleAdminMessage_LoginSingleStepAttemptsPassword(t *testing.T) {
	svc := NewService(&fakeAdminRepoFlow{hasSession: false}, &fakeMemberRepoFlow{member: &members.Member{IsAdmin: true}}, &config.Config{})
	var sent []string
	h := &Handler{
		service: svc,
		sendFn: func(c tgbotapi.Chattable) (tgbotapi.Message, error) {
			if m, ok := c.(tgbotapi.MessageConfig); ok {
				sent = append(sent, m.Text)
			}
			return tgbotapi.Message{}, nil
		},
	}

	handled := h.HandleAdminMessage(context.Background(), 1, 77, "/login wrongpass")
	if !handled {
		t.Fatalf("expected handled")
	}
	if len(sent) != 1 || !strings.HasPrefix(sent[0], "❌") {
		t.Fatalf("expected invalid password response, got %#v", sent)
	}
	if state := svc.GetState(77); state != nil {
		t.Fatalf("expected no awaiting_password state after single-step attempt")
	}
}

type fakeMemberRepoPicker struct {
	member           *members.Member
	usersWithoutRole []*members.Member
	usersWithRole    []*members.Member
	usersWithoutErr  error
	usersWithErr     error
}

func (f *fakeMemberRepoPicker) GetByUserID(ctx context.Context, userID int64) (*members.Member, error) {
	if f.member != nil {
		return f.member, nil
	}
	return nil, errors.New("not found")
}
func (f *fakeMemberRepoPicker) GetUsersWithoutRole(ctx context.Context) ([]*members.Member, error) {
	if f.usersWithoutErr != nil {
		return nil, f.usersWithoutErr
	}
	return f.usersWithoutRole, nil
}
func (f *fakeMemberRepoPicker) GetUsersWithRole(ctx context.Context) ([]*members.Member, error) {
	if f.usersWithErr != nil {
		return nil, f.usersWithErr
	}
	return f.usersWithRole, nil
}
func (f *fakeMemberRepoPicker) UpdateRole(ctx context.Context, userID int64, role string) error {
	return nil
}
func (f *fakeMemberRepoPicker) UpdateAdminFlag(ctx context.Context, userID int64, isAdmin bool) error {
	return nil
}

func TestAssignRole_UserButtonSelectMovesToRoleTextState(t *testing.T) {
	repo := &fakeMemberRepoPicker{
		member:           &members.Member{IsAdmin: true},
		usersWithoutRole: []*members.Member{{UserID: 101, Username: "alice", FirstName: "Alice"}},
	}
	svc := NewService(&fakeAdminRepoFlow{hasSession: true}, repo, &config.Config{})
	var sent []string
	h := &Handler{service: svc, sendFn: func(c tgbotapi.Chattable) (tgbotapi.Message, error) {
		if m, ok := c.(tgbotapi.MessageConfig); ok {
			sent = append(sent, m.Text)
		}
		return tgbotapi.Message{}, nil
	}}

	if !h.HandleAdminMessage(context.Background(), 1, 77, "Назначить роль") {
		t.Fatalf("expected handled")
	}
	if !h.HandleAdminMessage(context.Background(), 1, 77, "👤 @alice · id:101") {
		t.Fatalf("expected handled")
	}

	state := svc.GetState(77)
	if state == nil || state.State != StateAssignRoleText {
		t.Fatalf("expected %s state, got %#v", StateAssignRoleText, state)
	}
	if len(sent) == 0 || !strings.Contains(sent[len(sent)-1], "Введите роль") {
		t.Fatalf("expected role prompt, got %#v", sent)
	}
}

func TestUserPicker_PaginationNextPrev(t *testing.T) {
	users := make([]*members.Member, 0, 9)
	for i := 1; i <= 9; i++ {
		users = append(users, &members.Member{UserID: int64(1000 + i), Username: fmt.Sprintf("u%d", i), FirstName: "User"})
	}
	repo := &fakeMemberRepoPicker{member: &members.Member{IsAdmin: true}, usersWithoutRole: users}
	svc := NewService(&fakeAdminRepoFlow{hasSession: true}, repo, &config.Config{})
	h := &Handler{service: svc, sendFn: func(c tgbotapi.Chattable) (tgbotapi.Message, error) { return tgbotapi.Message{}, nil }}

	h.HandleAdminMessage(context.Background(), 1, 77, "Назначить роль")
	state := svc.GetState(77)
	if state == nil {
		t.Fatalf("expected state")
	}
	data, ok := state.Data.(*UserPickerData)
	if !ok {
		t.Fatalf("expected picker data")
	}
	if data.PageIndex != 0 {
		t.Fatalf("expected page 0")
	}

	h.HandleAdminMessage(context.Background(), 1, 77, userPickerNextButton)
	if data.PageIndex != 1 {
		t.Fatalf("expected page 1, got %d", data.PageIndex)
	}

	h.HandleAdminMessage(context.Background(), 1, 77, userPickerPrevButton)
	if data.PageIndex != 0 {
		t.Fatalf("expected page 0 after prev, got %d", data.PageIndex)
	}
}

func TestUserPicker_BackClearsStateAndShowsMenu(t *testing.T) {
	repo := &fakeMemberRepoPicker{
		member:           &members.Member{IsAdmin: true},
		usersWithoutRole: []*members.Member{{UserID: 101, Username: "alice", FirstName: "Alice"}},
	}
	svc := NewService(&fakeAdminRepoFlow{hasSession: true}, repo, &config.Config{})
	var markups []interface{}
	h := &Handler{service: svc, sendFn: func(c tgbotapi.Chattable) (tgbotapi.Message, error) {
		if m, ok := c.(tgbotapi.MessageConfig); ok {
			markups = append(markups, m.ReplyMarkup)
		}
		return tgbotapi.Message{}, nil
	}}

	h.HandleAdminMessage(context.Background(), 1, 77, "Назначить роль")
	h.HandleAdminMessage(context.Background(), 1, 77, userPickerBackButton)

	if state := svc.GetState(77); state != nil {
		t.Fatalf("expected cleared state")
	}
	if len(markups) == 0 {
		t.Fatalf("expected keyboard markup messages")
	}
}

func TestUserPicker_InvalidInputReRenders(t *testing.T) {
	repo := &fakeMemberRepoPicker{
		member:           &members.Member{IsAdmin: true},
		usersWithoutRole: []*members.Member{{UserID: 101, Username: "alice", FirstName: "Alice"}},
	}
	svc := NewService(&fakeAdminRepoFlow{hasSession: true}, repo, &config.Config{})
	var sent []string
	h := &Handler{service: svc, sendFn: func(c tgbotapi.Chattable) (tgbotapi.Message, error) {
		if m, ok := c.(tgbotapi.MessageConfig); ok {
			sent = append(sent, m.Text)
		}
		return tgbotapi.Message{}, nil
	}}

	h.HandleAdminMessage(context.Background(), 1, 77, "Назначить роль")
	h.HandleAdminMessage(context.Background(), 1, 77, "какой-то текст")

	found := false
	for _, s := range sent {
		if strings.Contains(s, "Некорректный выбор") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected invalid selection message, got %#v", sent)
	}
}

func TestUserPicker_PageLabelClickNoInvalidSelectionMessageAndKeepsState(t *testing.T) {
	users := make([]*members.Member, 0, 9)
	for i := 1; i <= 9; i++ {
		users = append(users, &members.Member{UserID: int64(3000 + i), Username: fmt.Sprintf("p%d", i), FirstName: "Page"})
	}
	repo := &fakeMemberRepoPicker{member: &members.Member{IsAdmin: true}, usersWithoutRole: users}
	svc := NewService(&fakeAdminRepoFlow{hasSession: true}, repo, &config.Config{})
	var sent []string
	h := &Handler{service: svc, sendFn: func(c tgbotapi.Chattable) (tgbotapi.Message, error) {
		if m, ok := c.(tgbotapi.MessageConfig); ok {
			sent = append(sent, m.Text)
		}
		return tgbotapi.Message{}, nil
	}}

	h.HandleAdminMessage(context.Background(), 1, 77, "Назначить роль")
	h.HandleAdminMessage(context.Background(), 1, 77, userPickerNextButton)

	stateBefore := svc.GetState(77)
	if stateBefore == nil || stateBefore.State != StateAssignRoleSelect {
		t.Fatalf("expected picker state before page-label click, got %#v", stateBefore)
	}
	dataBefore, ok := stateBefore.Data.(*UserPickerData)
	if !ok {
		t.Fatalf("expected picker data before page-label click")
	}
	if dataBefore.PageIndex != 1 {
		t.Fatalf("expected second page before label click, got %d", dataBefore.PageIndex)
	}

	sentCountBeforeLabelClick := len(sent)
	h.HandleAdminMessage(context.Background(), 1, 77, "  сТР  2 / 2   ")

	stateAfter := svc.GetState(77)
	if stateAfter == nil || stateAfter.State != StateAssignRoleSelect {
		t.Fatalf("expected picker state after page-label click, got %#v", stateAfter)
	}
	dataAfter, ok := stateAfter.Data.(*UserPickerData)
	if !ok {
		t.Fatalf("expected picker data after page-label click")
	}
	if dataAfter.PageIndex != 1 {
		t.Fatalf("expected page index unchanged after label click, got %d", dataAfter.PageIndex)
	}

	for _, s := range sent {
		if strings.Contains(s, "Некорректный выбор") {
			t.Fatalf("did not expect invalid-selection message for page label click, got %#v", sent)
		}
	}
	if len(sent) != sentCountBeforeLabelClick {
		t.Fatalf("expected page-label click to be no-op without new messages, before=%d after=%d", sentCountBeforeLabelClick, len(sent))
	}
}

func TestUserPicker_ExpiredStateHandledGracefully(t *testing.T) {
	repo := &fakeMemberRepoPicker{member: &members.Member{IsAdmin: true}}
	svc := NewService(&fakeAdminRepoFlow{hasSession: true}, repo, &config.Config{})
	var sent []string
	h := &Handler{service: svc, sendFn: func(c tgbotapi.Chattable) (tgbotapi.Message, error) {
		if m, ok := c.(tgbotapi.MessageConfig); ok {
			sent = append(sent, m.Text)
		}
		return tgbotapi.Message{}, nil
	}}

	svc.SetState(77, StateAssignRoleSelect, &UserPickerData{Mode: UserPickerAssignWithoutRole, UsersSnapshot: []*members.Member{{UserID: 1}}, PageSize: 8})
	svc.states[77].ExpiresAt = time.Now().Add(-time.Minute)

	h.handleAssignRoleSelect(context.Background(), 1, 77, "👤 test · id:1")

	if len(sent) == 0 || !strings.Contains(sent[0], "Состояние сброшено") {
		t.Fatalf("expected graceful reset message, got %#v", sent)
	}
}

func TestStartAssignRole_EmptyListShowsMessage(t *testing.T) {
	repo := &fakeMemberRepoPicker{member: &members.Member{IsAdmin: true}, usersWithoutRole: []*members.Member{}}
	svc := NewService(&fakeAdminRepoFlow{hasSession: true}, repo, &config.Config{})
	var sent []string
	h := &Handler{service: svc, sendFn: func(c tgbotapi.Chattable) (tgbotapi.Message, error) {
		if m, ok := c.(tgbotapi.MessageConfig); ok {
			sent = append(sent, m.Text)
		}
		return tgbotapi.Message{}, nil
	}}

	h.HandleAdminMessage(context.Background(), 1, 77, "Назначить роль")

	if len(sent) == 0 || !strings.Contains(sent[0], "Все пользователи уже имеют роли") {
		t.Fatalf("expected empty-list message, got %#v", sent)
	}
	if state := svc.GetState(77); state != nil {
		t.Fatalf("expected no picker state for empty list")
	}
}

func TestStartAssignRole_ErrorShowsErrorMessage(t *testing.T) {
	repo := &fakeMemberRepoPicker{member: &members.Member{IsAdmin: true}, usersWithoutErr: errors.New("db down")}
	svc := NewService(&fakeAdminRepoFlow{hasSession: true}, repo, &config.Config{})
	var sent []string
	h := &Handler{service: svc, sendFn: func(c tgbotapi.Chattable) (tgbotapi.Message, error) {
		if m, ok := c.(tgbotapi.MessageConfig); ok {
			sent = append(sent, m.Text)
		}
		return tgbotapi.Message{}, nil
	}}

	h.HandleAdminMessage(context.Background(), 1, 77, "Назначить роль")

	found := false
	for _, msg := range sent {
		if strings.Contains(msg, "Ошибка получения списка пользователей") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected fetch error message, got %#v", sent)
	}
	if state := svc.GetState(77); state != nil {
		t.Fatalf("expected no picker state on fetch error")
	}
}

func TestStartChangeRole_EmptyListShowsMessage(t *testing.T) {
	repo := &fakeMemberRepoPicker{member: &members.Member{IsAdmin: true}, usersWithRole: []*members.Member{}}
	svc := NewService(&fakeAdminRepoFlow{hasSession: true}, repo, &config.Config{})
	var sent []string
	h := &Handler{service: svc, sendFn: func(c tgbotapi.Chattable) (tgbotapi.Message, error) {
		if m, ok := c.(tgbotapi.MessageConfig); ok {
			sent = append(sent, m.Text)
		}
		return tgbotapi.Message{}, nil
	}}

	h.HandleAdminMessage(context.Background(), 1, 77, "Сменить роль")

	if len(sent) == 0 || !strings.Contains(sent[0], "Нет пользователей с назначенными ролями") {
		t.Fatalf("expected empty-list message, got %#v", sent)
	}
	if state := svc.GetState(77); state != nil {
		t.Fatalf("expected no picker state for empty list")
	}
}

func TestStartChangeRole_ErrorShowsErrorMessage(t *testing.T) {
	repo := &fakeMemberRepoPicker{member: &members.Member{IsAdmin: true}, usersWithErr: errors.New("query failed")}
	svc := NewService(&fakeAdminRepoFlow{hasSession: true}, repo, &config.Config{})
	var sent []string
	h := &Handler{service: svc, sendFn: func(c tgbotapi.Chattable) (tgbotapi.Message, error) {
		if m, ok := c.(tgbotapi.MessageConfig); ok {
			sent = append(sent, m.Text)
		}
		return tgbotapi.Message{}, nil
	}}

	h.HandleAdminMessage(context.Background(), 1, 77, "Сменить роль")

	found := false
	for _, msg := range sent {
		if strings.Contains(msg, "Ошибка получения списка пользователей") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected fetch error message, got %#v", sent)
	}
	if state := svc.GetState(77); state != nil {
		t.Fatalf("expected no picker state on fetch error")
	}
}

func TestUserPicker_PaginationEdgesStayInBounds(t *testing.T) {
	users := make([]*members.Member, 0, 9)
	for i := 1; i <= 9; i++ {
		users = append(users, &members.Member{UserID: int64(2000 + i), Username: fmt.Sprintf("edge%d", i), FirstName: "Edge"})
	}
	repo := &fakeMemberRepoPicker{member: &members.Member{IsAdmin: true}, usersWithoutRole: users}
	svc := NewService(&fakeAdminRepoFlow{hasSession: true}, repo, &config.Config{})
	h := &Handler{service: svc, sendFn: func(c tgbotapi.Chattable) (tgbotapi.Message, error) { return tgbotapi.Message{}, nil }}

	h.HandleAdminMessage(context.Background(), 1, 77, "Назначить роль")
	state := svc.GetState(77)
	data := state.Data.(*UserPickerData)

	// already at first page: prev should keep 0
	h.HandleAdminMessage(context.Background(), 1, 77, userPickerPrevButton)
	if data.PageIndex != 0 {
		t.Fatalf("expected first page to stay at 0, got %d", data.PageIndex)
	}

	// go to last page
	h.HandleAdminMessage(context.Background(), 1, 77, userPickerNextButton)
	if data.PageIndex != 1 {
		t.Fatalf("expected to move to last page 1, got %d", data.PageIndex)
	}

	// already at last page: next should stay last
	h.HandleAdminMessage(context.Background(), 1, 77, userPickerNextButton)
	if data.PageIndex != 1 {
		t.Fatalf("expected last page to stay at 1, got %d", data.PageIndex)
	}
}

func TestUserPicker_LongNameTruncation(t *testing.T) {
	longName := strings.Repeat("А", 80)
	btn := formatUserPickerButton(&members.Member{UserID: 12345, FirstName: longName})
	if !strings.Contains(btn, "[12345]") {
		t.Fatalf("expected stable id marker, got %q", btn)
	}
	if len([]rune(btn)) > 40 {
		t.Fatalf("expected truncated button text, got %q", btn)
	}
}

func TestFormatMemberForPicker_UsernameAndIDFallback(t *testing.T) {
	role := "модератор"
	withUsername := &members.Member{UserID: 101, Username: "alice", Role: &role}
	withoutUsername := &members.Member{UserID: 202, Role: &role}

	if got := formatMemberForPicker(withUsername); got != "[модератор] @alice" {
		t.Fatalf("unexpected username format: %q", got)
	}
	if got := formatMemberForPicker(withoutUsername); got != "[модератор] [202]" {
		t.Fatalf("unexpected id fallback format: %q", got)
	}
}

func TestFormatMemberForPicker_NormalizesAtPrefix(t *testing.T) {
	role := "админ"
	member := &members.Member{UserID: 303, Username: "@bob", Role: &role}
	if got := formatMemberForPicker(member); got != "[админ] @bob" {
		t.Fatalf("unexpected normalized username format: %q", got)
	}
}

func TestFormatMemberForPicker_DoesNotUppercaseRole(t *testing.T) {
	role := "Мяу"
	member := &members.Member{UserID: 404, Username: "kysxDDD", Role: &role}

	got := formatMemberForPicker(member)
	if got != "[Мяу] @kysxDDD" {
		t.Fatalf("unexpected role formatting: %q", got)
	}
	if strings.Contains(got, "[МЯУ]") {
		t.Fatalf("role must not be uppercased: %q", got)
	}
}

func TestShowKeyboard_EditOK_DoesNotSendNewMessage(t *testing.T) {
	svc := NewService(&fakeAdminRepoFlow{hasSession: true}, &fakeMemberRepoFlow{member: &members.Member{IsAdmin: true}}, &config.Config{})
	sendCalls := 0
	h := &Handler{
		service: svc,
		sendFn: func(c tgbotapi.Chattable) (tgbotapi.Message, error) {
			sendCalls++
			return tgbotapi.Message{}, nil
		},
		editFn: func(chatID int64, messageID int, text string, keyboard tgbotapi.InlineKeyboardMarkup) error {
			return nil
		},
	}

	if err := h.showKeyboard(1, 77, 123); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sendCalls != 0 {
		t.Fatalf("send should not be called when edit succeeds, got %d", sendCalls)
	}
}

func TestShowKeyboard_EditNotModified_DoesNotSendNewMessage(t *testing.T) {
	svc := NewService(&fakeAdminRepoFlow{hasSession: true}, &fakeMemberRepoFlow{member: &members.Member{IsAdmin: true}}, &config.Config{})
	sendCalls := 0
	h := &Handler{
		service: svc,
		sendFn: func(c tgbotapi.Chattable) (tgbotapi.Message, error) {
			sendCalls++
			return tgbotapi.Message{}, nil
		},
		editFn: func(chatID int64, messageID int, text string, keyboard tgbotapi.InlineKeyboardMarkup) error {
			return &tgbotapi.Error{Code: 400, Message: "Bad Request: message is not modified"}
		},
	}

	if err := h.showKeyboard(1, 77, 123); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sendCalls != 0 {
		t.Fatalf("send should not be called for not modified, got %d", sendCalls)
	}
}

func TestShowKeyboard_EditNotFound_FallsBackToSendAndUpdatesPanelID(t *testing.T) {
	svc := NewService(&fakeAdminRepoFlow{hasSession: true}, &fakeMemberRepoFlow{member: &members.Member{IsAdmin: true}}, &config.Config{})
	sendCalls := 0
	h := &Handler{
		service: svc,
		sendFn: func(c tgbotapi.Chattable) (tgbotapi.Message, error) {
			sendCalls++
			return tgbotapi.Message{MessageID: 555}, nil
		},
		editFn: func(chatID int64, messageID int, text string, keyboard tgbotapi.InlineKeyboardMarkup) error {
			return &tgbotapi.Error{Code: 400, Message: "Bad Request: message to edit not found"}
		},
	}

	if err := h.showKeyboard(1, 77, 123); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sendCalls != 1 {
		t.Fatalf("expected send fallback once, got %d", sendCalls)
	}
	if got := svc.GetPanelMessageID(77); got != 555 {
		t.Fatalf("expected updated panel message id=555, got %d", got)
	}
}

func TestShowKeyboard_EditForbidden_ReturnsErrorAndDoesNotSend(t *testing.T) {
	svc := NewService(&fakeAdminRepoFlow{hasSession: true}, &fakeMemberRepoFlow{member: &members.Member{IsAdmin: true}}, &config.Config{})
	sendCalls := 0
	h := &Handler{
		service: svc,
		sendFn: func(c tgbotapi.Chattable) (tgbotapi.Message, error) {
			sendCalls++
			return tgbotapi.Message{}, nil
		},
		editFn: func(chatID int64, messageID int, text string, keyboard tgbotapi.InlineKeyboardMarkup) error {
			return &tgbotapi.Error{Code: 403, Message: "Forbidden: bot was blocked by the user"}
		},
	}

	var buf bytes.Buffer
	originalOut := log.StandardLogger().Out
	log.SetOutput(&buf)
	defer log.SetOutput(originalOut)

	err := h.showKeyboard(1, 77, 123)
	if err == nil {
		t.Fatalf("expected forbidden error")
	}
	if sendCalls != 0 {
		t.Fatalf("send should not be called for forbidden edit, got %d", sendCalls)
	}
	if buf.Len() == 0 {
		t.Fatalf("expected diagnostic log entry")
	}
}

func TestClassifyEditError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want editErrorKind
	}{
		{name: "not modified", err: &tgbotapi.Error{Code: 400, Message: "Bad Request: message is not modified"}, want: editErrNotModified},
		{name: "not found", err: &tgbotapi.Error{Code: 400, Message: "Bad Request: message to edit not found"}, want: editErrNotFound},
		{name: "forbidden", err: &tgbotapi.Error{Code: 403, Message: "Forbidden: bot was blocked by the user"}, want: editErrForbidden},
		{name: "other", err: errors.New("internal server error"), want: editErrOther},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, _ := classifyEditError(tt.err)
			if got != tt.want {
				t.Fatalf("want %s, got %s", tt.want, got)
			}
		})
	}
}

func TestClassifyEditError_ForbiddenCaseInsensitive(t *testing.T) {
	tests := []string{
		"Bot was blocked by the user",
		"bot was blocked by the user",
		"FORBIDDEN: bot was blocked by the user",
	}

	for _, msg := range tests {
		t.Run(msg, func(t *testing.T) {
			err := &tgbotapi.Error{Code: 403, Message: msg}
			kind, code, text := classifyEditError(err)
			if kind != editErrForbidden {
				t.Fatalf("expected forbidden kind, got %s", kind)
			}
			if code != 403 {
				t.Fatalf("expected code 403, got %d", code)
			}
			if text != msg {
				t.Fatalf("expected original text %q, got %q", msg, text)
			}
		})
	}
}
