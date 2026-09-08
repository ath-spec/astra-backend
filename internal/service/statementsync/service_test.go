package statementsync

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
	stmt  *idbi.FullStatementResponse
	calls int
}

func (f *fakeProvider) GetFullAccountStatement(_ context.Context, _ idbi.FullStatementRequest) (*idbi.FullStatementResponse, error) {
	f.calls++
	return f.stmt, nil
}

type fakeRepo struct {
	link     *repository.CustomerLink
	accounts []repository.MirroredAccount
	upserts  []idbimap.SpendRow
	last     time.Time
}

func (r *fakeRepo) GetCustomerLink(_ context.Context, _ uuid.UUID) (repository.CustomerLink, error) {
	if r.link == nil {
		return repository.CustomerLink{}, repository.ErrNoCustomerLink
	}
	return *r.link, nil
}
func (r *fakeRepo) ListAccounts(_ context.Context, _ uuid.UUID) ([]repository.MirroredAccount, error) {
	return r.accounts, nil
}
func (r *fakeRepo) UpsertSpendTransactions(_ context.Context, _ uuid.UUID, _ string, rows []idbimap.SpendRow) (int, error) {
	n := 0
	for _, row := range rows {
		if row.ExternalID == "" || row.OccurredAt.IsZero() {
			continue
		}
		r.upserts = append(r.upserts, row)
		n++
	}
	return n, nil
}
func (r *fakeRepo) LatestSpendSync(_ context.Context, _ uuid.UUID, _ string) (time.Time, error) {
	return r.last, nil
}

func TestSyncUser_NoLink(t *testing.T) {
	s := New(&fakeProvider{}, &fakeRepo{}, Config{}, nil)
	if _, err := s.SyncUser(context.Background(), uuid.New()); err == nil {
		t.Fatal("expected ErrNoCustomerLink")
	}
}

func TestSyncUser_NoAccounts(t *testing.T) {
	fr := &fakeRepo{link: &repository.CustomerLink{CifID: "98655854"}}
	s := New(&fakeProvider{}, fr, Config{}, nil)
	if _, err := s.SyncUser(context.Background(), uuid.New()); err == nil {
		t.Fatal("expected error for no mirrored accounts")
	}
}

func TestSyncUser_WritesTransactions(t *testing.T) {
	var stmt idbi.FullStatementResponse
	loadResp(t, "Development_getFullAccountStatementWithPaginationtest", &stmt)

	fp := &fakeProvider{stmt: &stmt}
	fr := &fakeRepo{
		link: &repository.CustomerLink{CifID: "98655854", CustID: "68453002"},
		accounts: []repository.MirroredAccount{
			{AccountNumber: "660100100003", BranchID: "105", AccountType: "SAVINGS"},
		},
	}
	s := New(fp, fr, Config{}, nil)

	n, err := s.SyncUser(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("SyncUser: %v", err)
	}
	if n == 0 || n != len(fr.upserts) {
		t.Fatalf("wrote %d, upserts recorded %d", n, len(fr.upserts))
	}
	if fp.calls != 1 {
		t.Errorf("393 called %d times, want 1 (hasMoreData=N)", fp.calls)
	}
	for _, row := range fr.upserts {
		if row.AccountRef != "660100100003" {
			t.Errorf("row AccountRef = %q", row.AccountRef)
		}
		if row.ExternalID == "" || row.OccurredAt.IsZero() {
			t.Errorf("bad row %+v", row)
		}
	}
}
