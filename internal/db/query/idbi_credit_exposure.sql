-- name: UpsertCreditExposure :exec
INSERT INTO idbi_credit_exposure (
    user_id, cif_id, customer_name, account_manager, cust_rating,
    total_limit, funded_limit, non_funded_limit, total_outstanding, utilisation_pct,
    loan_count, total_overdue, max_dpd, worst_npa_status, risk_band, synced_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, now())
ON CONFLICT (user_id) DO UPDATE SET
    cif_id            = EXCLUDED.cif_id,
    customer_name     = EXCLUDED.customer_name,
    account_manager   = EXCLUDED.account_manager,
    cust_rating       = EXCLUDED.cust_rating,
    total_limit       = EXCLUDED.total_limit,
    funded_limit      = EXCLUDED.funded_limit,
    non_funded_limit  = EXCLUDED.non_funded_limit,
    total_outstanding = EXCLUDED.total_outstanding,
    utilisation_pct   = EXCLUDED.utilisation_pct,
    loan_count        = EXCLUDED.loan_count,
    total_overdue     = EXCLUDED.total_overdue,
    max_dpd           = EXCLUDED.max_dpd,
    worst_npa_status  = EXCLUDED.worst_npa_status,
    risk_band         = EXCLUDED.risk_band,
    synced_at         = now();

-- name: GetCreditExposure :one
SELECT user_id, cif_id, customer_name, account_manager, cust_rating,
       total_limit, funded_limit, non_funded_limit, total_outstanding, utilisation_pct,
       loan_count, total_overdue, max_dpd, worst_npa_status, risk_band, synced_at
FROM idbi_credit_exposure
WHERE user_id = $1;
