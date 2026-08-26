// Package payments defines the wire types for the Payments, Funds Transfer &
// Mandate Management domain, matching the IDBI sandbox spec doc's field
// names.
package payments

import "time"

type PaymentRequest struct {
	TxnID         string  `json:"txn_id"` // client-generated idempotency key
	Amount        float64 `json:"amount"`
	PaymentMode   string  `json:"payment_mode"` // UPI / NETBANKING / CARD
	UPIID         string  `json:"upi_id,omitempty"`
	BankAccountID string  `json:"bank_account_id,omitempty"` // optional; defaults to the user's primary account
	Purpose       string  `json:"purpose"`                   // MF_PURCHASE / STOCK_ORDER / FD_BOOKING / WALLET_FUNDING
}

type Payment struct {
	PaymentID    string     `json:"payment_id"`
	TxnID        string     `json:"txn_id"`
	Status       string     `json:"status"` // SUCCESS / FAILURE / PENDING / CANCELLED
	Mode         string     `json:"mode"`
	BankRefNum   *string    `json:"bank_ref_num,omitempty"`
	ErrorCode    *string    `json:"error_code,omitempty"`
	ErrorMessage *string    `json:"error_message,omitempty"`
	ProcessedAt  *time.Time `json:"processed_at,omitempty"`
}

type MandateRequest struct {
	BankAccountID    string  `json:"bank_account_id,omitempty"`
	UPIID            string  `json:"upi_id,omitempty"`
	MandateType      string  `json:"mandate_type,omitempty"` // defaults to UPI_AUTOPAY
	PayeeName        string  `json:"payee_name,omitempty"`
	PayeeVPAOrID     string  `json:"payee_vpa_or_id,omitempty"`
	MandateAmount    float64 `json:"mandate_amount"`
	MandateFrequency string  `json:"mandate_frequency"` // MONTHLY / QUARTERLY
	MandateStartDate string  `json:"mandate_start_date"`
	MandateEndDate   string  `json:"mandate_end_date,omitempty"`
}

type Mandate struct {
	MandateID     string     `json:"mandate_id"`
	MandateType   string     `json:"mandate_type"`
	PayeeName     *string    `json:"payee_name,omitempty"`
	PayeeVPAOrID  *string    `json:"payee_vpa_or_id,omitempty"`
	MaxAmount     float64    `json:"max_amount"`
	Frequency     string     `json:"frequency"`
	NextDebitDate *string    `json:"next_debit_date,omitempty"`
	Status        string     `json:"status"`
	ApprovedAt    *time.Time `json:"approved_at,omitempty"`
	CreatedAt     string     `json:"created_at,omitempty"`
}

type MandateActionRequest struct {
	Action         string `json:"action"` // PAUSE / RESUME / CANCEL
	PauseUntilDate string `json:"pause_until_date,omitempty"`
}

type MandateActionResult struct {
	MandateID     string `json:"mandate_id"`
	Status        string `json:"status"`
	EffectiveFrom string `json:"effective_from"`
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
)
