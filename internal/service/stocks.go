package service

import (
	"context"

	"github.com/google/uuid"

	stocksdomain "github.com/yourusername/astra-backend/internal/domain/stocks"
	stocksprovider "github.com/yourusername/astra-backend/internal/provider/stocks"
)

// StocksService is a thin orchestration layer over the swappable
// stocksprovider.Provider — it's the seam where cross-cutting rules
// (auditing, additional validation) can be added later without touching the
// HTTP handler or any provider implementation.
type StocksService struct {
	provider stocksprovider.Provider
}

func NewStocksService(provider stocksprovider.Provider) *StocksService {
	return &StocksService{provider: provider}
}

func (s *StocksService) GetHoldings(ctx context.Context, userID uuid.UUID) ([]stocksdomain.Holding, error) {
	return s.provider.GetHoldings(ctx, userID)
}

func (s *StocksService) GetQuote(ctx context.Context, exchange, tradingSymbol string) (*stocksdomain.Quote, error) {
	return s.provider.GetQuote(ctx, exchange, tradingSymbol)
}

func (s *StocksService) PlaceOrder(ctx context.Context, userID uuid.UUID, req stocksdomain.OrderRequest) (*stocksdomain.Order, error) {
	return s.provider.PlaceOrder(ctx, userID, req)
}

func (s *StocksService) ModifyOrder(ctx context.Context, userID uuid.UUID, orderID string, req stocksdomain.OrderRequest) (*stocksdomain.Order, error) {
	return s.provider.ModifyOrder(ctx, userID, orderID, req)
}

func (s *StocksService) CancelOrder(ctx context.Context, userID uuid.UUID, orderID string) (*stocksdomain.Order, error) {
	return s.provider.CancelOrder(ctx, userID, orderID)
}

func (s *StocksService) GetOrder(ctx context.Context, userID uuid.UUID, orderID string) (*stocksdomain.Order, error) {
	return s.provider.GetOrder(ctx, userID, orderID)
}
