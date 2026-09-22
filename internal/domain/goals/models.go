// Package goals defines the wire types for the Goals domain: user-created
// savings/investment targets with progress tracking.
package goals

import "github.com/yourusername/astra-backend/internal/apitime"

type CreateGoalRequest struct {
	Name          string  `json:"name"`
	TargetAmount  float64 `json:"target_amount"`
	CurrentAmount float64 `json:"current_amount,omitempty"`
	Deadline      *int64  `json:"deadline,omitempty"` // epoch seconds
}

// Goal's ProgressPct and DaysLeft are computed on every read from
// CurrentAmount/TargetAmount/Deadline — never persisted — so they can't go
// stale.
type Goal struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	TargetAmount  float64       `json:"target_amount"`
	CurrentAmount float64       `json:"current_amount"`
	Deadline      *apitime.Time `json:"deadline,omitempty"`
	Status        string        `json:"status"`
	ProgressPct   float64       `json:"progress_pct"`
	DaysLeft      *int          `json:"days_left,omitempty"`
	CreatedAt     apitime.Time  `json:"created_at"`
}

type SummaryResult struct {
	TotalTargetAmount  float64 `json:"total_target_amount"`
	TotalCurrentAmount float64 `json:"total_current_amount"`
	ActiveCount        int     `json:"active_count"`
	CompletedCount     int     `json:"completed_count"`
	InactiveCount      int     `json:"inactive_count"`
}

const (
	StatusActive    = "ACTIVE"
	StatusCompleted = "COMPLETED"
	StatusInactive  = "INACTIVE"
)
