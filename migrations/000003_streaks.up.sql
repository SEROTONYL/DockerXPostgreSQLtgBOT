-- Миграция 3: Стрик-система
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
CREATE INDEX IF NOT EXISTS idx_streaks_last_message ON streaks(last_message_at);
