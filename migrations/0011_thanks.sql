ALTER TABLE karma_logs
    ADD COLUMN IF NOT EXISTS reward_amount BIGINT NOT NULL DEFAULT 20;

UPDATE karma_logs
SET reward_amount = 20
WHERE reward_amount IS NULL;

CREATE INDEX IF NOT EXISTS idx_karma_logs_from_to_created_at
    ON karma_logs(from_user_id, to_user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_karma_logs_to_from_created_at
    ON karma_logs(to_user_id, from_user_id, created_at DESC);
