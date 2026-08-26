// Package analytics defines the swappable interface behind the spend
// analytics engine's data source. A future real integration (the AA
// transaction feed, once that domain is wired in — see
// internal/handler/aa_handler.go) implements the same TransactionSource
// interface in place of the seeded mock below; the analyzer engine and every
// handler above it stays unchanged.
package analytics

import (
	"context"
	"time"

	"github.com/google/uuid"

	analyticsdomain "github.com/yourusername/astra-backend/internal/domain/analytics"
)

type TransactionSource interface {
	// GetTransactions returns transactions for userID with OccurredAt in
	// [from, to], ordered oldest first.
	GetTransactions(ctx context.Context, userID uuid.UUID, from, to time.Time) ([]analyticsdomain.Transaction, error)
}
