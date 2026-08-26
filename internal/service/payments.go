package service

import (
	"context"

	"github.com/google/uuid"

	paymentsdomain "github.com/yourusername/astra-backend/internal/domain/payments"
	paymentsprovider "github.com/yourusername/astra-backend/internal/provider/payments"
)

type PaymentsService struct {
	provider paymentsprovider.Provider
}

func NewPaymentsService(provider paymentsprovider.Provider) *PaymentsService {
	return &PaymentsService{provider: provider}
}

func (s *PaymentsService) InitiatePayment(ctx context.Context, userID uuid.UUID, req paymentsdomain.PaymentRequest) (*paymentsdomain.Payment, error) {
	return s.provider.InitiatePayment(ctx, userID, req)
}

func (s *PaymentsService) GetPayment(ctx context.Context, userID uuid.UUID, paymentID string) (*paymentsdomain.Payment, error) {
	return s.provider.GetPayment(ctx, userID, paymentID)
}

func (s *PaymentsService) CreateMandate(ctx context.Context, userID uuid.UUID, req paymentsdomain.MandateRequest) (*paymentsdomain.Mandate, error) {
	return s.provider.CreateMandate(ctx, userID, req)
}

func (s *PaymentsService) ListMandates(ctx context.Context, userID uuid.UUID, statusFilter string) ([]paymentsdomain.Mandate, error) {
	return s.provider.ListMandates(ctx, userID, statusFilter)
}

func (s *PaymentsService) MandateAction(ctx context.Context, userID uuid.UUID, mandateID string, req paymentsdomain.MandateActionRequest) (*paymentsdomain.MandateActionResult, error) {
	return s.provider.MandateAction(ctx, userID, mandateID, req)
}
