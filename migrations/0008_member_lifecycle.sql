-- Миграция 8: жизненный цикл участников (active/left + grace period)
ALTER TABLE members ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active';
ALTER TABLE members ADD COLUMN IF NOT EXISTS left_at TIMESTAMP NULL;
ALTER TABLE members ADD COLUMN IF NOT EXISTS delete_after TIMESTAMP NULL;
ALTER TABLE members ADD COLUMN IF NOT EXISTS last_seen_at TIMESTAMP NULL;
ALTER TABLE members ADD COLUMN IF NOT EXISTS last_known_name TEXT NULL;

-- joined_at уже есть в схеме, но для совместимости приводим к nullable и выставляем где нужно
ALTER TABLE members ALTER COLUMN joined_at DROP NOT NULL;

CREATE INDEX IF NOT EXISTS idx_members_status ON members(status);
CREATE INDEX IF NOT EXISTS idx_members_delete_after_left ON members(delete_after) WHERE status = 'left';
