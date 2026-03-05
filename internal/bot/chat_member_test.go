package bot

import (
	"testing"

	models "github.com/mymmrac/telego"
)

func TestClassifyMemberStatus(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   membershipAction
	}{
		{name: "owner active", status: "creator", want: membershipActionActive},
		{name: "admin active", status: "administrator", want: membershipActionActive},
		{name: "member active", status: "member", want: membershipActionActive},
		{name: "left", status: "left", want: membershipActionLeft},
		{name: "kicked", status: "kicked", want: membershipActionLeft},
		{name: "restricted treated as left", status: "restricted", want: membershipActionLeft},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyMemberStatus(tc.status); got != tc.want {
				t.Fatalf("classifyMemberStatus(%q) = %q, want %q", tc.status, got, tc.want)
			}
		})
	}
}

func TestExtractChatMemberUpdate(t *testing.T) {
	chatMemberUpdate := &models.ChatMemberUpdated{Chat: models.Chat{ID: 100}}
	myChatMemberUpdate := &models.ChatMemberUpdated{Chat: models.Chat{ID: 200}}

	if got := extractChatMemberUpdate(models.Update{ChatMember: chatMemberUpdate}); got != chatMemberUpdate {
		t.Fatal("expected ChatMember update to be returned")
	}
	if got := extractChatMemberUpdate(models.Update{MyChatMember: myChatMemberUpdate}); got != myChatMemberUpdate {
		t.Fatal("expected MyChatMember update to be returned")
	}
	if got := extractChatMemberUpdate(models.Update{}); got != nil {
		t.Fatal("expected nil when no membership update present")
	}
}

func TestChatMemberUser(t *testing.T) {
	u := models.User{ID: 42, Username: "john", FirstName: "John", LastName: "Doe"}

	member := &models.ChatMemberMember{Status: "member", User: u}
	got, ok := chatMemberUser(member)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if got.ID != u.ID || got.Username != u.Username {
		t.Fatalf("unexpected user: %+v", got)
	}
}
