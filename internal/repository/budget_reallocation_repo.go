package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	budgetdomain "github.com/yourusername/astra-backend/internal/domain/budget"
)

var ErrBudgetSuggestionNotFound = errors.New("budget suggestion not found")

// ─── Learned parameters (per user, one row) ────────────────────────────

type LearnedParams struct {
	DonorRatios           json.RawMessage
	Relationships         json.RawMessage
	AdherenceMetrics      json.RawMessage
	ScoringWeights        json.RawMessage
	RidgePredictions      json.RawMessage
	RidgeLastTrainedMonth string
	Version               int
	Found                 bool
}

func (r *BudgetRepository) GetLearnedParams(ctx context.Context, userID uuid.UUID) (LearnedParams, error) {
	var p LearnedParams
	err := r.pool.QueryRow(ctx, `
		SELECT donor_ratios, relationships, adherence_metrics, scoring_weights,
		       ridge_predictions, ridge_last_trained_month, version
		FROM budget_learned_parameters WHERE user_id = $1
	`, userID).Scan(&p.DonorRatios, &p.Relationships, &p.AdherenceMetrics, &p.ScoringWeights,
		&p.RidgePredictions, &p.RidgeLastTrainedMonth, &p.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return LearnedParams{Found: false}, nil
	}
	if err != nil {
		return LearnedParams{}, fmt.Errorf("get learned params: %w", err)
	}
	p.Found = true
	return p, nil
}

func (r *BudgetRepository) UpsertLearnedParams(ctx context.Context, userID uuid.UUID, p LearnedParams) error {
	j := func(b json.RawMessage) []byte {
		if len(b) == 0 {
			return []byte("{}")
		}
		return b
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO budget_learned_parameters
			(user_id, donor_ratios, relationships, adherence_metrics, scoring_weights,
			 ridge_predictions, ridge_last_trained_month)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (user_id) DO UPDATE SET
			donor_ratios             = EXCLUDED.donor_ratios,
			relationships            = EXCLUDED.relationships,
			adherence_metrics        = EXCLUDED.adherence_metrics,
			scoring_weights          = EXCLUDED.scoring_weights,
			ridge_predictions        = EXCLUDED.ridge_predictions,
			ridge_last_trained_month = EXCLUDED.ridge_last_trained_month,
			version                  = budget_learned_parameters.version + 1,
			updated_at               = now()
	`, userID, j(p.DonorRatios), j(p.Relationships), j(p.AdherenceMetrics), j(p.ScoringWeights),
		j(p.RidgePredictions), p.RidgeLastTrainedMonth)
	return err
}

// ─── Reallocation sessions + moves ────────────────────────────────────

type ReallocationSessionRow struct {
	ID             uuid.UUID
	TargetCategory string
	Status         string
}

func (r *BudgetRepository) CreateReallocationSession(ctx context.Context, userID uuid.UUID,
	target string, minRatio float64, initial, updated, spend map[string]float64,
	uncovered, overspent float64, moves []budgetdomain.Reallocation) (uuid.UUID, error) {

	mj := func(m map[string]float64) []byte { b, _ := json.Marshal(m); return b }

	var id uuid.UUID
	err := pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			INSERT INTO budget_reallocation_sessions
				(user_id, target_category, min_remaining_ratio, initial_budget, updated_budget,
				 current_spend, uncovered_overspend, overspent_amount, status)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'pending')
			RETURNING id
		`, userID, target, minRatio, mj(initial), mj(updated), mj(spend), uncovered, overspent).Scan(&id); err != nil {
			return err
		}
		for _, m := range moves {
			if _, err := tx.Exec(ctx, `
				INSERT INTO budget_reallocations (session_id, user_id, from_category, to_category, amount, reason)
				VALUES ($1, $2, $3, $4, $5, $6)
			`, id, userID, m.FromCategory, m.ToCategory, m.Amount, m.Reason); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("create reallocation session: %w", err)
	}
	return id, nil
}

func (r *BudgetRepository) SetReallocationSessionStatus(ctx context.Context, userID, sessionID uuid.UUID, status string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE budget_reallocation_sessions SET status = $3, updated_at = now()
		WHERE id = $1 AND user_id = $2
	`, sessionID, userID, status)
	return err
}

func (r *BudgetRepository) MarkReallocationsApplied(ctx context.Context, sessionID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE budget_reallocations SET status = 'applied', applied_at = now()
		WHERE session_id = $1 AND status = 'pending'
	`, sessionID)
	return err
}

// ─── ML suggestions ──────────────────────────────────────────────────

func (r *BudgetRepository) CreateSuggestion(ctx context.Context, userID uuid.UUID, sessionID *uuid.UUID,
	sType string, details []byte, confidence float64, priority string) (uuid.UUID, error) {
	var id uuid.UUID
	var pr *string
	if priority != "" {
		pr = &priority
	}
	err := r.pool.QueryRow(ctx, `
		INSERT INTO budget_ml_suggestions (user_id, session_id, suggestion_type, suggestion_details, confidence_score, priority)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, userID, sessionID, sType, details, confidence, pr).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("create suggestion: %w", err)
	}
	return id, nil
}

type SuggestionRow struct {
	ID              uuid.UUID
	SessionID       *uuid.UUID
	SuggestionType  string
	Details         json.RawMessage
	ConfidenceScore float64
	Priority        string
	UserResponse    string
	CreatedAt       time.Time
}

func (r *BudgetRepository) ListSuggestions(ctx context.Context, userID uuid.UUID, limit int) ([]SuggestionRow, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, session_id, suggestion_type, suggestion_details,
		       COALESCE(confidence_score, 0), COALESCE(priority, ''), user_response, created_at
		FROM budget_ml_suggestions
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("list suggestions: %w", err)
	}
	defer rows.Close()

	var out []SuggestionRow
	for rows.Next() {
		var s SuggestionRow
		if err := rows.Scan(&s.ID, &s.SessionID, &s.SuggestionType, &s.Details,
			&s.ConfidenceScore, &s.Priority, &s.UserResponse, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan suggestion: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *BudgetRepository) GetSuggestion(ctx context.Context, userID, id uuid.UUID) (SuggestionRow, error) {
	var s SuggestionRow
	err := r.pool.QueryRow(ctx, `
		SELECT id, session_id, suggestion_type, suggestion_details,
		       COALESCE(confidence_score, 0), COALESCE(priority, ''), user_response, created_at
		FROM budget_ml_suggestions WHERE id = $1 AND user_id = $2
	`, id, userID).Scan(&s.ID, &s.SessionID, &s.SuggestionType, &s.Details,
		&s.ConfidenceScore, &s.Priority, &s.UserResponse, &s.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return SuggestionRow{}, ErrBudgetSuggestionNotFound
	}
	if err != nil {
		return SuggestionRow{}, fmt.Errorf("get suggestion: %w", err)
	}
	return s, nil
}

func (r *BudgetRepository) SetSuggestionResponse(ctx context.Context, userID, id uuid.UUID, response, reason string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE budget_ml_suggestions
		SET user_response = $3, user_response_reason = NULLIF($4,''), user_response_at = now(), updated_at = now()
		WHERE id = $1 AND user_id = $2
	`, id, userID, response, reason)
	return err
}

// ─── Cooldowns ──────────────────────────────────────────────────────

// UpsertPairCooldown creates or extends a category-pair cooldown with
// exponential backoff (1 -> 3 -> 7 -> 14 days).
func (r *BudgetRepository) UpsertPairCooldown(ctx context.Context, userID uuid.UUID, from, to string) error {
	var prevCount int
	_ = r.pool.QueryRow(ctx, `
		SELECT rejection_count FROM budget_ml_cooldowns
		WHERE user_id = $1 AND cooldown_type = 'category_pair' AND from_category = $2 AND to_category = $3
	`, userID, from, to).Scan(&prevCount)

	days := []int{1, 3, 7, 14}
	idx := prevCount
	if idx >= len(days) {
		idx = len(days) - 1
	}
	until := time.Now().Add(time.Duration(days[idx]) * 24 * time.Hour)

	_, err := r.pool.Exec(ctx, `
		INSERT INTO budget_ml_cooldowns
			(user_id, cooldown_type, from_category, to_category, cooldown_until, reason, rejection_count, is_active)
		VALUES ($1, 'category_pair', $2, $3, $4, 'reallocation rejected', $5, true)
		ON CONFLICT (user_id, from_category, to_category) WHERE cooldown_type = 'category_pair'
		DO UPDATE SET cooldown_until = EXCLUDED.cooldown_until,
		              rejection_count = budget_ml_cooldowns.rejection_count + 1,
		              is_active = true, updated_at = now()
	`, userID, from, to, until, prevCount+1)
	return err
}

type CooldownRow struct {
	ID             uuid.UUID
	CooldownType   string
	FromCategory   string
	ToCategory     string
	Category       string
	CooldownUntil  time.Time
	RejectionCount int
	Reason         string
}

func (r *BudgetRepository) ListActiveCooldowns(ctx context.Context, userID uuid.UUID) ([]CooldownRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, cooldown_type, COALESCE(from_category,''), COALESCE(to_category,''),
		       COALESCE(category,''), cooldown_until, rejection_count, COALESCE(reason,'')
		FROM budget_ml_cooldowns
		WHERE user_id = $1 AND is_active = true AND cooldown_until > now()
		ORDER BY cooldown_until DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list cooldowns: %w", err)
	}
	defer rows.Close()

	var out []CooldownRow
	for rows.Next() {
		var c CooldownRow
		if err := rows.Scan(&c.ID, &c.CooldownType, &c.FromCategory, &c.ToCategory,
			&c.Category, &c.CooldownUntil, &c.RejectionCount, &c.Reason); err != nil {
			return nil, fmt.Errorf("scan cooldown: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
