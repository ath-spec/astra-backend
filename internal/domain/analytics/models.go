// Package analytics defines the wire types for the Spend Analytics engine —
// a set of reusable analyzers over a user's transaction history (weekday vs
// weekend, trends, category/merchant analysis, recurring detection, impulse
// scoring). See internal/service/analytics for the analyzer implementations
// and internal/provider/analytics for the swappable transaction source.
package analytics

import (
	"time"

	"github.com/yourusername/astra-backend/internal/apitime"
)

type TxnType string

const (
	TxnDebit  TxnType = "DEBIT"
	TxnCredit TxnType = "CREDIT"
)

// Transaction is the common shape every analyzer operates on, independent of
// where it came from (seeded mock data today; a real AA feed later).
type Transaction struct {
	ID         string
	Amount     float64
	Type       TxnType
	Category   string
	Merchant   string
	OccurredAt time.Time
}

// TransactionListItem is the wire format for the raw transaction feed (see
// GET /api/v1/analytics/spend/transactions) — the Transactions screen's data
// source. Unlike Transaction (the engine's internal, JSON-tag-free working
// type), this has epoch-seconds timestamps for the wire.
type TransactionListItem struct {
	ID         string       `json:"id"`
	Amount     float64      `json:"amount"`
	Type       TxnType      `json:"type"` // DEBIT / CREDIT
	Category   string       `json:"category"`
	Merchant   string       `json:"merchant"`
	OccurredAt apitime.Time `json:"occurred_at"`
}

// TransactionPage is the paginated envelope for GET /transactions.
// Total is the unfiltered count for the same window/filters so the client
// knows how many pages remain and when to stop infinite-scrolling.
type TransactionPage struct {
	Items  []TransactionListItem `json:"items"`
	Total  int                   `json:"total"`
	Limit  int                   `json:"limit"`
	Offset int                   `json:"offset"`
}

// --- A: Weekday vs weekend ---

type WeekdayWeekendResult struct {
	WeekdayTotal          float64            `json:"weekday_total"`
	WeekendTotal          float64            `json:"weekend_total"`
	WeekdaySharePct       float64            `json:"weekday_share_pct"`
	WeekendSharePct       float64            `json:"weekend_share_pct"`
	WeekdayDailyAvg       float64            `json:"weekday_daily_avg"`
	WeekendDailyAvg       float64            `json:"weekend_daily_avg"`
	WeekendVsWeekdayRatio float64            `json:"weekend_vs_weekday_ratio"`
	PeakDay               string             `json:"peak_day"`
	LowestActiveDay       string             `json:"lowest_active_day,omitempty"`
	PerDay                map[string]float64 `json:"per_day"`
	PriorPeriodRatio      *float64           `json:"prior_period_ratio,omitempty"`
	RatioDeviationPct     *float64           `json:"ratio_deviation_pct,omitempty"`
}

// --- B: Trend analytics ---

type TrendPoint struct {
	PeriodStart apitime.Time `json:"period_start"`
	Total       float64      `json:"total"`
}

type TrendResult struct {
	Period                 string        `json:"period"` // daily / weekly / monthly
	Points                 []TrendPoint  `json:"points"`
	PeakPeriod             *apitime.Time `json:"peak_period,omitempty"`
	PeakAmount             float64       `json:"peak_amount"`
	TroughPeriod           *apitime.Time `json:"trough_period,omitempty"`
	TroughAmount           float64       `json:"trough_amount"`
	FirstVsLastPct         float64       `json:"first_vs_last_pct"`
	Direction              string        `json:"direction"`        // INCREASING / DECREASING / FLAT / VOLATILE
	VolatilityLevel        string        `json:"volatility_level"` // LOW / MODERATE / HIGH
	CoefficientOfVariation float64       `json:"coefficient_of_variation"`
	SpendVelocityPerDay    float64       `json:"spend_velocity_per_day"`
	Projected30Day         float64       `json:"projected_30_day"`
	OverallTrendAlert      bool          `json:"overall_trend_alert"`
	OverallTrendChangePct  float64       `json:"overall_trend_change_pct"`
}

// --- C/D: Category trend, anomalies & momentum ---

type CategoryStat struct {
	Category     string  `json:"category"`
	CurrentTotal float64 `json:"current_total"`
	PriorTotal   float64 `json:"prior_total"`
	ChangePct    float64 `json:"change_pct"`
	Direction    string  `json:"direction"` // UP / DOWN / FLAT
	IsAnomaly    bool    `json:"is_anomaly"`
	SharePct     float64 `json:"share_pct"`
}

type CategoryTrendResult struct {
	Categories          []CategoryStat `json:"categories"` // sorted by share desc
	TopCategory         string         `json:"top_category,omitempty"`
	TopCategorySharePct float64        `json:"top_category_share_pct"`
}

type CategoryMomentumResult struct {
	Rising  []CategoryStat `json:"rising"`
	Falling []CategoryStat `json:"falling"`
}

// --- E: Average transaction stats ---

type AverageStatsResult struct {
	AvgDebit         float64 `json:"avg_debit"`
	AvgCredit        float64 `json:"avg_credit"`
	AvgOverall       float64 `json:"avg_overall"`
	MaxDebit         float64 `json:"max_debit"`
	MinDebit         float64 `json:"min_debit"`
	TransactionCount int     `json:"transaction_count"`
}

// --- F: Merchant concentration & spikes ---

type MerchantStat struct {
	Merchant         string   `json:"merchant"`
	Total            float64  `json:"total"`
	SharePct         float64  `json:"share_pct"`
	VisitCount       int      `json:"visit_count"`
	PriorMonthlyAvg  float64  `json:"prior_monthly_avg,omitempty"`
	IsFrequencySpike bool     `json:"is_frequency_spike,omitempty"`
	MoMChangePct     *float64 `json:"mom_change_pct,omitempty"`
	IsMoMSpike       bool     `json:"is_mom_spike,omitempty"`
}

type ReactivatedMerchant struct {
	Merchant        string  `json:"merchant"`
	RecentTotal     float64 `json:"recent_total"`
	PriorVisitCount int     `json:"prior_visit_count"`
}

type MerchantAnalysisResult struct {
	TopMerchants         []MerchantStat        `json:"top_merchants"`
	Top3ConcentrationPct float64               `json:"top3_concentration_pct"`
	FrequencySpikes      []MerchantStat        `json:"frequency_spikes"`
	MoMSpikes            []MerchantStat        `json:"mom_spikes"`
	ReactivatedMerchants []ReactivatedMerchant `json:"reactivated_merchants"`
}

// --- G: Recurring / subscription detection ---

type RecurringExpense struct {
	Merchant        string       `json:"merchant"`
	AvgAmount       float64      `json:"avg_amount"`
	Occurrences     int          `json:"occurrences"`
	AvgIntervalDays float64      `json:"avg_interval_days"`
	Frequency       string       `json:"frequency"`  // MONTHLY / WEEKLY / IRREGULAR
	Confidence      string       `json:"confidence"` // HIGH / MEDIUM
	LastOccurredAt  apitime.Time `json:"last_occurred_at"`
}

type RecurringResult struct {
	Recurring            []RecurringExpense `json:"recurring"`
	TotalMonthlyEstimate float64            `json:"total_monthly_estimate"`
}

// --- H: Night spend & impulse ---

type ImpulseTransaction struct {
	ID         string       `json:"id"`
	Merchant   string       `json:"merchant"`
	Amount     float64      `json:"amount"`
	OccurredAt apitime.Time `json:"occurred_at"`
	Reason     string       `json:"reason"` // NIGHT_SPEND / SMALL_FREQUENT
}

type NightImpulseResult struct {
	NightSpendTotal     float64              `json:"night_spend_total"`
	NightSpendSharePct  float64              `json:"night_spend_share_pct"`
	ImpulseScore        float64              `json:"impulse_score"` // 0-100
	ImpulseTotal        float64              `json:"impulse_total"`
	ImpulseTransactions []ImpulseTransaction `json:"impulse_transactions"`
}

// --- I: Spending pattern summary ---

type TransactionRef struct {
	ID         string       `json:"id"`
	Merchant   string       `json:"merchant"`
	Amount     float64      `json:"amount"`
	OccurredAt apitime.Time `json:"occurred_at"`
}

type PatternSummaryResult struct {
	DailyAvg              float64            `json:"daily_avg"`
	WeeklyAvg             float64            `json:"weekly_avg"`
	MonthlyAvg            float64            `json:"monthly_avg"`
	LargestTransaction    *TransactionRef    `json:"largest_transaction,omitempty"`
	SmallestTransaction   *TransactionRef    `json:"smallest_transaction,omitempty"`
	MostExpensiveDay      *apitime.Time      `json:"most_expensive_day,omitempty"`
	MostExpensiveDayTotal float64            `json:"most_expensive_day_total"`
	PerWeekdayTotals      map[string]float64 `json:"per_weekday_totals"`
}

// --- J: Home snapshot ---

type SpendChartPoint struct {
	Label string  `json:"label"`
	Value float64 `json:"value"`
}

type SnapshotResult struct {
	TodaySpend               float64           `json:"today_spend"`
	WeeklySpend              float64           `json:"weekly_spend"`
	GrowthPct                float64           `json:"growth_pct"`
	CurrentBalance           *float64          `json:"current_balance,omitempty"`
	BalanceDrawdownTodayPct  *float64          `json:"balance_drawdown_today_pct,omitempty"`
	BalanceDrawdownWeeklyPct *float64          `json:"balance_drawdown_weekly_pct,omitempty"`
	SpendChart               []SpendChartPoint `json:"spend_chart"`
}

// --- K: Comparison ---

type ComparisonEntry struct {
	Name     string  `json:"name"`
	Total    float64 `json:"total"`
	SharePct float64 `json:"share_pct"`
}

type ComparisonResult struct {
	By      string            `json:"by"` // category | merchant
	Entries []ComparisonEntry `json:"entries"`
	Total   float64           `json:"total"`
}

// --- L: Investment consistency (over stock_orders BUY activity) ---

// InvestmentEvent is a single completed BUY order, as sourced from the
// Stocks domain's stock_orders table (see provider/analytics.InvestmentSource).
type InvestmentEvent struct {
	OccurredAt time.Time
	Amount     float64
}

type InvestmentConsistencyResult struct {
	MonthsTracked       int     `json:"months_tracked"`
	ActiveMonths        int     `json:"active_months"`
	ActiveMonthPct      float64 `json:"active_month_pct"`
	CurrentStreakMonths int     `json:"current_streak_months"`
	MissedMonths        int     `json:"missed_months"`
	AvgMonthlyInvested  float64 `json:"avg_monthly_invested"`
}

// --- M: BNPL exposure ---

type BNPLProviderStat struct {
	Provider       string  `json:"provider"`
	Total          float64 `json:"total"`
	RepaymentCount int     `json:"repayment_count"`
}

type BNPLExposureResult struct {
	Last30DayTotal           float64            `json:"last_30_day_total"`
	IncomeRatioPct           float64            `json:"income_ratio_pct"`
	HistoricalIncomeRatioPct float64            `json:"historical_income_ratio_pct"`
	IsDangerZone             bool               `json:"is_danger_zone"`
	Providers                []BNPLProviderStat `json:"providers"`
}

// --- N: Verified subscription load ---

type SubscriptionLoadResult struct {
	ActiveSubscriptions  []RecurringExpense `json:"active_subscriptions"`
	TotalVerifiedMonthly float64            `json:"total_verified_monthly"`
}

// --- O: Income analysis ---

type IncomeResult struct {
	AvgCreditAmount     float64       `json:"avg_credit_amount"`
	TypicalIntervalDays float64       `json:"typical_interval_days"`
	NextPredictedPayday *apitime.Time `json:"next_predicted_payday,omitempty"`
	StabilityLabel      string        `json:"stability_label"` // STABLE / VARIABLE / IRREGULAR
	FrequencyLabel      string        `json:"frequency_label"` // MONTHLY / OCCASIONAL / IRREGULAR
	PrimarySource       string        `json:"primary_source,omitempty"`
	CreditCount         int           `json:"credit_count"`
}
