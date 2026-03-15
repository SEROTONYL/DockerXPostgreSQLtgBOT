package config

import "testing"

func TestParseIDList(t *testing.T) {
	ids := parseIDList(" 123 , bad, , 456 , 123, 7x, 789 ")

	if len(ids) != 3 {
		t.Fatalf("expected 3 valid IDs, got %d", len(ids))
	}
	for _, id := range []int64{123, 456, 789} {
		if _, ok := ids[id]; !ok {
			t.Fatalf("expected ID %d to be parsed", id)
		}
	}
}

func TestParseIDList_EdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []int64
	}{
		{name: "simple", input: "123,456", want: []int64{123, 456}},
		{name: "trimmed", input: "123, 456 , 789", want: []int64{123, 456, 789}},
		{name: "ignores-empty-and-invalid", input: "123,,456,abc", want: []int64{123, 456}},
		{name: "empty", input: "", want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseIDList(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("len(parseIDList(%q)) = %d, want %d", tt.input, len(got), len(tt.want))
			}
			for _, id := range tt.want {
				if _, ok := got[id]; !ok {
					t.Fatalf("parseIDList(%q) missing id %d", tt.input, id)
				}
			}
		})
	}
}

func TestConfigValidate_LeaveDebug(t *testing.T) {
	tests := []struct {
		name    string
		value   int64
		wantErr bool
	}{
		{name: "disabled", value: 0, wantErr: false},
		{name: "positive", value: 123456789, wantErr: false},
		{name: "negative", value: -1, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				MemberSourceChatID:      -1001,
				AdminChatID:             -2002,
				LeaveDebug:              tt.value,
				BotMaxInflight:          1,
				BotUpdateTimeoutSeconds: 1,
				BotWorkers:              minBotWorkers,
				BotUpdateQueue:          minBotUpdateQueue,
				DBMaxConns:              1,
				DBMinConns:              0,
			}

			err := cfg.Validate()
			if tt.wantErr && err == nil {
				t.Fatal("expected validation error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}
