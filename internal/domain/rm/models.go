// Package rm defines the wire types for the RM/Admin console: staff
// identities, the round-robin auto-assignment of users to Relationship
// Managers, and the read-only 360° client views the console renders. These
// types are entirely separate from the user-facing app's domains — the RM
// console only ever *reads* user data, through the existing domain
// providers, never through the user HTTP layer.
package rm

import (
	"github.com/google/uuid"

	"github.com/yourusername/astra-backend/internal/apitime"
	dashboarddomain "github.com/yourusername/astra-backend/internal/domain/dashboard"
	fddomain "github.com/yourusername/astra-backend/internal/domain/fd"
	goalsdomain "github.com/yourusername/astra-backend/internal/domain/goals"
	mfdomain "github.com/yourusername/astra-backend/internal/domain/mf"
	paDomain "github.com/yourusername/astra-backend/internal/domain/portfolioanalysis"
	stocksdomain "github.com/yourusername/astra-backend/internal/domain/stocks"
)

const (
	RoleRM    = "rm"
	RoleAdmin = "admin"

	StatusActive   = "active"
	StatusInactive = "inactive"

	ActionAutoAssign = "auto_assign"
	ActionAssign     = "assign"
	ActionTransfer   = "transfer"
	ActionRemove     = "remove"
)

// StaffUser is the internal representation of a row in rm_users, including
// the password hash. It never crosses the wire — handlers return Public.
type StaffUser struct {
	ID            uuid.UUID
	Email         string
	PasswordHash  string
	Name          string
	PhoneNumber   *string
	Role          string
	Status        string
	MaxPortfolios int
	CreatedAt     apitime.Time
	UpdatedAt     apitime.Time
}

// Public is the client-safe projection of a staff user.
type Public struct {
	ID            uuid.UUID    `json:"id"`
	Email         string       `json:"email"`
	Name          string       `json:"name"`
	PhoneNumber   *string      `json:"phone_number,omitempty"`
	Role          string       `json:"role"`
	Status        string       `json:"status"`
	MaxPortfolios int          `json:"max_portfolios"`
	CreatedAt     apitime.Time `json:"created_at"`
}

func (s StaffUser) Public() Public {
	return Public{
		ID:            s.ID,
		Email:         s.Email,
		Name:          s.Name,
		PhoneNumber:   s.PhoneNumber,
		Role:          s.Role,
		Status:        s.Status,
		MaxPortfolios: s.MaxPortfolios,
		CreatedAt:     s.CreatedAt,
	}
}

// --- Auth DTOs ---

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	Role         string `json:"role"`
	RM           Public `json:"rm"`
}

// --- Client list / detail DTOs ---

type AssetMix struct {
	MutualFunds   float64 `json:"mf"`
	Stocks        float64 `json:"stocks"`
	FixedDeposits float64 `json:"fd"`
	Bank          float64 `json:"bank"`
}

type ClientListItem struct {
	UserID             uuid.UUID     `json:"user_id"`
	Name               string        `json:"name"`
	Phone              string        `json:"phone"`
	PAN                *string       `json:"pan,omitempty"`
	JoinedAt           apitime.Time  `json:"joined_at"`
	AssignedAt         *apitime.Time `json:"assigned_at,omitempty"`
	AssignedRMID       *uuid.UUID    `json:"assigned_rm_id,omitempty"`
	AssignedRMName     *string       `json:"assigned_rm_name,omitempty"`
	TotalWealth        float64       `json:"total_wealth"`
	OneDayChangeAmount float64       `json:"one_day_change_amount"`
	OneDayChangePct    float64       `json:"one_day_change_pct"`
	AssetMix           AssetMix      `json:"asset_mix"`
	GoalsCount         int           `json:"goals_count"`
}

type ClientList struct {
	Items []ClientListItem `json:"items"`
	Total int              `json:"total"`
}

type ClientProfile struct {
	UserID     uuid.UUID     `json:"user_id"`
	Name       string        `json:"name"`
	Phone      string        `json:"phone"`
	PAN        *string       `json:"pan,omitempty"`
	JoinedAt   apitime.Time  `json:"joined_at"`
	AssignedAt *apitime.Time `json:"assigned_at,omitempty"`
	RMID       *uuid.UUID    `json:"rm_id,omitempty"`
	RMName     *string       `json:"rm_name,omitempty"`
}

type BankAccount struct {
	BankName    string  `json:"bank_name"`
	AccountType string  `json:"account_type"`
	Balance     float64 `json:"balance"`
}

type Holdings struct {
	Stocks []stocksdomain.Holding `json:"stocks"`
	MF     []mfdomain.Folio       `json:"mf"`
	FD     []fddomain.Account     `json:"fd"`
}

type SpendSummary struct {
	Last30DaysTotal float64 `json:"last_30_days_total"`
	TxnCount        int     `json:"txn_count"`
}

type ClientDetail struct {
	Profile      ClientProfile                   `json:"profile"`
	Summary      *dashboarddomain.Summary        `json:"summary"`
	DNA          *paDomain.AllocationResult      `json:"dna"`
	BankAccounts []BankAccount                   `json:"bank_accounts"`
	Holdings     Holdings                        `json:"holdings"`
	Goals        []goalsdomain.Goal              `json:"goals"`
	SpendSummary SpendSummary                    `json:"spend_summary"`
	Growth       []dashboarddomain.SnapshotPoint `json:"growth"`
}

// --- Portfolio history (allocation & DNA drift over time) ---

// AllocationHistoryPoint is one day's asset-class split, derived from the
// portfolio_snapshots table.
type AllocationHistoryPoint struct {
	Date        apitime.Time `json:"date"`
	TotalWealth float64      `json:"total_wealth"`
	MFValue     float64      `json:"mf_value"`
	StocksValue float64      `json:"stocks_value"`
	FDValue     float64      `json:"fd_value"`
	BankValue   float64      `json:"bank_value"`
	MFPct       float64      `json:"mf_pct"`
	StocksPct   float64      `json:"stocks_pct"`
	FDPct       float64      `json:"fd_pct"`
	BankPct     float64      `json:"bank_pct"`
}

type PortfolioHistory struct {
	AllocationSeries []AllocationHistoryPoint   `json:"allocation_series"`
	DNASeries        []paDomain.DNAHistoryPoint `json:"dna_series"`
}

// --- RM book summary ---

type BookAlert struct {
	UserID uuid.UUID `json:"user_id"`
	Name   string    `json:"name"`
	Type   string    `json:"type"` // goal_off_track | portfolio_down
	Detail string    `json:"detail"`
}

type BookSummary struct {
	ClientCount       int         `json:"client_count"`
	TotalAUM          float64     `json:"total_aum"`
	AvgPortfolioValue float64     `json:"avg_portfolio_value"`
	Capacity          int         `json:"capacity"`
	Utilisation       float64     `json:"utilisation"` // 0..1
	Alerts            []BookAlert `json:"alerts"`
}

// --- Admin DTOs ---

type RosterItem struct {
	Public
	ClientCount int     `json:"client_count"`
	TotalAUM    float64 `json:"total_aum"`
	Utilisation float64 `json:"utilisation"`
}

type CreateRMRequest struct {
	Name          string `json:"name"`
	Email         string `json:"email"`
	Phone         string `json:"phone,omitempty"`
	Password      string `json:"password"`
	Role          string `json:"role,omitempty"` // defaults to "rm"
	MaxPortfolios int    `json:"max_portfolios,omitempty"`
}

type UpdateRMRequest struct {
	Name          *string `json:"name,omitempty"`
	Phone         *string `json:"phone,omitempty"`
	Status        *string `json:"status,omitempty"`
	MaxPortfolios *int    `json:"max_portfolios,omitempty"`
}

type AssignRequest struct {
	UserID uuid.UUID `json:"user_id"`
	RMID   uuid.UUID `json:"rm_id"`
}

type TransferRequest struct {
	UserID uuid.UUID `json:"user_id"`
	ToRMID uuid.UUID `json:"to_rm_id"`
	Reason string    `json:"reason,omitempty"`
}

type RemoveRequest struct {
	UserID uuid.UUID `json:"user_id"`
	Reason string    `json:"reason,omitempty"`
}

type OffboardRequest struct {
	Reason string `json:"reason,omitempty"`
}

type AdminOverview struct {
	TotalClients    int     `json:"total_clients"`
	TotalAUM        float64 `json:"total_aum"`
	UnassignedCount int     `json:"unassigned_count"`
	RMCount         int     `json:"rm_count"`
	ActiveRMCount   int     `json:"active_rm_count"`
	RMsAtCapacity   int     `json:"rms_at_capacity"`
}

type RMDetail struct {
	RM      RosterItem       `json:"rm"`
	Clients []ClientListItem `json:"clients"`
}

type AssignmentHistoryItem struct {
	ID          uuid.UUID    `json:"id"`
	UserID      uuid.UUID    `json:"user_id"`
	UserName    string       `json:"user_name"`
	FromRMID    *uuid.UUID   `json:"from_rm_id,omitempty"`
	FromRMName  *string      `json:"from_rm_name,omitempty"`
	ToRMID      *uuid.UUID   `json:"to_rm_id,omitempty"`
	ToRMName    *string      `json:"to_rm_name,omitempty"`
	Action      string       `json:"action"`
	Reason      *string      `json:"reason,omitempty"`
	ActorRMID   *uuid.UUID   `json:"actor_rm_id,omitempty"`
	ActorRMName *string      `json:"actor_rm_name,omitempty"`
	CreatedAt   apitime.Time `json:"created_at"`
}

type AssignmentHistory struct {
	Items []AssignmentHistoryItem `json:"items"`
	Total int                     `json:"total"`
}

// ListFilters is the shared query for client listing endpoints.
type ListFilters struct {
	Search string
	Sort   string // wealth | name | joined
	Order  string // asc | desc
	Limit  int
	Offset int
}
