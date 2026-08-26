package payments

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourusername/astra-backend/internal/apiresponse"
	paymentsdomain "github.com/yourusername/astra-backend/internal/domain/payments"
	"github.com/yourusername/astra-backend/internal/repository"
)

type MockProvider struct {
	pool     *pgxpool.Pool
	userRepo repository.UserRepository
}

func NewMockProvider(pool *pgxpool.Pool, userRepo repository.UserRepository) *MockProvider {
	return &MockProvider{pool: pool, userRepo: userRepo}
}

func newID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UTC().UnixNano())
}

// resolveBankAccountID returns the caller-specified bank account (verifying
// it belongs to userID) or the user's primary account if none was specified.
// Unlike the FD domain's equivalent, this never locks/debits the row —
// payments and mandates here don't move real money, only record the
// attempt/authorization against an account the user actually owns.
func (p *MockProvider) resolveBankAccountID(ctx context.Context, userID uuid.UUID, bankAccountID string) (uuid.UUID, error) {
	if bankAccountID == "" {
		acc, err := p.userRepo.GetPrimaryBankAccount(ctx, userID)
		if err != nil {
			return uuid.Nil, fmt.Errorf("resolve default bank account: %w", err)
		}
		return acc.ID, nil
	}

	accID, err := uuid.Parse(bankAccountID)
	if err != nil {
		return uuid.Nil, apiresponse.Validation("bank_account_id is not a valid identifier")
	}
	var exists bool
	if err := p.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM bank_accounts WHERE id = $1 AND user_id = $2)`, accID, userID).Scan(&exists); err != nil {
		return uuid.Nil, fmt.Errorf("verify bank account ownership: %w", err)
	}
	if !exists {
		return uuid.Nil, apiresponse.Validation("bank_account_id does not belong to this user")
	}
	return accID, nil
}

func validatePaymentRequest(req paymentsdomain.PaymentRequest) error {
	if req.TxnID == "" {
		return apiresponse.Validation("txn_id is required")
	}
	if req.Amount <= 0 {
		return apiresponse.Validation("amount must be positive")
	}
	switch req.PaymentMode {
	case "UPI", "NETBANKING", "CARD":
	default:
		return apiresponse.Validation("payment_mode must be one of UPI, NETBANKING, CARD")
	}
	if req.PaymentMode == "UPI" && req.UPIID == "" {
		return apiresponse.Validation("upi_id is required when payment_mode is UPI")
	}
	if req.Purpose == "" {
		return apiresponse.Validation("purpose is required")
	}
	return nil
}

func scanPayment(row interface{ Scan(dest ...any) error }) (paymentsdomain.Payment, error) {
	var pay paymentsdomain.Payment
	if err := row.Scan(&pay.PaymentID, &pay.TxnID, &pay.Status, &pay.Mode, &pay.BankRefNum, &pay.ErrorCode, &pay.ErrorMessage, &pay.ProcessedAt); err != nil {
		return paymentsdomain.Payment{}, fmt.Errorf("scan payment: %w", err)
	}
	return pay, nil
}

const paymentColumns = `payment_id, txn_id, status, mode, bank_ref_num, error_code, error_message, processed_at`

// InitiatePayment is idempotent on txn_id: replaying the same client-
// generated txn_id returns the original outcome instead of creating a
// second payment attempt.
func (p *MockProvider) InitiatePayment(ctx context.Context, userID uuid.UUID, req paymentsdomain.PaymentRequest) (*paymentsdomain.Payment, error) {
	if err := validatePaymentRequest(req); err != nil {
		return nil, err
	}

	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin payment tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if existing, err := scanPayment(tx.QueryRow(ctx, `SELECT `+paymentColumns+` FROM payments WHERE txn_id = $1 AND user_id = $2`, req.TxnID, userID)); err == nil {
		return &existing, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("check existing payment: %w", err)
	}

	bankAccountID, err := p.resolveBankAccountID(ctx, userID, req.BankAccountID)
	if err != nil {
		return nil, err
	}

	var balance float64
	if err := tx.QueryRow(ctx, `SELECT balance FROM bank_accounts WHERE id = $1 AND user_id = $2 FOR UPDATE`, bankAccountID, userID).Scan(&balance); err != nil {
		return nil, fmt.Errorf("lock bank account: %w", err)
	}

	paymentID := newID("PAYU")
	now := time.Now().UTC()
	pay := paymentsdomain.Payment{PaymentID: paymentID, TxnID: req.TxnID, Mode: req.PaymentMode, ProcessedAt: &now}

	if balance < req.Amount {
		pay.Status = paymentsdomain.PaymentStatusFailure
		code, msg := "INSUFFICIENT_FUNDS", "Source account balance is lower than the requested amount"
		pay.ErrorCode, pay.ErrorMessage = &code, &msg
	} else {
		if _, err := tx.Exec(ctx, `UPDATE bank_accounts SET balance = balance - $1 WHERE id = $2 AND user_id = $3`, req.Amount, bankAccountID, userID); err != nil {
			return nil, fmt.Errorf("debit source account: %w", err)
		}
		pay.Status = paymentsdomain.PaymentStatusSuccess
		ref := newID("REF")
		pay.BankRefNum = &ref
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO payments (payment_id, txn_id, user_id, bank_account_id, amount, payment_mode, upi_id, purpose, status, mode, bank_ref_num, error_code, error_message, processed_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
	`, paymentID, req.TxnID, userID, bankAccountID, req.Amount, req.PaymentMode, nullIfEmpty(req.UPIID), req.Purpose,
		pay.Status, pay.Mode, pay.BankRefNum, pay.ErrorCode, pay.ErrorMessage, pay.ProcessedAt); err != nil {
		return nil, fmt.Errorf("insert payment: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit payment: %w", err)
	}
	return &pay, nil
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (p *MockProvider) GetPayment(ctx context.Context, userID uuid.UUID, paymentID string) (*paymentsdomain.Payment, error) {
	pay, err := scanPayment(p.pool.QueryRow(ctx, `SELECT `+paymentColumns+` FROM payments WHERE payment_id = $1 AND user_id = $2`, paymentID, userID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apiresponse.NotFound("payment %s not found", paymentID)
		}
		return nil, err
	}
	return &pay, nil
}

func validateMandateRequest(req paymentsdomain.MandateRequest) (time.Time, error) {
	if req.MandateAmount <= 0 {
		return time.Time{}, apiresponse.Validation("mandate_amount must be positive")
	}
	switch req.MandateFrequency {
	case "MONTHLY", "QUARTERLY":
	default:
		return time.Time{}, apiresponse.Validation("mandate_frequency must be MONTHLY or QUARTERLY")
	}
	start, err := time.Parse("2006-01-02", req.MandateStartDate)
	if err != nil {
		return time.Time{}, apiresponse.Validation("mandate_start_date must be an ISO date (YYYY-MM-DD)")
	}
	return start, nil
}

func (p *MockProvider) CreateMandate(ctx context.Context, userID uuid.UUID, req paymentsdomain.MandateRequest) (*paymentsdomain.Mandate, error) {
	startDate, err := validateMandateRequest(req)
	if err != nil {
		return nil, err
	}
	bankAccountID, err := p.resolveBankAccountID(ctx, userID, req.BankAccountID)
	if err != nil {
		return nil, err
	}

	mandateType := req.MandateType
	if mandateType == "" {
		mandateType = paymentsdomain.MandateTypeUPIAutopay
	}

	var endDate *time.Time
	if req.MandateEndDate != "" {
		d, err := time.Parse("2006-01-02", req.MandateEndDate)
		if err != nil {
			return nil, apiresponse.Validation("mandate_end_date must be an ISO date (YYYY-MM-DD)")
		}
		endDate = &d
	}

	mandateID := newID("MNDT")
	now := time.Now().UTC()
	// A real mandate goes through PENDING until the payer approves it in
	// their bank app; the mock simulates instant approval since there's no
	// real bank app in the loop.
	status := paymentsdomain.MandateStatusActive

	if _, err := p.pool.Exec(ctx, `
		INSERT INTO mandates (
			mandate_id, user_id, bank_account_id, mandate_type, upi_id, payee_name, payee_vpa_or_id,
			max_amount, frequency, mandate_start_date, mandate_end_date, next_debit_date, status, approved_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
	`, mandateID, userID, bankAccountID, mandateType, nullIfEmpty(req.UPIID), nullIfEmpty(req.PayeeName), nullIfEmpty(req.PayeeVPAOrID),
		req.MandateAmount, req.MandateFrequency, startDate, endDate, startDate, status, now); err != nil {
		return nil, fmt.Errorf("insert mandate: %w", err)
	}

	nextDebit := startDate.Format("2006-01-02")
	return &paymentsdomain.Mandate{
		MandateID:     mandateID,
		MandateType:   mandateType,
		PayeeName:     nullIfEmpty(req.PayeeName),
		PayeeVPAOrID:  nullIfEmpty(req.PayeeVPAOrID),
		MaxAmount:     req.MandateAmount,
		Frequency:     req.MandateFrequency,
		NextDebitDate: &nextDebit,
		Status:        status,
		ApprovedAt:    &now,
		CreatedAt:     now.Format("2006-01-02"),
	}, nil
}

const mandateColumns = `mandate_id, mandate_type, payee_name, payee_vpa_or_id, max_amount, frequency, next_debit_date, status, approved_at, created_at`

func scanMandate(row interface{ Scan(dest ...any) error }) (paymentsdomain.Mandate, error) {
	var m paymentsdomain.Mandate
	var nextDebit *time.Time
	var createdAt time.Time
	if err := row.Scan(&m.MandateID, &m.MandateType, &m.PayeeName, &m.PayeeVPAOrID, &m.MaxAmount, &m.Frequency, &nextDebit, &m.Status, &m.ApprovedAt, &createdAt); err != nil {
		return paymentsdomain.Mandate{}, fmt.Errorf("scan mandate: %w", err)
	}
	if nextDebit != nil {
		d := nextDebit.Format("2006-01-02")
		m.NextDebitDate = &d
	}
	m.CreatedAt = createdAt.Format("2006-01-02")
	return m, nil
}

func (p *MockProvider) ListMandates(ctx context.Context, userID uuid.UUID, statusFilter string) ([]paymentsdomain.Mandate, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT `+mandateColumns+` FROM mandates
		WHERE user_id = $1 AND ($2 = '' OR status = $2)
		ORDER BY created_at DESC
	`, userID, statusFilter)
	if err != nil {
		return nil, fmt.Errorf("list mandates: %w", err)
	}
	defer rows.Close()

	mandates := make([]paymentsdomain.Mandate, 0)
	for rows.Next() {
		m, err := scanMandate(rows)
		if err != nil {
			return nil, err
		}
		mandates = append(mandates, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate mandates: %w", err)
	}
	return mandates, nil
}

var mandateTransitions = map[string]map[string]string{
	paymentsdomain.ActionPause:  {paymentsdomain.MandateStatusActive: paymentsdomain.MandateStatusPaused},
	paymentsdomain.ActionResume: {paymentsdomain.MandateStatusPaused: paymentsdomain.MandateStatusActive},
	paymentsdomain.ActionCancel: {
		paymentsdomain.MandateStatusActive:  paymentsdomain.MandateStatusRevoked,
		paymentsdomain.MandateStatusPaused:  paymentsdomain.MandateStatusRevoked,
		paymentsdomain.MandateStatusPending: paymentsdomain.MandateStatusRevoked,
	},
}

func (p *MockProvider) MandateAction(ctx context.Context, userID uuid.UUID, mandateID string, req paymentsdomain.MandateActionRequest) (*paymentsdomain.MandateActionResult, error) {
	transitionsForAction, ok := mandateTransitions[req.Action]
	if !ok {
		return nil, apiresponse.Validation("action must be one of PAUSE, RESUME, CANCEL")
	}

	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin mandate action tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var currentStatus string
	if err := tx.QueryRow(ctx, `SELECT status FROM mandates WHERE mandate_id = $1 AND user_id = $2 FOR UPDATE`, mandateID, userID).Scan(&currentStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apiresponse.NotFound("mandate %s not found", mandateID)
		}
		return nil, fmt.Errorf("lock mandate: %w", err)
	}

	newStatus, ok := transitionsForAction[currentStatus]
	if !ok {
		return nil, fmt.Errorf("mandate %s is %s and cannot be %sd: %w", mandateID, currentStatus, req.Action, apiresponse.ErrConflict)
	}

	if _, err := tx.Exec(ctx, `UPDATE mandates SET status = $1, updated_at = now() WHERE mandate_id = $2 AND user_id = $3`, newStatus, mandateID, userID); err != nil {
		return nil, fmt.Errorf("update mandate status: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit mandate action: %w", err)
	}

	return &paymentsdomain.MandateActionResult{
		MandateID:     mandateID,
		Status:        newStatus,
		EffectiveFrom: time.Now().UTC().Format("2006-01-02"),
	}, nil
}
