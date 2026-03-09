CREATE TABLE IF NOT EXISTS riddles (
    id BIGSERIAL PRIMARY KEY,
    state TEXT NOT NULL,
    post_text TEXT NOT NULL,
    reward_amount BIGINT NOT NULL,
    group_chat_id BIGINT,
    message_id BIGINT,
    created_by_admin_id BIGINT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    published_at TIMESTAMP,
    finished_at TIMESTAMP,
    expires_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS riddle_answers (
    id BIGSERIAL PRIMARY KEY,
    riddle_id BIGINT NOT NULL REFERENCES riddles(id) ON DELETE CASCADE,
    answer_raw TEXT NOT NULL,
    answer_normalized TEXT NOT NULL,
    winner_user_id BIGINT,
    winner_message_id BIGINT,
    winner_display TEXT,
    won_at TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_riddle_answers_riddle_id_answer_normalized
    ON riddle_answers (riddle_id, answer_normalized);

CREATE UNIQUE INDEX IF NOT EXISTS uq_riddles_single_active
    ON riddles ((state))
    WHERE state = 'active';

CREATE INDEX IF NOT EXISTS idx_riddles_state_created_at
    ON riddles (state, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_riddles_expires_at
    ON riddles (expires_at);
