-- Backing store for the spend analytics engine (weekday/weekend split,
-- trends, category/merchant analysis, recurring detection, impulse score,
-- etc). Deliberately separate from aa_transactions (see migration 000006) —
-- Account Aggregator stays untouched/deferred; this table is seeded mock
-- data the analytics engine runs against until a real transaction feed
-- (AA or otherwise) is wired in as a TransactionSource implementation.
CREATE TABLE IF NOT EXISTS spend_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    amount NUMERIC(18,2) NOT NULL,
    type VARCHAR(6) NOT NULL, -- DEBIT / CREDIT
    category VARCHAR(40) NOT NULL,
    merchant VARCHAR(100) NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_spend_transactions_user_occurred ON spend_transactions(user_id, occurred_at);
CREATE INDEX IF NOT EXISTS idx_spend_transactions_user_category ON spend_transactions(user_id, category);
