// Package catalog defines the wire types for the Fund & Scheme Catalog / NFO
// domain — shared reference data (not per-user), matching the IDBI sandbox
// spec doc's field names.
package catalog

type Fund struct {
	SchemeCode     string   `json:"scheme_code"`
	SchemeName     string   `json:"scheme_name"`
	AMCName        string   `json:"amc_name"`
	ISIN           string   `json:"isin"`
	Category       string   `json:"category"`
	RiskLevel      string   `json:"risk_level"`
	NAV            float64  `json:"nav"`
	NAVDate        string   `json:"nav_date"`
	ExpenseRatio   float64  `json:"expense_ratio"`
	AUM            float64  `json:"aum"`
	MinInvestment  float64  `json:"min_investment"`
	MinSIPAmount   float64  `json:"min_sip_amount"`
	Returns1Y      *float64 `json:"returns_1y,omitempty"`
	Returns3Y      *float64 `json:"returns_3y,omitempty"`
	Returns5Y      *float64 `json:"returns_5y,omitempty"`
	FundManager    *string  `json:"fund_manager,omitempty"`
	BenchmarkIndex *string  `json:"benchmark_index,omitempty"`
	LaunchDate     *string  `json:"launch_date,omitempty"`
}

type NFO struct {
	NFOID          string  `json:"nfo_id"`
	SchemeName     string  `json:"scheme_name"`
	AMCName        string  `json:"amc_name"`
	Category       string  `json:"category"`
	OfferOpenDate  string  `json:"offer_open_date"`
	OfferCloseDate string  `json:"offer_close_date"`
	OfferPrice     float64 `json:"offer_price"`
	MinInvestment  float64 `json:"min_investment"`
	AllotmentDate  *string `json:"allotment_date,omitempty"`
}

// SearchParams mirrors the spec doc's "Request — catalog search" fields.
type SearchParams struct {
	Category         string
	RiskLevel        string
	MinInvestmentMax *float64
	Page             int
	Limit            int
}
