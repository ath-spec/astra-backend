// Package budget orchestrates the budget feature: 6-month spend diagnosis,
// ML-assisted category suggestions, the setup-session wizard, finalize into
// active monthly budgets, and the active dashboard. Response shapes match
// the reference budget API so the ported Flutter client works unchanged.
package budget

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	analyticsdomain "github.com/yourusername/astra-backend/internal/domain/analytics"
	budgetdomain "github.com/yourusername/astra-backend/internal/domain/budget"
	analyticsprovider "github.com/yourusername/astra-backend/internal/provider/analytics"
	budgetprovider "github.com/yourusername/astra-backend/internal/provider/budget"
	"github.com/yourusername/astra-backend/internal/repository"
)

const (
	diagnosisHistoryMonths = 6
	diagnosisCacheTTL      = 24 * time.Hour
	sessionTTL             = 24 * time.Hour
)

// ConflictError is surfaced by SuggestCategories when the ML rejects the
// requested total; the handler forwards RawBody with HTTP 409.
type ConflictError struct{ RawBody []byte }

func (e *ConflictError) Error() string { return "budget suggestion conflict" }

type Service struct {
	repo  *repository.BudgetRepository
	txns  analyticsprovider.TransactionSource
	ml    *budgetprovider.Client
	cache *ttlCache

	catMu  sync.Mutex
	catIdx map[string]repository.CategoryMeta // lower(key) -> category
	catExp time.Time
}

func NewService(repo *repository.BudgetRepository, txns analyticsprovider.TransactionSource, ml *budgetprovider.Client) *Service {
	return &Service{repo: repo, txns: txns, ml: ml, cache: newTTLCache()}
}

// categoryIndex loads (and briefly caches) the DB-driven category taxonomy:
// lower-cased canonical name / display name / alias -> category. This is the
// the taxonomy is data (this table), not a hard-coded switch.
func (s *Service) categoryIndex(ctx context.Context) map[string]repository.CategoryMeta {
	s.catMu.Lock()
	defer s.catMu.Unlock()
	if s.catIdx != nil && time.Now().Before(s.catExp) {
		return s.catIdx
	}
	idx, err := s.repo.CategoryIndex(ctx)
	if err != nil || len(idx) == 0 {
		if s.catIdx != nil {
			return s.catIdx // serve stale rather than nothing
		}
		return map[string]repository.CategoryMeta{}
	}
	s.catIdx = idx
	s.catExp = time.Now().Add(10 * time.Minute)
	return idx
}

// canon resolves any raw transaction / bill category string to its canonical
// category name; unknown strings fall back to "other".
func (s *Service) canon(ctx context.Context, raw string) string {
	if m, ok := s.categoryIndex(ctx)[strings.ToLower(strings.TrimSpace(raw))]; ok {
		return m.Name
	}
	return "other"
}

// canonFunc returns canon bound to ctx, for threading into pure helpers.
func (s *Service) canonFunc(ctx context.Context) func(string) string {
	idx := s.categoryIndex(ctx)
	return func(raw string) string {
		if m, ok := idx[strings.ToLower(strings.TrimSpace(raw))]; ok {
			return m.Name
		}
		return "other"
	}
}

// categoryMeta resolves a name / display name / alias to its category via the
// cached index, falling back to "other".
func (s *Service) categoryMeta(ctx context.Context, key string) (repository.CategoryMeta, bool) {
	idx := s.categoryIndex(ctx)
	if m, ok := idx[strings.ToLower(strings.TrimSpace(key))]; ok {
		return m, true
	}
	m, ok := idx["other"]
	return m, ok
}

// ─── Diagnosis ─────────────────────────────────────────────────────────

func (s *Service) Diagnosis(ctx context.Context, userID uuid.UUID, req budgetdomain.DiagnosisRequest) (budgetdomain.BudgetDiagnosisResponse, error) {
	month := req.Month
	if month == "" {
		month = time.Now().Format("2006-01")
	}
	cacheKey := "diag:" + userID.String() + ":" + month

	if !req.ForceRefresh {
		if raw, ok := s.cache.get(cacheKey); ok {
			var cached budgetdomain.BudgetDiagnosisResponse
			if json.Unmarshal(raw, &cached) == nil {
				return cached, nil
			}
		}
	}

	target, err := time.Parse("2006-01", month)
	if err != nil {
		target = time.Now()
	}

	txns, err := s.fetchTransactions(ctx, userID, 40)
	if err != nil {
		return budgetdomain.BudgetDiagnosisResponse{}, err
	}

	hist := buildHistory(txns, target, s.canonFunc(ctx))
	suggestedBudget := hist.avgExpenses
	var suggestedCats []budgetdomain.SuggestedCategory
	var insights []budgetdomain.DiagnosisInsight

	if len(hist.categoryHistory) == 0 {
		// New user, no history: skip the ML round-trip entirely.
		suggestedBudget = 5000.0
		suggestedCats = computeFallbackCategories(suggestedBudget, hist.categoryHistory)
		insights = buildFallbackInsights(hist.avgExpenses, hist.avgIncome, hist.avgSavings)
	} else {
		// Ridge-weight persistence: if we hold weights trained on this user's
		// latest data month, pass them so the stateless model skips retraining
		// (identical data -> identical result). Otherwise send nothing and
		// store whatever it trains.
		existingLearned := map[string]interface{}{}
		latestDataMonth := latestHistoryMonth(hist)
		if lp, err := s.repo.GetLearnedParams(ctx, userID); err == nil && lp.Found &&
			lp.RidgeLastTrainedMonth == latestDataMonth && len(lp.RidgePredictions) > 2 &&
			string(lp.RidgePredictions) != "{}" {
			var ridge map[string]interface{}
			if json.Unmarshal(lp.RidgePredictions, &ridge) == nil {
				existingLearned["per_user_ridge_pred"] = ridge
				existingLearned["ridge_last_trained_month"] = lp.RidgeLastTrainedMonth
			}
		}

		mlResp, mlErr := s.ml.Diagnosis(ctx, userID.String(), budgetdomain.MLDiagnosisRequest{
			UserID:                userID.String(),
			Month:                 month,
			MonthlyVitals:         hist.monthlyVitals,
			CategoryHistory:       hist.categoryHistory,
			ExistingLearnedParams: existingLearned,
		})
		if mlErr == nil && mlResp != nil && mlResp.Data.SuggestedTotalBudget > 0 {
			suggestedBudget = mlResp.Data.SuggestedTotalBudget
			suggestedCats = mlResp.Data.Suggestions
			insights = mlResp.Data.DiagnosisInsights
			s.persistLearnedParams(ctx, userID, mlResp.Data.LearnedParams)
		} else {
			if hist.avgIncome > 0 {
				savingsRate := hist.avgSavings / hist.avgIncome
				if savingsRate <= 0.20 {
					suggestedBudget = hist.avgExpenses * 0.85
				}
			}
			if suggestedBudget <= 0 {
				suggestedBudget = 5000.0
			}
			suggestedCats = computeFallbackCategories(suggestedBudget, hist.categoryHistory)
			insights = buildFallbackInsights(hist.avgExpenses, hist.avgIncome, hist.avgSavings)
		}
	}

	s.decorateCategories(ctx, suggestedCats)

	resp := budgetdomain.BudgetDiagnosisResponse{
		Month:                    month,
		HistoricalSpending:       hist.historical,
		AverageIncome:            round2(hist.avgIncome),
		AverageExpenses:          round2(hist.avgExpenses),
		AverageSavings:           round2(hist.avgSavings),
		SuggestedTotalBudget:     round2(suggestedBudget),
		SuggestedCategories:      suggestedCats,
		SuggestedBudgetReasoning: buildSuggestedBudgetReasoning(hist.avgExpenses, hist.avgIncome, hist.avgSavings, suggestedBudget),
		DiagnosisInsights:        insights,
		GeneratedAt:              time.Now().UTC(),
	}
	if b, err := json.Marshal(resp); err == nil {
		s.cache.set(cacheKey, b, diagnosisCacheTTL)
	}
	return resp, nil
}

// ─── Suggest categories ────────────────────────────────────────────────

func (s *Service) SuggestCategories(ctx context.Context, userID uuid.UUID, req budgetdomain.SuggestCategoriesRequest) (budgetdomain.SuggestCategoriesResponse, error) {
	if req.TotalBudget <= 0 {
		return budgetdomain.SuggestCategoriesResponse{}, fmt.Errorf("total_budget must be positive")
	}
	month := req.Month
	if month == "" {
		month = time.Now().Format("2006-01")
	}
	cacheKey := "diag:" + userID.String() + ":" + month

	// ── Unchanged total: the category split was already computed during
	// diagnosis. Reuse it, skip the ML call entirely. ──
	//
	// ── Changed total (slider moved): invalidate the cached diagnosis and
	// fall through to the ML so it can re-check feasibility / conflicts. ──
	if raw, ok := s.cache.get(cacheKey); ok {
		var cached budgetdomain.BudgetDiagnosisResponse
		if json.Unmarshal(raw, &cached) == nil {
			if math.Round(cached.SuggestedTotalBudget) == math.Round(req.TotalBudget) {
				if len(cached.SuggestedCategories) > 0 {
					cats := cached.SuggestedCategories
					if cached.SuggestedTotalBudget > 0 && cached.SuggestedTotalBudget != req.TotalBudget {
						scale := req.TotalBudget / cached.SuggestedTotalBudget
						for i := range cats {
							cats[i].SuggestedAmount = round2(cats[i].SuggestedAmount * scale)
						}
					}
					var out budgetdomain.SuggestCategoriesResponse
					out.Status = "success"
					out.Data.TotalBudget = req.TotalBudget
					out.Data.Suggestions = cats
					return out, nil
				}
				// cache had no ML categories — fall through to the ML call.
			} else {
				// user moved the slider — drop the stale baseline
				s.cache.delete(cacheKey)
			}
		}
	}

	// A trend-fetch failure is non-fatal — proceed with empty history; the
	// empty-history branch below then returns no suggestions rather than
	// erroring.
	txns, _ := s.fetchTransactions(ctx, userID, 8)
	hist := buildHistory(txns, time.Now(), s.canonFunc(ctx))

	if len(hist.categoryHistory) == 0 {
		var out budgetdomain.SuggestCategoriesResponse
		out.Status = "success"
		out.Data.TotalBudget = req.TotalBudget
		out.Data.Suggestions = []budgetdomain.SuggestedCategory{}
		return out, nil
	}

	var protected []string
	if settings, err := s.repo.GetSettings(ctx, userID); err == nil {
		protected = normalizeList(settings.Bills, s.canonFunc(ctx))
	}

	mlResp, mlErr := s.ml.SuggestCategories(ctx, userID.String(), budgetdomain.MLSuggestRequest{
		UserID:                userID.String(),
		TotalBudget:           req.TotalBudget,
		Month:                 month,
		UserCategoryOverrides: req.UserCategoryOverrides,
		ProtectedCategories:   protected,
		History:               hist.categoryHistory,
	})
	if mlErr != nil {
		var conflict *budgetprovider.ConflictError
		if errors.As(mlErr, &conflict) {
			return budgetdomain.SuggestCategoriesResponse{}, &ConflictError{RawBody: conflict.RawBody}
		}
		// Graceful fallback: proportional split of historical spend.
		var out budgetdomain.SuggestCategoriesResponse
		out.Status = "success"
		out.Data.TotalBudget = req.TotalBudget
		out.Data.AlgorithmVersion = "fallback-v1"
		out.Data.Suggestions = computeFallbackCategories(req.TotalBudget, hist.categoryHistory)
		s.decorateCategories(ctx, out.Data.Suggestions)
		return out, nil
	}

	if mlResp.Data.ConflictDetails != nil {
		body, _ := json.Marshal(mlResp)
		return budgetdomain.SuggestCategoriesResponse{}, &ConflictError{RawBody: body}
	}

	// Rescale to the user's explicit total if the model returned a different one.
	if mlResp.Data.TotalBudget > 0 && mlResp.Data.TotalBudget != req.TotalBudget {
		scale := req.TotalBudget / mlResp.Data.TotalBudget
		for i := range mlResp.Data.Suggestions {
			mlResp.Data.Suggestions[i].SuggestedAmount = round2(mlResp.Data.Suggestions[i].SuggestedAmount * scale)
		}
		mlResp.Data.TotalBudget = req.TotalBudget
	}
	s.decorateCategories(ctx, mlResp.Data.Suggestions)
	return *mlResp, nil
}

// ─── Sessions ──────────────────────────────────────────────────────────

func (s *Service) CreateSession(ctx context.Context, userID uuid.UUID, month string) (budgetdomain.SessionResponse, error) {
	if month == "" {
		month = time.Now().Format("2006-01")
	}
	if err := s.repo.ExpirePendingSessions(ctx, userID, month); err != nil {
		return budgetdomain.SessionResponse{}, err
	}
	row, err := s.repo.CreateSession(ctx, userID, month, time.Now().Add(sessionTTL))
	if err != nil {
		return budgetdomain.SessionResponse{}, err
	}
	return sessionToResponse(row), nil
}

func (s *Service) GetSession(ctx context.Context, userID, sessionID uuid.UUID) (budgetdomain.SessionResponse, error) {
	row, err := s.repo.GetSession(ctx, userID, sessionID)
	if err != nil {
		return budgetdomain.SessionResponse{}, err
	}
	return sessionToResponse(row), nil
}

func (s *Service) UpdateSession(ctx context.Context, userID, sessionID uuid.UUID, req budgetdomain.UpdateSessionRequest) (budgetdomain.SessionResponse, error) {
	row, err := s.repo.UpdateSession(ctx, userID, sessionID, req.TotalBudget, req.CategoryAllocations)
	if err != nil {
		return budgetdomain.SessionResponse{}, err
	}
	return sessionToResponse(row), nil
}

func (s *Service) DeleteSession(ctx context.Context, userID, sessionID uuid.UUID) error {
	return s.repo.DeleteSession(ctx, userID, sessionID)
}

// FinalizeSession commits the wizard state into active monthly budgets +
// budget_settings.spending_limit.
func (s *Service) FinalizeSession(ctx context.Context, userID, sessionID uuid.UUID) error {
	row, err := s.repo.GetSession(ctx, userID, sessionID)
	if err != nil {
		return err
	}

	year, month := parseYearMonth(row.Month)

	var allocs []repository.FinalizedAllocation
	var totalFromCats float64
	for _, a := range row.Allocations {
		if !a.IsTracking {
			continue
		}
		meta, ok := s.categoryMeta(ctx, firstNonEmpty(a.CategoryID, a.CategoryName))
		if !ok {
			return fmt.Errorf("no categories configured")
		}
		allocs = append(allocs, repository.FinalizedAllocation{
			CategoryID: meta.ID,
			Amount:     a.Amount,
			IsHidden:   a.IsHidden,
		})
		totalFromCats += a.Amount
	}

	if len(allocs) > 0 {
		if err := s.repo.ReplaceMonth(ctx, userID, year, month, allocs); err != nil {
			return err
		}
	}

	total := totalFromCats
	if row.TotalBudget != nil && *row.TotalBudget > 0 {
		total = *row.TotalBudget
	}
	if total > 0 {
		if err := s.repo.SetSpendingLimitAndActivate(ctx, userID, total); err != nil {
			return err
		}
	}

	_ = s.repo.MarkSessionFinalized(ctx, userID, sessionID)
	s.cache.delete("diag:" + userID.String() + ":" + row.Month)
	return nil
}

// ─── Status / latest / patch / reset ───────────────────────────────────

func (s *Service) Status(ctx context.Context, userID uuid.UUID) (budgetdomain.StatusResponse, error) {
	rows, err := s.repo.LatestBudgets(ctx, userID, 100)
	if err != nil {
		return budgetdomain.StatusResponse{}, err
	}
	settings, _ := s.repo.GetSettings(ctx, userID)

	// A pending rollover draft (next month's budget the worker pre-computed).
	var rolloverID *string
	if draft, ok, _ := s.repo.LatestSessionByStatus(ctx, userID, "rollover_draft"); ok {
		id := draft.ID.String()
		rolloverID = &id
	}

	var total float64
	year, month := 0, 0
	if len(rows) > 0 {
		year, month = rows[0].Year, rows[0].Month
		for _, b := range rows {
			if b.Year == year && b.Month == month {
				total += b.BudgetedAmount
			}
		}
	}

	hasExplicit := false
	if settings.Found && settings.SpendingLimit != nil && *settings.SpendingLimit > 0 && settings.LastResetAt != nil {
		total = *settings.SpendingLimit
		hasExplicit = true
	}

	if len(rows) == 0 && !hasExplicit {
		return budgetdomain.StatusResponse{HasActiveBudget: false, PendingRolloverDraftID: rolloverID}, nil
	}
	if year == 0 {
		now := time.Now()
		year, month = now.Year(), int(now.Month())
	}
	return budgetdomain.StatusResponse{
		HasActiveBudget:        true,
		ActiveMonth:            fmt.Sprintf("%04d-%02d", year, month),
		TotalBudget:            round2(total),
		BudgetCount:            len(rows),
		LatestYear:             year,
		LatestMonth:            month,
		PendingRolloverDraftID: rolloverID,
	}, nil
}

func (s *Service) Latest(ctx context.Context, userID uuid.UUID, limit int) (budgetdomain.LatestBudgetResponse, error) {
	rows, err := s.repo.LatestBudgets(ctx, userID, limit)
	if err != nil {
		return budgetdomain.LatestBudgetResponse{}, err
	}
	settings, _ := s.repo.GetSettings(ctx, userID)

	if len(rows) == 0 {
		var explicit float64
		if settings.Found && settings.SpendingLimit != nil {
			explicit = *settings.SpendingLimit
		}
		return budgetdomain.LatestBudgetResponse{
			Budgets:     []budgetdomain.Budget{},
			TotalBudget: round2(explicit),
			Status:      "positive",
		}, nil
	}

	year, month := rows[0].Year, rows[0].Month
	spentByCat, totalSpent := s.actualSpendForMonth(ctx, userID, year, month)
	canon := s.canonFunc(ctx)

	now := time.Now()
	periodStart := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	periodEnd := periodStart.AddDate(0, 1, -1)
	daysInMonth := periodEnd.Day()
	daysElapsed := daysInMonth
	daysRemaining := 0
	if now.Year() == year && int(now.Month()) == month {
		daysElapsed = now.Day()
		daysRemaining = daysInMonth - now.Day()
	}

	var totalBudget float64
	out := make([]budgetdomain.Budget, 0, len(rows))
	for _, r := range rows {
		spent := spentByCat[canon(r.CategoryName)]
		remaining := r.BudgetedAmount - spent
		pct := 0.0
		if r.BudgetedAmount > 0 {
			pct = spent / r.BudgetedAmount * 100
		}
		out = append(out, budgetdomain.Budget{
			ID:                r.ID.String(),
			CategoryID:        r.CategoryID.String(),
			CategoryType:      r.CategoryType,
			CategoryName:      r.CategoryName,
			CategoryIcon:      r.CategoryIcon,
			CategoryColor:     r.CategoryColor,
			CategoryTextColor: "#000000",
			Year:              r.Year,
			Month:             r.Month,
			BudgetedAmount:    round2(r.BudgetedAmount),
			SpentAmount:       round2(spent),
			RemainingAmount:   round2(remaining),
			PercentageUsed:    round2(pct),
			Status:            categoryStatus(pct),
			IsActive:          r.IsActive,
			IsHidden:          r.IsHidden,
			Notes:             r.Notes,
			CreatedAt:         r.CreatedAt,
			UpdatedAt:         r.UpdatedAt,
		})
		totalBudget += r.BudgetedAmount
	}
	if settings.Found && settings.SpendingLimit != nil && *settings.SpendingLimit > 0 {
		totalBudget = *settings.SpendingLimit
	}

	projected := totalSpent
	if daysElapsed > 0 {
		projected = totalSpent / float64(daysElapsed) * float64(daysInMonth)
	}

	status := "on_track"
	switch {
	case totalBudget > 0 && totalSpent >= totalBudget:
		status = "exceeded"
	case totalBudget > 0 && projected > totalBudget:
		status = "at_risk"
	}

	health := 100.0
	if totalBudget > 0 {
		health = math.Max(0, math.Min(100, (1-projected/totalBudget)*100+50))
	}

	income := 0.0
	if settings.Found && settings.LinkedIncome != nil {
		income = *settings.LinkedIncome
	}
	if income == 0 {
		if raw, ok := s.cache.get("diag:" + userID.String() + ":" + fmt.Sprintf("%04d-%02d", year, month)); ok {
			var d budgetdomain.BudgetDiagnosisResponse
			if json.Unmarshal(raw, &d) == nil {
				income = d.AverageIncome
			}
		}
	}

	return budgetdomain.LatestBudgetResponse{
		Budgets:              out,
		Count:                len(out),
		LatestYear:           year,
		LatestMonth:          month,
		TotalBudget:          round2(totalBudget),
		TotalSpent:           round2(totalSpent),
		IncomeAmount:         round2(income),
		DaysRemainingInMonth: daysRemaining,
		ProjectedSpend:       round2(projected),
		BudgetPeriodStart:    periodStart.Format("2006-01-02"),
		BudgetPeriodEnd:      periodEnd.Format("2006-01-02"),
		Status:               status,
		HealthScore:          round2(health),
		LastUpdated:          rows[0].UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}, nil
}

func (s *Service) PatchLatestCategory(ctx context.Context, userID uuid.UUID, req budgetdomain.PatchCategoryRequest) error {
	catID, err := uuid.Parse(req.CategoryID)
	if err != nil {
		// Allow name / display name / alias too.
		meta, mok := s.categoryMeta(ctx, req.CategoryID)
		if !mok {
			return fmt.Errorf("invalid category_id")
		}
		catID = meta.ID
	}
	ok, err := s.repo.PatchCategoryAmount(ctx, userID, catID, req.Amount)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no active budget for that category")
	}
	return nil
}

func (s *Service) ResetActive(ctx context.Context, userID uuid.UUID) (string, error) {
	rows, _ := s.repo.LatestBudgets(ctx, userID, 1)
	now := time.Now()
	year, month := now.Year(), int(now.Month())
	if len(rows) > 0 {
		year, month = rows[0].Year, rows[0].Month
	}
	_ = s.repo.DeactivateMonth(ctx, userID, year, month)
	_ = s.repo.ClearActive(ctx, userID)
	s.cache.delete("diag:" + userID.String() + ":" + fmt.Sprintf("%04d-%02d", year, month))
	return fmt.Sprintf("Budget cleared for %s %d.", time.Month(month).String(), year), nil
}

// ─── Settings ──────────────────────────────────────────────────────────

func (s *Service) GetSettings(ctx context.Context, userID uuid.UUID) (budgetdomain.SettingsResponse, error) {
	res := budgetdomain.SettingsResponse{
		EssentialCategories: []string{},
		Bills:               []string{},
	}
	res.LinkedIncome.Source = "none"
	res.SpendingLimit.Source = "none"

	settings, err := s.repo.GetSettings(ctx, userID)
	if err != nil {
		return res, err
	}

	if settings.Found {
		if settings.LinkedIncome != nil {
			res.LinkedIncome.Amount = *settings.LinkedIncome
			res.LinkedIncome.Source = "user_set"
		}
		if settings.SpendingLimit != nil {
			res.SpendingLimit.Amount = *settings.SpendingLimit
			res.SpendingLimit.Source = "user_set"
		}
		if len(settings.EssentialCategories) > 0 {
			res.EssentialCategories = settings.EssentialCategories
		}
		if len(settings.Bills) > 0 {
			res.Bills = settings.Bills
		}
		if settings.LastResetAt != nil {
			t := settings.LastResetAt.Format("2006-01-02T15:04:05Z")
			res.LastReset = &t
		}
	}

	// spending_limit fallback: sum of the active month's budgets.
	if res.SpendingLimit.Source == "none" {
		rows, _ := s.repo.LatestBudgets(ctx, userID, 100)
		var total float64
		for _, b := range rows {
			total += b.BudgetedAmount
		}
		if total > 0 {
			res.SpendingLimit.Amount = round2(total)
			res.SpendingLimit.Source = "active_budget"
		}
	}

	// linked_income fallback: the diagnosis-derived average income
	// (\"auto_detected\" source).
	if res.LinkedIncome.Source == "none" {
		month := time.Now().Format("2006-01")
		if raw, ok := s.cache.get("diag:" + userID.String() + ":" + month); ok {
			var d budgetdomain.BudgetDiagnosisResponse
			if json.Unmarshal(raw, &d) == nil && d.AverageIncome > 0 {
				res.LinkedIncome.Amount = round2(d.AverageIncome)
				res.LinkedIncome.Source = "auto_detected"
			}
		}
	}

	// bills_total / essential_categories_total: this month's actual spend in
	// those categories.
	if len(res.Bills) > 0 || len(res.EssentialCategories) > 0 {
		now := time.Now()
		spentByCat, _ := s.actualSpendForMonth(ctx, userID, now.Year(), int(now.Month()))
		canon := s.canonFunc(ctx)
		for _, b := range res.Bills {
			res.BillsTotal += spentByCat[canon(b)]
		}
		for _, e := range res.EssentialCategories {
			res.EssentialCategoriesTotal += spentByCat[canon(e)]
		}
		res.BillsTotal = round2(res.BillsTotal)
		res.EssentialCategoriesTotal = round2(res.EssentialCategoriesTotal)
	}

	return res, nil
}

func (s *Service) UpdateSettings(ctx context.Context, userID uuid.UUID, req budgetdomain.UpdateSettingsRequest) error {
	return s.repo.UpsertSettings(ctx, userID, req)
}

func (s *Service) GlobalCategories(ctx context.Context) (budgetdomain.GlobalCategoriesResponse, error) {
	cats, err := s.repo.ListGlobalCategories(ctx)
	if err != nil {
		return budgetdomain.GlobalCategoriesResponse{}, err
	}
	return budgetdomain.GlobalCategoriesResponse{Categories: cats, Count: len(cats)}, nil
}

// Insights proxies budget-bloc GET /analytics/recommendations and maps each
// recommendation into a dashboard BudgetInsight. On any ML failure it
// returns an empty list so the dashboard degrades quietly.
func (s *Service) Insights(ctx context.Context, userID uuid.UUID) budgetdomain.InsightsResponse {
	empty := budgetdomain.InsightsResponse{
		Insights:    []budgetdomain.BudgetInsight{},
		GeneratedAt: time.Now().UTC(),
	}

	budgetName := fmt.Sprintf("astra_%s_main", userID)
	recs, err := s.ml.Recommendations(ctx, userID.String(), budgetName)
	if err != nil || recs == nil {
		return empty
	}

	uid8 := userID.String()
	if len(uid8) > 8 {
		uid8 = uid8[:8]
	}

	out := make([]budgetdomain.BudgetInsight, 0, len(recs.Data.Recommendations))
	for i, rec := range recs.Data.Recommendations {
		severity := rec.Severity
		if severity == "" {
			severity = "positive"
		}
		title := firstNonEmpty(rec.Type, rec.Category)
		if title == "" {
			title = fmt.Sprintf("Insight %d", i+1)
		}
		description := rec.Suggestion
		if description == "" {
			description = firstNonEmpty(rec.Message, rec.Reasoning)
		}
		out = append(out, budgetdomain.BudgetInsight{
			ID:              fmt.Sprintf("ins_%03d_%s", i+1, uid8),
			Title:           title,
			Description:     description,
			Severity:        severity,
			Category:        rec.Category,
			PotentialSaving: 0,
			ActionType:      mapActionType(rec.Type, rec.Severity),
		})
	}
	return budgetdomain.InsightsResponse{Insights: out, GeneratedAt: time.Now().UTC()}
}

// mapActionType converts a budget-bloc recommendation type/severity into the
// Flutter-friendly action bucket.
func mapActionType(recType, severity string) string {
	switch recType {
	case "overspending", "budget_exceeded":
		return "reduce_spend"
	case "reallocation", "transfer":
		return "reallocate"
	case "savings", "save_more":
		return "save_more"
	default:
		if severity == "critical" || severity == "warning" {
			return "reduce_spend"
		}
		return "save_more"
	}
}

// ─── internals ─────────────────────────────────────────────────────────

func (s *Service) fetchTransactions(ctx context.Context, userID uuid.UUID, monthsBack int) ([]analyticsdomain.Transaction, error) {
	now := time.Now().UTC()
	from := now.AddDate(0, -monthsBack, 0)
	return s.txns.GetTransactions(ctx, userID, from, now)
}

func (s *Service) actualSpendForMonth(ctx context.Context, userID uuid.UUID, year, month int) (map[string]float64, float64) {
	byCat := map[string]float64{}
	var total float64
	txns, err := s.fetchTransactions(ctx, userID, 3)
	if err != nil {
		return byCat, 0
	}
	canon := s.canonFunc(ctx)
	for _, t := range txns {
		if t.Type != analyticsdomain.TxnDebit {
			continue
		}
		if t.OccurredAt.Year() != year || int(t.OccurredAt.Month()) != month {
			continue
		}
		byCat[canon(t.Category)] += t.Amount
		total += t.Amount
	}
	return byCat, total
}

// decorateCategories fills in display name / icon / colour on ML suggestions
// from the DB category index (resolving the id through canonical name /
// display name / alias).
func (s *Service) decorateCategories(ctx context.Context, cats []budgetdomain.SuggestedCategory) {
	if len(cats) == 0 {
		return
	}
	idx := s.categoryIndex(ctx)
	for i := range cats {
		if m, ok := idx[strings.ToLower(strings.TrimSpace(cats[i].CategoryID))]; ok {
			cats[i].CategoryID = m.Name
			if cats[i].CategoryName == "" {
				cats[i].CategoryName = m.DisplayName
			}
			if cats[i].CategoryIcon == "" {
				cats[i].CategoryIcon = m.Icon
			}
			if cats[i].CategoryColor == "" {
				cats[i].CategoryColor = m.Color
			}
		} else if cats[i].CategoryName == "" {
			cats[i].CategoryName = titleize(cats[i].CategoryID)
		}
	}
}

type historyBundle struct {
	monthlyVitals   []budgetdomain.MonthlyVital
	categoryHistory map[string][]map[string]interface{}
	historical      []budgetdomain.HistoricalSpendingEntry
	avgIncome       float64
	avgExpenses     float64
	avgSavings      float64
}

// buildHistory aggregates raw transactions into the monthly + per-category
// shapes budget-bloc expects, using only months strictly before target.
// canon resolves a raw transaction category to its canonical name.
func buildHistory(txns []analyticsdomain.Transaction, target time.Time, canon func(string) string) historyBundle {
	type mk struct{ y, m int }
	incomeByMonth := map[mk]float64{}
	expenseByMonth := map[mk]float64{}
	catByMonth := map[string]map[mk]float64{}

	cutoff := time.Date(target.Year(), target.Month(), 1, 0, 0, 0, 0, time.UTC)

	for _, t := range txns {
		if !t.OccurredAt.Before(cutoff) {
			continue
		}
		key := mk{t.OccurredAt.Year(), int(t.OccurredAt.Month())}
		switch t.Type {
		case analyticsdomain.TxnCredit:
			incomeByMonth[key] += t.Amount
		case analyticsdomain.TxnDebit:
			expenseByMonth[key] += t.Amount
			cat := canon(t.Category)
			if catByMonth[cat] == nil {
				catByMonth[cat] = map[mk]float64{}
			}
			catByMonth[cat][key] += t.Amount
		}
	}

	// Collect the most recent N months that have any activity.
	monthsSet := map[mk]bool{}
	for k := range incomeByMonth {
		monthsSet[k] = true
	}
	for k := range expenseByMonth {
		monthsSet[k] = true
	}
	months := make([]mk, 0, len(monthsSet))
	for k := range monthsSet {
		months = append(months, k)
	}
	sort.Slice(months, func(i, j int) bool {
		if months[i].y != months[j].y {
			return months[i].y > months[j].y
		}
		return months[i].m > months[j].m
	})
	if len(months) > diagnosisHistoryMonths {
		months = months[:diagnosisHistoryMonths]
	}

	var b historyBundle
	b.categoryHistory = map[string][]map[string]interface{}{}
	var sumInc, sumExp float64
	var nInc, nExp int

	for _, k := range months {
		inc := incomeByMonth[k]
		exp := expenseByMonth[k]
		sav := inc - exp
		if sav < 0 {
			sav = 0
		}
		monthStr := fmt.Sprintf("%04d-%02d", k.y, k.m)
		b.historical = append(b.historical, budgetdomain.HistoricalSpendingEntry{
			Month: k.m, Year: k.y, Income: round2(inc), Expenses: round2(exp), Savings: round2(sav),
		})
		b.monthlyVitals = append(b.monthlyVitals, budgetdomain.MonthlyVital{
			Month: monthStr, Salary: round2(inc), Expense: round2(exp), Savings: round2(sav),
		})
		if exp > 0 {
			sumExp += exp
			nExp++
		}
		if inc > 0 {
			sumInc += inc
			nInc++
		}
	}

	inWindow := func(k mk) bool {
		for _, m := range months {
			if m == k {
				return true
			}
		}
		return false
	}
	for cat, byMonth := range catByMonth {
		for k, spent := range byMonth {
			if !inWindow(k) || spent <= 0 {
				continue
			}
			b.categoryHistory[cat] = append(b.categoryHistory[cat], map[string]interface{}{
				"spent": round2(spent),
				"month": fmt.Sprintf("%04d-%02d", k.y, k.m),
			})
		}
	}

	if nExp > 0 {
		b.avgExpenses = sumExp / float64(nExp)
	}
	if nInc > 0 {
		b.avgIncome = sumInc / float64(nInc)
	}
	b.avgSavings = b.avgIncome - b.avgExpenses
	if b.avgSavings < 0 {
		b.avgSavings = 0
	}
	return b
}

// latestHistoryMonth returns the most recent "YYYY-MM" present in the built
// history (used for the Ridge-weight staleness check).
func latestHistoryMonth(h historyBundle) string {
	latest := ""
	for _, e := range h.historical {
		m := fmt.Sprintf("%04d-%02d", e.Year, e.Month)
		if m > latest {
			latest = m
		}
	}
	return latest
}

func normalizeList(in []string, canon func(string) string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		out = append(out, canon(s))
	}
	return out
}

// computeFallbackCategories — proportional split of historical spend, or a
// sensible default distribution when there is no history.
func computeFallbackCategories(total float64, historyMap map[string][]map[string]interface{}) []budgetdomain.SuggestedCategory {
	if len(historyMap) == 0 {
		mk := func(id string, frac, conf, lo, hi float64) budgetdomain.SuggestedCategory {
			return budgetdomain.SuggestedCategory{
				CategoryID: id, SuggestedAmount: round2(total * frac), ConfidenceScore: conf,
				AdjustmentBounds: []float64{round2(lo * total), round2(hi * total)},
			}
		}
		// No-history default split.
		return []budgetdomain.SuggestedCategory{
			mk("shopping", 0.35, 0.99, 0.30, 0.40),
			mk("food_dining", 0.20, 0.90, 0.15, 0.25),
			mk("transportation", 0.15, 0.85, 0.10, 0.20),
			mk("utilities", 0.10, 0.95, 0.08, 0.12),
			mk("entertainment", 0.10, 0.95, 0.08, 0.12),
			mk("healthcare", 0.05, 0.80, 0.02, 0.08),
			mk("insurance", 0.05, 0.98, 0.04, 0.06),
		}
	}

	catTotals := map[string]float64{}
	var grand float64
	for cat, entries := range historyMap {
		var sum float64
		for _, e := range entries {
			if v, ok := e["spent"].(float64); ok {
				sum += v
			}
		}
		catTotals[cat] = sum
		grand += sum
	}
	if grand <= 0 {
		return computeFallbackCategories(total, nil)
	}

	var out []budgetdomain.SuggestedCategory
	for cat, sum := range catTotals {
		if sum <= 0 {
			continue
		}
		amt := round2(total * (sum / grand))
		out = append(out, budgetdomain.SuggestedCategory{
			CategoryID: cat, SuggestedAmount: amt, ConfidenceScore: 0.85,
			AdjustmentBounds: []float64{round2(amt * 0.8), round2(amt * 1.2)},
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SuggestedAmount > out[j].SuggestedAmount })
	return out
}

func buildFallbackInsights(avgExpenses, avgIncome, avgSavings float64) []budgetdomain.DiagnosisInsight {
	insights := []budgetdomain.DiagnosisInsight{}
	if avgIncome <= 0 {
		return append(insights, budgetdomain.DiagnosisInsight{
			Title:       "Set Up Your Budget",
			Description: "We don't have enough spending history yet. Start tracking your expenses to get personalized insights.",
			Severity:    "positive",
		})
	}
	rate := avgSavings / avgIncome
	switch {
	case rate >= 0.30:
		insights = append(insights, budgetdomain.DiagnosisInsight{Title: "Excellent Savings Rate",
			Description: fmt.Sprintf("You're saving %.0f%% of your income. That's outstanding financial discipline!", rate*100), Severity: "positive"})
	case rate >= 0.20:
		insights = append(insights, budgetdomain.DiagnosisInsight{Title: "Good Savings Rate",
			Description: fmt.Sprintf("You're saving %.0f%% of your income. You're on track with the recommended 20%% rule.", rate*100), Severity: "positive"})
	case rate >= 0.10:
		insights = append(insights, budgetdomain.DiagnosisInsight{Title: "Savings Could Improve",
			Description: fmt.Sprintf("You're saving %.0f%% of your income. Aim for 20%% to build a stronger cushion.", rate*100), Severity: "warning"})
	default:
		insights = append(insights, budgetdomain.DiagnosisInsight{Title: "Low Savings Alert",
			Description: "You're saving less than 10% of your income. Reducing discretionary spending could help significantly.", Severity: "critical"})
	}
	if avgExpenses > avgIncome*0.95 {
		insights = append(insights, budgetdomain.DiagnosisInsight{Title: "Spending Near Limit",
			Description: "Your expenses are very close to your income. A budget plan can help you stay in control.", Severity: "warning"})
	}
	return insights
}

func buildSuggestedBudgetReasoning(avgExpenses, avgIncome, avgSavings, suggested float64) []string {
	if avgExpenses <= 0 {
		return []string{
			"This is a sensible starting point for first-time budgeters.",
			"As you track more spending, we'll refine this suggestion based on your real patterns.",
		}
	}
	reasons := make([]string, 0, 3)
	if avgIncome > 0 {
		rate := avgSavings / avgIncome
		if rate <= 0.20 {
			reasons = append(reasons, fmt.Sprintf("your savings rate is currently %.0f%% of income. we nudged the budget to %.0f%% of your average spend to build a stronger cushion.",
				rate*100, (suggested/avgExpenses)*100))
		} else {
			reasons = append(reasons, fmt.Sprintf("you're already saving %.0f%% of your income — great discipline. we matched the budget to your average spend so you don't over-restrict.", rate*100))
		}
		monthlySaving := avgIncome - suggested
		if monthlySaving > 0 {
			reasons = append(reasons, fmt.Sprintf("sticking to this budget means saving roughly ₹%.0f/month — ₹%.0f over the year.", monthlySaving, monthlySaving*12))
		} else {
			reasons = append(reasons, "this budget is close to your income — keep an eye on discretionary categories to avoid dipping into savings.")
		}
	} else {
		reasons = append(reasons, fmt.Sprintf("based on your average monthly spend of ₹%.0f, this budget gives you a realistic but slightly tighter target.", avgExpenses))
	}
	if diff := avgExpenses - suggested; diff > 50 {
		reasons = append(reasons, fmt.Sprintf("it's ₹%.0f less than what you typically spend — achievable with small cuts in non-essential categories.", diff))
	} else {
		reasons = append(reasons, "this aligns closely with your typical spend — no dramatic lifestyle changes needed to stay on track.")
	}
	return reasons
}

func sessionToResponse(row repository.SessionRow) budgetdomain.SessionResponse {
	allocs := row.Allocations
	if allocs == nil {
		allocs = []budgetdomain.CategoryAllocation{}
	}
	return budgetdomain.SessionResponse{
		SessionID:           row.ID.String(),
		Month:               row.Month,
		TotalBudget:         row.TotalBudget,
		CategoryAllocations: allocs,
		Status:              row.Status,
		ExpiresAt:           row.ExpiresAt,
	}
}

func categoryStatus(pct float64) string {
	switch {
	case pct >= 100:
		return "critical"
	case pct >= 80:
		return "warning"
	default:
		return "positive"
	}
}

func parseYearMonth(s string) (int, int) {
	var y, m int
	if _, err := fmt.Sscanf(s, "%d-%d", &y, &m); err != nil || m < 1 || m > 12 {
		now := time.Now()
		return now.Year(), int(now.Month())
	}
	return y, m
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func titleize(s string) string {
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == '_' || r == ' ' })
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + strings.ToLower(p[1:])
	}
	return strings.Join(parts, " ")
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

// ─── tiny in-process TTL cache (single API instance today) ──────────────

type ttlEntry struct {
	data []byte
	exp  time.Time
}

type ttlCache struct {
	mu sync.Mutex
	m  map[string]ttlEntry
}

func newTTLCache() *ttlCache { return &ttlCache{m: map[string]ttlEntry{}} }

func (c *ttlCache) get(k string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[k]
	if !ok || time.Now().After(e.exp) {
		delete(c.m, k)
		return nil, false
	}
	return e.data, true
}

func (c *ttlCache) set(k string, v []byte, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[k] = ttlEntry{data: v, exp: time.Now().Add(ttl)}
}

func (c *ttlCache) delete(k string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.m, k)
}
