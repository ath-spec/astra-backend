-- name: UpsertSpendTransaction :exec
INSERT INTO spend_transactions
    (user_id, amount, type, category, merchant, occurred_at, source, external_id, account_ref)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (user_id, source, external_id) WHERE external_id IS NOT NULL
DO UPDATE SET
    amount      = EXCLUDED.amount,
    type        = EXCLUDED.type,
    category    = EXCLUDED.category,
    merchant    = EXCLUDED.merchant,
    occurred_at = EXCLUDED.occurred_at,
    account_ref = EXCLUDED.account_ref;
