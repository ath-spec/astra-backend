// Package mf defines the swappable interface behind the Mutual Fund
// investment domain. A future real integration (a CAMS/KFintech-style RTA,
// or MF Central/CAS once that provider is chosen — see internal/handler/mf.go's
// former 501 stub) implements the same Provider interface; the mock
// implementation here persists to the same mf_folios/mf_transactions tables
// that stub was already reserving.
package mf

import (
	"context"

	"github.com/google/uuid"

	mfdomain "github.com/yourusername/astra-backend/internal/domain/mf"
)

type Provider interface {
	GetHoldings(ctx context.Context, userID uuid.UUID) (*mfdomain.HoldingsResult, error)

	// GetHolding looks up a single scheme's holding, if any, without
	// triggering the starter-portfolio seed GetHoldings performs — so
	// browsing a fund's profile page never has the side effect of creating
	// holdings the user never asked for. Returns nil, nil if not held.
	GetHolding(ctx context.Context, userID uuid.UUID, schemeCode string) (*mfdomain.Folio, error)

	Purchase(ctx context.Context, userID uuid.UUID, req mfdomain.PurchaseRequest) (*mfdomain.Transaction, error)
	Redeem(ctx context.Context, userID uuid.UUID, req mfdomain.RedeemRequest) (*mfdomain.RedeemResult, error)
	GetTransactions(ctx context.Context, userID uuid.UUID, schemeCode string) ([]mfdomain.Transaction, error)
}
