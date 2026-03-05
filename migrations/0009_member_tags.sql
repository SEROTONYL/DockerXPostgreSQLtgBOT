-- Миграция 9: теги участников Telegram
ALTER TABLE members
    ADD COLUMN IF NOT EXISTS tag VARCHAR(64),
    ADD COLUMN IF NOT EXISTS tag_updated_at TIMESTAMP;
