package service

import (
	"context"

	"github.com/google/uuid"

	watchlistdomain "github.com/yourusername/astra-backend/internal/domain/watchlist"
	watchlistprovider "github.com/yourusername/astra-backend/internal/provider/watchlist"
)

type WatchlistService struct {
	provider *watchlistprovider.PostgresProvider
}

func NewWatchlistService(provider *watchlistprovider.PostgresProvider) *WatchlistService {
	return &WatchlistService{provider: provider}
}

func (s *WatchlistService) Add(ctx context.Context, userID uuid.UUID, schemeCode string) error {
	return s.provider.Add(ctx, userID, schemeCode)
}

func (s *WatchlistService) Remove(ctx context.Context, userID uuid.UUID, schemeCode string) error {
	return s.provider.Remove(ctx, userID, schemeCode)
}

func (s *WatchlistService) List(ctx context.Context, userID uuid.UUID) ([]watchlistdomain.Item, error) {
	return s.provider.List(ctx, userID)
}
