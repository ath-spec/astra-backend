-- Promotes mf_folios/mf_transactions (created as stub-only schema in
-- migration 000006) into real tables backing a stateful mock MF investment
-- provider — same pattern as Stocks: a mock engine now, swappable for a
-- real CAMS/KFintech-style RTA integration later without a schema change.

ALTER TABLE mf_folios ADD COLUMN IF NOT EXISTS is_sip BOOLEAN NOT NULL DEFAULT false;

-- One folio per user per scheme in this mock (a real RTA can have multiple
-- folios per scheme; this simplification keeps holding identity == scheme
-- within a user, which is what the mock purchase/redeem/read paths key on).
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'mf_folios_user_scheme_key'
    ) THEN
        ALTER TABLE mf_folios ADD CONSTRAINT mf_folios_user_scheme_key UNIQUE (user_id, scheme_code);
    END IF;
END $$;
