package service

import (
	"context"

	"github.com/google/uuid"

	goalsdomain "github.com/yourusername/astra-backend/internal/domain/goals"
	goalsprovider "github.com/yourusername/astra-backend/internal/provider/goals"
)

type GoalsService struct {
	provider goalsprovider.Provider
}

func NewGoalsService(provider goalsprovider.Provider) *GoalsService {
	return &GoalsService{provider: provider}
}

func (s *GoalsService) CreateGoal(ctx context.Context, userID uuid.UUID, req goalsdomain.CreateGoalRequest) (*goalsdomain.Goal, error) {
	return s.provider.CreateGoal(ctx, userID, req)
}

func (s *GoalsService) ListGoals(ctx context.Context, userID uuid.UUID) ([]goalsdomain.Goal, error) {
	return s.provider.ListGoals(ctx, userID)
}

func (s *GoalsService) GetGoal(ctx context.Context, userID uuid.UUID, goalID string) (*goalsdomain.Goal, error) {
	return s.provider.GetGoal(ctx, userID, goalID)
}

func (s *GoalsService) Summary(ctx context.Context, userID uuid.UUID) (goalsdomain.SummaryResult, error) {
	return s.provider.Summary(ctx, userID)
}
