package bot

import (
	"testing"
	"time"

	models "github.com/mymmrac/telego"

	"serotonyl.ru/telegram-bot/internal/config"
)

func TestBuildUpdateContext_Message(t *testing.T) {
	now := time.Now().UTC()
	cfg := &config.Config{AdminChatID: 100}
	upd := models.Update{Message: &models.Message{
		Chat: models.Chat{ID: 100, Type: models.ChatTypePrivate},
		From: &models.User{ID: 77, Username: "admin", FirstName: "Ada", LastName: "Lovelace"},
		Text: "hello",
	}}

	uc := BuildUpdateContext(upd, now, cfg)
	if uc.ChatID != 100 || uc.UserID != 77 {
		t.Fatalf("unexpected ids: chat=%d user=%d", uc.ChatID, uc.UserID)
	}
	if !uc.IsPrivate || uc.IsGroup {
		t.Fatalf("unexpected chat flags: private=%v group=%v", uc.IsPrivate, uc.IsGroup)
	}
	if !uc.IsAdminChat {
		t.Fatal("expected IsAdminChat=true when chatID matches ADMIN_CHAT_ID")
	}
	if uc.FullName != "Ada Lovelace" {
		t.Fatalf("unexpected full name: %q", uc.FullName)
	}
}

func TestBuildUpdateContext_Callback(t *testing.T) {
	now := time.Now().UTC()
	cfg := &config.Config{}
	upd := models.Update{CallbackQuery: &models.CallbackQuery{
		ID:      "cb1",
		From:    models.User{ID: 500, Username: "u500", FirstName: "Test"},
		Message: &models.Message{Chat: models.Chat{ID: 999, Type: models.ChatTypeSupergroup}},
	}}

	uc := BuildUpdateContext(upd, now, cfg)
	if uc.Callback == nil {
		t.Fatal("expected callback pointer")
	}
	if uc.ChatID != 999 || uc.UserID != 500 {
		t.Fatalf("unexpected ids: chat=%d user=%d", uc.ChatID, uc.UserID)
	}
	if uc.IsPrivate || !uc.IsGroup {
		t.Fatalf("unexpected chat flags: private=%v group=%v", uc.IsPrivate, uc.IsGroup)
	}
}

func TestBuildUpdateContext_ChatMember(t *testing.T) {
	now := time.Now().UTC()
	cfg := &config.Config{}
	user := &models.User{ID: 321, Username: "member", FirstName: "Mem", LastName: "Ber"}
	upd := models.Update{ChatMember: &models.ChatMemberUpdated{
		Chat:          models.Chat{ID: -100123, Type: models.ChatTypeSupergroup},
		NewChatMember: &models.ChatMemberMember{Status: "member", User: *user},
	}}

	uc := BuildUpdateContext(upd, now, cfg)
	if uc.ChatMember == nil {
		t.Fatal("expected chat member pointer")
	}
	if uc.ChatID != -100123 || uc.UserID != 321 {
		t.Fatalf("unexpected ids: chat=%d user=%d", uc.ChatID, uc.UserID)
	}
	if uc.FullName != "Mem Ber" {
		t.Fatalf("unexpected full name: %q", uc.FullName)
	}
}

func TestBuildUpdateContext_CallbackWithInaccessibleMessage(t *testing.T) {
	now := time.Now().UTC()
	cfg := &config.Config{}
	upd := models.Update{CallbackQuery: &models.CallbackQuery{
		ID:   "cb1",
		From: models.User{ID: 500, Username: "u500", FirstName: "Test"},
		Message: &models.InaccessibleMessage{
			Chat: models.Chat{ID: -200, Type: models.ChatTypePrivate},
		},
	}}

	uc := BuildUpdateContext(upd, now, cfg)
	if uc.ChatID != -200 || uc.UserID != 500 {
		t.Fatalf("unexpected ids: chat=%d user=%d", uc.ChatID, uc.UserID)
	}
	if !uc.IsPrivate || uc.IsGroup {
		t.Fatalf("unexpected chat flags: private=%v group=%v", uc.IsPrivate, uc.IsGroup)
	}
}
