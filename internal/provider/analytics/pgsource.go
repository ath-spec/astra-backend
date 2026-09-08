package analytics

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	analyticsdomain "github.com/yourusername/astra-backend/internal/domain/analytics"
)

// PgSource is a TransactionSource that reads spend_transactions as-is — no
// seeding. It's the source used when real transactions are being synced in
// (IDBI 393 / AA 595, via internal/service/statementsync) instead of the
// MockSource's generated data. Identical read path to MockSource, minus
// ensureSeeded.
type PgSource struct {
	pool *pgxpool.Pool
}

func NewPgSource(pool *pgxpool.Pool) *PgSource { return &PgSource{pool: pool} }

func (s *PgSource) GetTransactions(ctx context.Context, userID uuid.UUID, from, to time.Time) ([]analyticsdomain.Transaction, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, amount, type, category, merchant, occurred_at
		FROM spend_transactions
		WHERE user_id = $1 AND occurred_at >= $2 AND occurred_at <= $3
		ORDER BY occurred_at
	`, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("query spend transactions: %w", err)
	}
	defer rows.Close()

	txns := make([]analyticsdomain.Transaction, 0)
	for rows.Next() {
		var t analyticsdomain.Transaction
		var typ string
		if err := rows.Scan(&t.ID, &t.Amount, &typ, &t.Category, &t.Merchant, &t.OccurredAt); err != nil {
			return nil, fmt.Errorf("scan spend transaction: %w", err)
		}
		t.Type = analyticsdomain.TxnType(typ)
		txns = append(txns, t)
	}
	return txns, rows.Err()
}
