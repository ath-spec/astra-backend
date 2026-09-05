-- Budget feature: setup-wizard sessions ->
-- finalized monthly per-category budgets → active dashboard + settings).
-- Actual spend is NOT stored here; it is aggregated on the fly from
-- spend_transactions (migration 000007) via the analytics TransactionSource.

CREATE TABLE IF NOT EXISTS budget_global_categories (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    description  TEXT,
    icon         TEXT,
    color        TEXT,
    -- Raw transaction-category / merchant-bucket strings that resolve to this
    -- category. The budget service reads spend_transactions.category through
    -- this table (data, not a hard-coded switch).
    aliases      TEXT[] NOT NULL DEFAULT '{}',
    is_active    BOOLEAN NOT NULL DEFAULT true,
    sort_order   INT NOT NULL DEFAULT 0,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One row per (user, category, month). Finalizing a setup session replaces
-- the whole month: every existing row is deactivated, then the tracked
-- allocations are inserted fresh.
CREATE TABLE IF NOT EXISTS budgets (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category_id     UUID NOT NULL,
    category_type   TEXT NOT NULL DEFAULT 'global' CHECK (category_type IN ('global', 'user')),
    year            INT NOT NULL,
    month           INT NOT NULL CHECK (month BETWEEN 1 AND 12),
    budgeted_amount NUMERIC(15,2) NOT NULL CHECK (budgeted_amount >= 0),
    is_active       BOOLEAN NOT NULL DEFAULT true,
    is_hidden       BOOLEAN NOT NULL DEFAULT false,
    notes           TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, category_id, category_type, year, month)
);

CREATE INDEX IF NOT EXISTS idx_budgets_user_period ON budgets(user_id, year, month);
CREATE INDEX IF NOT EXISTS idx_budgets_user_active ON budgets(user_id, is_active);

-- Setup-wizard scratch state. status: pending | finalized | abandoned.
CREATE TABLE IF NOT EXISTS budget_setup_sessions (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id              UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    month                VARCHAR(7) NOT NULL,               -- "YYYY-MM"
    total_budget         NUMERIC(15,2),
    category_allocations JSONB NOT NULL DEFAULT '[]'::jsonb,
    status               VARCHAR(20) NOT NULL DEFAULT 'pending',
    expires_at           TIMESTAMPTZ NOT NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_budget_setup_sessions_user_status ON budget_setup_sessions(user_id, status);

-- Per-user budget settings. last_reset_at doubles as the "has an active
-- budget" flag: set to now() on finalize, cleared to NULL on reset.
CREATE TABLE IF NOT EXISTS budget_settings (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id              UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE UNIQUE,
    linked_income        NUMERIC(15,2),
    spending_limit       NUMERIC(15,2),
    essential_categories JSONB NOT NULL DEFAULT '[]'::jsonb,
    bills                JSONB NOT NULL DEFAULT '[]'::jsonb,
    last_reset_at        TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Canonical spend categories. The HF budget-bloc model and the Flutter
-- client see the same category keys. `aliases` carries the raw strings the seeded
-- transaction feed / analytics categorizer emit for each bucket.
INSERT INTO budget_global_categories (name, display_name, description, icon, color, sort_order, aliases) VALUES
('food_dining',    'Food & Dining',  'Restaurants, groceries, food delivery',              '🍽️', '#FF6B6B', 1,  ARRAY['groceries','food','dining','restaurants']),
('transportation', 'Transportation', 'Fuel, public transport, rideshare, car maintenance', '🚗', '#4ECDC4', 2,  ARRAY['transport','fuel','commute','cab','rideshare']),
('shopping',       'Shopping',       'Clothing, electronics, general shopping',            '🛍️', '#45B7D1', 3,  ARRAY['retail','apparel','electronics']),
('entertainment',  'Entertainment',  'Movies, games, subscriptions, hobbies',              '🎬', '#96CEB4', 4,  ARRAY['subscriptions','streaming','movies','games','hobbies']),
('healthcare',     'Healthcare',     'Medical expenses, pharmacy, fitness',                '🏥', '#FFEAA7', 5,  ARRAY['health','medical','pharmacy','fitness']),
('utilities',      'Utilities',      'Rent, electricity, water, internet, phone bills',    '⚡', '#DDA0DD', 6,  ARRAY['bills & utilities','bills','utility','rent','housing','mortgage','electricity','internet','broadband']),
('education',      'Education',       'Courses, books, educational materials',              '📚', '#98D8C8', 7,  ARRAY['courses','books','tuition']),
('travel',         'Travel',         'Vacations, business trips, accommodation',           '✈️', '#F7DC6F', 8,  ARRAY['vacation','flights','hotels','trips']),
('insurance',      'Insurance',      'Health, auto, life insurance premiums',              '🛡️', '#BB8FCE', 9,  ARRAY['premium','premiums']),
('savings',        'Savings',        'Emergency fund, investments, retirement',            '💰', '#85C1E9', 10, ARRAY['investment','investments','sip','emergency fund']),
('other',          'Other',          'Miscellaneous expenses not categorized',            '📦', '#D5DBDB', 11, ARRAY['bnpl','misc','miscellaneous','uncategorized'])
ON CONFLICT (name) DO NOTHING;
