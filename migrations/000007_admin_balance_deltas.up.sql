CREATE TABLE IF NOT EXISTS admin_balance_deltas (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL,
    name TEXT NOT NULL,
    amount BIGINT NOT NULL CHECK (amount > 0),
    created_by BIGINT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_admin_balance_deltas_chat_id ON admin_balance_deltas(chat_id);
