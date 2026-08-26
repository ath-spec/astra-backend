package goals

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourusername/astra-backend/internal/apiresponse"
	"github.com/yourusername/astra-backend/internal/apitime"
	goalsdomain "github.com/yourusername/astra-backend/internal/domain/goals"
)

type PostgresProvider struct {
	pool *pgxpool.Pool
}

func NewPostgresProvider(pool *pgxpool.Pool) *PostgresProvider {
	return &PostgresProvider{pool: pool}
}

func validateCreateRequest(req goalsdomain.CreateGoalRequest) (*time.Time, error) {
	if req.Name == "" {
		return nil, apiresponse.Validation("name is required")
	}
	if req.TargetAmount <= 0 {
		return nil, apiresponse.Validation("target_amount must be positive")
	}
	if req.CurrentAmount < 0 {
		return nil, apiresponse.Validation("current_amount cannot be negative")
	}
	return apitime.FromEpochPtr(req.Deadline), nil
}

func (p *PostgresProvider) CreateGoal(ctx context.Context, userID uuid.UUID, req goalsdomain.CreateGoalRequest) (*goalsdomain.Goal, error) {
	deadline, err := validateCreateRequest(req)
	if err != nil {
		return nil, err
	}

	row := p.pool.QueryRow(ctx, `
		INSERT INTO goals (user_id, name, target_amount, current_amount, deadline, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, name, target_amount, current_amount, deadline, status, created_at
	`, userID, req.Name, req.TargetAmount, req.CurrentAmount, deadline, goalsdomain.StatusActive)

	return scanGoal(row)
}

func (p *PostgresProvider) ListGoals(ctx context.Context, userID uuid.UUID) ([]goalsdomain.Goal, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT id, name, target_amount, current_amount, deadline, status, created_at
		FROM goals WHERE user_id = $1 ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list goals: %w", err)
	}
	defer rows.Close()

	goalsList := make([]goalsdomain.Goal, 0)
	for rows.Next() {
		g, err := scanGoal(rows)
		if err != nil {
			return nil, err
		}
		goalsList = append(goalsList, *g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate goals: %w", err)
	}
	return goalsList, nil
}

func (p *PostgresProvider) GetGoal(ctx context.Context, userID uuid.UUID, goalID string) (*goalsdomain.Goal, error) {
	id, err := uuid.Parse(goalID)
	if err != nil {
		return nil, apiresponse.NotFound("goal %s not found", goalID)
	}
	row := p.pool.QueryRow(ctx, `
		SELECT id, name, target_amount, current_amount, deadline, status, created_at
		FROM goals WHERE id = $1 AND user_id = $2
	`, id, userID)

	goal, err := scanGoal(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apiresponse.NotFound("goal %s not found", goalID)
		}
		return nil, err
	}
	return goal, nil
}

func (p *PostgresProvider) Summary(ctx context.Context, userID uuid.UUID) (goalsdomain.SummaryResult, error) {
	var res goalsdomain.SummaryResult
	err := p.pool.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(target_amount), 0),
			COALESCE(SUM(current_amount), 0),
			COUNT(*) FILTER (WHERE status = 'ACTIVE'),
			COUNT(*) FILTER (WHERE status = 'COMPLETED'),
			COUNT(*) FILTER (WHERE status = 'INACTIVE')
		FROM goals WHERE user_id = $1
	`, userID).Scan(&res.TotalTargetAmount, &res.TotalCurrentAmount, &res.ActiveCount, &res.CompletedCount, &res.InactiveCount)
	if err != nil {
		return goalsdomain.SummaryResult{}, fmt.Errorf("goals summary: %w", err)
	}
	return res, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanGoal(row scanner) (*goalsdomain.Goal, error) {
	var g goalsdomain.Goal
	var deadline *time.Time
	var createdAt time.Time
	if err := row.Scan(&g.ID, &g.Name, &g.TargetAmount, &g.CurrentAmount, &deadline, &g.Status, &createdAt); err != nil {
		return nil, fmt.Errorf("scan goal: %w", err)
	}
	g.CreatedAt = apitime.New(createdAt)
	g.ProgressPct = progressPct(g.CurrentAmount, g.TargetAmount)
	g.Deadline = apitime.NewPtr(deadline)
	if deadline != nil {
		days := int(time.Until(*deadline).Hours() / 24)
		g.DaysLeft = &days
	}
	return &g, nil
}

func progressPct(current, target float64) float64 {
	if target <= 0 {
		return 0
	}
	pct := current / target * 100
	return float64(int64(pct*100+0.5)) / 100
}
