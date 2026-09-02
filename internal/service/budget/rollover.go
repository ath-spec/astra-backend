package budget

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	budgetdomain "github.com/yourusername/astra-backend/internal/domain/budget"
)

const rolloverDraftTTL = 7 * 24 * time.Hour

// RolloverForUser pre-computes next month's budget for one user and saves it
// as a `rollover_draft` setup session the user can review + finalize.
// Returns the draft session id (empty
// if one already existed or the user has no active budget).
func (s *Service) RolloverForUser(ctx context.Context, userID uuid.UUID) (string, error) {
	rows, err := s.repo.LatestBudgets(ctx, userID, 100)
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", nil // nothing to roll over
	}

	next := time.Date(rows[0].Year, time.Month(rows[0].Month), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0)
	nextMonth := next.Format("2006-01")

	// Don't stack drafts / clash with an in-progress wizard for that month.
	if exists, _ := s.repo.SessionExistsForMonth(ctx, userID, nextMonth,
		[]string{"pending", "rollover_draft", "finalized"}); exists {
		return "", nil
	}

	// Draft from this month's ML diagnosis for next month.
	diag, err := s.Diagnosis(ctx, userID, budgetdomain.DiagnosisRequest{Month: nextMonth})
	if err != nil {
		return "", err
	}

	total := diag.SuggestedTotalBudget
	allocs := make([]budgetdomain.CategoryAllocation, 0, len(diag.SuggestedCategories))
	var allocSum float64
	for _, c := range diag.SuggestedCategories {
		allocs = append(allocs, budgetdomain.CategoryAllocation{
			CategoryID:    c.CategoryID,
			CategoryName:  c.CategoryName,
			CategoryColor: c.CategoryColor,
			Amount:        c.SuggestedAmount,
			IsTracking:    true,
		})
		allocSum += c.SuggestedAmount
	}
	if total <= 0 {
		total = allocSum
	}
	if len(allocs) == 0 {
		// Fall back to carrying this month's category budgets forward.
		for _, r := range rows {
			allocs = append(allocs, budgetdomain.CategoryAllocation{
				CategoryID:   r.CategoryName,
				CategoryName: r.CategoryName,
				Amount:       r.BudgetedAmount,
				IsTracking:   true,
			})
			allocSum += r.BudgetedAmount
		}
		if total <= 0 {
			total = allocSum
		}
	}

	draft, err := s.repo.CreateDraftSession(ctx, userID, nextMonth, "rollover_draft",
		total, allocs, time.Now().Add(rolloverDraftTTL))
	if err != nil {
		return "", err
	}
	return draft.ID.String(), nil
}

// RunRollover drafts next month's budget for every user that currently has an
// active budget. Best-effort per user; a single failure does not abort the run.
func (s *Service) RunRollover(ctx context.Context) (created int, failed int) {
	users, err := s.repo.UsersWithActiveBudgets(ctx)
	if err != nil {
		slog.Error("budget rollover: list users failed", "error", err)
		return 0, 0
	}
	for _, uid := range users {
		id, err := s.RolloverForUser(ctx, uid)
		switch {
		case err != nil:
			failed++
			slog.Warn("budget rollover: user failed", "user", uid, "error", err)
		case id != "":
			created++
		}
	}
	slog.Info("budget rollover: run complete", "users", len(users), "drafts_created", created, "failed", failed)
	return created, failed
}
