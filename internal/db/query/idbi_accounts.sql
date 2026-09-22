-- name: ListAccounts :many
SELECT account_number, account_type, currency, cif_id, cust_id,
       holder_name, status, branch_id, branch_name, opened_at,
       ledger_balance, available_balance, effective_balance, lien_amount, synced_at
FROM idbi_accounts
WHERE user_id = $1
ORDER BY account_type, account_number;

-- name: GetAccountByNumber :one
SELECT account_number, account_type, currency, cif_id, cust_id,
       holder_name, status, branch_id, branch_name, opened_at,
       ledger_balance, available_balance, effective_balance, lien_amount, synced_at
FROM idbi_accounts
WHERE user_id = $1 AND account_number = $2;

-- name: UpsertAccount :exec
INSERT INTO idbi_accounts (
    user_id, account_number, account_type, currency, cif_id, cust_id,
    holder_name, status, branch_id, branch_name, opened_at,
    ledger_balance, available_balance, effective_balance, lien_amount, synced_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, now())
ON CONFLICT (user_id, account_number) DO UPDATE SET
    account_type      = EXCLUDED.account_type,
    currency          = EXCLUDED.currency,
    cif_id            = CASE WHEN EXCLUDED.cif_id <> '' THEN EXCLUDED.cif_id ELSE idbi_accounts.cif_id END,
    cust_id           = CASE WHEN EXCLUDED.cust_id <> '' THEN EXCLUDED.cust_id ELSE idbi_accounts.cust_id END,
    holder_name       = CASE WHEN EXCLUDED.holder_name <> '' THEN EXCLUDED.holder_name ELSE idbi_accounts.holder_name END,
    status            = EXCLUDED.status,
    branch_id         = CASE WHEN EXCLUDED.branch_id <> '' THEN EXCLUDED.branch_id ELSE idbi_accounts.branch_id END,
    branch_name       = CASE WHEN EXCLUDED.branch_name <> '' THEN EXCLUDED.branch_name ELSE idbi_accounts.branch_name END,
    opened_at         = COALESCE(EXCLUDED.opened_at, idbi_accounts.opened_at),
    ledger_balance    = EXCLUDED.ledger_balance,
    available_balance = EXCLUDED.available_balance,
    effective_balance = EXCLUDED.effective_balance,
    lien_amount       = EXCLUDED.lien_amount,
    synced_at         = now();

-- name: DeleteAccountsExcept :exec
DELETE FROM idbi_accounts
WHERE user_id = $1 AND account_number <> ALL($2::text[]);

-- name: DeleteAllAccounts :exec
DELETE FROM idbi_accounts WHERE user_id = $1;
