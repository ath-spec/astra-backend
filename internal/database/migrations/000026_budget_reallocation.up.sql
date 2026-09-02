-- Smart Rebalance ("Smart Reallocation") + ML feedback loop, ported from the
-- A reallocation run asks budget-bloc to move
-- budget from flexible "donor" categories into an overspent one; the user
-- accepts / rejects each proposal, and rejections create cooldowns so the
-- model stops re-suggesting the same move.

-- One reallocation run.
CREATE TABLE IF NOT EXISTS budget_reallocation_sessions (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    target_category     TEXT NOT NULL,
    min_remaining_ratio NUMERIC(5,2) NOT NULL DEFAULT 0.60,
    initial_budget      JSONB NOT NULL DEFAULT '{}'::jsonb,
    updated_budget      JSONB NOT NULL DEFAULT '{}'::jsonb,
    current_spend       JSONB NOT NULL DEFAULT '{}'::jsonb,
    uncovered_overspend NUMERIC(15,2) NOT NULL DEFAULT 0,
    overspent_amount    NUMERIC(15,2) NOT NULL DEFAULT 0,
    status              TEXT NOT NULL DEFAULT 'pending'
                        CHECK (status IN ('pending', 'applied', 'rejected', 'expired')),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_budget_realloc_sessions_user ON budget_reallocation_sessions(user_id, status);

-- Each proposed donor -> target move within a run.
CREATE TABLE IF NOT EXISTS budget_reallocations (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id    UUID NOT NULL REFERENCES budget_reallocation_sessions(id) ON DELETE CASCADE,
    user_id       UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    from_category TEXT NOT NULL,
    to_category   TEXT NOT NULL,
    amount        NUMERIC(15,2) NOT NULL,
    reason        TEXT,
    status        TEXT NOT NULL DEFAULT 'pending'
                  CHECK (status IN ('pending', 'applied', 'rejected')),
    applied_at    TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_budget_reallocations_session ON budget_reallocations(session_id);

-- Surfaced ML suggestions the user can accept / reject / modify.
CREATE TABLE IF NOT EXISTS budget_ml_suggestions (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id            UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_id         UUID REFERENCES budget_reallocation_sessions(id) ON DELETE SET NULL,
    suggestion_type    TEXT NOT NULL,                        -- reallocation | budget_adjustment | spending_warning
    suggestion_details JSONB NOT NULL DEFAULT '{}'::jsonb,   -- {reallocations:[{from,to,amount}], ...}
    confidence_score   NUMERIC(5,4),
    priority           TEXT CHECK (priority IS NULL OR priority IN ('high', 'medium', 'low')),
    user_response      TEXT NOT NULL DEFAULT 'pending'
                       CHECK (user_response IN ('pending', 'accepted', 'rejected', 'modified', 'ignored')),
    user_response_at   TIMESTAMPTZ,
    user_response_reason TEXT,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_budget_ml_suggestions_user ON budget_ml_suggestions(user_id, user_response);

-- Cooldowns created when a category-pair reallocation is rejected. Exponential
-- backoff: 1 -> 3 -> 7 -> 14 days.
CREATE TABLE IF NOT EXISTS budget_ml_cooldowns (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    cooldown_type   TEXT NOT NULL DEFAULT 'category_pair'
                    CHECK (cooldown_type IN ('category_pair', 'category', 'suggestion_type', 'global')),
    from_category   TEXT,
    to_category     TEXT,
    category        TEXT,
    suggestion_type TEXT,
    cooldown_until  TIMESTAMPTZ NOT NULL,
    reason          TEXT,
    rejection_count INT NOT NULL DEFAULT 1,
    is_active       BOOLEAN NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_budget_ml_cooldowns_active
    ON budget_ml_cooldowns(user_id, is_active, cooldown_until) WHERE is_active = true;
CREATE UNIQUE INDEX IF NOT EXISTS idx_budget_ml_cooldowns_pair
    ON budget_ml_cooldowns(user_id, from_category, to_category) WHERE cooldown_type = 'category_pair';

-- Per-user learned parameters returned by budget-bloc (donor ratios, category
-- relationships, adherence, RL scoring weights, Ridge predictions). Loaded on
-- every diagnosis / reallocation so the stateless model can skip retraining.
CREATE TABLE IF NOT EXISTS budget_learned_parameters (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id                  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE UNIQUE,
    donor_ratios             JSONB NOT NULL DEFAULT '{}'::jsonb,
    relationships            JSONB NOT NULL DEFAULT '{}'::jsonb,
    adherence_metrics        JSONB NOT NULL DEFAULT '{}'::jsonb,
    scoring_weights          JSONB NOT NULL DEFAULT '{}'::jsonb,
    ridge_predictions        JSONB NOT NULL DEFAULT '{}'::jsonb,
    ridge_last_trained_month TEXT NOT NULL DEFAULT '',
    version                  INT NOT NULL DEFAULT 1,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT now()
);
