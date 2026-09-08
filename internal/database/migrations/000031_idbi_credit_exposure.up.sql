-- IDBI integration, feature 5: RM credit-risk / early-warning view.
--
-- idbi_credit_exposure is a per-client snapshot for the RM portal, assembled
-- from 442 fetchCustomerLimitDetails (CIF exposure: total / funded /
-- outstanding limits, rating) and 402 getLoanOverdueDetails (aggregated
-- DPD / overdue / worst NPA classification across the client's loans). RM
-- screens read this table. New table; nothing existing changes. Gated by
-- IDBI_RM_RISK_ENABLED.

CREATE TABLE IF NOT EXISTS idbi_credit_exposure (
    user_id            UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    cif_id             TEXT NOT NULL DEFAULT '',
    customer_name      TEXT NOT NULL DEFAULT '',
    account_manager    TEXT NOT NULL DEFAULT '',
    cust_rating        TEXT NOT NULL DEFAULT '',
    total_limit        NUMERIC(20,2) NOT NULL DEFAULT 0,
    funded_limit       NUMERIC(20,2) NOT NULL DEFAULT 0,
    non_funded_limit   NUMERIC(20,2) NOT NULL DEFAULT 0,
    total_outstanding  NUMERIC(20,2) NOT NULL DEFAULT 0,
    utilisation_pct    NUMERIC(6,2)  NOT NULL DEFAULT 0,
    -- aggregated overdue signal
    loan_count         INT           NOT NULL DEFAULT 0,
    total_overdue       NUMERIC(20,2) NOT NULL DEFAULT 0,
    max_dpd            INT           NOT NULL DEFAULT 0,
    worst_npa_status   TEXT NOT NULL DEFAULT '',
    risk_band          TEXT NOT NULL DEFAULT '',   -- LOW / WATCH / HIGH
    synced_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);
