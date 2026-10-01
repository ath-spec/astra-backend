package rmseed

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SeedSpendStress adds loan-EMI and credit-card-bill debits to the spend
// history of some RM-assigned clients, so debt pressure shows up in the data
// and SeedCreditExposure can derive risk from it instead of assigning it.
//
// The signup seeding in user_seed_spend.go gives every client the same saver
// profile (~25% of income saved), so without this nobody is ever stressed.
// This is deliberately a demo-only step here, not part of signup: ~60% of
// clients are left alone, ~20% are pushed to ~88% of income spent, ~15% to
// ~102% and ~5% to ~125%. The extra debit is sized per client from their own
// average income and spend, and ramps up toward the recent months so the
// trend worsens. Idempotent: clients that already have any EMI debit (seeded or
// real) are skipped. Gate behind RM_SEED_CREDIT_EXPOSURE=true.
func SeedSpendStress(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	tag, err := pool.Exec(ctx, `
		WITH todo AS (
			SELECT u.id, abs(hashtext(u.id::text || 'stress')) % 100 AS bucket
			FROM users u
			WHERE u.assigned_rm_id IS NOT NULL
			  AND NOT EXISTS (SELECT 1 FROM spend_transactions s
			                  WHERE s.user_id = u.id AND s.type = 'DEBIT' AND s.category = 'EMI')
		), avgs AS (
			SELECT user_id,
			       SUM(amount) FILTER (WHERE type = 'CREDIT') / 6.0 AS inc,
			       SUM(amount) FILTER (WHERE type = 'DEBIT')  / 6.0 AS deb
			FROM spend_transactions
			WHERE occurred_at >= date_trunc('month', now()) - INTERVAL '6 months'
			  AND occurred_at <  date_trunc('month', now())
			GROUP BY user_id
		), plan AS (
			SELECT t.id,
			       GREATEST(0, a.inc * CASE WHEN t.bucket < 60 THEN 0
			                                WHEN t.bucket < 80 THEN 0.88
			                                WHEN t.bucket < 95 THEN 1.02
			                                ELSE 1.25 END - a.deb) AS extra
			FROM todo t JOIN avgs a ON a.user_id = t.id
			WHERE a.inc > 0 AND t.bucket >= 60
		), months AS (
			SELECT m FROM generate_series(1, 6) m
		)
		INSERT INTO spend_transactions (user_id, amount, type, category, merchant, occurred_at)
		SELECT p.id,
		       round((p.extra * share * (0.8 + 0.4 * (7 - m) / 6.0))::numeric, 2),
		       'DEBIT', cat, merch,
		       date_trunc('month', now()) - (m || ' months')::interval + (day || ' days')::interval
		FROM plan p
		CROSS JOIN months
		CROSS JOIN (VALUES (0.55, 'EMI', 'Loan EMI', 4),
		                   (0.45, 'Credit Card Bill', 'Card Bill', 11)) AS parts(share, cat, merch, day)
		WHERE p.extra > 0
	`)
	if err != nil {
		return 0, fmt.Errorf("seed spend stress: %w", err)
	}
	return tag.RowsAffected(), nil
}
