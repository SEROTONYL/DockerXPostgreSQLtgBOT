-- Миграция 4: Карма
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
CREATE INDEX IF NOT EXISTS idx_karma_logs_to_user ON karma_logs(to_user_id);
CREATE INDEX IF NOT EXISTS idx_karma_logs_created_at ON karma_logs(created_at DESC);
