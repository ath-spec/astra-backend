// Package idbiloans is IDBI integration feature 3: mirror a customer's loan
// accounts (391 + 402) into idbi_loans and serve "My Loans"; foreclosure
// quotes (538) are computed on demand. Gated by IDBI_LOANS_ENABLED.
package idbiloans

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/yourusername/astra-backend/internal/idbimap"
	"github.com/yourusername/astra-backend/internal/provider/idbi"
	"github.com/yourusername/astra-backend/internal/repository"
)

type Provider interface {
	GetLoanOverdueDetails(ctx context.Context, req idbi.GetLoanOverdueDetailsRequest) (*idbi.GetLoanOverdueDetailsResponse, error)
	GetLoanAccountDetails(ctx context.Context, req idbi.GetLoanAccountDetailsRequest) (*idbi.GetLoanAccountDetailsResponse, error)
	InquireHPPayoff(ctx context.Context, req idbi.InquireHPPayoffRequest) (*idbi.InquireHPPayoffResponse, error)
}

type Repo interface {
	GetCustomerLink(ctx context.Context, userID uuid.UUID) (repository.CustomerLink, error)
	ReplaceLoans(ctx context.Context, userID uuid.UUID, loans []idbimap.Loan) error
	ListLoans(ctx context.Context, userID uuid.UUID) ([]repository.MirroredLoan, error)
}

type Config struct {
	CacheTTL   time.Duration // read cache; default 5m
	StaleAfter time.Duration // background refresh threshold; default 1h; 0 disables
}

type Service struct {
	prov Provider
	repo Repo
	cfg  Config
	log  *slog.Logger

	cache    sync.Map // userID -> cacheEntry
	inflight sync.Map
}

type cacheEntry struct {
	loans []Loan
	exp   time.Time
}

func New(prov Provider, repo Repo, cfg Config, log *slog.Logger) *Service {
	if cfg.CacheTTL == 0 {
		cfg.CacheTTL = 5 * time.Minute
	}
	if cfg.StaleAfter == 0 {
		cfg.StaleAfter = time.Hour
	}
	if log == nil {
		log = slog.Default()
	}
	return &Service{prov: prov, repo: repo, cfg: cfg, log: log}
}

// Loan is the outward shape.
type Loan struct {
	LoanAccountID      string    `json:"loan_account_id"`
	HolderName         string    `json:"holder_name,omitempty"`
	SanctionedAmount   float64   `json:"sanctioned_amount"`
	DisbursedAmount    float64   `json:"disbursed_amount"`
	InterestRate       float64   `json:"interest_rate"`
	TenureMonths       int       `json:"tenure_months"`
	RepaymentMethod    string    `json:"repayment_method,omitempty"`
	OutstandingBalance float64   `json:"outstanding_balance"`
	OverdueAmount      float64   `json:"overdue_amount"`
	DPD                int       `json:"dpd"`
	NpaStatus          string    `json:"npa_status,omitempty"`
	SyncedAt           time.Time `json:"synced_at"`
}

// PayoffQuote is the outward 538 shape.
type PayoffQuote struct {
	LoanAccountID          string  `json:"loan_account_id"`
	NetPayoffAmount        float64 `json:"net_payoff_amount"`
	PendingPrincipal       float64 `json:"pending_principal"`
	PendingNormalInterest  float64 `json:"pending_normal_interest"`
	PendingPenalInterest   float64 `json:"pending_penal_interest"`
	PendingOverdueInterest float64 `json:"pending_overdue_interest"`
	InterestRate           float64 `json:"interest_rate"`
}

// Refresh pulls the customer's loan accounts (402 -> list, 391 per loan for
// master detail) and replaces the mirror.
func (s *Service) Refresh(ctx context.Context, userID uuid.UUID) error {
	link, err := s.repo.GetCustomerLink(ctx, userID)
	if err != nil {
		return err
	}
	if link.CustID == "" {
		return fmt.Errorf("idbiloans: user %s has no custId — cannot call 402", userID)
	}

	ovResp, err := s.prov.GetLoanOverdueDetails(ctx, idbi.GetLoanOverdueDetailsRequest{CustomerID: link.CustID})
	if err != nil {
		return fmt.Errorf("idbiloans: 402 for cust %s: %w", link.CustID, err)
	}
	overdue := idbimap.OverdueByAccount(ovResp)
	if len(overdue) == 0 {
		return s.repo.ReplaceLoans(ctx, userID, nil)
	}

	loans := make([]idbimap.Loan, 0, len(overdue))
	for acctID, od := range overdue {
		l := idbimap.MergeOverdue(idbimap.Loan{LoanAccountID: acctID}, od)

		var dReq idbi.GetLoanAccountDetailsRequest
		dReq.Input.LoanAcctID.AcctID = acctID
		dReq.Input.CustID.CustID = link.CustID
		dReq.Input.Channel = "API"
		if det, derr := s.prov.GetLoanAccountDetails(ctx, dReq); derr == nil {
			l = mergeMaster(l, idbimap.LoanFromDetails(det))
		} else {
			s.log.Warn("idbiloans: 391 detail failed", "loan", acctID, "error", derr)
		}
		loans = append(loans, l)
	}

	if err := s.repo.ReplaceLoans(ctx, userID, loans); err != nil {
		return err
	}
	s.cache.Delete(userID)
	s.log.Info("idbiloans: refreshed", "user", userID, "loans", len(loans))
	return nil
}

func mergeMaster(pos, master idbimap.Loan) idbimap.Loan {
	out := pos
	if master.HolderName != "" {
		out.HolderName = master.HolderName
	}
	if master.CustID != "" {
		out.CustID = master.CustID
	}
	out.SanctionedAmount = master.SanctionedAmount
	out.DisbursedAmount = master.DisbursedAmount
	out.AvailableAmount = master.AvailableAmount
	out.InterestRate = master.InterestRate
	out.TenureMonths = master.TenureMonths
	out.RepaymentMethod = master.RepaymentMethod
	out.InterestMethod = master.InterestMethod
	if master.OpenedAt != "" {
		out.OpenedAt = master.OpenedAt
	}
	return out
}

// List returns the user's mirrored loans (cache -> DB -> stale background refresh).
func (s *Service) List(ctx context.Context, userID uuid.UUID) ([]Loan, error) {
	if v, ok := s.cache.Load(userID); ok {
		if e := v.(cacheEntry); time.Now().Before(e.exp) {
			return e.loans, nil
		}
	}
	rows, err := s.repo.ListLoans(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]Loan, 0, len(rows))
	var newest time.Time
	for _, r := range rows {
		if r.SyncedAt.After(newest) {
			newest = r.SyncedAt
		}
		out = append(out, Loan(r))
	}
	s.cache.Store(userID, cacheEntry{loans: out, exp: time.Now().Add(s.cfg.CacheTTL)})
	if s.cfg.StaleAfter > 0 && (newest.IsZero() || time.Since(newest) > s.cfg.StaleAfter) {
		s.refreshBG(userID)
	}
	return out, nil
}

// PayoffQuote computes a foreclosure quote for one loan (538, on demand).
func (s *Service) PayoffQuote(ctx context.Context, userID uuid.UUID, loanAccountID string) (PayoffQuote, error) {
	var req idbi.InquireHPPayoffRequest
	req.HPayOffInq.Foracid = loanAccountID
	resp, err := s.prov.InquireHPPayoff(ctx, req)
	if err != nil {
		return PayoffQuote{}, fmt.Errorf("idbiloans: 538 for %s: %w", loanAccountID, err)
	}
	q := idbimap.PayoffFromInquiry(resp)
	return PayoffQuote{
		LoanAccountID:          loanAccountID,
		NetPayoffAmount:        q.NetPayoffAmount,
		PendingPrincipal:       q.PendingPrincipal,
		PendingNormalInterest:  q.PendingNormalInterest,
		PendingPenalInterest:   q.PendingPenalInterest,
		PendingOverdueInterest: q.PendingOverdueInterest,
		InterestRate:           q.InterestRate,
	}, nil
}

func (s *Service) refreshBG(userID uuid.UUID) {
	if _, busy := s.inflight.LoadOrStore(userID, struct{}{}); busy {
		return
	}
	go func() {
		defer s.inflight.Delete(userID)
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := s.Refresh(ctx, userID); err != nil {
			s.log.Warn("idbiloans: background refresh failed", "user", userID, "error", err)
		}
	}()
}
