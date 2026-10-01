package rmseed

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SeedCreditExposure fills idbi_credit_exposure for every RM-assigned client
// that has no snapshot yet, so the RM console's risk tiers, priority list and
// exposure figures have data in dev/demo environments. Everything is derived
// from data that already exists:
//
//   - identity: cif_id from idbi_customer_link when the user is linked, else
//     a cust_id from idbi_seed_customers (the IDBI sandbox PAN-dedupe list),
//     else a hash-derived placeholder; customer_name from users.name.
//   - size: the credit limit is 12x the client's average monthly CREDIT in
//     spend_transactions over the last 3 complete calendar months (flat default if none).
//   - risk: spend pressure = monthly DEBIT / monthly CREDIT over the same 3 months.
//     <0.82 LOW (0 DPD); <0.95 WATCH (5-25 DPD); <1.10 HIGH (31-89 DPD);
//     above that HIGH at 90+ DPD, shown as Critical. Utilisation rises with
//     pressure. Clients with no spend history are LOW.
//
// Run SeedSpendStress first, otherwise the saver profiles from signup seeding
// put every client in LOW. DPD and overdue are modelled from spend pressure,
// not observed repayment behaviour. Re-runs are stable and never overwrite an
// existing row (including real IDBI-synced ones). Demo data only — gate
// behind RM_SEED_CREDIT_EXPOSURE=true.
func SeedCreditExposure(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	tag, err := pool.Exec(ctx, `
		WITH seed_ids AS (
			SELECT cust_id, row_number() OVER (ORDER BY cust_id) AS rn FROM idbi_seed_customers
		), seed_cnt AS (
			SELECT count(*) AS n FROM seed_ids
		), todo AS (
			SELECT u.id, COALESCE(u.name, 'Client') AS name,
			       row_number() OVER (ORDER BY u.created_at, u.id) AS rn,
			       abs(hashtext(u.id::text)) % 100 AS bucket
			FROM users u
			WHERE u.assigned_rm_id IS NOT NULL
			  AND NOT EXISTS (SELECT 1 FROM idbi_credit_exposure e WHERE e.user_id = u.id)
		), flows AS (
			SELECT user_id,
			       SUM(amount) FILTER (WHERE type = 'CREDIT') / 3.0 AS inc,
			       SUM(amount) FILTER (WHERE type = 'DEBIT')  / 3.0 AS deb
			FROM spend_transactions
			WHERE occurred_at >= date_trunc('month', now()) - INTERVAL '3 months'
			  AND occurred_at <  date_trunc('month', now())
			GROUP BY user_id
		), scored AS (
			SELECT t.id, t.name, t.rn,
			       GREATEST(COALESCE(f.inc, 100000), 25000) * 12 AS total_limit,
			       CASE WHEN f.inc > 0 THEN f.deb / f.inc ELSE 0 END AS ratio
			FROM todo t
			LEFT JOIN flows f ON f.user_id = t.id
		), shaped AS (
			SELECT sc.id, sc.name, sc.ratio, sc.total_limit,
			       COALESCE(l.cif_id, s.cust_id,
			                'CIF' || lpad((abs(hashtext(sc.id::text)) % 1000000)::text, 6, '0')) AS cif,
			       CASE WHEN sc.ratio < 0.82 THEN 'LOW' WHEN sc.ratio < 0.95 THEN 'WATCH' ELSE 'HIGH' END AS band,
			       CASE WHEN sc.ratio < 0.82 THEN 0
			            WHEN sc.ratio < 0.95 THEN 5 + round((sc.ratio - 0.82) / 0.13 * 20)
			            WHEN sc.ratio < 1.10 THEN 31 + round((sc.ratio - 0.95) / 0.15 * 58)
			            ELSE LEAST(180, 90 + round((sc.ratio - 1.10) * 200)) END::int AS dpd,
			       LEAST(95, GREATEST(15, round((sc.ratio - 0.4) * 100)))::int AS util,
			       abs(hashtext(sc.id::text)) % 3 AS loans_extra
			FROM scored sc
			LEFT JOIN idbi_customer_link l ON l.user_id = sc.id
			CROSS JOIN seed_cnt c
			LEFT JOIN seed_ids s ON s.rn = ((sc.rn - 1) % NULLIF(c.n, 0)) + 1
		)
		INSERT INTO idbi_credit_exposure
			(user_id, cif_id, customer_name, account_manager, cust_rating,
			 total_limit, funded_limit, non_funded_limit, total_outstanding, utilisation_pct,
			 loan_count, total_overdue, max_dpd, worst_npa_status, risk_band, synced_at)
		SELECT id, cif, name, 'Seeded',
		       CASE band WHEN 'LOW' THEN 'A' WHEN 'WATCH' THEN 'B' ELSE 'C' END,
		       round(total_limit), round(total_limit * 0.85), round(total_limit * 0.15),
		       round(total_limit * util / 100), util,
		       1 + loans_extra,
		       CASE WHEN dpd > 0 THEN round(total_limit * util / 100 * 0.04 * (1 + dpd / 30.0)) ELSE 0 END,
		       dpd,
		       CASE WHEN dpd >= 90 THEN 'SUBSTANDARD' ELSE 'STANDARD' END,
		       band,
		       now() - ((abs(hashtext(id::text)) % 48) || ' hours')::interval
		FROM shaped
		ON CONFLICT (user_id) DO NOTHING
	`)
	if err != nil {
		return 0, fmt.Errorf("seed credit exposure: %w", err)
	}
	return tag.RowsAffected(), nil
}
