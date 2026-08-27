// Package payments defines the wire types for the Payments, Funds Transfer &
// Mandate Management domain, matching the IDBI sandbox spec doc's field
// names.
package payments

import "github.com/yourusername/astra-backend/internal/apitime"

type PaymentRequest struct {
	TxnID         string  `json:"txn_id"` // client-generated idempotency key
	Amount        float64 `json:"amount"`
	PaymentMode   string  `json:"payment_mode"` // UPI / NETBANKING / CARD
	UPIID         string  `json:"upi_id,omitempty"`
	BankAccountID string  `json:"bank_account_id,omitempty"` // optional; defaults to the user's primary account
	Purpose       string  `json:"purpose"`                   // MF_PURCHASE / STOCK_ORDER / FD_BOOKING / WALLET_FUNDING
}

type Payment struct {
	PaymentID    string        `json:"payment_id"`
	TxnID        string        `json:"txn_id"`
	Status       string        `json:"status"` // SUCCESS / FAILURE / PENDING / CANCELLED
	Mode         string        `json:"mode"`
	BankRefNum   *string       `json:"bank_ref_num,omitempty"`
	ErrorCode    *string       `json:"error_code,omitempty"`
	ErrorMessage *string       `json:"error_message,omitempty"`
	ProcessedAt  *apitime.Time `json:"processed_at,omitempty"`
}

type MandateRequest struct {
	BankAccountID    string  `json:"bank_account_id,omitempty"`
	UPIID            string  `json:"upi_id,omitempty"`
	MandateType      string  `json:"mandate_type,omitempty"` // defaults to UPI_AUTOPAY
	PayeeName        string  `json:"payee_name,omitempty"`
	PayeeVPAOrID     string  `json:"payee_vpa_or_id,omitempty"`
	Category         string  `json:"category,omitempty"` // SUBSCRIPTION / BILL / OTHER; defaults to OTHER
	MandateAmount    float64 `json:"mandate_amount"`
	MandateFrequency string  `json:"mandate_frequency"`          // MONTHLY / QUARTERLY / YEARLY
	MandateStartDate int64   `json:"mandate_start_date"`         // epoch seconds
	MandateEndDate   *int64  `json:"mandate_end_date,omitempty"` // epoch seconds
}

type Mandate struct {
	MandateID     string        `json:"mandate_id"`
	MandateType   string        `json:"mandate_type"`
	PayeeName     *string       `json:"payee_name,omitempty"`
	PayeeVPAOrID  *string       `json:"payee_vpa_or_id,omitempty"`
	Category      string        `json:"category"`
	BankName      *string       `json:"bank_name,omitempty"`
	MaxAmount     float64       `json:"max_amount"`
	Frequency     string        `json:"frequency"`
	NextDebitDate *apitime.Time `json:"next_debit_date,omitempty"`
	Status        string        `json:"status"`
	ApprovedAt    *apitime.Time `json:"approved_at,omitempty"`
	CreatedAt     apitime.Time  `json:"created_at"`
}

// MandateExecution is one past (successful or failed) debit attempt against
// a mandate, recorded so the Recurring screen's history view has real
// occurrences to show instead of a live-computed projection.
type MandateExecution struct {
	ScheduledDate apitime.Time `json:"scheduled_date"`
	Amount        float64      `json:"amount"`
	Status        string       `json:"status"` // SUCCESS / FAILED
	FailureReason *string      `json:"failure_reason,omitempty"`
	ExecutedAt    apitime.Time `json:"executed_at"`
}

// ExecutionPage is the paginated envelope for GET /mandates/{id}/history.
type ExecutionPage struct {
	Items  []MandateExecution `json:"items"`
	Total  int                `json:"total"`
	Limit  int                `json:"limit"`
	Offset int                `json:"offset"`
}

// RecurringSummary backs the Recurring screen's upcoming/overdue/paid stat
// tiles and the Home screen's "Track your bills" teaser.
type RecurringSummary struct {
	UpcomingCount      int     `json:"upcoming_count"`
	UpcomingTotal      float64 `json:"upcoming_total"`
	OverdueCount       int     `json:"overdue_count"`
	OverdueTotal       float64 `json:"overdue_total"`
	PaidThisMonthCount int     `json:"paid_this_month_count"`
	PaidThisMonthTotal float64 `json:"paid_this_month_total"`
}

type MandateActionRequest struct {
	Action         string `json:"action"`                     // PAUSE / RESUME / CANCEL
	PauseUntilDate *int64 `json:"pause_until_date,omitempty"` // epoch seconds
}

type MandateActionResult struct {
	MandateID     string       `json:"mandate_id"`
	Status        string       `json:"status"`
	EffectiveFrom apitime.Time `json:"effective_from"`
}

const (
	PaymentStatusSuccess   = "SUCCESS"
	PaymentStatusFailure   = "FAILURE"
	PaymentStatusPending   = "PENDING"
	PaymentStatusCancelled = "CANCELLED"

	MandateTypeUPIAutopay = "UPI_AUTOPAY"

	MandateStatusPending = "PENDING"
	MandateStatusActive  = "ACTIVE"
	MandateStatusPaused  = "PAUSED"
	MandateStatusRevoked = "REVOKED"
	MandateStatusExpired = "EXPIRED"

	ActionPause  = "PAUSE"
	ActionResume = "RESUME"
	ActionCancel = "CANCEL"

	FrequencyMonthly   = "MONTHLY"
	FrequencyQuarterly = "QUARTERLY"
	FrequencyYearly    = "YEARLY"

	ExecutionSuccess = "SUCCESS"
	ExecutionFailed  = "FAILED"

	DefaultCategory = "OTHER"
)
