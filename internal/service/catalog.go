package service

import (
	"context"

	"github.com/google/uuid"

	catalogdomain "github.com/yourusername/astra-backend/internal/domain/catalog"
	catalogprovider "github.com/yourusername/astra-backend/internal/provider/catalog"
	mfprovider "github.com/yourusername/astra-backend/internal/provider/mf"
)

type CatalogService struct {
	provider catalogprovider.Provider
	mf       mfprovider.Provider
}

func NewCatalogService(provider catalogprovider.Provider, mf mfprovider.Provider) *CatalogService {
	return &CatalogService{provider: provider, mf: mf}
}

func (s *CatalogService) SearchFunds(ctx context.Context, params catalogdomain.SearchParams) ([]catalogdomain.Fund, error) {
	return s.provider.SearchFunds(ctx, params)
}

func (s *CatalogService) GetFund(ctx context.Context, schemeCode string) (*catalogdomain.Fund, error) {
	return s.provider.GetFund(ctx, schemeCode)
}

func (s *CatalogService) ListNFOs(ctx context.Context) ([]catalogdomain.NFO, error) {
	return s.provider.ListNFOs(ctx)
}

// GetFundProfile returns a fund's full profile, cross-referencing the MF
// domain to fill in UserHolding when userID currently holds this fund (the
// catalog provider itself has no notion of per-user holdings).
func (s *CatalogService) GetFundProfile(ctx context.Context, userID uuid.UUID, schemeCode string) (*catalogdomain.FundProfile, error) {
	profile, err := s.provider.GetFundProfile(ctx, schemeCode)
	if err != nil {
		return nil, err
	}

	holding, err := s.mf.GetHolding(ctx, userID, schemeCode)
	if err != nil {
		return nil, err
	}
	if holding != nil {
		profile.UserHolding = &catalogdomain.UserHolding{
			UnitsHeld:     holding.UnitsHeld,
			InvestedValue: holding.InvestedValue,
			CurrentValue:  holding.CurrentValue,
			ReturnsPct:    holding.ReturnsPct,
		}
	}
	return profile, nil
}
