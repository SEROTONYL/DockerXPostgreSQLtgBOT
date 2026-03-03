package config

import "testing"

func TestConfigValidate_BotPoolBounds(t *testing.T) {
	cfg := &Config{
		FloodChatID:             1,
		MainGroupID:             2,
		BotMaxInflight:          1,
		BotUpdateTimeoutSeconds: 1,
		BotWorkers:              4,
		BotUpdateQueue:          100,
		DBMaxConns:              10,
		DBMinConns:              1,
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
}

func TestConfigValidate_BotWorkersOutOfRange(t *testing.T) {
	cfg := &Config{
		FloodChatID:             1,
		MainGroupID:             2,
		BotMaxInflight:          1,
		BotUpdateTimeoutSeconds: 1,
		BotWorkers:              0,
		BotUpdateQueue:          100,
		DBMaxConns:              10,
		DBMinConns:              1,
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected error for BOT_WORKERS out of range")
	}
}

func TestConfigValidate_BotUpdateQueueOutOfRange(t *testing.T) {
	cfg := &Config{
		FloodChatID:             1,
		MainGroupID:             2,
		BotMaxInflight:          1,
		BotUpdateTimeoutSeconds: 1,
		BotWorkers:              4,
		BotUpdateQueue:          5,
		DBMaxConns:              10,
		DBMinConns:              1,
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected error for BOT_UPDATE_QUEUE out of range")
	}
}

func TestConfigValidate_MainGroupRequired(t *testing.T) {
	cfg := &Config{
		FloodChatID:             1,
		MainGroupID:             0,
		BotMaxInflight:          1,
		BotUpdateTimeoutSeconds: 1,
		BotWorkers:              4,
		BotUpdateQueue:          100,
		DBMaxConns:              10,
		DBMinConns:              1,
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected error for MAIN_GROUP_ID == 0")
	}
}
