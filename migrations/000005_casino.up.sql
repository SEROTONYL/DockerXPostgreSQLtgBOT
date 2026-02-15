-- Миграция 5: Казино
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
CREATE INDEX IF NOT EXISTS idx_casino_games_created_at ON casino_games(created_at DESC);
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
