package rm

import (
	analyticsdomain "github.com/yourusername/astra-backend/internal/domain/analytics"
)

// ClientSpendIntelligence is the RM-facing "personal spend view" for one
// client: every analyzer in the spend-analytics engine (internal/service/analytics),
// plus the client's active monthly budget if they have set one. Each analyzer
// field is a pointer and omitted entirely when the client has too little
// transaction history for that metric to mean anything — the frontend renders
// only the cards it receives rather than showing zeroed-out charts.
type ClientSpendIntelligence struct {
	WeekdayWeekend        *analyticsdomain.WeekdayWeekendResult        `json:"weekday_weekend,omitempty"`
	Trend                 *analyticsdomain.TrendResult                 `json:"trend,omitempty"`
	CategoryTrend         *analyticsdomain.CategoryTrendResult         `json:"category_trend,omitempty"`
	CategoryMomentum      *analyticsdomain.CategoryMomentumResult      `json:"category_momentum,omitempty"`
	AverageStats          *analyticsdomain.AverageStatsResult          `json:"average_stats,omitempty"`
	MerchantAnalysis      *analyticsdomain.MerchantAnalysisResult      `json:"merchant_analysis,omitempty"`
	Recurring             *analyticsdomain.RecurringResult             `json:"recurring,omitempty"`
	NightImpulse          *analyticsdomain.NightImpulseResult          `json:"night_impulse,omitempty"`
	PatternSummary        *analyticsdomain.PatternSummaryResult        `json:"pattern_summary,omitempty"`
	Snapshot              *analyticsdomain.SnapshotResult              `json:"snapshot,omitempty"`
	CategoryComparison    *analyticsdomain.ComparisonResult            `json:"category_comparison,omitempty"`
	InvestmentConsistency *analyticsdomain.InvestmentConsistencyResult `json:"investment_consistency,omitempty"`
	BNPLExposure          *analyticsdomain.BNPLExposureResult          `json:"bnpl_exposure,omitempty"`
	SubscriptionLoad      *analyticsdomain.SubscriptionLoadResult      `json:"subscription_load,omitempty"`
	Income                *analyticsdomain.IncomeResult                `json:"income,omitempty"`
	Budget                *BudgetSnapshot                              `json:"budget,omitempty"`
}

// BudgetSnapshot is the RM-facing read of a client's active monthly budget —
// a thin projection of budgetdomain.LatestBudgetResponse. HasBudget is false
// (and every other field zero) when the client has never set one up.
type BudgetSnapshot struct {
	HasBudget      bool                `json:"has_budget"`
	ActiveMonth    string              `json:"active_month,omitempty"`
	TotalBudget    float64             `json:"total_budget"`
	TotalSpent     float64             `json:"total_spent"`
	ProjectedSpend float64             `json:"projected_spend"`
	DaysRemaining  int                 `json:"days_remaining_in_month"`
	Status         string              `json:"status,omitempty"` // on_track / at_risk / exceeded / positive
	HealthScore    float64             `json:"health_score"`
	Categories     []BudgetCategoryRow `json:"categories,omitempty"`
}

// BudgetCategoryRow is one category line of the client's active budget.
type BudgetCategoryRow struct {
	Name            string  `json:"name"`
	Icon            string  `json:"icon,omitempty"`
	Color           string  `json:"color,omitempty"`
	BudgetedAmount  float64 `json:"budgeted_amount"`
	SpentAmount     float64 `json:"spent_amount"`
	RemainingAmount float64 `json:"remaining_amount"`
	PercentageUsed  float64 `json:"percentage_used"`
	Status          string  `json:"status"`
}
