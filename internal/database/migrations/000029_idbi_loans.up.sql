-- IDBI integration, feature 3: My Loans.
--
-- idbi_loans mirrors a customer's loan accounts, assembled from
-- 402 getLoanOverdueDetails (the list of loan accountIds + DPD/NPA/overdue)
-- and 391 getLoanAccountDetails (per-loan master: amount, rate, tenure,
-- disbursed). Foreclosure quotes (538) and EMI schedules (473) are computed
-- on demand and not stored. New table; nothing existing changes. Gated by
-- IDBI_LOANS_ENABLED.

CREATE TABLE IF NOT EXISTS idbi_loans (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id            UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    loan_account_id    TEXT NOT NULL,
    cust_id            TEXT NOT NULL DEFAULT '',
    holder_name        TEXT NOT NULL DEFAULT '',
    -- 391 master
    sanctioned_amount  NUMERIC(18,2) NOT NULL DEFAULT 0,
    disbursed_amount   NUMERIC(18,2) NOT NULL DEFAULT 0,
    available_amount   NUMERIC(18,2) NOT NULL DEFAULT 0,
    interest_rate      NUMERIC(9,4)  NOT NULL DEFAULT 0,
    tenure_months      INT           NOT NULL DEFAULT 0,
    repayment_method   TEXT NOT NULL DEFAULT '',   -- EMI, ...
    interest_method    TEXT NOT NULL DEFAULT '',   -- Reducing Balance, ...
    opened_at          TIMESTAMPTZ,
    -- 402 overdue position
    outstanding_balance NUMERIC(18,2) NOT NULL DEFAULT 0,
    overdue_amount      NUMERIC(18,2) NOT NULL DEFAULT 0,
    dpd                 INT           NOT NULL DEFAULT 0,   -- days past due
    npa_status          TEXT NOT NULL DEFAULT '',          -- SA / SMA-0/1/2 / NPA
    npa_date            TIMESTAMPTZ,
    synced_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, loan_account_id)
);

CREATE INDEX IF NOT EXISTS idbi_loans_user_idx ON idbi_loans (user_id);
