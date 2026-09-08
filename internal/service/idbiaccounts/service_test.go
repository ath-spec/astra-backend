package idbiaccounts

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

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
		t.Fatalf("read fixture %s: %v", name, err)
	}
	var env struct {
		Response json.RawMessage `json:"response"`
	}
	if err := json.Unmarshal(b, &env); err != nil {
		t.Fatalf("decode envelope %s: %v", name, err)
	}
	if err := json.Unmarshal(env.Response, dst); err != nil {
		t.Fatalf("decode response %s: %v", name, err)
	}
}

// --- fakes -------------------------------------------------------------

type fakeProvider struct {
	list         *idbi.GetCustomerAccountsByCustIDResponse
	enquiryByAcc map[string]*idbi.PerformAccountEnquiryResponse
	listCalls    int
	enqCalls     int
}

func (f *fakeProvider) GetCustomerAccountsByCustID(_ context.Context, _ idbi.GetCustomerAccountsByCustIDRequest) (*idbi.GetCustomerAccountsByCustIDResponse, error) {
	f.listCalls++
	return f.list, nil
}

func (f *fakeProvider) PerformAccountEnquiry(_ context.Context, req idbi.PerformAccountEnquiryRequest) (*idbi.PerformAccountEnquiryResponse, error) {
	f.enqCalls++
	if r, ok := f.enquiryByAcc[req.AcctID]; ok {
		return r, nil
	}
	return &idbi.PerformAccountEnquiryResponse{AcctID: req.AcctID}, nil
}

type fakeRepo struct {
	link     *repository.CustomerLink
	accounts []idbimap.Account
	synced   time.Time
	replaces int
}

func (r *fakeRepo) GetCustomerLink(_ context.Context, _ uuid.UUID) (repository.CustomerLink, error) {
	if r.link == nil {
		return repository.CustomerLink{}, repository.ErrNoCustomerLink
	}
	return *r.link, nil
}

func (r *fakeRepo) UpsertCustomerLink(_ context.Context, userID uuid.UUID, cifID, custID, source string) error {
	r.link = &repository.CustomerLink{UserID: userID, CifID: cifID, CustID: custID, Source: source}
	return nil
}

func (r *fakeRepo) ReplaceAccounts(_ context.Context, _ uuid.UUID, accs []idbimap.Account) error {
	r.replaces++
	r.accounts = accs
	r.synced = time.Now()
	return nil
}

func (r *fakeRepo) ListAccounts(_ context.Context, _ uuid.UUID) ([]repository.MirroredAccount, error) {
	out := make([]repository.MirroredAccount, 0, len(r.accounts))
	for _, a := range r.accounts {
		out = append(out, repository.MirroredAccount{
			AccountNumber:    a.AccountNumber,
			AccountType:      a.AccountType,
			Currency:         a.Currency,
			HolderName:       a.HolderName,
			Status:           a.Status,
			BranchName:       a.BranchName,
			LedgerBalance:    a.LedgerBalance,
			AvailableBalance: a.AvailableBalance,
			EffectiveBalance: a.EffectiveBalance,
			LienAmount:       a.LienAmount,
			SyncedAt:         r.synced,
		})
	}
	return out, nil
}

// --- tests -----------------------------------------------------------

func newFakes(t *testing.T) (*fakeProvider, *fakeRepo) {
	t.Helper()
	var list idbi.GetCustomerAccountsByCustIDResponse
	loadResp(t, "Development_getCustomerAccountsByCustIdtest", &list) // cif 98655854 -> 1 acct

	var enq660003 idbi.PerformAccountEnquiryResponse
	loadResp(t, "Development_performAccountEnquirytest__Sample1", &enq660003)

	fp := &fakeProvider{
		list: &list,
		enquiryByAcc: map[string]*idbi.PerformAccountEnquiryResponse{
			"660100100003": &enq660003,
		},
	}
	fr := &fakeRepo{}
	return fp, fr
}

func TestRefresh_NoLink(t *testing.T) {
	fp, fr := newFakes(t)
	s := New(fp, fr, Config{}, nil)
	err := s.Refresh(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected ErrNoCustomerLink")
	}
}

func TestRefresh_SeededLink_MirrorsAccounts(t *testing.T) {
	fp, fr := newFakes(t)
	s := New(fp, fr, Config{}, nil)
	uid := uuid.New()

	if err := s.SeedLink(context.Background(), uid, "98655854", "68453002"); err != nil {
		t.Fatalf("SeedLink: %v", err)
	}
	if err := s.Refresh(context.Background(), uid); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	if fr.replaces != 1 {
		t.Fatalf("ReplaceAccounts called %d times, want 1", fr.replaces)
	}
	if len(fr.accounts) != 1 {
		t.Fatalf("mirrored %d accounts, want 1", len(fr.accounts))
	}
	a := fr.accounts[0]
	if a.AccountNumber != "660100100003" {
		t.Errorf("AccountNumber = %q", a.AccountNumber)
	}
	// 365 enrichment merged in:
	if a.CustID != "68453002" {
		t.Errorf("CustID = %q (365 enrichment missing?)", a.CustID)
	}
	if a.HolderName != "PRIYA PATIL" {
		t.Errorf("HolderName = %q", a.HolderName)
	}
	if a.AvailableBalance != 55780.25 {
		t.Errorf("AvailableBalance = %v, want 55780.25 (from 365)", a.AvailableBalance)
	}
	if a.LienAmount != 5000 {
		t.Errorf("LienAmount = %v, want 5000 (from 365)", a.LienAmount)
	}
	if fp.enqCalls != 1 {
		t.Errorf("PerformAccountEnquiry called %d times, want 1", fp.enqCalls)
	}
}

func TestRefresh_SkipPerAccountEnquiry(t *testing.T) {
	fp, fr := newFakes(t)
	s := New(fp, fr, Config{SkipPerAccountEnquiry: true}, nil)
	uid := uuid.New()
	_ = s.SeedLink(context.Background(), uid, "98655854", "68453002")
	if err := s.Refresh(context.Background(), uid); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if fp.enqCalls != 0 {
		t.Errorf("PerformAccountEnquiry called %d times, want 0", fp.enqCalls)
	}
	// still get the 394 balance
	if fr.accounts[0].LedgerBalance != 56780.25 {
		t.Errorf("LedgerBalance = %v, want 56780.25 (from 394)", fr.accounts[0].LedgerBalance)
	}
}

func TestList_CacheHitAvoidsRepo(t *testing.T) {
	fp, fr := newFakes(t)
	s := New(fp, fr, Config{CacheTTL: time.Minute, StaleAfter: 0}, nil)
	uid := uuid.New()
	_ = s.SeedLink(context.Background(), uid, "98655854", "68453002")
	_ = s.Refresh(context.Background(), uid)

	got1, err := s.List(context.Background(), uid)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got1) != 1 || got1[0].AccountNumber != "660100100003" {
		t.Fatalf("List returned %+v", got1)
	}

	// mutate the repo underneath; a cache hit must not see it
	fr.accounts = nil
	got2, _ := s.List(context.Background(), uid)
	if len(got2) != 1 {
		t.Errorf("expected cached result of len 1, got %d", len(got2))
	}
}

func TestSeedLink_RequiresCif(t *testing.T) {
	fp, fr := newFakes(t)
	s := New(fp, fr, Config{}, nil)
	if err := s.SeedLink(context.Background(), uuid.New(), "", "68453002"); err == nil {
		t.Fatal("expected error for empty cifId")
	}
}
