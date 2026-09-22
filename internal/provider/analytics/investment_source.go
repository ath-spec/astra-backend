package analytics

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	analyticsdomain "github.com/yourusername/astra-backend/internal/domain/analytics"
)

// InvestmentSource is the swappable interface behind investment-consistency
// analysis. The Postgres implementation reads the Stocks domain's
// stock_orders table directly (read-only, no new table) — a future real
// provider would read whatever ledger a real brokerage integration keeps.
type InvestmentSource interface {
	GetInvestmentEvents(ctx context.Context, userID uuid.UUID, from, to time.Time) ([]analyticsdomain.InvestmentEvent, error)
}

type PgInvestmentSource struct {
	pool *pgxpool.Pool
}

func NewPgInvestmentSource(pool *pgxpool.Pool) *PgInvestmentSource {
	return &PgInvestmentSource{pool: pool}
}

func (s *PgInvestmentSource) GetInvestmentEvents(ctx context.Context, userID uuid.UUID, from, to time.Time) ([]analyticsdomain.InvestmentEvent, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT order_timestamp, quantity * COALESCE(average_price, 0) AS invested_amount
		FROM stock_orders
		WHERE user_id = $1 AND transaction_type = 'BUY' AND status = 'COMPLETE'
		  AND order_timestamp >= $2 AND order_timestamp <= $3
		ORDER BY order_timestamp
	`, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("query investment events: %w", err)
	}
	defer rows.Close()

	events := make([]analyticsdomain.InvestmentEvent, 0)
	for rows.Next() {
		var e analyticsdomain.InvestmentEvent
		if err := rows.Scan(&e.OccurredAt, &e.Amount); err != nil {
			return nil, fmt.Errorf("scan investment event: %w", err)
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate investment events: %w", err)
	}
	return events, nil
}
