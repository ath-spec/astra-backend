// Package stocks defines the swappable interface behind the Demat & Exchange
// domain. The mock implementation in this package persists realistic,
// deterministic per-user data to Postgres. A future real integration (the
// existing single-broker/Kite-style feed, or the IDBI sandbox equivalent)
// implements the same Provider interface — handlers, services and routes
// never change when that swap happens.
package stocks

import (
	"context"

	"github.com/google/uuid"
	"github.com/yourusername/astra-backend/internal/domain/stocks"
)

type Provider interface {
	// GetHoldings returns the user's current demat holdings, lazily seeding
	// a realistic starter portfolio the first time a given user is fetched.
	GetHoldings(ctx context.Context, userID uuid.UUID) ([]stocks.Holding, error)

	// GetQuote returns a live-style quote for a single instrument.
	GetQuote(ctx context.Context, exchange, tradingSymbol string) (*stocks.Quote, error)

	// PlaceOrder validates and places a new order, simulating an
	// exchange fill where applicable and updating holdings accordingly.
	PlaceOrder(ctx context.Context, userID uuid.UUID, req stocks.OrderRequest) (*stocks.Order, error)

	// ModifyOrder updates a still-OPEN order's terms and re-attempts a fill.
	ModifyOrder(ctx context.Context, userID uuid.UUID, orderID string, req stocks.OrderRequest) (*stocks.Order, error)

	// CancelOrder cancels a still-OPEN order's remaining pending quantity.
	CancelOrder(ctx context.Context, userID uuid.UUID, orderID string) (*stocks.Order, error)

	// GetOrder returns the current state of a single order.
	GetOrder(ctx context.Context, userID uuid.UUID, orderID string) (*stocks.Order, error)

	// ListOrders returns every order the user has ever placed, most recent
	// first, optionally filtered by status.
	ListOrders(ctx context.Context, userID uuid.UUID, statusFilter string) ([]stocks.Order, error)
}
