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

// StaffUser is the internal representation of a row in rm_users. It never
// crosses the wire — handlers return Public.
type StaffUser struct {
	ID            uuid.UUID
	EmployeeCode  string
	Email         string
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
	EmployeeCode  string       `json:"employee_code"`
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
		EmployeeCode:  s.EmployeeCode,
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

// OTPSendRequest starts a login: identifier is the staff member's employee
// code or their email.
type OTPSendRequest struct {
	Identifier string `json:"identifier"`
}

// OTPVerifyRequest completes a login with the code delivered to the staff
// member's registered phone.
type OTPVerifyRequest struct {
	Identifier string `json:"identifier"`
	OTP        string `json:"otp"`
}

// OTPSendResponse tells the client where the code went (masked) so it can
// render "code sent to •••••1234".
type OTPSendResponse struct {
	Sent        bool   `json:"sent"`
	MaskedPhone string `json:"masked_phone,omitempty"`
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

// ClientPortfolioAnalysis is the full portfolio-analysis payload for the RM
// console: the same Allocation / Discipline / Performance analysis the client
// sees in-app, exposed read-only to the client's RM. Each section may be nil
// if that engine returned an error or the client has no relevant holdings.
type ClientPortfolioAnalysis struct {
	Allocation  *paDomain.AllocationResult  `json:"allocation"`
	Discipline  *paDomain.DisciplineResult  `json:"discipline"`
	Performance *paDomain.PerformanceResult `json:"performance"`
}

// --- Advisory: actionable, calculated insights for the RM ---

// AdvisoryAction is one concrete thing the RM could do for a client, ranked by
// Priority (1 = most urgent). All values are computed from live data.
type AdvisoryAction struct {
	Kind     string  `json:"kind"`   // deploy_cash | goal_gap | sip_lapsed | fd_maturing | retention_call | drawdown_call
	Priority int     `json:"priority"`
	Title    string  `json:"title"`
	Detail   string  `json:"detail"`
	Amount   float64 `json:"amount,omitempty"`
}

// ClientActionItem attaches an action to the client it concerns — used for the
// book-wide "who to call" queue.
type ClientActionItem struct {
	UserID uuid.UUID      `json:"user_id"`
	Name   string         `json:"name"`
	Phone  string         `json:"phone"`
	Action AdvisoryAction `json:"action"`
}

// IdleCashResult sizes the cash a client is holding beyond a sensible
// emergency buffer (~6 months of their own spend) — i.e. deployable.
type IdleCashResult struct {
	BankTotal         float64 `json:"bank_total"`
	AvgMonthlySpend   float64 `json:"avg_monthly_spend"`
	EmergencyBuffer   float64 `json:"emergency_buffer"`
	IdleAmount        float64 `json:"idle_amount"`
	MonthsOfSpendHeld float64 `json:"months_of_spend_held"`
}

// HoldingXIRR is the money-weighted return of a single holding.
type HoldingXIRR struct {
	Name     string  `json:"name"`
	Type     string  `json:"type"` // MF | STOCK
	XIRRPct  float64 `json:"xirr_pct"`
	Computed bool    `json:"computed"`
}

// XIRRResult is the client's true cash-flow-weighted return, overall and per
// holding, from dated transaction history + current value.
type XIRRResult struct {
	OverallXIRRPct float64       `json:"overall_xirr_pct"`
	Computed       bool          `json:"computed"`
	Holdings       []HoldingXIRR `json:"holdings"`
}

// GoalProjection forward-projects a goal at the client's current savings
// run-rate and flags the funding gap.
type GoalProjection struct {
	GoalID                    string        `json:"goal_id"`
	Name                      string        `json:"name"`
	TargetAmount              float64       `json:"target_amount"`
	CurrentAmount             float64       `json:"current_amount"`
	TargetDate                *apitime.Time `json:"target_date,omitempty"`
	MonthsLeft                int           `json:"months_left"`
	AssumedReturnPct          float64       `json:"assumed_return_pct"`
	EstimatedMonthlyToGoal    float64       `json:"estimated_monthly_to_goal"`
	ProjectedAmount           float64       `json:"projected_amount"`
	ProjectedShortfall        float64       `json:"projected_shortfall"`
	RequiredMonthly           float64       `json:"required_monthly"`
	AdditionalMonthlyRequired float64       `json:"additional_monthly_required"`
	OnTrack                   bool          `json:"on_track"`
}

// MaturingFD is an FD coming due within the look-ahead window.
type MaturingFD struct {
	FDAccountNumber string       `json:"fd_account_number"`
	BankName        string       `json:"bank_name,omitempty"`
	PrincipalAmount float64      `json:"principal_amount"`
	MaturityAmount  float64      `json:"maturity_amount"`
	MaturityDate    apitime.Time `json:"maturity_date"`
	DaysToMaturity  int          `json:"days_to_maturity"`
	InterestRate    float64      `json:"interest_rate"`
}

// ClientMaturingFD attaches a maturing FD to its owner for the book view.
type ClientMaturingFD struct {
	UserID uuid.UUID  `json:"user_id"`
	Name   string     `json:"name"`
	FD     MaturingFD `json:"fd"`
}

// ClientAdvisory is the per-client advisory bundle for Client 360.
type ClientAdvisory struct {
	NextBestAction  *AdvisoryAction  `json:"next_best_action"`
	Actions         []AdvisoryAction `json:"actions"`
	XIRR            *XIRRResult      `json:"xirr"`
	IdleCash        *IdleCashResult  `json:"idle_cash"`
	GoalProjections []GoalProjection `json:"goal_projections"`
	MaturingFDs     []MaturingFD     `json:"maturing_fds"`
}

// BookIdleCash aggregates deployable cash across the RM's whole book.
type BookIdleCash struct {
	TotalIdle   float64 `json:"total_idle"`
	ClientCount int     `json:"client_count"`
}

// BookInsights is the book-wide intelligence panel on the RM dashboard.
type BookInsights struct {
	NextBestActions []ClientActionItem `json:"next_best_actions"`
	IdleCash        BookIdleCash       `json:"idle_cash"`
	MaturingFDs     []ClientMaturingFD `json:"maturing_fds"`
	RetentionAlerts []ClientActionItem `json:"retention_alerts"`
}

// --- Client interaction log (call notes / follow-ups) ---

// Interaction is one entry in a client's shared, server-persisted RM log.
// EventType is "interaction" for RM-entered rows or "assignment" for rows
// synthesised from rm_assignment_history so the timeline reads as one story.
type Interaction struct {
	ID         uuid.UUID     `json:"id"`
	EventType  string        `json:"event_type"` // interaction | assignment
	Kind       string        `json:"kind"`       // note | call | meeting | email | task | auto_assign | assign | transfer | remove
	Body       string        `json:"body"`
	RMName     string        `json:"rm_name,omitempty"`
	FollowUpAt *apitime.Time `json:"follow_up_at,omitempty"`
	DoneAt     *apitime.Time `json:"done_at,omitempty"`
	CreatedAt  apitime.Time  `json:"created_at"`
}

// AddInteractionRequest is the POST body for logging a note/call/task.
type AddInteractionRequest struct {
	Kind       string  `json:"kind"`
	Body       string  `json:"body"`
	FollowUpAt *int64  `json:"follow_up_at,omitempty"` // epoch seconds
}

// PendingFollowUp is an open follow-up task surfaced on the RM dashboard.
type PendingFollowUp struct {
	ID         uuid.UUID    `json:"id"`
	UserID     uuid.UUID    `json:"user_id"`
	ClientName string       `json:"client_name"`
	Kind       string       `json:"kind"`
	Body       string       `json:"body"`
	FollowUpAt apitime.Time `json:"follow_up_at"`
	Overdue    bool         `json:"overdue"`
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
	EmployeeCode  string `json:"employee_code"`
	Email         string `json:"email"`
	Phone         string `json:"phone"`          // required — OTP is delivered here
	Role          string `json:"role,omitempty"` // defaults to "rm"
	MaxPortfolios int    `json:"max_portfolios,omitempty"`
}

type UpdateRMRequest struct {
	Name          *string `json:"name,omitempty"`
	Email         *string `json:"email,omitempty"`
	Phone         *string `json:"phone,omitempty"`
	Status        *string `json:"status,omitempty"`
	MaxPortfolios *int    `json:"max_portfolios,omitempty"`
}

// UpdateProfileRequest is the self-service PATCH /api/rm/auth/me payload —
// a staff member editing their own name / login email / OTP phone.
type UpdateProfileRequest struct {
	Name  *string `json:"name,omitempty"`
	Email *string `json:"email,omitempty"`
	Phone *string `json:"phone,omitempty"`
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
