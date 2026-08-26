package service

import (
	"context"

	catalogdomain "github.com/yourusername/astra-backend/internal/domain/catalog"
	catalogprovider "github.com/yourusername/astra-backend/internal/provider/catalog"
)

type CatalogService struct {
	provider catalogprovider.Provider
}

func NewCatalogService(provider catalogprovider.Provider) *CatalogService {
	return &CatalogService{provider: provider}
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
