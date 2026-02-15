// Package config загружает конфигурацию бота из переменных окружения.
// Используется библиотека envconfig для автоматического маппинга
// переменных окружения на поля структуры.
//
// Пример использования:
//
//	cfg, err := config.Load()
//	if err != nil {
//	    log.Fatal("не удалось загрузить конфигурацию:", err)
//	}
//	fmt.Println(cfg.TelegramBotToken)
package config

import (
	"fmt"
	"time"

	"github.com/kelseyhightower/envconfig"
)

// Config содержит ВСЕ настройки приложения.
// Каждое поле соответствует переменной окружения с префиксом или без.
// Теги `envconfig` указывают имя переменной, `default` — значение по умолчанию.
type Config struct {
	// --- Telegram ---
	// Токен бота, полученный от @BotFather
	TelegramBotToken string `envconfig:"TELEGRAM_BOT_TOKEN" required:"true"`
	// ID чата, в котором бот работает (единственный разрешённый групповой чат)
	FloodChatID int64 `envconfig:"FLOOD_CHAT_ID" required:"true"`

	// --- Database ---
	DBHost     string `envconfig:"DB_HOST" default:"localhost"`
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

	// --- Admin ---
	// Хеш пароля для админ-панели (Argon2id)
	AdminPasswordHash string `envconfig:"ADMIN_PASSWORD_HASH" required:"true"`

	// --- Streak ---
	StreakMessagesNeed      int `envconfig:"STREAK_MESSAGES_NEED" default:"50"`
	StreakReminderThreshold int `envconfig:"STREAK_REMINDER_THRESHOLD" default:"7"`
	StreakInactiveHours     int `envconfig:"STREAK_INACTIVE_HOURS" default:"10"`

	// --- Karma ---
	KarmaDailyLimit            int `envconfig:"KARMA_DAILY_LIMIT" default:"2"`
	KarmaCooldownSameUserHours int `envconfig:"KARMA_COOLDOWN_SAME_USER_HOURS" default:"24"`

	// --- Casino ---
	CasinoSlotsBet  int64   `envconfig:"CASINO_SLOTS_BET" default:"50"`
	CasinoInitRTP   float64 `envconfig:"CASINO_INITIAL_RTP" default:"96.00"`
	CasinoMinRTP    float64 `envconfig:"CASINO_MIN_RTP" default:"94.00"`
	CasinoMaxRTP    float64 `envconfig:"CASINO_MAX_RTP" default:"98.00"`

	// --- Economy ---
	EconomyStartingBalance int64  `envconfig:"ECONOMY_STARTING_BALANCE" default:"0"`
	EconomyCurrencyName    string `envconfig:"ECONOMY_CURRENCY_NAME" default:"пленки"`

	// --- Rate Limiting ---
	RateLimitRequests int           `envconfig:"RATE_LIMIT_REQUESTS" default:"10"`
	RateLimitWindow   time.Duration `envconfig:"RATE_LIMIT_WINDOW" default:"1m"`

	// --- Feature Flags ---
	FeatureCasinoEnabled  bool `envconfig:"FEATURE_CASINO_ENABLED" default:"true"`
	FeatureKarmaEnabled   bool `envconfig:"FEATURE_KARMA_ENABLED" default:"true"`
	FeatureStreaksEnabled  bool `envconfig:"FEATURE_STREAKS_ENABLED" default:"true"`
}

// DatabaseDSN возвращает строку подключения к PostgreSQL в формате DSN.
// Пример: "postgres://botuser:pass@localhost:5432/telegram_bot?sslmode=disable"
func (c *Config) DatabaseDSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName, c.DBSSLMode,
	)
}

// Load читает переменные окружения и заполняет структуру Config.
// Если обязательная переменная не задана — возвращает ошибку.
func Load() (*Config, error) {
	var cfg Config
	// envconfig.Process("", &cfg) читает переменные без общего префикса
	if err := envconfig.Process("", &cfg); err != nil {
		return nil, fmt.Errorf("не удалось загрузить конфигурацию: %w", err)
	}
	return &cfg, nil
}
