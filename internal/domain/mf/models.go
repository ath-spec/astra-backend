// Package mf defines the wire types for the Mutual Fund investment domain —
// per-user folios/holdings, purchase (lumpsum/SIP) and redemption, backed
// today by a stateful mock provider (see internal/provider/mf) rather than a
// real CAMS/KFintech-style RTA integration. Field names are original to this
// backend (the IDBI sandbox spec doc left MF Central unspecified pending a
// vendor choice), chosen to map directly onto the Holdings/Explore/Portfolio
// screens in the Flutter app.
package mf

import "github.com/yourusername/astra-backend/internal/apitime"

type Folio struct {
	FolioNumber        string       `json:"folio_number"`
	AMCName            string       `json:"amc_name"`
	SchemeCode         string       `json:"scheme_code"`
	SchemeName         string       `json:"scheme_name"`
	ISIN               string       `json:"isin,omitempty"`
	Category           string       `json:"category"`
	PlanType           string       `json:"plan_type"`
	IsSIP              bool         `json:"is_sip"`
	UnitsHeld          float64      `json:"units_held"`
	NAV                float64      `json:"nav"`
	NAVDate            apitime.Time `json:"nav_date"`
	InvestedValue      float64      `json:"invested_value"`
	CurrentValue       float64      `json:"current_value"`
	ReturnsAmount      float64      `json:"returns_amount"`
	ReturnsPct         float64      `json:"returns_pct"`
	OneDayChangeAmount float64      `json:"one_day_change_amount"`
	OneDayChangePct    float64      `json:"one_day_change_pct"`
	XIRRPct            float64      `json:"xirr_pct"`
	FirstPurchaseDate  apitime.Time `json:"first_purchase_date"`
}

// HoldingsSummary aggregates every folio into the top-of-screen totals the
// Holdings screen (and the Home dashboard's MF row) needs.
type HoldingsSummary struct {
	InvestedValue      float64 `json:"invested_value"`
	CurrentValue       float64 `json:"current_value"`
	ReturnsAmount      float64 `json:"returns_amount"`
	ReturnsPct         float64 `json:"returns_pct"`
	XIRRPct            float64 `json:"xirr_pct"`
	OneDayChangeAmount float64 `json:"one_day_change_amount"`
	OneDayChangePct    float64 `json:"one_day_change_pct"`
	FolioCount         int     `json:"folio_count"`
}

type HoldingsResult struct {
	Summary HoldingsSummary `json:"summary"`
	Folios  []Folio         `json:"folios"`
}

type PurchaseRequest struct {
	SchemeCode string  `json:"scheme_code"`
	Amount     float64 `json:"amount"`
	IsSIP      bool    `json:"is_sip,omitempty"`
}

type RedeemRequest struct {
	SchemeCode string   `json:"scheme_code"`
	Units      *float64 `json:"units,omitempty"` // nil/omitted = redeem the full holding
}

type RedeemResult struct {
	SchemeCode     string  `json:"scheme_code"`
	UnitsRedeemed  float64 `json:"units_redeemed"`
	Amount         float64 `json:"amount"`
	RemainingUnits float64 `json:"remaining_units"`
	Status         string  `json:"status"`
}

type Transaction struct {
	SchemeCode      string       `json:"scheme_code"`
	SchemeName      string       `json:"scheme_name"`
	TransactionType string       `json:"transaction_type"` // PURCHASE / SIP / REDEEM
	TransactionDate apitime.Time `json:"transaction_date"`
	Amount          float64      `json:"amount"`
	Units           float64      `json:"units"`
	Price           float64      `json:"price"`
}

const (
	TxnPurchase = "PURCHASE"
	TxnSIP      = "SIP"
	TxnRedeem   = "REDEEM"

	RedeemStatusSuccess = "SUCCESS"
)
