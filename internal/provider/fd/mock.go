package fd

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourusername/astra-backend/internal/apiresponse"
	"github.com/yourusername/astra-backend/internal/apitime"
	fddomain "github.com/yourusername/astra-backend/internal/domain/fd"
	"github.com/yourusername/astra-backend/internal/repository"
)

// MockProvider books FDs against the user's own (mock) bank_accounts
// balance — opening one debits the source account, and withdrawing credits
// the net payout back — so the numbers stay internally consistent even
// though no real bank is involved yet.
type MockProvider struct {
	pool     *pgxpool.Pool
	userRepo repository.UserRepository
}

func NewMockProvider(pool *pgxpool.Pool, userRepo repository.UserRepository) *MockProvider {
	return &MockProvider{pool: pool, userRepo: userRepo}
}

// rateForTenure is a static published-rate slab, standing in for the bank's
// real rate card.
func rateForTenure(months int) float64 {
	switch {
	case months < 6:
		return 6.00
	case months < 12:
		return 6.50
	case months < 24:
		return 7.00
	case months < 36:
		return 7.25
	case months <= 60:
		return 7.50
	default:
		return 7.10
	}
}

// maturityAmount compounds quarterly, the common convention for bank FDs.
// For MONTHLY/QUARTERLY payout FDs the interest is disbursed periodically
// rather than compounded into a lump sum, so the maturity payout is simply
// the principal.
func maturityAmount(principal, annualRatePct float64, tenureMonths int, payout string) float64 {
	if payout != fddomain.InterestPayoutMaturity {
		return round2(principal)
	}
	years := float64(tenureMonths) / 12
	amount := principal * math.Pow(1+annualRatePct/400, 4*years)
	return round2(amount)
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func newFDAccountNumber() string {
	return fmt.Sprintf("FD%011d", time.Now().UTC().UnixNano()%1e11)
}

func validateOpenRequest(req fddomain.OpenRequest) error {
	if req.PrincipalAmount <= 0 {
		return apiresponse.Validation("principal_amount must be positive")
	}
	if req.TenureMonths <= 0 {
		return apiresponse.Validation("tenure_months must be positive")
	}
	switch req.InterestPayout {
	case fddomain.InterestPayoutMaturity, fddomain.InterestPayoutMonthly, fddomain.InterestPayoutQuarterly:
	default:
		return apiresponse.Validation("interest_payout must be one of MATURITY, MONTHLY, QUARTERLY")
	}
	return nil
}

// resolveBankAccount returns the caller-specified bank account (verifying it
// belongs to userID) or the user's primary account if none was specified.
func (p *MockProvider) resolveBankAccount(ctx context.Context, tx pgx.Tx, userID uuid.UUID, bankAccountID string) (uuid.UUID, float64, error) {
	if bankAccountID == "" {
		acc, err := p.userRepo.GetPrimaryBankAccount(ctx, userID)
		if err != nil {
			return uuid.Nil, 0, fmt.Errorf("resolve default bank account: %w", err)
		}
		bankAccountID = acc.ID.String()
	}

	accID, err := uuid.Parse(bankAccountID)
	if err != nil {
		return uuid.Nil, 0, apiresponse.Validation("bank_account_id is not a valid identifier")
	}

	var balance float64
	err = tx.QueryRow(ctx, `
		SELECT balance FROM bank_accounts WHERE id = $1 AND user_id = $2 FOR UPDATE
	`, accID, userID).Scan(&balance)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, 0, apiresponse.Validation("bank_account_id does not belong to this user")
		}
		return uuid.Nil, 0, fmt.Errorf("lock bank account: %w", err)
	}
	return accID, balance, nil
}

func (p *MockProvider) OpenFD(ctx context.Context, userID uuid.UUID, req fddomain.OpenRequest) (*fddomain.Account, error) {
	if err := validateOpenRequest(req); err != nil {
		return nil, err
	}

	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin open fd tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	bankAccountID, balance, err := p.resolveBankAccount(ctx, tx, userID, req.BankAccountID)
	if err != nil {
		return nil, err
	}
	if balance < req.PrincipalAmount {
		return nil, apiresponse.Validation("insufficient balance: have %.2f, need %.2f", balance, req.PrincipalAmount)
	}

	if _, err := tx.Exec(ctx, `UPDATE bank_accounts SET balance = balance - $1 WHERE id = $2 AND user_id = $3`, req.PrincipalAmount, bankAccountID, userID); err != nil {
		return nil, fmt.Errorf("debit source account: %w", err)
	}

	rate := rateForTenure(req.TenureMonths)
	bookingDate := time.Now().UTC()
	maturityDate := bookingDate.AddDate(0, req.TenureMonths, 0)
	maturity := maturityAmount(req.PrincipalAmount, rate, req.TenureMonths, req.InterestPayout)
	fdAccountNumber := newFDAccountNumber()

	var nominee *string
	if req.NomineeName != "" {
		nominee = &req.NomineeName
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO fd_accounts (
			fd_account_number, user_id, bank_account_id, principal_amount, interest_rate, tenure_months,
			interest_payout, auto_renewal, nominee_name, booking_date, maturity_date, maturity_amount, status
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
	`, fdAccountNumber, userID, bankAccountID, req.PrincipalAmount, rate, req.TenureMonths,
		req.InterestPayout, req.AutoRenewal, nominee, bookingDate, maturityDate, maturity, fddomain.StatusActive); err != nil {
		return nil, fmt.Errorf("insert fd account: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit open fd: %w", err)
	}

	return &fddomain.Account{
		FDAccountNumber: fdAccountNumber,
		PrincipalAmount: req.PrincipalAmount,
		InterestRate:    rate,
		TenureMonths:    req.TenureMonths,
		InterestPayout:  req.InterestPayout,
		AutoRenewal:     req.AutoRenewal,
		NomineeName:     nominee,
		BookingDate:     apitime.New(bookingDate),
		MaturityDate:    apitime.New(maturityDate),
		MaturityAmount:  maturity,
		Status:          fddomain.StatusActive,
	}, nil
}

const fdColumns = `fd_account_number, principal_amount, interest_rate, tenure_months, interest_payout,
	auto_renewal, nominee_name, booking_date, maturity_date, maturity_amount, status`

type fdScanner interface {
	Scan(dest ...any) error
}

func scanFD(row fdScanner) (fddomain.Account, error) {
	var a fddomain.Account
	var bookingDate, maturityDate time.Time
	if err := row.Scan(
		&a.FDAccountNumber, &a.PrincipalAmount, &a.InterestRate, &a.TenureMonths, &a.InterestPayout,
		&a.AutoRenewal, &a.NomineeName, &bookingDate, &maturityDate, &a.MaturityAmount, &a.Status,
	); err != nil {
		return fddomain.Account{}, fmt.Errorf("scan fd account: %w", err)
	}
	a.BookingDate = apitime.New(bookingDate)
	a.MaturityDate = apitime.New(maturityDate)
	return a, nil
}

func (p *MockProvider) ListFDs(ctx context.Context, userID uuid.UUID) ([]fddomain.Account, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT `+fdColumns+` FROM fd_accounts WHERE user_id = $1 ORDER BY booking_date DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list fds: %w", err)
	}
	defer rows.Close()

	accounts := make([]fddomain.Account, 0)
	for rows.Next() {
		a, err := scanFD(rows)
		if err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate fds: %w", err)
	}
	return accounts, nil
}

func (p *MockProvider) GetFD(ctx context.Context, userID uuid.UUID, fdAccountNumber string) (*fddomain.Account, error) {
	row := p.pool.QueryRow(ctx, `
		SELECT `+fdColumns+` FROM fd_accounts WHERE fd_account_number = $1 AND user_id = $2
	`, fdAccountNumber, userID)
	a, err := scanFD(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apiresponse.NotFound("FD account %s not found", fdAccountNumber)
		}
		return nil, err
	}
	return &a, nil
}

// prematureWithdrawalPenaltyRate is a flat published penalty applied to
// early closures, standing in for the bank's real penalty schedule.
const prematureWithdrawalPenaltyRate = 1.00

func (p *MockProvider) Withdraw(ctx context.Context, userID uuid.UUID, fdAccountNumber string, req fddomain.WithdrawRequest) (*fddomain.WithdrawResult, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin withdraw tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var bankAccountID uuid.UUID
	var principal, rate float64
	var tenureMonths int
	var payout, status string
	err = tx.QueryRow(ctx, `
		SELECT bank_account_id, principal_amount, interest_rate, tenure_months, interest_payout, status
		FROM fd_accounts WHERE fd_account_number = $1 AND user_id = $2 FOR UPDATE
	`, fdAccountNumber, userID).Scan(&bankAccountID, &principal, &rate, &tenureMonths, &payout, &status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apiresponse.NotFound("FD account %s not found", fdAccountNumber)
		}
		return nil, fmt.Errorf("lock fd account: %w", err)
	}
	if status != fddomain.StatusActive {
		return nil, fmt.Errorf("FD %s is %s and cannot be withdrawn: %w", fdAccountNumber, status, apiresponse.ErrConflict)
	}

	withdrawAmount := principal
	isFullClosure := true
	if req.WithdrawalAmount != nil {
		if *req.WithdrawalAmount <= 0 || *req.WithdrawalAmount > principal {
			return nil, apiresponse.Validation("withdrawal_amount must be between 0 and the principal amount")
		}
		withdrawAmount = *req.WithdrawalAmount
		isFullClosure = withdrawAmount == principal
	}

	netPayable := round2(withdrawAmount * (1 - prematureWithdrawalPenaltyRate/100))

	if _, err := tx.Exec(ctx, `UPDATE bank_accounts SET balance = balance + $1 WHERE id = $2 AND user_id = $3`, netPayable, bankAccountID, userID); err != nil {
		return nil, fmt.Errorf("credit source account: %w", err)
	}

	newStatus := fddomain.StatusActive
	if isFullClosure {
		newStatus = fddomain.StatusPrematureClose
		if _, err := tx.Exec(ctx, `UPDATE fd_accounts SET status = $1, updated_at = now() WHERE fd_account_number = $2 AND user_id = $3`, newStatus, fdAccountNumber, userID); err != nil {
			return nil, fmt.Errorf("close fd account: %w", err)
		}
	} else {
		remainingPrincipal := round2(principal - withdrawAmount)
		remainingMaturity := maturityAmount(remainingPrincipal, rate, tenureMonths, payout)
		if _, err := tx.Exec(ctx, `
			UPDATE fd_accounts SET principal_amount = $1, maturity_amount = $2, updated_at = now()
			WHERE fd_account_number = $3 AND user_id = $4
		`, remainingPrincipal, remainingMaturity, fdAccountNumber, userID); err != nil {
			return nil, fmt.Errorf("update fd account after partial withdrawal: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit withdraw: %w", err)
	}

	return &fddomain.WithdrawResult{
		FDAccountNumber:  fdAccountNumber,
		PenaltyRate:      prematureWithdrawalPenaltyRate,
		NetPayableAmount: netPayable,
		Status:           newStatus,
	}, nil
}
