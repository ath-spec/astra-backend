package budget

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	budgetdomain "github.com/yourusername/astra-backend/internal/domain/budget"
	budgetprovider "github.com/yourusername/astra-backend/internal/provider/budget"
	"github.com/yourusername/astra-backend/internal/repository"
)

const defaultMinRemainingRatio = 0.60

func (s *Service) budgetName(userID uuid.UUID) string {
	return fmt.Sprintf("astra_%s_main", userID)
}

// RunReallocation asks budget-bloc to move budget from flexible donor
// categories into an overspent target, stores the run + its proposals, and
// surfaces a matching ML suggestion for the feedback loop.
func (s *Service) RunReallocation(ctx context.Context, userID uuid.UUID, req budgetdomain.RunReallocationRequest) (budgetdomain.RunReallocationResponse, error) {
	if strings.TrimSpace(req.TargetCategory) == "" {
		return budgetdomain.RunReallocationResponse{}, fmt.Errorf("target_category is required")
	}
	if req.MinRemainingRatio < 0 || req.MinRemainingRatio > 1 {
		return budgetdomain.RunReallocationResponse{}, fmt.Errorf("min_remaining_ratio must be between 0 and 1")
	}
	if req.MinRemainingRatio == 0 {
		req.MinRemainingRatio = defaultMinRemainingRatio
	}

	canon := s.canonFunc(ctx)
	target := canon(req.TargetCategory)

	rows, err := s.repo.LatestBudgets(ctx, userID, 100)
	if err != nil {
		return budgetdomain.RunReallocationResponse{}, err
	}
	if len(rows) == 0 {
		return budgetdomain.RunReallocationResponse{}, fmt.Errorf("no active budget to rebalance")
	}
	year, month := rows[0].Year, rows[0].Month

	currentBudgets := map[string]float64{}
	for _, r := range rows {
		currentBudgets[canon(r.CategoryName)] += r.BudgetedAmount
	}
	currentSpend, _ := s.actualSpendForMonth(ctx, userID, year, month)
	for k := range currentBudgets {
		if _, ok := currentSpend[k]; !ok {
			currentSpend[k] = 0
		}
	}

	txns, _ := s.fetchTransactions(ctx, userID, 8)
	hist := buildHistory(txns, time.Now(), canon)

	mlReq := budgetdomain.MLRunReallocationRequest{
		BudgetName:        s.budgetName(userID),
		CurrentBudgets:    currentBudgets,
		CurrentSpend:      currentSpend,
		History:           hist.categoryHistory,
		TargetCategory:    target,
		MinRemainingRatio: req.MinRemainingRatio,
		Debug:             req.Debug,
	}
	if lp, err := s.repo.GetLearnedParams(ctx, userID); err == nil && lp.Found {
		_ = json.Unmarshal(lp.Relationships, &mlReq.LearnedRelationships)
		_ = json.Unmarshal(lp.DonorRatios, &mlReq.LearnedDonorRatios)
		_ = json.Unmarshal(lp.AdherenceMetrics, &mlReq.LearnedAdherence)
		_ = json.Unmarshal(lp.ScoringWeights, &mlReq.LearnedWeights)
	}

	mlResp, mlErr := s.ml.RunReallocation(ctx, userID.String(), mlReq)
	if mlErr != nil {
		return budgetdomain.RunReallocationResponse{}, fmt.Errorf("%w: %v", budgetprovider.ErrUnavailable, mlErr)
	}

	d := mlResp.Data
	sessionID, err := s.repo.CreateReallocationSession(ctx, userID, target, req.MinRemainingRatio,
		d.InitialBudget, d.UpdatedBudget, d.CurrentSpend, d.UncoveredOverspend, d.OverspentAmount, d.Reallocations)
	if err != nil {
		return budgetdomain.RunReallocationResponse{}, err
	}

	// Surface a suggestion the user can accept / reject.
	if len(d.Reallocations) > 0 {
		details, _ := json.Marshal(map[string]interface{}{
			"target_category":     target,
			"reallocations":       d.Reallocations,
			"uncovered_overspend": d.UncoveredOverspend,
		})
		priority := "medium"
		if d.UncoveredOverspend > 0 {
			priority = "high"
		}
		_, _ = s.repo.CreateSuggestion(ctx, userID, &sessionID, "reallocation", details, 0, priority)
	}

	var out budgetdomain.RunReallocationResponse
	out.Status = "success"
	out.Data.Message = d.Message
	out.Data.SessionID = sessionID.String()
	out.Data.InitialBudget = d.InitialBudget
	out.Data.UpdatedBudget = d.UpdatedBudget
	out.Data.CurrentSpend = d.CurrentSpend
	out.Data.Reallocations = d.Reallocations
	out.Data.UncoveredOverspend = d.UncoveredOverspend
	out.Data.TargetCategory = d.TargetCategory
	out.Data.OverspentAmount = d.OverspentAmount
	return out, nil
}

// ApplyReallocation commits an accepted rebalance: budget-bloc computes the
// new per-category limits, and each affected active budget row is patched.
func (s *Service) ApplyReallocation(ctx context.Context, userID uuid.UUID, req budgetdomain.ApplyReallocationRequest) (budgetdomain.ApplyReallocationResponse, error) {
	if len(req.Reallocations) == 0 {
		return budgetdomain.ApplyReallocationResponse{}, fmt.Errorf("no reallocations provided")
	}
	month := req.Month
	if month == "" {
		month = time.Now().Format("2006-01")
	}

	mlResp, mlErr := s.ml.ApplyReallocation(ctx, userID.String(), budgetdomain.MLApplyReallocationRequest{
		BudgetName:    s.budgetName(userID),
		Month:         month,
		SessionID:     req.SessionID,
		Reallocations: req.Reallocations,
	})
	if mlErr != nil {
		return budgetdomain.ApplyReallocationResponse{}, fmt.Errorf("%w: %v", budgetprovider.ErrUnavailable, mlErr)
	}

	// Apply the new per-category limits to the active month.
	applied := map[string]bool{}
	for _, rl := range mlResp.Data.AppliedReallocations {
		for _, name := range []string{rl.FromCategory, rl.ToCategory} {
			if applied[name] {
				continue
			}
			newAmt, ok := mlResp.Data.NewBudget[name]
			if !ok {
				continue
			}
			if meta, mok := s.categoryMeta(ctx, name); mok {
				_, _ = s.repo.PatchCategoryAmount(ctx, userID, meta.ID, newAmt)
			}
			applied[name] = true
		}
	}

	if sid, err := uuid.Parse(req.SessionID); err == nil {
		_ = s.repo.SetReallocationSessionStatus(ctx, userID, sid, "applied")
		_ = s.repo.MarkReallocationsApplied(ctx, sid)
	}
	s.cache.delete("diag:" + userID.String() + ":" + month)

	var out budgetdomain.ApplyReallocationResponse
	out.Status = "success"
	out.Data.Message = mlResp.Data.Message
	out.Data.AppliedReallocations = mlResp.Data.AppliedReallocations
	out.Data.NewBudget = mlResp.Data.NewBudget
	return out, nil
}

// ─── ML feedback loop ─────────────────────────────────────────────────

func (s *Service) ListSuggestions(ctx context.Context, userID uuid.UUID) budgetdomain.MLSuggestionsResponse {
	rows, err := s.repo.ListSuggestions(ctx, userID, 20)
	out := budgetdomain.MLSuggestionsResponse{Status: "success", Suggestions: []budgetdomain.MLSuggestion{}}
	if err != nil {
		return out
	}
	for _, r := range rows {
		out.Suggestions = append(out.Suggestions, budgetdomain.MLSuggestion{
			ID:              r.ID.String(),
			SuggestionType:  r.SuggestionType,
			Details:         r.Details,
			ConfidenceScore: r.ConfidenceScore,
			Priority:        r.Priority,
			UserResponse:    r.UserResponse,
			CreatedAt:       r.CreatedAt,
		})
	}
	out.Count = len(out.Suggestions)
	return out
}

func (s *Service) ListCooldowns(ctx context.Context, userID uuid.UUID) budgetdomain.CooldownsResponse {
	rows, err := s.repo.ListActiveCooldowns(ctx, userID)
	out := budgetdomain.CooldownsResponse{Status: "success", Cooldowns: []budgetdomain.Cooldown{}}
	if err != nil {
		return out
	}
	for _, r := range rows {
		out.Cooldowns = append(out.Cooldowns, budgetdomain.Cooldown{
			ID:             r.ID.String(),
			CooldownType:   r.CooldownType,
			FromCategory:   r.FromCategory,
			ToCategory:     r.ToCategory,
			Category:       r.Category,
			CooldownUntil:  r.CooldownUntil,
			RejectionCount: r.RejectionCount,
			Reason:         r.Reason,
		})
	}
	out.Count = len(out.Cooldowns)
	return out
}

// RespondToSuggestion records accept / reject / modify. Rejections create
// per-pair cooldowns (exp backoff); accepts / modifications feed the choice
// back to budget-bloc /ml/learn and persist the returned learned params.
// Records the user's feedback on a surfaced ML suggestion.
func (s *Service) RespondToSuggestion(ctx context.Context, userID, suggestionID uuid.UUID, req budgetdomain.RespondSuggestionRequest) error {
	switch req.Action {
	case "accepted", "rejected", "modified":
	default:
		return fmt.Errorf("action must be accepted, rejected or modified")
	}

	sug, err := s.repo.GetSuggestion(ctx, userID, suggestionID)
	if err != nil {
		return err
	}

	if err := s.repo.SetSuggestionResponse(ctx, userID, suggestionID, req.Action, req.Reason); err != nil {
		return err
	}

	var details struct {
		Reallocations []budgetdomain.Reallocation `json:"reallocations"`
	}
	_ = json.Unmarshal(sug.Details, &details)

	if req.Action == "rejected" {
		for _, m := range details.Reallocations {
			_ = s.repo.UpsertPairCooldown(ctx, userID, m.FromCategory, m.ToCategory)
		}
		if sug.SessionID != nil {
			_ = s.repo.SetReallocationSessionStatus(ctx, userID, *sug.SessionID, "rejected")
		}
		return nil
	}

	// accepted / modified -> feed back to the model.
	rows, _ := s.repo.LatestBudgets(ctx, userID, 100)
	canon := s.canonFunc(ctx)
	currentBudgets := map[string]float64{}
	for _, r := range rows {
		currentBudgets[canon(r.CategoryName)] += r.BudgetedAmount
	}
	learnResp, lerr := s.ml.Learn(ctx, userID.String(), budgetdomain.MLLearnRequest{
		UserID:         userID.String(),
		CurrentBudgets: currentBudgets,
		SessionData: map[string]interface{}{
			"suggestion_type": sug.SuggestionType,
			"action":          req.Action,
			"reallocations":   details.Reallocations,
			"modified_values": req.ModifiedValues,
		},
		CurrentState: map[string]interface{}{},
	})
	if lerr == nil && learnResp != nil {
		s.persistLearnedParams(ctx, userID, learnResp.Data.UpdatedState)
	}
	return nil
}

// persistLearnedParams stores the map budget-bloc returns (from /ml/learn or
// /ml/diagnosis) into budget_learned_parameters.
func (s *Service) persistLearnedParams(ctx context.Context, userID uuid.UUID, state map[string]interface{}) {
	if len(state) == 0 {
		return
	}
	pick := func(keys ...string) json.RawMessage {
		for _, k := range keys {
			if v, ok := state[k]; ok {
				if b, err := json.Marshal(v); err == nil && string(b) != "null" {
					return b
				}
			}
		}
		return nil
	}
	lp := repository.LearnedParams{
		DonorRatios:      pick("learned_donor_ratios", "donor_ratios"),
		Relationships:    pick("learned_relationships", "relationships"),
		AdherenceMetrics: pick("learned_adherence", "adherence_metrics"),
		ScoringWeights:   pick("rl_state", "scoring_weights"),
		RidgePredictions: pick("per_user_ridge_pred", "ridge_predictions"),
	}
	if v, ok := state["ridge_last_trained_month"].(string); ok {
		lp.RidgeLastTrainedMonth = v
	}
	_ = s.repo.UpsertLearnedParams(ctx, userID, lp)
}
