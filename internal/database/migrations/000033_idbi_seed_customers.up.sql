-- IDBI sandbox seed customers.
-- Stores the PAN/CustID/CustomerName rows from docs/IDBI APIs - Data.csv so
-- that demo-user linking, the dedupe-check (456), and CKYC (415) flows can
-- look up real sandbox identities without hard-coding values in Go.
-- Populated once via scripts/seed_idbi_customers.go (idempotent upsert).

CREATE TABLE IF NOT EXISTS idbi_seed_customers (
    cust_id       TEXT PRIMARY KEY,
    pan_number    VARCHAR(10) NOT NULL,
    customer_name TEXT        NOT NULL,
    account_type  TEXT        NOT NULL,  -- e.g. SAV_REGULAR, CUR_CORP
    account_code  TEXT        NOT NULL,  -- e.g. SB001, CA005
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_idbi_seed_customers_pan  ON idbi_seed_customers (pan_number);
CREATE INDEX IF NOT EXISTS idx_idbi_seed_customers_type ON idbi_seed_customers (account_type);
