-- name: ListLoans :many
SELECT loan_account_id, cust_id, holder_name,
       sanctioned_amount, disbursed_amount, available_amount, interest_rate,
       tenure_months, repayment_method, interest_method, opened_at,
       outstanding_balance, overdue_amount, dpd, npa_status, npa_date, synced_at
FROM idbi_loans
WHERE user_id = $1
ORDER BY loan_account_id;

-- name: GetLoan :one
SELECT loan_account_id, cust_id, holder_name,
       sanctioned_amount, disbursed_amount, available_amount, interest_rate,
       tenure_months, repayment_method, interest_method, opened_at,
       outstanding_balance, overdue_amount, dpd, npa_status, npa_date, synced_at
FROM idbi_loans
WHERE user_id = $1 AND loan_account_id = $2;

-- name: UpsertLoan :exec
INSERT INTO idbi_loans (
    user_id, loan_account_id, cust_id, holder_name,
    sanctioned_amount, disbursed_amount, available_amount, interest_rate,
    tenure_months, repayment_method, interest_method, opened_at,
    outstanding_balance, overdue_amount, dpd, npa_status, npa_date, synced_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, now())
ON CONFLICT (user_id, loan_account_id) DO UPDATE SET
    cust_id             = CASE WHEN EXCLUDED.cust_id <> '' THEN EXCLUDED.cust_id ELSE idbi_loans.cust_id END,
    holder_name         = CASE WHEN EXCLUDED.holder_name <> '' THEN EXCLUDED.holder_name ELSE idbi_loans.holder_name END,
    sanctioned_amount   = EXCLUDED.sanctioned_amount,
    disbursed_amount    = EXCLUDED.disbursed_amount,
    available_amount    = EXCLUDED.available_amount,
    interest_rate       = EXCLUDED.interest_rate,
    tenure_months       = EXCLUDED.tenure_months,
    repayment_method    = EXCLUDED.repayment_method,
    interest_method     = EXCLUDED.interest_method,
    opened_at           = COALESCE(EXCLUDED.opened_at, idbi_loans.opened_at),
    outstanding_balance = EXCLUDED.outstanding_balance,
    overdue_amount      = EXCLUDED.overdue_amount,
    dpd                 = EXCLUDED.dpd,
    npa_status          = EXCLUDED.npa_status,
    npa_date            = EXCLUDED.npa_date,
    synced_at           = now();

-- name: DeleteLoansExcept :exec
DELETE FROM idbi_loans
WHERE user_id = $1 AND loan_account_id <> ALL($2::text[]);

-- name: DeleteAllLoans :exec
DELETE FROM idbi_loans WHERE user_id = $1;

-- name: LatestSpendSync :one
SELECT COALESCE(MAX(occurred_at), '1970-01-01 00:00:00+00'::timestamptz)::timestamptz AS latest_sync
FROM spend_transactions
WHERE user_id = $1 AND source = $2;
