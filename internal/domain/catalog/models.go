// Package catalog defines the wire types for the Fund & Scheme Catalog / NFO
// domain — shared reference data (not per-user), matching the IDBI sandbox
// spec doc's field names.
package catalog

import "github.com/yourusername/astra-backend/internal/apitime"

type Fund struct {
	SchemeCode     string        `json:"scheme_code"`
	SchemeName     string        `json:"scheme_name"`
	AMCName        string        `json:"amc_name"`
	ISIN           string        `json:"isin"`
	Category       string        `json:"category"`
	RiskLevel      string        `json:"risk_level"`
	NAV            float64       `json:"nav"`
	NAVDate        apitime.Time  `json:"nav_date"`
	ExpenseRatio   float64       `json:"expense_ratio"`
	AUM            float64       `json:"aum"`
	MinInvestment  float64       `json:"min_investment"`
	MinSIPAmount   float64       `json:"min_sip_amount"`
	Returns1Y      *float64      `json:"returns_1y,omitempty"`
	Returns3Y      *float64      `json:"returns_3y,omitempty"`
	Returns5Y      *float64      `json:"returns_5y,omitempty"`
	FundManager    *string       `json:"fund_manager,omitempty"`
	BenchmarkIndex *string       `json:"benchmark_index,omitempty"`
	LaunchDate     *apitime.Time `json:"launch_date,omitempty"`
}

type NFO struct {
	NFOID          string        `json:"nfo_id"`
	SchemeName     string        `json:"scheme_name"`
	AMCName        string        `json:"amc_name"`
	Category       string        `json:"category"`
	OfferOpenDate  apitime.Time  `json:"offer_open_date"`
	OfferCloseDate apitime.Time  `json:"offer_close_date"`
	OfferPrice     float64       `json:"offer_price"`
	MinInvestment  float64       `json:"min_investment"`
	AllotmentDate  *apitime.Time `json:"allotment_date,omitempty"`
}

// SearchParams mirrors the spec doc's "Request — catalog search" fields.
type SearchParams struct {
	Category         string
	RiskLevel        string
	MinInvestmentMax *float64
	Page             int
	Limit            int
}

// DistributionItem is one slice of a breakdown (a sector, a top holding, an
// asset-class bucket) shown on the Explore fund-profile page.
type DistributionItem struct {
	Title      string  `json:"title"`
	Percentage float64 `json:"percentage"`
}

// AllocationBreakdown is static reference data (see migration
// 000012_fund_allocation) — not derived from a live AMC holdings-disclosure
// feed, since none is wired in yet.
type AllocationBreakdown struct {
	EquityPct   float64            `json:"equity_pct"`
	DebtPct     float64            `json:"debt_pct"`
	OtherPct    float64            `json:"other_pct"`
	Sectors     []DistributionItem `json:"sectors"`
	TopHoldings []DistributionItem `json:"top_holdings"`
}

// ChartPoint is one point of a fund's NAV history chart. This history is
// synthesized (trending from the fund's disclosed 1-year return to its
// current NAV, with deterministic day-to-day jitter) rather than a real
// recorded NAV series — there's no historical price feed behind this mock
// catalog. See provider/catalog's chartPoints for the exact method.
type ChartPoint struct {
	Date apitime.Time `json:"date"`
	NAV  float64      `json:"nav"`
}

// UserHolding is present on a FundProfile only when the requesting user
// currently holds this fund (see internal/service.CatalogService, which
// cross-references the MF domain to populate this — the catalog provider
// itself has no notion of per-user holdings).
type UserHolding struct {
	UnitsHeld     float64 `json:"units_held"`
	InvestedValue float64 `json:"invested_value"`
	CurrentValue  float64 `json:"current_value"`
	ReturnsPct    float64 `json:"returns_pct"`
}

type FundProfile struct {
	Fund
	Allocation  AllocationBreakdown `json:"allocation"`
	ChartPoints []ChartPoint        `json:"chart_points"`
	UserHolding *UserHolding        `json:"user_holding,omitempty"`
}
