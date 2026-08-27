// Package payments defines the swappable interface behind the Payments,
// Funds Transfer & Mandate Management domain. A future real integration
// (the existing payment-gateway integration, plus a UPI Autopay/NACH
// mandate provider) implements the same Provider interface.
package payments

import (
	"context"

	"github.com/google/uuid"

	paymentsdomain "github.com/yourusername/astra-backend/internal/domain/payments"
)

type Provider interface {
	InitiatePayment(ctx context.Context, userID uuid.UUID, req paymentsdomain.PaymentRequest) (*paymentsdomain.Payment, error)
	GetPayment(ctx context.Context, userID uuid.UUID, paymentID string) (*paymentsdomain.Payment, error)

	CreateMandate(ctx context.Context, userID uuid.UUID, req paymentsdomain.MandateRequest) (*paymentsdomain.Mandate, error)
	ListMandates(ctx context.Context, userID uuid.UUID, statusFilter string) ([]paymentsdomain.Mandate, error)
	MandateAction(ctx context.Context, userID uuid.UUID, mandateID string, req paymentsdomain.MandateActionRequest) (*paymentsdomain.MandateActionResult, error)
	MandateHistory(ctx context.Context, userID uuid.UUID, mandateID string, limit, offset int) (paymentsdomain.ExecutionPage, error)
	RecurringSummary(ctx context.Context, userID uuid.UUID) (paymentsdomain.RecurringSummary, error)
}
