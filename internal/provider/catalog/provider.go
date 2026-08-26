// Package catalog defines the swappable interface behind the Fund & Scheme
// Catalog / NFO domain. A future real integration (an AMFI/RTA feed) would
// implement the same Provider interface backing live data instead of the
// seeded reference table this mock reads from.
package catalog

import (
	"context"

	catalogdomain "github.com/yourusername/astra-backend/internal/domain/catalog"
)

type Provider interface {
	SearchFunds(ctx context.Context, params catalogdomain.SearchParams) ([]catalogdomain.Fund, error)
	GetFund(ctx context.Context, schemeCode string) (*catalogdomain.Fund, error)
	ListNFOs(ctx context.Context) ([]catalogdomain.NFO, error)
}
