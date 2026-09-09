// Package idbileads is IDBI integration: submit a product-interest lead to
// IDBI's CRM via 362 createLead. Thin passthrough — leads live in IDBI's
// CRM, nothing is mirrored locally. Gated by IDBI_LEADS_ENABLED.
package idbileads

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/yourusername/astra-backend/internal/provider/idbi"
)

// Provider is the subset of *idbi.Client this service needs.
type Provider interface {
	CreateLead(ctx context.Context, req idbi.CreateLeadRequest) (*idbi.CreateLeadResponse, error)
}

// Config carries the branch / channel identity IDBI expects on every lead.
type Config struct {
	DefaultSolID string // input.solid — originating branch SOL id
	LeadChannel  string // input.leadChannel, e.g. "Online"
	LeadSource   string // input.leadSource, e.g. "Website"
}

type Service struct {
	prov Provider
	cfg  Config
}

func New(prov Provider, cfg Config) *Service {
	return &Service{prov: prov, cfg: cfg}
}

// Applicant is the person the lead is for. Name / mobile / PAN are normally
// filled from the signed-in user by the handler; the rest are optional.
type Applicant struct {
	FirstName string
	LastName  string
	Mobile    string
	Email     string
	PAN       string
	Address   string
	Pincode   string
	State     string
}

// Lead is the inward request: which product the applicant is interested in.
type Lead struct {
	Applicant       Applicant
	Product         string // "Home Loan", "Car Loan", ...
	Category        string // "Loans"
	SubCategory     string // "Housing Loan", "Auto Loan", ...
	EstimatedAmount float64
	SolID           string // optional per-lead override of Config.DefaultSolID
}

// Result is the outward shape.
type Result struct {
	LeadID       string `json:"lead_id"`
	Message      string `json:"message"`
	AlreadyExist bool   `json:"already_exists"`
}

// Submit sends the lead to 362 createLead.
func (s *Service) Submit(ctx context.Context, in Lead) (Result, error) {
	if strings.TrimSpace(in.Product) == "" {
		return Result{}, fmt.Errorf("idbileads: product is required")
	}
	if strings.TrimSpace(in.Applicant.Mobile) == "" {
		return Result{}, fmt.Errorf("idbileads: applicant mobile is required")
	}

	leadID := "LD" + time.Now().UTC().Format("20060102") + numeric(9)

	var req idbi.CreateLeadRequest
	req.Input.LeadType = "NEW"
	req.Input.CustomerType = "INDIVIDUAL"
	req.Input.FirstName = strings.ToUpper(strings.TrimSpace(in.Applicant.FirstName))
	req.Input.LastName = strings.ToUpper(strings.TrimSpace(in.Applicant.LastName))
	req.Input.MobileNo = strings.TrimSpace(in.Applicant.Mobile)
	req.Input.EmailID = strings.TrimSpace(in.Applicant.Email)
	req.Input.Pancard = strings.ToUpper(strings.TrimSpace(in.Applicant.PAN))
	req.Input.AddressLine1 = strings.TrimSpace(in.Applicant.Address)
	req.Input.Pincode = strings.TrimSpace(in.Applicant.Pincode)
	req.Input.State = strings.TrimSpace(in.Applicant.State)
	req.Input.Product = strings.TrimSpace(in.Product)
	req.Input.ProdCategory = strings.TrimSpace(in.Category)
	req.Input.ProdSubCategory = strings.TrimSpace(in.SubCategory)
	if in.EstimatedAmount > 0 {
		req.Input.EstimatedAmount = fmt.Sprintf("%.0f", in.EstimatedAmount)
	}
	req.Input.Solid = firstNonEmpty(in.SolID, s.cfg.DefaultSolID)
	req.Input.LeadChannel = s.cfg.LeadChannel
	req.Input.LeadSource = s.cfg.LeadSource
	req.Input.LeadID = leadID

	resp, err := s.prov.CreateLead(ctx, req)
	if err != nil {
		return Result{}, fmt.Errorf("idbileads: 362 createLead: %w", err)
	}
	msg := resp.Result.Message
	out := Result{
		LeadID:       firstNonEmpty(resp.Result.LeadID, leadID),
		Message:      msg,
		AlreadyExist: strings.Contains(strings.ToLower(msg), "already"),
	}
	return out, nil
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func numeric(n int) string {
	const digits = "0123456789"
	b := make([]byte, n)
	for i := range b {
		x, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			b[i] = '0'
			continue
		}
		b[i] = digits[x.Int64()]
	}
	return string(b)
}
