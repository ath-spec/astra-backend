// Package rmcreditrisk is IDBI integration feature 5: a per-client credit-risk
// / early-warning snapshot for the RM portal, from 442 fetchCustomerLimitDetails
// (CIF exposure) + 402 getLoanOverdueDetails (overdue aggregate). Gated by
// IDBI_RM_RISK_ENABLED. RM-side only — no user-app surface.
package rmcreditrisk

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/yourusername/astra-backend/internal/idbimap"
	"github.com/yourusername/astra-backend/internal/provider/idbi"
	"github.com/yourusername/astra-backend/internal/repository"
)

type Provider interface {
	FetchCustomerLimitDetails(ctx context.Context, req idbi.FetchCustomerLimitDetailsRequest) (*idbi.FetchCustomerLimitDetailsResponse, error)
	GetLoanOverdueDetails(ctx context.Context, req idbi.GetLoanOverdueDetailsRequest) (*idbi.GetLoanOverdueDetailsResponse, error)
}

type Repo interface {
	GetCustomerLink(ctx context.Context, userID uuid.UUID) (repository.CustomerLink, error)
	UpsertCreditExposure(ctx context.Context, userID uuid.UUID, e idbimap.CreditExposure) error
	GetCreditExposure(ctx context.Context, userID uuid.UUID) (repository.CreditExposureRow, bool, error)
}

type Service struct {
	prov Provider
	repo Repo
	log  *slog.Logger
}

func New(prov Provider, repo Repo, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{prov: prov, repo: repo, log: log}
}

// Exposure is the outward shape.
type Exposure struct {
	CifID            string    `json:"cif_id"`
	CustomerName     string    `json:"customer_name"`
	AccountManager   string    `json:"account_manager"`
	CustRating       string    `json:"cust_rating"`
	TotalLimit       float64   `json:"total_limit"`
	FundedLimit      float64   `json:"funded_limit"`
	NonFundedLimit   float64   `json:"non_funded_limit"`
	TotalOutstanding float64   `json:"total_outstanding"`
	UtilisationPct   float64   `json:"utilisation_pct"`
	LoanCount        int       `json:"loan_count"`
	TotalOverdue     float64   `json:"total_overdue"`
	MaxDPD           int       `json:"max_dpd"`
	WorstNpaStatus   string    `json:"worst_npa_status"`
	RiskBand         string    `json:"risk_band"`
	SyncedAt         time.Time `json:"synced_at"`
}

// Refresh pulls 442 + 402 for the client and stores the snapshot.
func (s *Service) Refresh(ctx context.Context, clientUserID uuid.UUID) error {
	link, err := s.repo.GetCustomerLink(ctx, clientUserID)
	if err != nil {
		return err
	}
	e := idbimap.CreditExposure{CifID: link.CifID}

	if link.CifID != "" {
		if lim, lerr := s.prov.FetchCustomerLimitDetails(ctx, idbi.FetchCustomerLimitDetailsRequest{CustCifID: link.CifID}); lerr == nil {
			e = idbimap.ExposureFromLimitDetails(lim)
		} else {
			s.log.Warn("rmcreditrisk: 442 failed", "cif", link.CifID, "error", lerr)
		}
	}
	if link.CustID != "" {
		ov, oerr := s.prov.GetLoanOverdueDetails(ctx, idbi.GetLoanOverdueDetailsRequest{CustomerID: link.CustID})
		if oerr != nil {
			s.log.Warn("rmcreditrisk: 402 failed", "cust", link.CustID, "error", oerr)
		}
		e = idbimap.MergeOverdueSignal(e, ov)
	} else {
		e = idbimap.MergeOverdueSignal(e, nil)
	}

	if err := s.repo.UpsertCreditExposure(ctx, clientUserID, e); err != nil {
		return err
	}
	s.log.Info("rmcreditrisk: refreshed", "client", clientUserID, "risk_band", e.RiskBand)
	return nil
}

// Get returns the stored snapshot, refreshing first if there is none.
func (s *Service) Get(ctx context.Context, clientUserID uuid.UUID) (Exposure, error) {
	row, ok, err := s.repo.GetCreditExposure(ctx, clientUserID)
	if err != nil {
		return Exposure{}, err
	}
	if !ok {
		if rerr := s.Refresh(ctx, clientUserID); rerr != nil {
			return Exposure{}, rerr
		}
		row, ok, err = s.repo.GetCreditExposure(ctx, clientUserID)
		if err != nil {
			return Exposure{}, err
		}
		if !ok {
			return Exposure{}, fmt.Errorf("rmcreditrisk: no exposure after refresh for %s", clientUserID)
		}
	}
	return Exposure(row), nil
}
