package config

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kelseyhightower/envconfig"
)

const (
	minBotWorkers = 1
	maxBotWorkers = 64

	minBotUpdateQueue = 10
	maxBotUpdateQueue = 5000
)

type Config struct {
	// Telegram
	AdminIDsRaw     string             `envconfig:"ADMIN_IDS" required:"true"`
	AdminIDs        []int64            `envconfig:"-"`
	AdminIDSet      map[int64]struct{} `envconfig:"-"`
	ModeratorIDsRaw string             `envconfig:"MODERATOR_IDS" default:""`
	ModeratorIDs    []int64            `envconfig:"-"`
	ModeratorIDSet  map[int64]struct{} `envconfig:"-"`

	TelegramBotToken   string `envconfig:"TELEGRAM_BOT_TOKEN" required:"true"`
	MemberSourceChatID int64  `envconfig:"MEMBER_SOURCE_CHAT_ID" default:"0"`
	AdminChatID        int64  `envconfig:"ADMIN_CHAT_ID" required:"true"`
	LeaveDebug         int64  `envconfig:"LeaveDebug" default:"0"`

	// Database
	DBHost     string `envconfig:"DB_HOST" default:"postgres"`
	DBPort     int    `envconfig:"DB_PORT" default:"5432"`
	DBUser     string `envconfig:"DB_USER" default:"botuser"`
	DBPassword string `envconfig:"DB_PASSWORD" required:"true"`
	DBName     string `envconfig:"DB_NAME" default:"telegram_bot"`
	DBSSLMode  string `envconfig:"DB_SSLMODE" default:"disable"`
	DBMaxConns int32  `envconfig:"DB_MAX_CONNS" default:"25"`
	DBMinConns int32  `envconfig:"DB_MIN_CONNS" default:"5"`

	// Application
	AppEnv      string `envconfig:"APP_ENV" default:"development"`
	AppLogLevel string `envconfig:"APP_LOG_LEVEL" default:"debug"`
	AppTimezone string `envconfig:"APP_TIMEZONE" default:"Europe/Moscow"`

	// Bot runtime
	BotMaxInflight          int `envconfig:"BOT_MAX_INFLIGHT" default:"64"`
	BotUpdateTimeoutSeconds int `envconfig:"BOT_UPDATE_TIMEOUT_SECONDS" default:"60"`
	BotWorkers              int `envconfig:"BOT_WORKERS" default:"4"`
	BotUpdateQueue          int `envconfig:"BOT_UPDATE_QUEUE" default:"100"`

	// Admin
	AdminPasswordHash string `envconfig:"ADMIN_PASSWORD_HASH" required:"true"`

	// Streak
	StreakMessagesNeed      int `envconfig:"STREAK_MESSAGES_NEED" default:"50"`
	StreakReminderThreshold int `envconfig:"STREAK_REMINDER_THRESHOLD" default:"7"`
	StreakInactiveHours     int `envconfig:"STREAK_INACTIVE_HOURS" default:"10"`

	// Karma / Thanks
	KarmaDailyLimit            int `envconfig:"KARMA_DAILY_LIMIT" default:"2"`
	KarmaCooldownSameUserHours int `envconfig:"KARMA_COOLDOWN_SAME_USER_HOURS" default:"24"`
	ThanksDailyLimit           int `envconfig:"THANKS_DAILY_LIMIT" default:"3"`

	// Casino
	CasinoSlotsBet int64   `envconfig:"CASINO_SLOTS_BET" default:"50"`
	CasinoInitRTP  float64 `envconfig:"CASINO_INITIAL_RTP" default:"96.00"`
	CasinoMinRTP   float64 `envconfig:"CASINO_MIN_RTP" default:"94.00"`
	CasinoMaxRTP   float64 `envconfig:"CASINO_MAX_RTP" default:"98.00"`

	// Economy
	EconomyStartingBalance int64  `envconfig:"ECONOMY_STARTING_BALANCE" default:"0"`
	EconomyCurrencyName    string `envconfig:"ECONOMY_CURRENCY_NAME" default:"плюшки"`

	// Rate limiting
	RateLimitRequests int           `envconfig:"RATE_LIMIT_REQUESTS" default:"10"`
	RateLimitWindow   time.Duration `envconfig:"RATE_LIMIT_WINDOW" default:"1m"`

	// Feature flags
	FeatureCasinoEnabled  bool `envconfig:"FEATURE_CASINO_ENABLED" default:"true"`
	FeatureKarmaEnabled   bool `envconfig:"FEATURE_KARMA_ENABLED" default:"true"`
	FeatureStreaksEnabled bool `envconfig:"FEATURE_STREAKS_ENABLED" default:"true"`
}

func (c *Config) DatabaseDSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName, c.DBSSLMode,
	)
}

func (c *Config) Validate() error {
	if c.MemberSourceChatID == 0 {
		return fmt.Errorf("MEMBER_SOURCE_CHAT_ID is required and cannot be 0")
	}
	if c.AdminChatID == 0 {
		return fmt.Errorf("ADMIN_CHAT_ID is required and cannot be 0")
	}
	if c.MemberSourceChatID == c.AdminChatID {
		return fmt.Errorf("MEMBER_SOURCE_CHAT_ID and ADMIN_CHAT_ID must be different")
	}
	if c.LeaveDebug < 0 {
		return fmt.Errorf("LeaveDebug must be >= 0")
	}
	if c.BotMaxInflight <= 0 {
		return fmt.Errorf("BOT_MAX_INFLIGHT must be > 0")
	}
	if c.BotUpdateTimeoutSeconds <= 0 {
		return fmt.Errorf("BOT_UPDATE_TIMEOUT_SECONDS must be > 0")
	}
	if c.BotWorkers < minBotWorkers || c.BotWorkers > maxBotWorkers {
		return fmt.Errorf("BOT_WORKERS must be in range [%d..%d]", minBotWorkers, maxBotWorkers)
	}
	if c.BotUpdateQueue < minBotUpdateQueue || c.BotUpdateQueue > maxBotUpdateQueue {
		return fmt.Errorf("BOT_UPDATE_QUEUE must be in range [%d..%d]", minBotUpdateQueue, maxBotUpdateQueue)
	}
	if c.DBMaxConns <= 0 || c.DBMinConns < 0 || c.DBMinConns > c.DBMaxConns {
		return fmt.Errorf("invalid DB_MIN_CONNS/DB_MAX_CONNS values")
	}
	return nil
}

func Load() (*Config, error) {
	var cfg Config
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, fmt.Errorf("failed to process environment config: %w", err)
	}

	cfg.AdminIDSet = parseIDList(cfg.AdminIDsRaw)
	cfg.AdminIDs = sortedIDList(cfg.AdminIDSet)
	cfg.ModeratorIDSet = parseIDList(cfg.ModeratorIDsRaw)
	cfg.ModeratorIDs = sortedIDList(cfg.ModeratorIDSet)

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func parseIDList(envValue string) map[int64]struct{} {
	ids := make(map[int64]struct{})
	for _, part := range strings.Split(envValue, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			continue
		}
		ids[id] = struct{}{}
	}
	return ids
}

func sortedIDList(ids map[int64]struct{}) []int64 {
	if len(ids) == 0 {
		return nil
	}
	out := make([]int64, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
