-- IDBI integration, feature 1: account balances.
--
-- idbi_customer_link maps an astra user to their IDBI customer identity
-- (cifId / custId), resolved once via account-number -> 365 performAccountEnquiry
-- or seeded for demo users. Every later IDBI feature keys off this table.
--
-- idbi_accounts is the local mirror of a customer's IDBI deposit accounts,
-- refreshed from 394 getCustomerAccountsByCustId (+ 365 for the richer
-- per-account detail). Screens read this table, never the gateway directly.
-- Everything is additive; nothing here changes existing tables. The feature is
-- gated by IDBI_ACCOUNTS_ENABLED — with it off these tables simply stay empty.

CREATE TABLE IF NOT EXISTS idbi_customer_link (
    user_id     UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    cif_id      TEXT NOT NULL,
    cust_id     TEXT NOT NULL DEFAULT '',
    -- how the link was established: 'account_lookup' (365), 'pan' (456/415),
    -- 'seed' (demo fixture), 'manual'.
    source      TEXT NOT NULL DEFAULT 'manual',
    linked_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idbi_customer_link_cif_idx ON idbi_customer_link (cif_id);
CREATE INDEX IF NOT EXISTS idbi_customer_link_cust_idx ON idbi_customer_link (cust_id);

CREATE TABLE IF NOT EXISTS idbi_accounts (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id            UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    account_number     TEXT NOT NULL,
    account_type       TEXT NOT NULL DEFAULT '',   -- SAVINGS, CURRENT, TERM_DEPOSIT, SALARY, ...
    currency           TEXT NOT NULL DEFAULT 'INR',
    cif_id             TEXT NOT NULL DEFAULT '',
    cust_id            TEXT NOT NULL DEFAULT '',
    holder_name        TEXT NOT NULL DEFAULT '',
    status             TEXT NOT NULL DEFAULT '',    -- 'A' / 'ACTIVE' / ...
    branch_id          TEXT NOT NULL DEFAULT '',
    branch_name        TEXT NOT NULL DEFAULT '',
    opened_at          TIMESTAMPTZ,
    ledger_balance     NUMERIC(18,2) NOT NULL DEFAULT 0,
    available_balance  NUMERIC(18,2) NOT NULL DEFAULT 0,
    effective_balance  NUMERIC(18,2) NOT NULL DEFAULT 0,
    lien_amount        NUMERIC(18,2) NOT NULL DEFAULT 0,
    synced_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, account_number)
);

CREATE INDEX IF NOT EXISTS idbi_accounts_user_idx ON idbi_accounts (user_id);
