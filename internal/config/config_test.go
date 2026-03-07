package config

import "testing"

func TestConfigValidate_BotPoolBounds(t *testing.T) {
	cfg := &Config{
		MemberSourceChatID:      2,
		AdminChatID:             3,
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
	if cfg.MemberSourceChatID != 2 || cfg.MainGroupID != 2 || cfg.FloodChatID != 2 {
		t.Fatalf("expected normalized chat IDs to be 2, got member_source=%d main=%d flood=%d", cfg.MemberSourceChatID, cfg.MainGroupID, cfg.FloodChatID)
	}
}

func TestConfigValidate_BotWorkersOutOfRange(t *testing.T) {
	cfg := &Config{
		MemberSourceChatID:      2,
		AdminChatID:             3,
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
		MemberSourceChatID:      2,
		AdminChatID:             3,
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

func TestConfigValidate_MemberSourceChatRequired(t *testing.T) {
	cfg := &Config{
		MemberSourceChatID:      0,
		BotMaxInflight:          1,
		BotUpdateTimeoutSeconds: 1,
		BotWorkers:              4,
		BotUpdateQueue:          100,
		DBMaxConns:              10,
		DBMinConns:              1,
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected error for MEMBER_SOURCE_CHAT_ID == 0 without aliases")
	}
}

func TestConfigValidate_AdminChatRequired(t *testing.T) {
	cfg := &Config{
		MemberSourceChatID:      2,
		AdminChatID:             0,
		BotMaxInflight:          1,
		BotUpdateTimeoutSeconds: 1,
		BotWorkers:              4,
		BotUpdateQueue:          100,
		DBMaxConns:              10,
		DBMinConns:              1,
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected error for ADMIN_CHAT_ID == 0")
	}
}

func TestConfigValidate_LegacyAliasesStillResolveMemberSourceChatID(t *testing.T) {
	cfg := &Config{
		MainGroupID:             2,
		FloodChatID:             2,
		AdminChatID:             3,
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
	if cfg.MemberSourceChatID != 2 {
		t.Fatalf("expected MemberSourceChatID=2 from legacy aliases, got %d", cfg.MemberSourceChatID)
	}
	if cfg.MainGroupID != 2 || cfg.FloodChatID != 2 {
		t.Fatalf("expected legacy fields normalized to 2, got main=%d flood=%d", cfg.MainGroupID, cfg.FloodChatID)
	}
}

func TestConfigValidate_LegacyAliasConflictRequiresExplicitMemberSourceChatID(t *testing.T) {
	cfg := &Config{
		MainGroupID:             2,
		FloodChatID:             9,
		AdminChatID:             3,
		BotMaxInflight:          1,
		BotUpdateTimeoutSeconds: 1,
		BotWorkers:              4,
		BotUpdateQueue:          100,
		DBMaxConns:              10,
		DBMinConns:              1,
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected error for conflicting legacy aliases")
	}
}

func TestConfigValidate_OnlyMemberSourceConfigured_BackfillsLegacyAliases(t *testing.T) {
	cfg := &Config{
		MemberSourceChatID:      42,
		AdminChatID:             3,
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
	if cfg.MemberSourceChatID != 42 || cfg.MainGroupID != 42 || cfg.FloodChatID != 42 {
		t.Fatalf("expected all participant chat IDs to be 42, got member_source=%d main=%d flood=%d", cfg.MemberSourceChatID, cfg.MainGroupID, cfg.FloodChatID)
	}
}

func TestConfigValidate_OnlyMainGroupConfigured_DerivesCanonical(t *testing.T) {
	cfg := &Config{
		MainGroupID:             77,
		AdminChatID:             3,
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
	if cfg.MemberSourceChatID != 77 || cfg.MainGroupID != 77 || cfg.FloodChatID != 77 {
		t.Fatalf("expected all participant chat IDs to be 77, got member_source=%d main=%d flood=%d", cfg.MemberSourceChatID, cfg.MainGroupID, cfg.FloodChatID)
	}
}

func TestConfigValidate_OnlyFloodConfigured_DerivesCanonical(t *testing.T) {
	cfg := &Config{
		FloodChatID:             88,
		AdminChatID:             3,
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
	if cfg.MemberSourceChatID != 88 || cfg.MainGroupID != 88 || cfg.FloodChatID != 88 {
		t.Fatalf("expected all participant chat IDs to be 88, got member_source=%d main=%d flood=%d", cfg.MemberSourceChatID, cfg.MainGroupID, cfg.FloodChatID)
	}
}

func TestConfigValidate_MemberSourceConflictsWithLegacyAlias_Fails(t *testing.T) {
	cfg := &Config{
		MemberSourceChatID:      11,
		MainGroupID:             12,
		AdminChatID:             3,
		BotMaxInflight:          1,
		BotUpdateTimeoutSeconds: 1,
		BotWorkers:              4,
		BotUpdateQueue:          100,
		DBMaxConns:              10,
		DBMinConns:              1,
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() expected error for MEMBER_SOURCE_CHAT_ID vs legacy alias conflict")
	}
}
