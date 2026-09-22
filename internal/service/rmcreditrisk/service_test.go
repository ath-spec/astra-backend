package rmcreditrisk

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"

	"github.com/yourusername/astra-backend/internal/idbimap"
	"github.com/yourusername/astra-backend/internal/provider/idbi"
	"github.com/yourusername/astra-backend/internal/repository"
)

func load(t *testing.T, name string, dst any) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("../../provider/idbi/testdata", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var env struct {
		Response json.RawMessage `json:"response"`
	}
	_ = json.Unmarshal(b, &env)
	if err := json.Unmarshal(env.Response, dst); err != nil {
		t.Fatal(err)
	}
}

type fakeProv struct {
	lim *idbi.FetchCustomerLimitDetailsResponse
	ov  *idbi.GetLoanOverdueDetailsResponse
}

func (f *fakeProv) FetchCustomerLimitDetails(_ context.Context, _ idbi.FetchCustomerLimitDetailsRequest) (*idbi.FetchCustomerLimitDetailsResponse, error) {
	return f.lim, nil
}
func (f *fakeProv) GetLoanOverdueDetails(_ context.Context, _ idbi.GetLoanOverdueDetailsRequest) (*idbi.GetLoanOverdueDetailsResponse, error) {
	return f.ov, nil
}

type fakeRepo struct {
	link *repository.CustomerLink
	row  *repository.CreditExposureRow
}

func (r *fakeRepo) GetCustomerLink(_ context.Context, _ uuid.UUID) (repository.CustomerLink, error) {
	if r.link == nil {
		return repository.CustomerLink{}, repository.ErrNoCustomerLink
	}
	return *r.link, nil
}
func (r *fakeRepo) UpsertCreditExposure(_ context.Context, _ uuid.UUID, e idbimap.CreditExposure) error {
	row := repository.CreditExposureRow{
		CifID: e.CifID, CustomerName: e.CustomerName, AccountManager: e.AccountManager, CustRating: e.CustRating,
		TotalLimit: e.TotalLimit, FundedLimit: e.FundedLimit, NonFundedLimit: e.NonFundedLimit,
		TotalOutstanding: e.TotalOutstanding, UtilisationPct: e.UtilisationPct,
		LoanCount: e.LoanCount, TotalOverdue: e.TotalOverdue, MaxDPD: e.MaxDPD,
		WorstNpaStatus: e.WorstNpaStatus, RiskBand: e.RiskBand,
	}
	r.row = &row
	return nil
}
func (r *fakeRepo) GetCreditExposure(_ context.Context, _ uuid.UUID) (repository.CreditExposureRow, bool, error) {
	if r.row == nil {
		return repository.CreditExposureRow{}, false, nil
	}
	return *r.row, true, nil
}

func TestRefreshAndGet(t *testing.T) {
	var lim idbi.FetchCustomerLimitDetailsResponse
	load(t, "Development_fetchCustomerLimitDetailstest", &lim)
	var ov idbi.GetLoanOverdueDetailsResponse
	load(t, "Development_getLoanOverdueDetailstest", &ov)

	fp := &fakeProv{lim: &lim, ov: &ov}
	fr := &fakeRepo{link: &repository.CustomerLink{CifID: "98655854", CustID: "68453002"}}
	s := New(fp, fr, nil)

	e, err := s.Get(context.Background(), uuid.New()) // triggers Refresh (no row yet)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if e.CifID != "98655854" {
		t.Errorf("CifID = %q", e.CifID)
	}
	if e.TotalLimit <= 0 || e.TotalOutstanding <= 0 {
		t.Errorf("limits not populated: %+v", e)
	}
	if e.UtilisationPct <= 0 {
		t.Errorf("UtilisationPct = %v", e.UtilisationPct)
	}
	if e.LoanCount == 0 {
		t.Errorf("LoanCount = 0, expected >0 from 402")
	}
	if e.RiskBand == "" {
		t.Error("RiskBand not set")
	}
}

func TestRefresh_NoLink(t *testing.T) {
	s := New(&fakeProv{}, &fakeRepo{}, nil)
	if err := s.Refresh(context.Background(), uuid.New()); err == nil {
		t.Fatal("expected ErrNoCustomerLink")
	}
}
