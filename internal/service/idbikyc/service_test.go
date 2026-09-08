package idbikyc

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"

	"github.com/yourusername/astra-backend/internal/provider/idbi"
)

type fakeProvider struct {
	resp *idbi.SearchCkycResponse
	last idbi.SearchCkycRequest
}

func (f *fakeProvider) SearchCkycDetails(_ context.Context, req idbi.SearchCkycRequest) (*idbi.SearchCkycResponse, error) {
	f.last = req
	return f.resp, nil
}

func TestVerifyPAN(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("../../provider/idbi/testdata", "Development_searchCkycDetailstest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var env struct {
		Response json.RawMessage `json:"response"`
	}
	_ = json.Unmarshal(b, &env)
	var resp idbi.SearchCkycResponse
	if err := json.Unmarshal(env.Response, &resp); err != nil {
		t.Fatal(err)
	}

	fp := &fakeProvider{resp: &resp}
	s := New(fp, nil, Config{ParentCompany: "AAAAA8597P", BranchCode: "105"})

	res, err := s.VerifyPAN(context.Background(), uuid.New(), "fghpp4567t")
	if err != nil {
		t.Fatalf("VerifyPAN: %v", err)
	}
	if res.PAN != "FGHPP4567T" {
		t.Errorf("PAN not upper-cased: %q", res.PAN)
	}
	if !res.CkycAvailable {
		t.Error("expected CkycAvailable true")
	}
	if res.CkycID != "600098765432" {
		t.Errorf("CkycID = %q", res.CkycID)
	}
	if len(res.IDTypes) == 0 {
		t.Error("expected id types")
	}
	// request shaped correctly
	got := fp.last.Input.SearchInCkycRequestDetails
	if len(got) != 1 || got[0].InputIDNo != "FGHPP4567T" || got[0].InputIDType != "C" {
		t.Errorf("bad request detail: %+v", got)
	}
}

func TestVerifyPAN_BadLength(t *testing.T) {
	s := New(&fakeProvider{}, nil, Config{})
	if _, err := s.VerifyPAN(context.Background(), uuid.New(), "ABC"); err == nil {
		t.Fatal("expected length error")
	}
}
