package telegram

import (
	"encoding/json"
	"testing"
)

func TestSendMessage_DoesNotSerializeNullReplyMarkup(t *testing.T) {
	params := buildSendMessageParams(12345, "hello", nil, nil)

	payload, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal(payload, &data); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if value, exists := data["reply_markup"]; exists {
		t.Fatalf("reply_markup must be omitted for nil markup, got: %v", value)
	}
}

func TestEditMessage_DoesNotSerializeNullReplyMarkup(t *testing.T) {
	params := buildEditMessageTextParams(12345, 42, "hello", nil, nil)

	payload, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal(payload, &data); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if value, exists := data["reply_markup"]; exists {
		t.Fatalf("reply_markup must be omitted for nil markup, got: %v", value)
	}
}
