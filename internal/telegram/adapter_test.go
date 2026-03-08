package telegram

import (
	"encoding/json"
	"testing"
)

func TestSendMessage_DoesNotSerializeNullReplyMarkup(t *testing.T) {
	params := buildSendMessageParams(SendOptions{ChatID: 12345, Text: "hello"})

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
	params := buildEditMessageTextParams(EditOptions{ChatID: 12345, MessageID: 42, Text: "hello"})

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

func TestSendMessage_WithReplyAndDisabledPreview(t *testing.T) {
	params := buildSendMessageParams(SendOptions{ChatID: 12345, Text: "hello", ReplyToMessageID: 77, DisableWebPagePreview: true})
	if params.ReplyParameters == nil || params.ReplyParameters.MessageID != 77 {
		t.Fatalf("expected reply parameters with message_id=77, got %#v", params.ReplyParameters)
	}
	if params.LinkPreviewOptions == nil || !params.LinkPreviewOptions.IsDisabled {
		t.Fatalf("expected disabled link preview, got %#v", params.LinkPreviewOptions)
	}
}
