CREATE INDEX IF NOT EXISTS idx_transactions_from_user_created_at
    ON transactions (from_user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_transactions_to_user_created_at
    ON transactions (to_user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_admin_sessions_active_user_expires_auth
    ON admin_sessions (user_id, expires_at DESC, authenticated_at DESC)
    WHERE is_active = TRUE;

CREATE INDEX IF NOT EXISTS idx_admin_login_attempts_failed_user_attempt_time
    ON admin_login_attempts (user_id, attempt_time DESC)
    WHERE success = FALSE;

CREATE INDEX IF NOT EXISTS idx_admin_flow_states_state_expires_at
    ON admin_flow_states (state_expires_at);
