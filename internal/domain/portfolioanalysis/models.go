// Package portfolioanalysis defines the wire types for the Portfolio
// Analysis screen's Allocation tab: a live aggregation of the user's actual
// MF/Stocks/FD holdings — not fabricated data. Sector/equity-debt-other
// weights per fund come from the Catalog domain's fund_allocation reference
// data (see migration 000012); volatility bucketing and the overall
// "allocation level" are a documented scoring heuristic applied to real
// holding values, the same way any investing app has to define its own
// risk-scoring formula.
package portfolioanalysis

// VolatilityBucket groups holdings by risk tier: STABLE (FDs), LOW/MEDIUM/HIGH
// (MF holdings bucketed by the held fund's catalog risk_level, individual
// stock holdings always counted HIGH).
type VolatilityBucket struct {
	Label    string  `json:"label"` // STABLE / LOW / MEDIUM / HIGH
	Amount   float64 `json:"amount"`
	SharePct float64 `json:"share_pct"`
}

type SectorExposure struct {
	Sector     string  `json:"sector"`
	Amount     float64 `json:"amount"`
	Percentage float64 `json:"percentage"`
}

// PortfolioGenome represents the 7-axis DNA of the user's blended portfolio.
type PortfolioGenome struct {
	Growth              float64   `json:"growth"`
	Income              float64   `json:"income"`
	CapitalPreservation float64   `json:"capital_preservation"`
	InflationDefense    float64   `json:"inflation_defense"`
	Liquidity           float64   `json:"liquidity"`
	Sustainability      float64   `json:"sustainability"`
	RealAssets          float64   `json:"real_assets"`
	Values              []float64 `json:"values"`
}

// AllocationResult backs the Allocation tab: equity/debt/other split (value-
// weighted across every held MF scheme's real static allocation breakdown,
// plus stock holdings counted as 100% equity), a volatility-bucket
// breakdown, blended sector exposure, and the computed 7-axis Portfolio Genome.
type AllocationResult struct {
	Level             string             `json:"level"` // CONSERVATIVE / MODERATE_CONSERVATIVE / BALANCED / AGGRESSIVE / VERY_AGGRESSIVE
	TotalValue        float64            `json:"total_value"`
	EquityAmount      float64            `json:"equity_amount"`
	DebtAmount        float64            `json:"debt_amount"`
	OtherAmount       float64            `json:"other_amount"`
	EquityPct         float64            `json:"equity_pct"`
	DebtPct           float64            `json:"debt_pct"`
	OtherPct          float64            `json:"other_pct"`
	VolatilityBuckets []VolatilityBucket `json:"volatility_buckets"`
	SectorExposure    []SectorExposure   `json:"sector_exposure"`
	Genome            PortfolioGenome    `json:"genome"`
	// Holdings is the flat per-instrument breakdown (bank, FD, MF, stock) with
	// each holding's value, share of the total, and volatility bucket — backs
	// the "allocation factors" drill-down.
	Holdings []HoldingBreakdown `json:"holdings"`
	// EquityExposure backs the index-fund / market-cap-split section.
	EquityExposure *EquityExposure `json:"equity_exposure"`
}

// HoldingBreakdown is one instrument in the portfolio.
type HoldingBreakdown struct {
	Name       string  `json:"name"`
	Subtitle   string  `json:"subtitle"`
	Type       string  `json:"type"`       // BANK / FD / MF / STOCK
	Value      float64 `json:"value"`
	Pct        float64 `json:"pct"`        // of total holdings value
	Volatility string  `json:"volatility"` // STABLE / LOW / MEDIUM / HIGH
}

// MarketCapSlice is one company-size band of the equity book.
type MarketCapSlice struct {
	Label string  `json:"label"` // Large Cap / Mid Cap / Small Cap / Micro Cap
	Value float64 `json:"value"`
	Pct   float64 `json:"pct"` // of total equity exposure
}

// EquityExposure summarises passive (index-fund) exposure vs. peers and the
// company-size split of the equity book.
type EquityExposure struct {
	TotalEquityValue float64          `json:"total_equity_value"`
	IndexFundValue   float64          `json:"index_fund_value"`
	IndexFundPct     float64          `json:"index_fund_pct"`      // of equity book
	PeerIndexFundPct float64          `json:"peer_index_fund_pct"` // avg across other users
	MarketCap        []MarketCapSlice `json:"market_cap"`
}

// DNAHistoryPoint is one dated snapshot of a user's portfolio DNA, backing
// the RM console's "how has this client's allocation & DNA changed over
// time" view. Persisted in portfolio_dna_snapshots (migration 000018).
type DNAHistoryPoint struct {
	Date              int64              `json:"date"` // epoch seconds
	Level             string             `json:"level"`
	TotalValue        float64            `json:"total_value"`
	EquityPct         float64            `json:"equity_pct"`
	DebtPct           float64            `json:"debt_pct"`
	OtherPct          float64            `json:"other_pct"`
	Genome            PortfolioGenome    `json:"genome"`
	SectorExposure    []SectorExposure   `json:"sector_exposure"`
	VolatilityBuckets []VolatilityBucket `json:"volatility_buckets"`
}

const (
	LevelConservative         = "CONSERVATIVE"
	LevelModerateConservative = "MODERATE_CONSERVATIVE"
	LevelBalanced             = "BALANCED"
	LevelAggressive           = "AGGRESSIVE"
	LevelVeryAggressive       = "VERY_AGGRESSIVE"

	VolatilityStable = "STABLE"
	VolatilityLow    = "LOW"
	VolatilityMedium = "MEDIUM"
	VolatilityHigh   = "HIGH"

	DisciplinePoor      = "POOR"
	DisciplineModerate  = "MODERATE"
	DisciplineGood      = "GOOD"
	DisciplineExcellent = "EXCELLENT"

	PerformanceSignificantlyBelow = "SIGNIFICANTLY_BELOW"
	PerformanceBelowAverage       = "BELOW_AVERAGE"
	PerformanceInLine             = "IN_LINE"
	PerformanceStrong             = "STRONG"
	PerformanceVeryStrong         = "VERY_STRONG"
)

// MonthlyInvestmentPoint captures one month's investment activity across MF & Stocks.
type MonthlyInvestmentPoint struct {
	MonthName     string  `json:"month_name"` // e.g. "Jan", "Feb"
	YearMonth     string  `json:"year_month"` // e.g. "2026-01"
	Amount        float64 `json:"amount"`     // gross invested (== BuyAmount), kept for compatibility
	BuyAmount     float64 `json:"buy_amount"`
	SellAmount    float64 `json:"sell_amount"` // MF redemptions + stock sells
	NetAmount     float64 `json:"net_amount"`  // BuyAmount - SellAmount
	OrderCount    int     `json:"order_count"`
	HasInvestment bool    `json:"has_investment"`
}

// YearlyInvestmentPoint aggregates a calendar year's buy / sell / net flow.
type YearlyInvestmentPoint struct {
	Year       int     `json:"year"`
	BuyAmount  float64 `json:"buy_amount"`
	SellAmount float64 `json:"sell_amount"`
	NetAmount  float64 `json:"net_amount"`
}

// DisciplineResult backs the Discipline tab: calculates SIP regularity, streak,
// active-months ratio, and automation percentage from real transaction & mandate history.
type DisciplineResult struct {
	Level               string                   `json:"level"`           // POOR / MODERATE / GOOD / EXCELLENT
	Score               float64                  `json:"score"`           // 0.0 to 1.0
	ActiveSegments      int                      `json:"active_segments"` // 1 to 5
	SIPConsistencyPct   float64                  `json:"sip_consistency_pct"`
	CurrentStreakMonths int                      `json:"current_streak_months"`
	MissedMonths        int                      `json:"missed_months"`
	AvgMonthlyInvested  float64                  `json:"avg_monthly_invested"`
	SIPAutomationPct    float64                  `json:"sip_automation_pct"`
	ActiveMandatesCount int                      `json:"active_mandates_count"`
	MonthlyHistory      []MonthlyInvestmentPoint `json:"monthly_history"`
	YearlyHistory       []YearlyInvestmentPoint  `json:"yearly_history"`
}

// BenchmarkComparison compares the user's blended portfolio returns against standard benchmarks.
type BenchmarkComparison struct {
	Name               string  `json:"name"` // "Nifty 50", "Gold ETF", "Fixed Deposit (7.1%)"
	BenchmarkReturnPct float64 `json:"benchmark_return_pct"`
	PortfolioReturnPct float64 `json:"portfolio_return_pct"`
	AlphaPct           float64 `json:"alpha_pct"` // PortfolioReturnPct - BenchmarkReturnPct
	BeatingBenchmark   bool    `json:"beating_benchmark"`
}

// ExpensiveFundItem highlights held funds whose expense ratio exceeds category norms (>1.5%).
type ExpensiveFundItem struct {
	SchemeCode              string  `json:"scheme_code"`
	SchemeName              string  `json:"scheme_name"`
	ExpenseRatio            float64 `json:"expense_ratio"`
	CategoryAvgExpenseRatio float64 `json:"category_avg_expense_ratio"`
	AnnualCostEstimate      float64 `json:"annual_cost_estimate"`
	Recommendation          string  `json:"recommendation"`
}

// FundPerformanceItem details individual held fund performance and ranking.
type FundPerformanceItem struct {
	SchemeCode      string  `json:"scheme_code"`
	SchemeName      string  `json:"scheme_name"`
	InvestedValue   float64 `json:"invested_value"`
	CurrentValue    float64 `json:"current_value"`
	GainAmount      float64 `json:"gain_amount"`
	ReturnsPct      float64 `json:"returns_pct"`
	PerformanceRank string  `json:"performance_rank"` // "TOP" / "AVERAGE" / "UNDERPERFORMER"
}

// PerformanceResult backs the Performance tab: calculates total gain, Alpha vs
// market benchmarks, and scans for high-cost or underperforming funds.
type PerformanceResult struct {
	Level               string                `json:"level"`           // SIGNIFICANTLY_BELOW / BELOW_AVERAGE / IN_LINE / STRONG / VERY_STRONG
	ActiveSegments      int                   `json:"active_segments"` // 1 to 5
	TotalInvested       float64               `json:"total_invested"`
	TotalCurrent        float64               `json:"total_current"`
	TotalGainAmount     float64               `json:"total_gain_amount"`
	TotalReturnPct      float64               `json:"total_return_pct"`
	AnnualizedReturnPct float64               `json:"annualized_return_pct"`
	Benchmarks          []BenchmarkComparison `json:"benchmarks"`
	ExpensiveFunds      []ExpensiveFundItem   `json:"expensive_funds"`
	FundsPerformance    []FundPerformanceItem `json:"funds_performance"`
}
