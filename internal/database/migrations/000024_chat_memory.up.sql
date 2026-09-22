-- Cross-session ("long-term") memory for the consumer AI chat.
--
-- The chat already has two layers:
--   1. a live, token-dense ground-truth snapshot recomputed every turn, and
--   2. a sliding window of recent dialogue turns.
--
-- This adds layer 3/4: durable, user-specific facts, preferences, standing
-- instructions and codewords that the assistant extracted from earlier
-- conversations, plus one rolling summary row. Each fact carries captured_at so
-- the assistant can date it and tell it apart from the always-fresh live state.
CREATE TABLE IF NOT EXISTS chat_memories (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind         VARCHAR(16) NOT NULL DEFAULT 'fact'
                   CHECK (kind IN ('fact','preference','personal','instruction','goal_note','summary')),
    content      TEXT NOT NULL,
    -- normalised topic key ("codeword", "wedding_plan", ...) — the dedupe /
    -- supersede handle, so a later statement on the same topic overwrites the
    -- earlier one instead of piling up.
    key_norm     VARCHAR(160) NOT NULL,
    salience     SMALLINT NOT NULL DEFAULT 3,
    source       VARCHAR(8) NOT NULL DEFAULT 'auto' CHECK (source IN ('auto','user')),
    captured_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at   TIMESTAMPTZ,
    CONSTRAINT chat_memories_user_key_uniq UNIQUE (user_id, key_norm)
);

CREATE INDEX IF NOT EXISTS idx_chat_memories_user
    ON chat_memories(user_id, salience DESC, captured_at DESC);
