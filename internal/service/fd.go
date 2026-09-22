package service

import (
	"context"

	"github.com/google/uuid"

	fddomain "github.com/yourusername/astra-backend/internal/domain/fd"
	fdprovider "github.com/yourusername/astra-backend/internal/provider/fd"
)

type FDService struct {
	provider fdprovider.Provider
}

func NewFDService(provider fdprovider.Provider) *FDService {
	return &FDService{provider: provider}
}

func (s *FDService) OpenFD(ctx context.Context, userID uuid.UUID, req fddomain.OpenRequest) (*fddomain.Account, error) {
	return s.provider.OpenFD(ctx, userID, req)
}

func (s *FDService) ListFDs(ctx context.Context, userID uuid.UUID) ([]fddomain.Account, error) {
	return s.provider.ListFDs(ctx, userID)
}

func (s *FDService) GetFD(ctx context.Context, userID uuid.UUID, fdAccountNumber string) (*fddomain.Account, error) {
	return s.provider.GetFD(ctx, userID, fdAccountNumber)
}

func (s *FDService) Withdraw(ctx context.Context, userID uuid.UUID, fdAccountNumber string, req fddomain.WithdrawRequest) (*fddomain.WithdrawResult, error) {
	return s.provider.Withdraw(ctx, userID, fdAccountNumber, req)
}
