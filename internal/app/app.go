// Package app РёРЅРёС†РёР°Р»РёР·РёСЂСѓРµС‚ РІСЃРµ РєРѕРјРїРѕРЅРµРЅС‚С‹ РїСЂРёР»РѕР¶РµРЅРёСЏ.
// app.go вЂ” С‚РѕС‡РєР° СЃР±РѕСЂРєРё: СЃРѕР·РґР°С‘С‚ Р‘Р”-РїСѓР», СЂРµРїРѕР·РёС‚РѕСЂРёРё, СЃРµСЂРІРёСЃС‹, РѕР±СЂР°Р±РѕС‚С‡РёРєРё,
// С„РёР»СЊС‚СЂС‹ Рё СЃРѕР±РёСЂР°РµС‚ РІСЃС‘ РІ РѕРґРёРЅ РѕР±СЉРµРєС‚ Bot.
package app

import (
	"context"
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	log "github.com/sirupsen/logrus"

	"serotonyl.ru/telegram-bot/internal/bot"
	"serotonyl.ru/telegram-bot/internal/bot/filters"
	"serotonyl.ru/telegram-bot/internal/config"
	"serotonyl.ru/telegram-bot/internal/db/postgres"
	"serotonyl.ru/telegram-bot/internal/features/admin"
	"serotonyl.ru/telegram-bot/internal/features/casino"
	"serotonyl.ru/telegram-bot/internal/features/economy"
	"serotonyl.ru/telegram-bot/internal/features/karma"
	"serotonyl.ru/telegram-bot/internal/features/members"
	"serotonyl.ru/telegram-bot/internal/features/streak"
	"serotonyl.ru/telegram-bot/internal/jobs"
)

// App СЃРѕРґРµСЂР¶РёС‚ РІСЃРµ РєРѕРјРїРѕРЅРµРЅС‚С‹ РїСЂРёР»РѕР¶РµРЅРёСЏ.
type App struct {
	Bot       *bot.Bot
	Scheduler *jobs.Scheduler
	DB        *pgxpool.Pool
	BotAPI    *tgbotapi.BotAPI
}

// New СЃРѕР·РґР°С‘С‚ Рё РёРЅРёС†РёР°Р»РёР·РёСЂСѓРµС‚ РїСЂРёР»РѕР¶РµРЅРёРµ.
// РџРѕСЂСЏРґРѕРє РёРЅРёС†РёР°Р»РёР·Р°С†РёРё РІР°Р¶РµРЅ вЂ” РєРѕРјРїРѕРЅРµРЅС‚С‹ Р·Р°РІРёСЃСЏС‚ РґСЂСѓРі РѕС‚ РґСЂСѓРіР°.
func New(ctx context.Context, cfg *config.Config) (*App, error) {
	// === 1. Р‘Р°Р·Р° РґР°РЅРЅС‹С… ===
	pool, err := postgres.NewPool(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("РѕС€РёР±РєР° РїРѕРґРєР»СЋС‡РµРЅРёСЏ Рє Р‘Р”: %w", err)
	}

	// Р—Р°РїСѓСЃРєР°РµРј РјРёРіСЂР°С†РёРё
	if err := runMigrations(ctx, pool); err != nil {
		return nil, fmt.Errorf("РѕС€РёР±РєР° РјРёРіСЂР°С†РёР№: %w", err)
	}

	// === 2. Telegram Bot API ===
	botAPI, err := tgbotapi.NewBotAPI(cfg.TelegramBotToken)
	if err != nil {
		return nil, fmt.Errorf("РѕС€РёР±РєР° СЃРѕР·РґР°РЅРёСЏ Telegram API: %w", err)
	}
	botAPI.Debug = cfg.AppEnv == "development"
	log.Infof("РђРІС‚РѕСЂРёР·РѕРІР°РЅ РєР°Рє @%s", botAPI.Self.UserName)

	// === 3. Р РµРїРѕР·РёС‚РѕСЂРёРё ===
	memberRepo := members.NewRepository(pool)
	economyRepo := economy.NewRepository(pool)
	streakRepo := streak.NewRepository(pool)
	karmaRepo := karma.NewRepository(pool)
	casinoRepo := casino.NewRepository(pool)
	adminRepo := admin.NewRepository(pool)

	// === 4. РЎРµСЂРІРёСЃС‹ ===
	memberService := members.NewService(memberRepo)
	economyService := economy.NewService(economyRepo)
	streakService := streak.NewService(streakRepo, economyService, cfg)
	karmaService := karma.NewService(karmaRepo, cfg)
	casinoService := casino.NewService(casinoRepo, economyService, cfg)
	adminService := admin.NewService(adminRepo, memberRepo, cfg)

	// === 5. РћР±СЂР°Р±РѕС‚С‡РёРєРё ===
	memberHandler := members.NewHandler(memberService)
	economyHandler := economy.NewHandler(economyService, memberService, botAPI)
	streakHandler := streak.NewHandler(streakService, botAPI, cfg)
	karmaHandler := karma.NewHandler(karmaService, botAPI)
	casinoHandler := casino.NewHandler(casinoService, botAPI)
	adminHandler := admin.NewHandler(adminService, memberService, botAPI)

	// === 6. Р¤РёР»СЊС‚СЂС‹ ===
	chatFilter := filters.NewChatFilter(cfg.FloodChatID, memberService, botAPI)

	// === 7. РЎРѕР±РёСЂР°РµРј Р±РѕС‚Р° ===
	b := bot.New(
		botAPI, cfg,
		memberService, memberHandler,
		economyService, economyHandler,
		streakService, streakHandler,
		karmaService, karmaHandler,
		casinoService, casinoHandler,
		adminService, adminHandler,
		chatFilter,
	)

	// === 8. РџР»Р°РЅРёСЂРѕРІС‰РёРє Р·Р°РґР°С‡ ===
	scheduler := jobs.NewScheduler(streakService, b.SendMessageToUser)

	return &App{
		Bot:       b,
		Scheduler: scheduler,
		DB:        pool,
		BotAPI:    botAPI,
	}, nil
}

// runMigrations РІС‹РїРѕР»РЅСЏРµС‚ РІСЃРµ SQL-РјРёРіСЂР°С†РёРё.
func runMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	// РРЅРёС†РёР°Р»РёР·РёСЂСѓРµРј СЃРёСЃС‚РµРјСѓ РјРёРіСЂР°С†РёР№
	if err := postgres.RunMigrations(ctx, pool, "migrations"); err != nil {
		return err
	}

	// Р’С‹РїРѕР»РЅСЏРµРј РјРёРіСЂР°С†РёРё РїРѕ РїРѕСЂСЏРґРєСѓ
	migrations := []struct {
		version int
		sql     string
	}{
		{1, migration001Members},
		{2, migration002Economy},
		{3, migration003Streaks},
		{4, migration004Karma},
		{5, migration005Casino},
		{6, migration006Admin},
	}

	for _, m := range migrations {
		if err := postgres.ExecMigrationSQL(ctx, pool, m.version, m.sql); err != nil {
			return fmt.Errorf("РјРёРіСЂР°С†РёСЏ %d: %w", m.version, err)
		}
		log.Infof("РњРёРіСЂР°С†РёСЏ %d РїСЂРёРјРµРЅРµРЅР°", m.version)
	}

	return nil
}

// SQL-РјРёРіСЂР°С†РёРё РІСЃС‚СЂРѕРµРЅС‹ РІ РєРѕРґ РґР»СЏ СѓРїСЂРѕС‰РµРЅРёСЏ РґРµРїР»РѕСЏ.
// РўР°РєР¶Рµ РґРѕСЃС‚СѓРїРЅС‹ РєР°Рє .sql С„Р°Р№Р»С‹ РІ РїР°РїРєРµ migrations/.

var migration001Members = `
CREATE TABLE IF NOT EXISTS members (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT UNIQUE NOT NULL,
    username VARCHAR(255),
    first_name VARCHAR(255) NOT NULL,
    last_name VARCHAR(255),
    role VARCHAR(64),
    is_admin BOOLEAN DEFAULT FALSE,
    is_banned BOOLEAN DEFAULT FALSE,
    joined_at TIMESTAMP DEFAULT NOW(),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_members_user_id ON members(user_id);
CREATE INDEX IF NOT EXISTS idx_members_username ON members(username);
`

var migration002Economy = `
CREATE TABLE IF NOT EXISTS balances (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT UNIQUE NOT NULL REFERENCES members(user_id),
    balance BIGINT DEFAULT 0,
    total_earned BIGINT DEFAULT 0,
    total_spent BIGINT DEFAULT 0,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS transactions (
    id BIGSERIAL PRIMARY KEY,
    from_user_id BIGINT REFERENCES members(user_id),
    to_user_id BIGINT REFERENCES members(user_id),
    amount BIGINT NOT NULL,
    transaction_type VARCHAR(50) NOT NULL,
    description TEXT,
    created_at TIMESTAMP DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_transactions_from_user ON transactions(from_user_id);
CREATE INDEX IF NOT EXISTS idx_transactions_to_user ON transactions(to_user_id);
CREATE INDEX IF NOT EXISTS idx_transactions_created_at ON transactions(created_at DESC);
`

var migration003Streaks = `
CREATE TABLE IF NOT EXISTS streaks (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT UNIQUE NOT NULL REFERENCES members(user_id),
    current_streak INTEGER DEFAULT 0,
    longest_streak INTEGER DEFAULT 0,
    messages_today INTEGER DEFAULT 0,
    quota_completed_today BOOLEAN DEFAULT FALSE,
    last_quota_completion DATE,
    last_message_at TIMESTAMP,
    total_quotas_completed INTEGER DEFAULT 0,
    reminder_sent_today BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_streaks_user_id ON streaks(user_id);
`

var migration004Karma = `
CREATE TABLE IF NOT EXISTS karma (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT UNIQUE NOT NULL REFERENCES members(user_id),
    karma_points INTEGER DEFAULT 0,
    positive_received INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS karma_logs (
    id BIGSERIAL PRIMARY KEY,
    from_user_id BIGINT REFERENCES members(user_id),
    to_user_id BIGINT REFERENCES members(user_id),
    points INTEGER DEFAULT 1,
    created_at TIMESTAMP DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_karma_logs_from_user ON karma_logs(from_user_id);
CREATE INDEX IF NOT EXISTS idx_karma_logs_created_at ON karma_logs(created_at DESC);
`

var migration005Casino = `
CREATE TABLE IF NOT EXISTS casino_games (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT REFERENCES members(user_id),
    game_type VARCHAR(50) DEFAULT 'slots',
    bet_amount BIGINT DEFAULT 50,
    result_amount BIGINT NOT NULL,
    game_data JSONB,
    rtp_percentage DECIMAL(5,2),
    created_at TIMESTAMP DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_casino_games_user_id ON casino_games(user_id);
CREATE TABLE IF NOT EXISTS casino_stats (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT UNIQUE REFERENCES members(user_id),
    total_spins INTEGER DEFAULT 0,
    total_wagered BIGINT DEFAULT 0,
    total_won BIGINT DEFAULT 0,
    biggest_win BIGINT DEFAULT 0,
    current_rtp DECIMAL(5,2) DEFAULT 96.00,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);
`

var migration006Admin = `
CREATE TABLE IF NOT EXISTS admin_sessions (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT REFERENCES members(user_id),
    session_token VARCHAR(255) UNIQUE,
    authenticated_at TIMESTAMP DEFAULT NOW(),
    expires_at TIMESTAMP,
    last_activity TIMESTAMP DEFAULT NOW(),
    is_active BOOLEAN DEFAULT TRUE
);
CREATE INDEX IF NOT EXISTS idx_admin_sessions_user_id ON admin_sessions(user_id);
CREATE TABLE IF NOT EXISTS admin_login_attempts (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT,
    attempt_time TIMESTAMP DEFAULT NOW(),
    success BOOLEAN DEFAULT FALSE
);
`
