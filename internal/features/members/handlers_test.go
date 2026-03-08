package members

import (
	"context"
	"strings"
	"testing"
	"time"

	models "github.com/mymmrac/telego"

	"serotonyl.ru/telegram-bot/internal/config"
	"serotonyl.ru/telegram-bot/internal/telegram"
)

type fakeMembersTG struct {
	callbackText string
	callbackID   string
}

func (f *fakeMembersTG) SendMessage(chatID int64, text string, markup *models.InlineKeyboardMarkup) (int, error) {
	return 1, nil
}
func (f *fakeMembersTG) EditMessage(chatID int64, messageID int, text string, markup *models.InlineKeyboardMarkup) error {
	return nil
}
func (f *fakeMembersTG) EditReplyMarkup(chatID int64, messageID int, markup *models.InlineKeyboardMarkup) error {
	return nil
}
func (f *fakeMembersTG) DeleteMessage(chatID int64, messageID int) error { return nil }
func (f *fakeMembersTG) GetChatMember(chatID int64, userID int64) (models.ChatMember, error) {
	return nil, nil
}
func (f *fakeMembersTG) AnswerCallbackQuery(callbackID string, text string, showAlert bool) error {
	f.callbackID = callbackID
	f.callbackText = text
	return nil
}

type noopBalanceProvider struct{}

func (noopBalanceProvider) GetBalance(ctx context.Context, userID int64) (int64, error) {
	return 0, nil
}

func TestHandleMembersList_WrongChat_NoPanicNoop(t *testing.T) {
	h := &Handler{cfg: &config.Config{MemberSourceChatID: 100}}
	h.HandleMembersList(context.Background(), 200, 77)
}

func TestHandleMembersCallback_NonOwnerGetsNotice(t *testing.T) {
	tg := &fakeMembersTG{}
	h := NewHandler(&Service{}, noopBalanceProvider{}, telegram.NewOps(tg), &config.Config{MemberSourceChatID: 100})

	ok := h.HandleMembersCallback(context.Background(), &models.CallbackQuery{
		ID:   "cb-1",
		From: models.User{ID: 999},
		Data: "members:list:77:1",
		Message: &models.Message{
			MessageID: 10,
			Chat:      models.Chat{ID: 100},
		},
	})
	if !ok {
		t.Fatal("expected members callback handled")
	}
	if tg.callbackID == "" {
		t.Fatal("expected callback answer")
	}
	if tg.callbackText == "" {
		t.Fatal("expected non-owner notice text")
	}
}

type fakeMembersRepo struct {
	withRole    []*Member
	withoutRole []*Member
}

func (f *fakeMembersRepo) UpsertActiveMember(ctx context.Context, userID int64, username, name string, isBot bool, joinedAt time.Time) error {
	return nil
}
func (f *fakeMembersRepo) MarkMemberLeft(ctx context.Context, userID int64, leftAt, deleteAfter time.Time) error {
	return nil
}
func (f *fakeMembersRepo) IsActiveMember(ctx context.Context, userID int64) (bool, error) {
	return true, nil
}
func (f *fakeMembersRepo) PurgeExpiredLeftMembers(ctx context.Context, now time.Time, limit int) (int, error) {
	return 0, nil
}
func (f *fakeMembersRepo) GetByUserID(ctx context.Context, userID int64) (*Member, error) {
	return nil, nil
}
func (f *fakeMembersRepo) GetByUsername(ctx context.Context, username string) (*Member, error) {
	return nil, nil
}
func (f *fakeMembersRepo) EnsureMemberSeen(ctx context.Context, userID int64, username, name string, isBot bool, seenAt time.Time) error {
	return nil
}
func (f *fakeMembersRepo) EnsureActiveMemberSeen(ctx context.Context, userID int64, username, name string, isBot bool, seenAt time.Time) error {
	return nil
}
func (f *fakeMembersRepo) TouchLastSeen(ctx context.Context, userID int64, seenAt time.Time) error {
	return nil
}
func (f *fakeMembersRepo) ListActiveUserIDs(ctx context.Context) ([]int64, error) { return nil, nil }
func (f *fakeMembersRepo) ListRefreshCandidateUserIDs(ctx context.Context) ([]int64, error) {
	return nil, nil
}
func (f *fakeMembersRepo) UpdateMemberTag(ctx context.Context, userID int64, tag *string, updatedAt time.Time) error {
	return nil
}
func (f *fakeMembersRepo) CountMembersByStatus(ctx context.Context) (active int, left int, err error) {
	return 0, 0, nil
}
func (f *fakeMembersRepo) CountPendingPurge(ctx context.Context, now time.Time) (int, error) {
	return 0, nil
}
func (f *fakeMembersRepo) GetUsersWithRole(ctx context.Context) ([]*Member, error) {
	return f.withRole, nil
}
func (f *fakeMembersRepo) GetUsersWithoutRole(ctx context.Context) ([]*Member, error) {
	return f.withoutRole, nil
}

type fakeMembersBalance struct{ values map[int64]int64 }

func (f fakeMembersBalance) GetBalance(ctx context.Context, userID int64) (int64, error) {
	return f.values[userID], nil
}

type fakeMembersTGWithOptions struct {
	sentOpts []telegram.SendOptions
	editOpts []telegram.EditOptions
}

func (f *fakeMembersTGWithOptions) SendMessage(chatID int64, text string, markup *models.InlineKeyboardMarkup) (int, error) {
	return 0, nil
}
func (f *fakeMembersTGWithOptions) EditMessage(chatID int64, messageID int, text string, markup *models.InlineKeyboardMarkup) error {
	return nil
}
func (f *fakeMembersTGWithOptions) EditReplyMarkup(chatID int64, messageID int, markup *models.InlineKeyboardMarkup) error {
	return nil
}
func (f *fakeMembersTGWithOptions) DeleteMessage(chatID int64, messageID int) error { return nil }
func (f *fakeMembersTGWithOptions) GetChatMember(chatID int64, userID int64) (models.ChatMember, error) {
	return nil, nil
}
func (f *fakeMembersTGWithOptions) SendMessageWithOptions(opts telegram.SendOptions) (int, error) {
	f.sentOpts = append(f.sentOpts, opts)
	return 100, nil
}
func (f *fakeMembersTGWithOptions) EditMessageWithOptions(opts telegram.EditOptions) error {
	f.editOpts = append(f.editOpts, opts)
	return nil
}

func TestHandleMembersList_SortedAsTopWithoutBodyPageLabelAndDisabledPreview(t *testing.T) {
	repo := &fakeMembersRepo{withRole: []*Member{
		{UserID: 10, Username: "u10", Role: strPtr("Бета")},
		{UserID: 20, Username: "u20", Role: strPtr("Альфа")},
		{UserID: 5, Username: "u5", Role: strPtr("Гамма")},
	}}
	tg := &fakeMembersTGWithOptions{}
	h := NewHandler(&Service{repo: repo}, fakeMembersBalance{values: map[int64]int64{10: 5, 20: 100, 5: 5}}, telegram.NewOps(tg), &config.Config{MemberSourceChatID: 100})

	h.HandleMembersList(context.Background(), 100, 77)
	if len(tg.sentOpts) != 1 {
		t.Fatalf("expected one send call, got %d", len(tg.sentOpts))
	}
	text := tg.sentOpts[0].Text
	if strings.Contains(text, "Стр ") {
		t.Fatalf("body must not contain page label, got %q", text)
	}
	if !strings.Contains(text, "🏆 Топ участников") {
		t.Fatalf("expected top title, got %q", text)
	}
	if !tg.sentOpts[0].DisableWebPagePreview {
		t.Fatal("expected link previews disabled")
	}
	pos20 := strings.Index(text, "Альфа")
	pos5 := strings.Index(text, "Бета")
	if pos20 == -1 || pos5 == -1 || pos20 > pos5 {
		t.Fatalf("expected higher balance first, got %q", text)
	}
	pos5a := strings.Index(text, "Гамма")
	if pos5 == -1 || pos5a == -1 || pos5a > pos5 {
		t.Fatalf("expected tie-break by user_id asc, got %q", text)
	}
}

func strPtr(s string) *string { return &s }
