// Package fd defines the swappable interface behind the Fixed Deposits
// domain. A future real integration (the bank's FD booking API) implements
// the same Provider interface in place of the persisted mock below.
package fd

import (
	"context"

	"github.com/google/uuid"

	fddomain "github.com/yourusername/astra-backend/internal/domain/fd"
)

type Provider interface {
	OpenFD(ctx context.Context, userID uuid.UUID, req fddomain.OpenRequest) (*fddomain.Account, error)
	ListFDs(ctx context.Context, userID uuid.UUID) ([]fddomain.Account, error)
	GetFD(ctx context.Context, userID uuid.UUID, fdAccountNumber string) (*fddomain.Account, error)
	Withdraw(ctx context.Context, userID uuid.UUID, fdAccountNumber string, req fddomain.WithdrawRequest) (*fddomain.WithdrawResult, error)
}
