// Package config загружает конфигурацию бота из переменных окружения.
// Используется envconfig для маппинга переменных окружения на поля структуры.
package config

import (
	"fmt"
	"time"
    "strconv"
    "strings"
	"github.com/kelseyhightower/envconfig"
)

// Config содержит ВСЕ настройки приложения.
type Config struct {
	// --- Telegram ---
    AdminIDsRaw string `envconfig:"ADMIN_IDS" required:"true"`
    AdminIDs    []int64 `envconfig:"-"` // заполним вручную
	TelegramBotToken string `envconfig:"TELEGRAM_BOT_TOKEN" required:"true"`
	// ID чата, в котором бот работает (единственный разрешённый групповой чат)
	FloodChatID int64 `envconfig:"FLOOD_CHAT_ID" required:"true"`

	// --- Database ---
	// В Docker внутри контейнера "localhost" почти всегда неправильно.
	// Дефолт ставим "postgres" (имя сервиса в docker-compose), а для локалки переопределяй DB_HOST=localhost.
	DBHost     string `envconfig:"DB_HOST" default:"postgres"`
	DBPort     int    `envconfig:"DB_PORT" default:"5432"`
	DBUser     string `envconfig:"DB_USER" default:"botuser"`
	DBPassword string `envconfig:"DB_PASSWORD" required:"true"`
	DBName     string `envconfig:"DB_NAME" default:"telegram_bot"`
	DBSSLMode  string `envconfig:"DB_SSLMODE" default:"disable"`
	DBMaxConns int32  `envconfig:"DB_MAX_CONNS" default:"25"`
	DBMinConns int32  `envconfig:"DB_MIN_CONNS" default:"5"`

	// --- Application ---
	AppEnv      string `envconfig:"APP_ENV" default:"development"`
	AppLogLevel string `envconfig:"APP_LOG_LEVEL" default:"debug"`
	AppTimezone string `envconfig:"APP_TIMEZONE" default:"Europe/Moscow"`

	// --- Bot runtime ---
	// Сколько апдейтов обрабатываем параллельно. Иначе "go на каждый апдейт" = утечка памяти при флуде.
	BotMaxInflight int `envconfig:"BOT_MAX_INFLIGHT" default:"64"`
	// Таймаут long polling (секунды)
	BotUpdateTimeoutSeconds int `envconfig:"BOT_UPDATE_TIMEOUT_SECONDS" default:"60"`

	// --- Admin ---
	AdminPasswordHash string `envconfig:"ADMIN_PASSWORD_HASH" required:"true"`

	// --- Streak ---
	StreakMessagesNeed      int `envconfig:"STREAK_MESSAGES_NEED" default:"50"`
	StreakReminderThreshold int `envconfig:"STREAK_REMINDER_THRESHOLD" default:"7"`
	StreakInactiveHours     int `envconfig:"STREAK_INACTIVE_HOURS" default:"10"`

	// --- Karma ---
	KarmaDailyLimit            int `envconfig:"KARMA_DAILY_LIMIT" default:"2"`
	KarmaCooldownSameUserHours int `envconfig:"KARMA_COOLDOWN_SAME_USER_HOURS" default:"24"`

	// --- Casino ---
	CasinoSlotsBet int64   `envconfig:"CASINO_SLOTS_BET" default:"50"`
	CasinoInitRTP  float64 `envconfig:"CASINO_INITIAL_RTP" default:"96.00"`
	CasinoMinRTP   float64 `envconfig:"CASINO_MIN_RTP" default:"94.00"`
	CasinoMaxRTP   float64 `envconfig:"CASINO_MAX_RTP" default:"98.00"`

	// --- Economy ---
	EconomyStartingBalance int64  `envconfig:"ECONOMY_STARTING_BALANCE" default:"0"`
	EconomyCurrencyName    string `envconfig:"ECONOMY_CURRENCY_NAME" default:"пленки"`

	// --- Rate Limiting ---
	RateLimitRequests int           `envconfig:"RATE_LIMIT_REQUESTS" default:"10"`
	RateLimitWindow   time.Duration `envconfig:"RATE_LIMIT_WINDOW" default:"1m"`

	// --- Feature Flags ---
	FeatureCasinoEnabled   bool `envconfig:"FEATURE_CASINO_ENABLED" default:"true"`
	FeatureKarmaEnabled    bool `envconfig:"FEATURE_KARMA_ENABLED" default:"true"`
	FeatureStreaksEnabled  bool `envconfig:"FEATURE_STREAKS_ENABLED" default:"true"`
}

// DatabaseDSN возвращает строку подключения к PostgreSQL в формате DSN.
func (c *Config) DatabaseDSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName, c.DBSSLMode,
	)
}

func (c *Config) Validate() error {
	if c.FloodChatID == 0 {
		return fmt.Errorf("FLOOD_CHAT_ID не задан или равен 0")
	}
	if c.BotMaxInflight <= 0 {
		return fmt.Errorf("BOT_MAX_INFLIGHT должен быть > 0")
	}
	if c.BotUpdateTimeoutSeconds <= 0 {
		return fmt.Errorf("BOT_UPDATE_TIMEOUT_SECONDS должен быть > 0")
	}
	if c.DBMaxConns <= 0 || c.DBMinConns < 0 || c.DBMinConns > c.DBMaxConns {
		return fmt.Errorf("некорректные DB_MIN_CONNS/DB_MAX_CONNS")
	}
	return nil
}

// Load читает переменные окружения и заполняет структуру Config.
func Load() (*Config, error) {
    var cfg Config
    if err := envconfig.Process("", &cfg); err != nil {
        return nil, fmt.Errorf("не удалось загрузить конфигурацию: %w", err)
    }

    ids, err := parseInt64CSV(cfg.AdminIDsRaw)
    if err != nil {
        return nil, fmt.Errorf("ADMIN_IDS parse: %w", err)
    }
    cfg.AdminIDs = ids

    if err := cfg.Validate(); err != nil {
        return nil, err
    }
    return &cfg, nil
}
func parseInt64CSV(s string) ([]int64, error) {
    s = strings.TrimSpace(s)
    if s == "" {
        return nil, nil
    }
    parts := strings.Split(s, ",")
    out := make([]int64, 0, len(parts))
    for _, p := range parts {
        p = strings.TrimSpace(p)
        v, err := strconv.ParseInt(p, 10, 64)
        if err != nil {
            return nil, fmt.Errorf("bad int64 %q: %w", p, err)
        }
        out = append(out, v)
    }
    return out, nil
}