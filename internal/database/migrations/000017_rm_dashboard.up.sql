-- RM dashboard: staff identity (Relationship Managers + Admins), the
-- auto-assignment "active queue" that routes every new user to an RM, and
-- the audit trail behind admin assign/transfer/remove operations. This is a
-- separate concern from the user-facing app (its own tables, its own auth,
-- its own JWT) — see internal/handler/rm_*.

CREATE TABLE IF NOT EXISTS rm_users (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email          VARCHAR(255) NOT NULL UNIQUE,
    password_hash  VARCHAR(255) NOT NULL,                 -- bcrypt
    name           VARCHAR(255) NOT NULL,
    phone_number   VARCHAR(50),
    role           VARCHAR(10)  NOT NULL DEFAULT 'rm'     CHECK (role IN ('rm', 'admin')),
    status         VARCHAR(10)  NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    max_portfolios INTEGER      NOT NULL DEFAULT 150,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- Staff refresh tokens, rotated on every use exactly like the user-side
-- refresh_tokens table (migration 000010). Only the SHA-256 hash is stored.
CREATE TABLE IF NOT EXISTS rm_refresh_tokens (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rm_id      UUID NOT NULL REFERENCES rm_users(id) ON DELETE CASCADE,
    token_hash VARCHAR(64) NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_rm_refresh_tokens_rm_id ON rm_refresh_tokens(rm_id);

-- Denormalised "current RM" on the user profile. This is the source of
-- truth for who owns a portfolio right now; rm_assignment_history below is
-- the append-only log of how it got there. ON DELETE SET NULL so removing
-- an RM row drops their clients into the unassigned pool rather than
-- cascading a user delete.
ALTER TABLE users ADD COLUMN IF NOT EXISTS assigned_rm_id UUID REFERENCES rm_users(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_users_assigned_rm_id ON users(assigned_rm_id);

CREATE TABLE IF NOT EXISTS rm_assignment_history (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    from_rm_id  UUID REFERENCES rm_users(id) ON DELETE SET NULL,
    to_rm_id    UUID REFERENCES rm_users(id) ON DELETE SET NULL,
    action      VARCHAR(12) NOT NULL CHECK (action IN ('auto_assign', 'assign', 'transfer', 'remove')),
    reason      VARCHAR(300),
    actor_rm_id UUID REFERENCES rm_users(id) ON DELETE SET NULL,  -- NULL = system (auto_assign)
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_rm_assignment_history_user_id ON rm_assignment_history(user_id);
CREATE INDEX IF NOT EXISTS idx_rm_assignment_history_to_rm_id ON rm_assignment_history(to_rm_id);
CREATE INDEX IF NOT EXISTS idx_rm_assignment_history_created_at ON rm_assignment_history(created_at DESC);

-- Single-row table holding the round-robin rotation pointer. The
-- auto-assign path locks this row (SELECT ... FOR UPDATE) so concurrent
-- signups can't hand the same "next" slot to two users.
CREATE TABLE IF NOT EXISTS rm_queue_state (
    id                  BOOLEAN PRIMARY KEY DEFAULT true CHECK (id),
    last_assigned_rm_id UUID REFERENCES rm_users(id) ON DELETE SET NULL,
    rotation_seq        BIGINT NOT NULL DEFAULT 0,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO rm_queue_state (id) VALUES (true) ON CONFLICT DO NOTHING;
