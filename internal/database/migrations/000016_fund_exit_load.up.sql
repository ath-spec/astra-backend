-- Exit load is genuine fund reference data (like expense_ratio) missing
-- from the catalog until now — the fund profile screen was hardcoding a
-- single "1% if redeemed within 365 days" string for every fund regardless
-- of category, which is wrong for debt/liquid funds (which typically carry
-- no exit load) and hybrid funds (shorter windows). Backfilled by category,
-- the same static-reference-data pattern as the rest of fund_catalog.
ALTER TABLE fund_catalog ADD COLUMN IF NOT EXISTS exit_load_pct NUMERIC(4,2) NOT NULL DEFAULT 0;
ALTER TABLE fund_catalog ADD COLUMN IF NOT EXISTS exit_load_period_days INTEGER NOT NULL DEFAULT 0;

UPDATE fund_catalog SET exit_load_pct = 1.00, exit_load_period_days = 365
WHERE category LIKE 'Equity%';

UPDATE fund_catalog SET exit_load_pct = 1.00, exit_load_period_days = 90
WHERE category LIKE 'Hybrid%';

UPDATE fund_catalog SET exit_load_pct = 0.25, exit_load_period_days = 90
WHERE category LIKE 'Other%';

UPDATE fund_catalog SET exit_load_pct = 0, exit_load_period_days = 0
WHERE category LIKE 'Debt%';
