package service

import (
	"context"

	"github.com/google/uuid"

	mfdomain "github.com/yourusername/astra-backend/internal/domain/mf"
	mfprovider "github.com/yourusername/astra-backend/internal/provider/mf"
)

type MFService struct {
	provider mfprovider.Provider
}

func NewMFService(provider mfprovider.Provider) *MFService {
	return &MFService{provider: provider}
}

func (s *MFService) GetHoldings(ctx context.Context, userID uuid.UUID) (*mfdomain.HoldingsResult, error) {
	return s.provider.GetHoldings(ctx, userID)
}

func (s *MFService) Purchase(ctx context.Context, userID uuid.UUID, req mfdomain.PurchaseRequest) (*mfdomain.Transaction, error) {
	return s.provider.Purchase(ctx, userID, req)
}

func (s *MFService) Redeem(ctx context.Context, userID uuid.UUID, req mfdomain.RedeemRequest) (*mfdomain.RedeemResult, error) {
	return s.provider.Redeem(ctx, userID, req)
}

func (s *MFService) GetTransactions(ctx context.Context, userID uuid.UUID, schemeCode string) ([]mfdomain.Transaction, error) {
	return s.provider.GetTransactions(ctx, userID, schemeCode)
}
