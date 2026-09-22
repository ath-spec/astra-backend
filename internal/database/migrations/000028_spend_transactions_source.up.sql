-- IDBI integration, feature 2: real transactions in the spend pipeline.
--
-- spend_transactions already backs the analytics engine (weekday/weekend,
-- trends, recurring, budget diagnosis). Rather than a parallel idbi_transactions
-- table the engine can't see, we EXPAND this table so real IDBI / AA rows flow
-- through the same pipe as the seeded ones:
--
--   source       where the row came from: 'seed' (mock), 'idbi' (393),
--                'aa' (595/739). Existing rows are 'seed'.
--   external_id  the provider's own txn id (393 txnId, AA txnId) — lets a
--                re-sync be idempotent instead of duplicating.
--   account_ref  which account the txn belongs to (IDBI acctId / AA
--                linkReferenceNumber); empty for seed rows.
--
-- All columns are nullable / defaulted, so every existing query and the
-- seeded MockSource keep working untouched. Feature gated by
-- IDBI_SPEND_ENABLED — off => the seeded MockSource is still the source.

ALTER TABLE spend_transactions
    ADD COLUMN IF NOT EXISTS source      VARCHAR(8)  NOT NULL DEFAULT 'seed',
    ADD COLUMN IF NOT EXISTS external_id VARCHAR(64),
    ADD COLUMN IF NOT EXISTS account_ref VARCHAR(64);

-- Idempotent re-sync: one row per (user, source, external_id) when we have an
-- external id. Seed rows (external_id NULL) are unaffected by this index.
CREATE UNIQUE INDEX IF NOT EXISTS idx_spend_transactions_external
    ON spend_transactions (user_id, source, external_id)
    WHERE external_id IS NOT NULL;
