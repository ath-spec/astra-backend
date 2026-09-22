// Package goals holds the Goals domain's data access. Unlike the other
// provider/ packages in this backend, there's no "real vendor" to eventually
// swap in here — goals are purely user-declared, not sourced from any
// external system — so PostgresProvider (in postgres.go) is the real
// implementation, kept behind this interface only for structural
// consistency with the rest of the codebase.
package goals

import (
	"context"

	"github.com/google/uuid"

	goalsdomain "github.com/yourusername/astra-backend/internal/domain/goals"
)

type Provider interface {
	CreateGoal(ctx context.Context, userID uuid.UUID, req goalsdomain.CreateGoalRequest) (*goalsdomain.Goal, error)
	ListGoals(ctx context.Context, userID uuid.UUID) ([]goalsdomain.Goal, error)
	GetGoal(ctx context.Context, userID uuid.UUID, goalID string) (*goalsdomain.Goal, error)
	Summary(ctx context.Context, userID uuid.UUID) (goalsdomain.SummaryResult, error)
}
