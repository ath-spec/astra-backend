// Package catalog defines the wire types for the Fund & Scheme Catalog / NFO
// domain – shared reference data (not per-user), matching the IDBI sandbox
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

type SearchParams struct {
	Category         string
	RiskLevel        string
	MinInvestmentMax *float64
	Page             int
	Limit            int
}

type DistributionItem struct {
	Title      string  `json:"title"`
	Percentage float64 `json:"percentage"`
}

type AllocationBreakdown struct {
	EquityPct   float64            `json:"equity_pct"`
	DebtPct     float64            `json:"debt_pct"`
	OtherPct    float64            `json:"other_pct"`
	Sectors     []DistributionItem `json:"sectors"`
	TopHoldings []DistributionItem `json:"top_holdings"`
}

type ChartPoint struct {
	Date apitime.Time `json:"date"`
	NAV  float64      `json:"nav"`
}

type UserHolding struct {
	UnitsHeld     float64 `json:"units_held"`
	InvestedValue float64 `json:"invested_value"`
	CurrentValue  float64 `json:"current_value"`
	ReturnsPct    float64 `json:"returns_pct"`
}

type DeepDiveInfo struct {
	PrimaryRole   string `json:"primary_role"`
	SecondaryRole string `json:"secondary_role"`
	Strengths     string `json:"strengths"`
	TradeOffs     string `json:"trade_offs"`
	Contribution  string `json:"contribution"`
}

type FundInsights struct {
	IsPositiveImpact     bool      `json:"is_positive_impact"`
	WhyGetFund           string    `json:"why_get_fund"`
	SuitableFor          string    `json:"suitable_for"`
	AvoidIf              string    `json:"avoid_if"`
	ImpactText           string    `json:"impact_text"`
	WhatItDoesRightNow   string    `json:"what_it_does_right_now"`
	WhatBuyingMoreWillDo string    `json:"what_buying_more_will_do"`
	CurrentValues        []float64 `json:"current_values"`
	ProjectedValues      []float64 `json:"projected_values"`
}

type FundProfile struct {
	Fund
	Allocation  AllocationBreakdown `json:"allocation"`
	ChartPoints []ChartPoint        `json:"chart_points"`
	DeepDive    DeepDiveInfo        `json:"deep_dive"`
	Insights    FundInsights        `json:"insights"`
	UserHolding *UserHolding        `json:"user_holding,omitempty"`
}
