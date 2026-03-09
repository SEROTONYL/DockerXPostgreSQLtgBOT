CREATE TABLE IF NOT EXISTS economy_transfer_confirmations (
    token TEXT PRIMARY KEY,
    chat_id BIGINT NOT NULL,
    message_id INTEGER NOT NULL,
    owner_user_id BIGINT NOT NULL,
    sender_user_id BIGINT NOT NULL,
    target_user_id BIGINT NOT NULL,
    amount BIGINT NOT NULL CHECK (amount > 0),
    recipient_display TEXT NOT NULL,
    state TEXT NOT NULL,
    metadata JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMP NOT NULL,
    consumed_at TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_economy_transfer_confirmations_owner_user_id
    ON economy_transfer_confirmations(owner_user_id);
CREATE INDEX IF NOT EXISTS idx_economy_transfer_confirmations_expires_at
    ON economy_transfer_confirmations(expires_at);

CREATE TABLE IF NOT EXISTS admin_flow_states (
    user_id BIGINT PRIMARY KEY,
    state_name TEXT NOT NULL DEFAULT '',
    state_payload JSONB,
    state_expires_at TIMESTAMP,
    panel_chat_id BIGINT,
    panel_message_id INTEGER,
    panel_updated_at TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_admin_flow_states_panel_updated_at
    ON admin_flow_states(panel_updated_at);
