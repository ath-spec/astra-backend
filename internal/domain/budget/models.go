// Package budget defines the wire types for the budget feature. The JSON
// shapes here match the reference budget API 1:1 so the ported Flutter
// client (features/budget) works unchanged. Responses are emitted raw
// (no apiresponse envelope).
package budget

import (
	"encoding/json"
	"time"
)

// ─── Categories ──────────────────────────────────────────────────────────

type GlobalCategory struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	Color       string `json:"color"`
	IsActive    bool   `json:"is_active"`
	SortOrder   int    `json:"sort_order"`
}

type GlobalCategoriesResponse struct {
	Categories []GlobalCategory `json:"categories"`
	Count      int              `json:"count"`
}

// ─── Diagnosis (POST /diagnosis) ─────────────────────────────────────────

type DiagnosisRequest struct {
	Month        string `json:"month"`
	ForceRefresh bool   `json:"force_refresh"`
}

type HistoricalSpendingEntry struct {
	Month    int     `json:"month"`
	Year     int     `json:"year"`
	Income   float64 `json:"income"`
	Expenses float64 `json:"expenses"`
	Savings  float64 `json:"savings"`
}

type DiagnosisInsight struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Severity    string `json:"severity"` // positive | warning | critical
}

type BudgetDiagnosisResponse struct {
	Month                    string                    `json:"month"`
	HistoricalSpending       []HistoricalSpendingEntry `json:"historical_spending"`
	AverageIncome            float64                   `json:"average_income"`
	AverageExpenses          float64                   `json:"average_expenses"`
	AverageSavings           float64                   `json:"average_savings"`
	SuggestedTotalBudget     float64                   `json:"suggested_total_budget"`
	SuggestedCategories      []SuggestedCategory       `json:"suggested_categories,omitempty"`
	SuggestedBudgetReasoning []string                  `json:"suggested_budget_reasoning"`
	DiagnosisInsights        []DiagnosisInsight        `json:"diagnosis_insights"`
	GeneratedAt              time.Time                 `json:"generated_at"`
}

// ─── Suggest categories (POST /suggest/categories) ───────────────────────

type SuggestCategoriesRequest struct {
	TotalBudget           float64            `json:"total_budget"`
	Month                 string             `json:"month"`
	UserCategoryOverrides map[string]float64 `json:"user_category_overrides,omitempty"`
}

type SuggestedCategory struct {
	CategoryID       string    `json:"category_id"`
	CategoryName     string    `json:"category_name,omitempty"`
	CategoryIcon     string    `json:"category_icon,omitempty"`
	CategoryColor    string    `json:"category_color,omitempty"`
	SuggestedAmount  float64   `json:"suggested_amount"`
	ConfidenceScore  float64   `json:"confidence_score"`
	AdjustmentBounds []float64 `json:"adjustment_bounds"` // [min, max]
	Reasoning        string    `json:"reasoning,omitempty"`
}

type ConflictDetails struct {
	Type          string   `json:"type"`
	Problem       string   `json:"problem"`
	OverageAmount float64  `json:"overage_amount"`
	Conflicts     []string `json:"conflicts"`
}

type SuggestCategoriesResponse struct {
	Status string `json:"status"`
	Data   struct {
		TotalBudget      float64             `json:"total_budget"`
		AlgorithmVersion string              `json:"algorithm_version"`
		Suggestions      []SuggestedCategory `json:"suggestions"`
		ConflictDetails  *ConflictDetails    `json:"conflict_details,omitempty"`
	} `json:"data"`
}

// ─── Setup sessions ─────────────────────────────────────────────────────

type CreateSessionRequest struct {
	Month string `json:"month"`
}

type CategoryAllocation struct {
	CategoryID    string  `json:"category_id"`
	CategoryName  string  `json:"category_name,omitempty"`
	CategoryIcon  string  `json:"category_icon,omitempty"`
	CategoryColor string  `json:"category_color,omitempty"`
	Amount        float64 `json:"amount"`
	IsTracking    bool    `json:"is_tracking"`
	IsHidden      bool    `json:"is_hidden"`
}

type UpdateSessionRequest struct {
	TotalBudget         *float64             `json:"total_budget,omitempty"`
	CategoryAllocations []CategoryAllocation `json:"category_allocations,omitempty"`
}

type SessionResponse struct {
	SessionID           string               `json:"session_id"`
	Month               string               `json:"month"`
	TotalBudget         *float64             `json:"total_budget"`
	CategoryAllocations []CategoryAllocation `json:"category_allocations"`
	Status              string               `json:"status"`
	ExpiresAt           time.Time            `json:"expires_at"`
}

// ─── Active budget / dashboard ─────────────────────────────────────────

type Budget struct {
	ID                string    `json:"id"`
	CategoryID        string    `json:"category_id"`
	CategoryType      string    `json:"category_type"`
	CategoryName      string    `json:"category_name"`
	CategoryIcon      string    `json:"category_icon"`
	CategoryColor     string    `json:"category_color"`
	CategoryTextColor string    `json:"category_text_color"`
	Year              int       `json:"year"`
	Month             int       `json:"month"`
	BudgetedAmount    float64   `json:"budgeted_amount"`
	SpentAmount       float64   `json:"spent_amount"`
	RemainingAmount   float64   `json:"remaining_amount"`
	PercentageUsed    float64   `json:"percentage_used"`
	Status            string    `json:"status"`
	IsActive          bool      `json:"is_active"`
	IsHidden          bool      `json:"is_hidden"`
	Notes             string    `json:"notes"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type LatestBudgetResponse struct {
	Budgets              []Budget `json:"budgets"`
	Count                int      `json:"count"`
	LatestYear           int      `json:"latest_year"`
	LatestMonth          int      `json:"latest_month"`
	TotalBudget          float64  `json:"total_budget"`
	TotalSpent           float64  `json:"total_spent"`
	IncomeAmount         float64  `json:"income_amount"`
	DaysRemainingInMonth int      `json:"days_remaining_in_month"`
	ProjectedSpend       float64  `json:"projected_spend"`
	BudgetPeriodStart    string   `json:"budget_period_start"`
	BudgetPeriodEnd      string   `json:"budget_period_end"`
	Status               string   `json:"status"`
	HealthScore          float64  `json:"health_score"`
	LastUpdated          string   `json:"last_updated"`
}

type StatusResponse struct {
	HasActiveBudget        bool    `json:"has_active_budget"`
	ActiveMonth            string  `json:"active_month"`
	TotalBudget            float64 `json:"total_budget"`
	BudgetCount            int     `json:"budget_count"`
	LatestYear             int     `json:"latest_year"`
	LatestMonth            int     `json:"latest_month"`
	PendingRolloverDraftID *string `json:"pending_rollover_draft_id"`
}

type PatchCategoryRequest struct {
	CategoryID string  `json:"category_id"`
	Amount     float64 `json:"amount"`
}

// ─── Settings ─────────────────────────────────────────────────────────

type LinkedIncomeSource struct {
	Amount       float64 `json:"amount"`
	Source       string  `json:"source"` // none | user_set | auto_detected
	LastDetected *string `json:"last_detected"`
}

type SpendingLimitSource struct {
	Amount float64 `json:"amount"`
	Source string  `json:"source"` // none | user_set | active_budget
}

type SettingsResponse struct {
	LinkedIncome             LinkedIncomeSource  `json:"linked_income"`
	SpendingLimit            SpendingLimitSource `json:"spending_limit"`
	EssentialCategories      []string            `json:"essential_categories"`
	EssentialCategoriesTotal float64             `json:"essential_categories_total"`
	Bills                    []string            `json:"bills"`
	BillsTotal               float64             `json:"bills_total"`
	LastReset                *string             `json:"last_reset"`
}

type UpdateSettingsRequest struct {
	LinkedIncome        *float64  `json:"linked_income,omitempty"`
	SpendingLimit       *float64  `json:"spending_limit,omitempty"`
	EssentialCategories *[]string `json:"essential_categories,omitempty"`
	Bills               *[]string `json:"bills,omitempty"`
}

// BudgetInsight is one dashboard recommendation (GET /insights). Shape
// matches the reference BudgetInsight and the Flutter BudgetInsight model.
type BudgetInsight struct {
	ID              string  `json:"id"`
	Title           string  `json:"title"`
	Description     string  `json:"description"`
	Severity        string  `json:"severity"` // positive | warning | critical
	Category        string  `json:"category"`
	PotentialSaving float64 `json:"potential_saving"`
	ActionType      string  `json:"action_type"` // reduce_spend | reallocate | save_more
}

type InsightsResponse struct {
	Insights    []BudgetInsight `json:"insights"`
	GeneratedAt time.Time       `json:"generated_at"`
}

// ─── budget-bloc (Hugging Face) client payloads ──────────────────────────
// These hit exactly two endpoints, used by the budget-creation flow:
//   POST {base}/ml/diagnosis
//   POST {base}/suggest/categories

type MonthlyVital struct {
	Month   string  `json:"month"`
	Salary  float64 `json:"salary"`
	Expense float64 `json:"expense"`
	Savings float64 `json:"savings"`
}

type MLDiagnosisRequest struct {
	UserID                string                              `json:"user_id"`
	Month                 string                              `json:"month"`
	MonthlyVitals         []MonthlyVital                      `json:"monthly_vitals"`
	CategoryHistory       map[string][]map[string]interface{} `json:"category_history"`
	ProtectedCategories   []string                            `json:"protected_categories,omitempty"`
	ExistingLearnedParams map[string]interface{}              `json:"existing_learned_params,omitempty"`
}

type MLDiagnosisResponse struct {
	Status string `json:"status"`
	Data   struct {
		SuggestedTotalBudget float64                `json:"suggested_total_budget"`
		Suggestions          []SuggestedCategory    `json:"suggestions"`
		ConfidenceScore      float64                `json:"confidence_score"`
		DiagnosisInsights    []DiagnosisInsight     `json:"diagnosis_insights"`
		Reasoning            string                 `json:"reasoning"`
		LearnedParams        map[string]interface{} `json:"learned_params"`
	} `json:"data"`
}

type MLSuggestRequest struct {
	UserID                string                              `json:"user_id"`
	TotalBudget           float64                             `json:"total_budget"`
	Month                 string                              `json:"month,omitempty"`
	UserCategoryOverrides map[string]float64                  `json:"user_category_overrides,omitempty"`
	ProtectedCategories   []string                            `json:"protected_categories,omitempty"`
	History               map[string][]map[string]interface{} `json:"history,omitempty"`
}

// MLRecommendation is one entry from budget-bloc GET /analytics/recommendations.
type MLRecommendation struct {
	Type            string  `json:"type"`
	Category        string  `json:"category"`
	Message         string  `json:"message"`
	Severity        string  `json:"severity"`
	Suggestion      string  `json:"suggestion"`
	ConfidenceScore float64 `json:"confidence_score"`
	Reasoning       string  `json:"reasoning"`
}

type MLRecommendationsResponse struct {
	Status string `json:"status"`
	Data   struct {
		Recommendations []MLRecommendation `json:"recommendations"`
		OverallStatus   string             `json:"overall_status"`
		PriorityActions []string           `json:"priority_actions"`
	} `json:"data"`
}

// ─── Smart Rebalance: reallocation (POST /reallocation/run|apply) ────────

type RunReallocationRequest struct {
	TargetCategory    string  `json:"target_category"`
	MinRemainingRatio float64 `json:"min_remaining_ratio,omitempty"`
	Debug             bool    `json:"debug,omitempty"`
}

// Reallocation is one donor -> target move.
type Reallocation struct {
	FromCategory string  `json:"from_category"`
	ToCategory   string  `json:"to_category"`
	Amount       float64 `json:"amount"`
	Reason       string  `json:"reason,omitempty"`
}

type RunReallocationResponse struct {
	Status string `json:"status"`
	Data   struct {
		Message            string             `json:"message"`
		SessionID          string             `json:"session_id"`
		InitialBudget      map[string]float64 `json:"initial_budget"`
		UpdatedBudget      map[string]float64 `json:"updated_budget"`
		CurrentSpend       map[string]float64 `json:"current_spend"`
		Reallocations      []Reallocation     `json:"reallocations"`
		UncoveredOverspend float64            `json:"uncovered_overspend"`
		TargetCategory     string             `json:"target_category"`
		OverspentAmount    float64            `json:"overspent_amount"`
	} `json:"data"`
}

type ApplyReallocationRequest struct {
	SessionID      string         `json:"session_id"`
	Month          string         `json:"month,omitempty"`
	TargetCategory string         `json:"target_category,omitempty"`
	Reallocations  []Reallocation `json:"reallocations"`
}

type ApplyReallocationResponse struct {
	Status string `json:"status"`
	Data   struct {
		Message              string             `json:"message"`
		AppliedReallocations []Reallocation     `json:"applied_reallocations"`
		NewBudget            map[string]float64 `json:"new_budget"`
	} `json:"data"`
}

// ─── ML feedback loop (GET /ml/suggestions, POST .../respond, GET /ml/cooldowns) ──

type MLSuggestion struct {
	ID              string          `json:"id"`
	SuggestionType  string          `json:"suggestion_type"`
	Details         json.RawMessage `json:"suggestion_details"`
	ConfidenceScore float64         `json:"confidence_score"`
	Priority        string          `json:"priority,omitempty"`
	UserResponse    string          `json:"user_response"`
	CreatedAt       time.Time       `json:"created_at"`
}

type MLSuggestionsResponse struct {
	Status      string         `json:"status"`
	Suggestions []MLSuggestion `json:"suggestions"`
	Count       int            `json:"count"`
}

type RespondSuggestionRequest struct {
	Action         string                 `json:"action"` // accepted | rejected | modified
	ModifiedValues map[string]interface{} `json:"modified_values,omitempty"`
	Reason         string                 `json:"reason,omitempty"`
}

type Cooldown struct {
	ID             string    `json:"id"`
	CooldownType   string    `json:"cooldown_type"`
	FromCategory   string    `json:"from_category,omitempty"`
	ToCategory     string    `json:"to_category,omitempty"`
	Category       string    `json:"category,omitempty"`
	CooldownUntil  time.Time `json:"cooldown_until"`
	RejectionCount int       `json:"rejection_count"`
	Reason         string    `json:"reason,omitempty"`
}

type CooldownsResponse struct {
	Status    string     `json:"status"`
	Cooldowns []Cooldown `json:"cooldowns"`
	Count     int        `json:"count"`
}

// ─── budget-bloc reallocation / learn payloads ─────────────────────────

type MLRunReallocationRequest struct {
	BudgetName           string                              `json:"budget_name"`
	CurrentBudgets       map[string]float64                  `json:"current_budgets"`
	CurrentSpend         map[string]float64                  `json:"current_spend"`
	History              map[string][]map[string]interface{} `json:"history,omitempty"`
	TargetCategory       string                              `json:"target_category"`
	MinRemainingRatio    float64                             `json:"min_remaining_ratio,omitempty"`
	Weights              []float64                           `json:"weights,omitempty"`
	Debug                bool                                `json:"debug,omitempty"`
	LearnedRelationships map[string]float64                  `json:"learned_relationships,omitempty"`
	LearnedDonorRatios   map[string]float64                  `json:"learned_donor_ratios,omitempty"`
	LearnedAdherence     map[string]map[string]float64       `json:"learned_adherence,omitempty"`
	LearnedWeights       []float64                           `json:"learned_weights,omitempty"`
}

type MLRunReallocationResponse struct {
	Status string `json:"status"`
	Data   struct {
		Message            string             `json:"message"`
		InitialBudget      map[string]float64 `json:"initial_budget"`
		UpdatedBudget      map[string]float64 `json:"updated_budget"`
		CurrentSpend       map[string]float64 `json:"current_spend"`
		Reallocations      []Reallocation     `json:"reallocations"`
		UncoveredOverspend float64            `json:"uncovered_overspend"`
		TargetCategory     string             `json:"target_category"`
		OverspentAmount    float64            `json:"overspent_amount"`
	} `json:"data"`
}

type MLApplyReallocationRequest struct {
	BudgetName    string         `json:"budget_name"`
	Month         string         `json:"month"`
	SessionID     string         `json:"session_id"`
	Reallocations []Reallocation `json:"reallocations"`
}

type MLApplyReallocationResponse struct {
	Status string `json:"status"`
	Data   struct {
		Message              string             `json:"message"`
		AppliedReallocations []Reallocation     `json:"applied_reallocations"`
		NewBudget            map[string]float64 `json:"new_budget"`
	} `json:"data"`
}

type MLLearnRequest struct {
	UserID         string                 `json:"user_id"`
	CurrentBudgets map[string]float64     `json:"current_budgets"`
	SessionData    map[string]interface{} `json:"session_data"`
	CurrentState   map[string]interface{} `json:"current_state"`
}

type MLLearnResponse struct {
	Status string `json:"status"`
	Data   struct {
		UpdatedState map[string]interface{} `json:"updated_state"`
	} `json:"data"`
}
