package config

import "testing"

func baseValidConfig() *Config {
	return &Config{
		MemberSourceChatID:      2,
		AdminChatID:             3,
		BotMaxInflight:          1,
		BotUpdateTimeoutSeconds: 1,
		BotWorkers:              4,
		BotUpdateQueue:          100,
		DBMaxConns:              10,
		DBMinConns:              1,
	}
}

func TestConfigValidate_BotPoolBounds(t *testing.T) {
	cfg := baseValidConfig()

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
	if cfg.MemberSourceChatID != 2 {
		t.Fatalf("expected MemberSourceChatID to stay 2, got %d", cfg.MemberSourceChatID)
	}
}

func TestConfigValidate_BotWorkersOutOfRange(t *testing.T) {
	cfg := baseValidConfig()
	cfg.BotWorkers = 0

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected error for BOT_WORKERS out of range")
	}
}

func TestConfigValidate_BotUpdateQueueOutOfRange(t *testing.T) {
	cfg := baseValidConfig()
	cfg.BotUpdateQueue = 5

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected error for BOT_UPDATE_QUEUE out of range")
	}
}

func TestConfigValidate_MemberSourceChatRequired(t *testing.T) {
	cfg := baseValidConfig()
	cfg.MemberSourceChatID = 0

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected error for MEMBER_SOURCE_CHAT_ID == 0")
	}
}

func TestConfigValidate_AdminChatRequired(t *testing.T) {
	cfg := baseValidConfig()
	cfg.AdminChatID = 0

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected error for ADMIN_CHAT_ID == 0")
	}
}

func TestConfigValidate_MemberSourceAndAdminChatMustDiffer(t *testing.T) {
	cfg := baseValidConfig()
	cfg.AdminChatID = cfg.MemberSourceChatID

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected error when MEMBER_SOURCE_CHAT_ID equals ADMIN_CHAT_ID")
	}
}
