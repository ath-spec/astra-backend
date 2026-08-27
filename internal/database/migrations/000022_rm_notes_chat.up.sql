-- RM client interaction log (call notes, meetings, follow-up tasks) — replaces
-- the browser-local notes with a shared, auditable, server-side timeline.
CREATE TABLE IF NOT EXISTS rm_client_interactions (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    rm_id        UUID NOT NULL REFERENCES rm_users(id) ON DELETE CASCADE,
    kind         VARCHAR(12) NOT NULL DEFAULT 'note'
                   CHECK (kind IN ('note','call','meeting','email','task')),
    body         TEXT NOT NULL,
    follow_up_at TIMESTAMPTZ,
    done_at      TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_rm_client_interactions_user
    ON rm_client_interactions(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_rm_client_interactions_followup
    ON rm_client_interactions(rm_id, follow_up_at)
    WHERE follow_up_at IS NOT NULL AND done_at IS NULL;

-- Staff-facing chat copilot sessions, kept entirely separate from the user
-- app's chat_sessions. One row per (staff member, scope).
CREATE TABLE IF NOT EXISTS rm_chat_sessions (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rm_id      UUID NOT NULL REFERENCES rm_users(id) ON DELETE CASCADE,
    scope      VARCHAR(8) NOT NULL DEFAULT 'rm' CHECK (scope IN ('rm','admin')),
    messages   JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (rm_id, scope)
);
