DROP INDEX IF EXISTS idx_spend_transactions_external;
ALTER TABLE spend_transactions
    DROP COLUMN IF EXISTS account_ref,
    DROP COLUMN IF EXISTS external_id,
    DROP COLUMN IF EXISTS source;
