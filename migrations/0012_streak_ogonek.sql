ALTER TABLE streaks
    ADD COLUMN IF NOT EXISTS progress_date DATE,
    ADD COLUMN IF NOT EXISTS last_rewarded_day DATE;

CREATE TABLE IF NOT EXISTS streak_processed_messages (
    user_id BIGINT NOT NULL REFERENCES members(user_id) ON DELETE CASCADE,
    message_id BIGINT NOT NULL,
    streak_day DATE NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, message_id)
);

CREATE INDEX IF NOT EXISTS idx_streaks_current_streak_user
    ON streaks (current_streak DESC, user_id ASC);

CREATE INDEX IF NOT EXISTS idx_streak_processed_messages_day
    ON streak_processed_messages (streak_day);
