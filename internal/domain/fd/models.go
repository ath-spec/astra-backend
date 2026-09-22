// Package fd defines the wire types for the Fixed Deposits domain, matching
// the IDBI sandbox spec doc's field names.
package fd

import "github.com/yourusername/astra-backend/internal/apitime"

type OpenRequest struct {
	BankAccountID   string  `json:"bank_account_id,omitempty"` // optional; defaults to the user's primary account
	PrincipalAmount float64 `json:"principal_amount"`
	TenureMonths    int     `json:"tenure_months"`
	InterestPayout  string  `json:"interest_payout"` // MATURITY / MONTHLY / QUARTERLY
	AutoRenewal     bool    `json:"auto_renewal,omitempty"`
	NomineeName     string  `json:"nominee_name,omitempty"`
}

type Account struct {
	FDAccountNumber string       `json:"fd_account_number"`
	PrincipalAmount float64      `json:"principal_amount"`
	InterestRate    float64      `json:"interest_rate"`
	TenureMonths    int          `json:"tenure_months"`
	InterestPayout  string       `json:"interest_payout"`
	AutoRenewal     bool         `json:"auto_renewal"`
	NomineeName     *string      `json:"nominee_name,omitempty"`
	BookingDate     apitime.Time `json:"booking_date"`
	MaturityDate    apitime.Time `json:"maturity_date"`
	MaturityAmount  float64      `json:"maturity_amount"`
	Status          string       `json:"status"`
}

// WithdrawRequest is the "premature withdrawal" request. A nil
// WithdrawalAmount means full closure.
type WithdrawRequest struct {
	WithdrawalAmount *float64 `json:"withdrawal_amount,omitempty"`
}

type WithdrawResult struct {
	FDAccountNumber  string  `json:"fd_account_number"`
	PenaltyRate      float64 `json:"penalty_rate"`
	NetPayableAmount float64 `json:"net_payable_amount"`
	Status           string  `json:"status"`
}

const (
	InterestPayoutMaturity  = "MATURITY"
	InterestPayoutMonthly   = "MONTHLY"
	InterestPayoutQuarterly = "QUARTERLY"

	StatusActive         = "ACTIVE"
	StatusMatured        = "MATURED"
	StatusClosed         = "CLOSED"
	StatusPrematureClose = "PREMATURE_CLOSED"
)
