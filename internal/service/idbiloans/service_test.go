package idbiloans

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

const fixtureDir = "../../provider/idbi/testdata"

func loadResp(t *testing.T, name string, dst any) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(fixtureDir, name+".json"))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var env struct {
		Response json.RawMessage `json:"response"`
	}
	if err := json.Unmarshal(b, &env); err != nil {
		t.Fatalf("envelope %s: %v", name, err)
	}
	if err := json.Unmarshal(env.Response, dst); err != nil {
		t.Fatalf("response %s: %v", name, err)
	}
}

type fakeProvider struct {
	overdue  *idbi.GetLoanOverdueDetailsResponse
	detail   *idbi.GetLoanAccountDetailsResponse
	payoff   *idbi.InquireHPPayoffResponse
	limits   *idbi.FetchLoanAccountLimitsResponse
	position *idbi.GetLoanOverduePositionResponse
	schedule *idbi.GenerateRepaymentScheduleResponse
}

func (f *fakeProvider) GetLoanOverdueDetails(_ context.Context, _ idbi.GetLoanOverdueDetailsRequest) (*idbi.GetLoanOverdueDetailsResponse, error) {
	return f.overdue, nil
}
func (f *fakeProvider) GetLoanAccountDetails(_ context.Context, _ idbi.GetLoanAccountDetailsRequest) (*idbi.GetLoanAccountDetailsResponse, error) {
	return f.detail, nil
}
func (f *fakeProvider) InquireHPPayoff(_ context.Context, _ idbi.InquireHPPayoffRequest) (*idbi.InquireHPPayoffResponse, error) {
	return f.payoff, nil
}
func (f *fakeProvider) FetchLoanAccountLimits(_ context.Context, _ idbi.FetchLoanAccountLimitsRequest) (*idbi.FetchLoanAccountLimitsResponse, error) {
	return f.limits, nil
}
func (f *fakeProvider) GetLoanOverduePosition(_ context.Context, _ idbi.GetLoanOverduePositionRequest) (*idbi.GetLoanOverduePositionResponse, error) {
	return f.position, nil
}
func (f *fakeProvider) GenerateRepaymentSchedule(_ context.Context, _ idbi.GenerateRepaymentScheduleRequest) (*idbi.GenerateRepaymentScheduleResponse, error) {
	return f.schedule, nil
}

type fakeRepo struct {
	link  *repository.CustomerLink
	loans []idbimap.Loan
}

func (r *fakeRepo) GetCustomerLink(_ context.Context, _ uuid.UUID) (repository.CustomerLink, error) {
	if r.link == nil {
		return repository.CustomerLink{}, repository.ErrNoCustomerLink
	}
	return *r.link, nil
}
func (r *fakeRepo) ReplaceLoans(_ context.Context, _ uuid.UUID, loans []idbimap.Loan) error {
	r.loans = loans
	return nil
}
func (r *fakeRepo) ListLoans(_ context.Context, _ uuid.UUID) ([]repository.MirroredLoan, error) {
	out := make([]repository.MirroredLoan, 0, len(r.loans))
	for _, l := range r.loans {
		out = append(out, repository.MirroredLoan{
			LoanAccountID:      l.LoanAccountID,
			HolderName:         l.HolderName,
			SanctionedAmount:   l.SanctionedAmount,
			DisbursedAmount:    l.DisbursedAmount,
			InterestRate:       l.InterestRate,
			TenureMonths:       l.TenureMonths,
			RepaymentMethod:    l.RepaymentMethod,
			OutstandingBalance: l.OutstandingBalance,
			OverdueAmount:      l.OverdueAmount,
			DPD:                l.DPD,
			NpaStatus:          l.NpaStatus,
		})
	}
	return out, nil
}

func newFakes(t *testing.T) (*fakeProvider, *fakeRepo) {
	t.Helper()
	var ov idbi.GetLoanOverdueDetailsResponse
	loadResp(t, "Development_getLoanOverdueDetailstest", &ov)
	var det idbi.GetLoanAccountDetailsResponse
	loadResp(t, "Development_getLoanAccountDetailstest", &det)
	var po idbi.InquireHPPayoffResponse
	loadResp(t, "Development_InquireHPAyofftest", &po)
	return &fakeProvider{overdue: &ov, detail: &det, payoff: &po},
		&fakeRepo{link: &repository.CustomerLink{CifID: "98655854", CustID: "68453002"}}
}

func TestRefresh_MirrorsLoans(t *testing.T) {
	fp, fr := newFakes(t)
	s := New(fp, fr, Config{}, nil)
	if err := s.Refresh(context.Background(), uuid.New()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if len(fr.loans) == 0 {
		t.Fatal("no loans mirrored")
	}
	// 402 fixture for cust 68453002 has 3 loan accounts
	if len(fr.loans) != 3 {
		t.Errorf("mirrored %d loans, want 3", len(fr.loans))
	}
	var haveMaster bool
	for _, l := range fr.loans {
		if l.OutstandingBalance == 0 && l.LoanAccountID == "" {
			t.Errorf("empty loan: %+v", l)
		}
		if l.InterestRate > 0 { // came from 391 detail
			haveMaster = true
		}
	}
	if !haveMaster {
		t.Error("expected at least one loan enriched with 391 master (interest rate)")
	}
}

func TestRefresh_NoLink(t *testing.T) {
	fp, _ := newFakes(t)
	s := New(fp, &fakeRepo{}, Config{}, nil)
	if err := s.Refresh(context.Background(), uuid.New()); err == nil {
		t.Fatal("expected ErrNoCustomerLink")
	}
}

func TestPayoffQuote(t *testing.T) {
	fp, fr := newFakes(t)
	s := New(fp, fr, Config{}, nil)
	q, err := s.PayoffQuote(context.Background(), uuid.New(), "660100100003")
	if err != nil {
		t.Fatalf("PayoffQuote: %v", err)
	}
	if q.NetPayoffAmount != 400537 {
		t.Errorf("NetPayoffAmount = %v, want 400537", q.NetPayoffAmount)
	}
	if q.LoanAccountID != "660100100003" {
		t.Errorf("LoanAccountID = %q", q.LoanAccountID)
	}
}

func TestLoanLimits(t *testing.T) {
	fp, fr := newFakes(t)
	var lim idbi.FetchLoanAccountLimitsResponse
	loadResp(t, "Development_fetchLoanAccountLimitstest", &lim)
	fp.limits = &lim

	s := New(fp, fr, Config{}, nil)
	res, err := s.LoanLimits(context.Background(), "660100100003")
	if err != nil {
		t.Fatalf("LoanLimits: %v", err)
	}
	// Sanction history has one entry: 4,000,000.
	if res.SanctionedLimit != 4000000 {
		t.Errorf("SanctionedLimit = %v, want 4000000", res.SanctionedLimit)
	}
	// Drawing power: latest applicableDate is 2022-07-29 -> 4,000,000.
	if res.DrawingPower != 4000000 {
		t.Errorf("DrawingPower = %v, want 4000000 (latest by date)", res.DrawingPower)
	}
	if len(res.DrawingHistory) != 5 || res.DrawingHistory[0].EffectiveDate < res.DrawingHistory[1].EffectiveDate {
		t.Errorf("drawing history not newest-first: %+v", res.DrawingHistory)
	}
}

func TestOverduePosition(t *testing.T) {
	fp, fr := newFakes(t)
	var pos idbi.GetLoanOverduePositionResponse
	loadResp(t, "Development_getLoanOverduePositionEnquirytest", &pos)
	fp.position = &pos

	s := New(fp, fr, Config{}, nil)
	res, err := s.OverduePosition(context.Background(), uuid.New(), "660100100003")
	if err != nil {
		t.Fatalf("OverduePosition: %v", err)
	}
	if res.PrincipalDemanded != 34601.39 {
		t.Errorf("PrincipalDemanded = %v, want 34601.39", res.PrincipalDemanded)
	}
	if res.InterestDemanded != 268 {
		t.Errorf("InterestDemanded = %v, want 268", res.InterestDemanded)
	}
}

func TestOverduePosition_NoCustID(t *testing.T) {
	fp, _ := newFakes(t)
	s := New(fp, &fakeRepo{link: &repository.CustomerLink{CifID: "98655854"}}, Config{}, nil)
	if _, err := s.OverduePosition(context.Background(), uuid.New(), "660100100003"); err == nil {
		t.Fatal("expected error when custId is missing")
	}
}

func TestRepaymentSchedule(t *testing.T) {
	fp, fr := newFakes(t)
	var sch idbi.GenerateRepaymentScheduleResponse
	loadResp(t, "Development_generateLoanRepaymentScheduletest", &sch)
	fp.schedule = &sch

	s := New(fp, fr, Config{}, nil)
	res, err := s.RepaymentSchedule(context.Background(), uuid.New(), "660100100003")
	if err != nil {
		t.Fatalf("RepaymentSchedule: %v", err)
	}
	if res.InstalmentAmount != 4218.95 {
		t.Errorf("InstalmentAmount = %v, want 4218.95", res.InstalmentAmount)
	}
	if res.InstalmentCount != 24 {
		t.Errorf("InstalmentCount = %v, want 24", res.InstalmentCount)
	}
	if len(res.Rows) < 2 {
		t.Fatalf("want >=2 amort rows, got %d", len(res.Rows))
	}
	if res.Rows[0].Serial != 1 || res.Rows[0].PrincipalComponent != 4118.95 {
		t.Errorf("row 1 = %+v", res.Rows[0])
	}
}
