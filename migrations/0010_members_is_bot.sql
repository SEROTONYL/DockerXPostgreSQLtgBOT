-- Миграция 10: признак бот-аккаунта Telegram у участников
ALTER TABLE members
    ADD COLUMN IF NOT EXISTS is_bot BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_members_is_bot ON members(is_bot);
