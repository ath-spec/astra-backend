// Package idbikyc is IDBI integration feature 6: CKYC verification via
// 415 searchCkycDetails. Backs POST /api/v1/kyc/pan/verify (previously a
// notConfigured stub). Gated by IDBI_KYC_ENABLED.
package idbikyc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourusername/astra-backend/internal/provider/idbi"
)

// Provider is the subset of *idbi.Client this service needs.
type Provider interface {
	SearchCkycDetails(ctx context.Context, req idbi.SearchCkycRequest) (*idbi.SearchCkycResponse, error)
}

// Config carries the CKYC caller identity IDBI expects in the request. All
// values come from the caller (config.Load / IDBI_CKYC_* env vars); this
// package holds no fallback literals.
type Config struct {
	APIToken      string // input.apiToken
	ParentCompany string // input.parentCompany (e.g. "AAAAA8597P")
	BranchCode    string // searchInCkycRequestDetails[].branchCode
	SourceSystem  string // e.g. "Finacle"
}

type Service struct {
	prov Provider
	pool *pgxpool.Pool
	cfg  Config
}

func New(prov Provider, pool *pgxpool.Pool, cfg Config) *Service {
	return &Service{prov: prov, pool: pool, cfg: cfg}
}

// Result is the outward shape.
type Result struct {
	PAN           string    `json:"pan"`
	CkycAvailable bool      `json:"ckyc_available"`
	CkycID        string    `json:"ckyc_id,omitempty"`
	CkycName      string    `json:"ckyc_name,omitempty"`
	AccountType   string    `json:"account_type,omitempty"`
	GeneratedDate string    `json:"generated_date,omitempty"`
	IDTypes       []string  `json:"id_types,omitempty"`
	VerifiedAt    time.Time `json:"verified_at"`
}

// VerifyPAN runs a CKYC search for a PAN and persists the result.
func (s *Service) VerifyPAN(ctx context.Context, userID uuid.UUID, pan string) (Result, error) {
	pan = strings.ToUpper(strings.TrimSpace(pan))
	if len(pan) != 10 {
		return Result{}, fmt.Errorf("idbikyc: pan must be 10 characters")
	}

	var req idbi.SearchCkycRequest
	req.Input.APIToken = s.cfg.APIToken
	req.Input.ParentCompany = s.cfg.ParentCompany
	qid := uuid.NewString()
	req.Input.SearchInCkycRequestDetails = append(req.Input.SearchInCkycRequestDetails, idbi.CkycRequestDetail{
		QueryID:          qid,
		RecordIdentifier: qid,
		BranchCode:       s.cfg.BranchCode,
		InputIDType:      "C", // PAN
		InputIDNo:        pan,
		SourceSystem:     s.cfg.SourceSystem,
	})

	resp, err := s.prov.SearchCkycDetails(ctx, req)
	if err != nil {
		return Result{}, fmt.Errorf("idbikyc: 415 for pan %s: %w", pan, err)
	}

	d := resp.Result.Details.SearchInCkycResponseDetails
	res := Result{
		PAN:           pan,
		CkycAvailable: strings.EqualFold(d.CkycAvailable, "Yes"),
		CkycID:        d.CkycID,
		CkycName:      d.CkycName,
		AccountType:   d.CkycAccType,
		GeneratedDate: d.CkycGenDate,
		VerifiedAt:    time.Now().UTC(),
	}
	for _, id := range d.CkycIDDetails.ID {
		if id.CkycAvailableIDType != "" {
			res.IDTypes = append(res.IDTypes, id.CkycAvailableIDType)
		}
	}

	if s.pool != nil {
		_, _ = s.pool.Exec(ctx, `
			INSERT INTO idbi_ckyc (user_id, pan, ckyc_available, ckyc_id, ckyc_name, ckyc_acc_type, ckyc_gen_date, id_types, verified_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8, now())
			ON CONFLICT (user_id, pan) DO UPDATE SET
				ckyc_available = EXCLUDED.ckyc_available,
				ckyc_id        = EXCLUDED.ckyc_id,
				ckyc_name      = EXCLUDED.ckyc_name,
				ckyc_acc_type  = EXCLUDED.ckyc_acc_type,
				ckyc_gen_date  = EXCLUDED.ckyc_gen_date,
				id_types       = EXCLUDED.id_types,
				verified_at    = now()
		`, userID, pan, res.CkycAvailable, res.CkycID, res.CkycName, res.AccountType, res.GeneratedDate, res.IDTypes)
	}
	return res, nil
}
