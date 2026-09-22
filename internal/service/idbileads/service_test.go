package idbileads

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yourusername/astra-backend/internal/provider/idbi"
)

type fakeProv struct {
	resp *idbi.CreateLeadResponse
	got  idbi.CreateLeadRequest
}

func (f *fakeProv) CreateLead(_ context.Context, req idbi.CreateLeadRequest) (*idbi.CreateLeadResponse, error) {
	f.got = req
	return f.resp, nil
}

func loadResp(t *testing.T, name string, dst any) {
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

func TestSubmit_BuildsRequestAndFlagsExisting(t *testing.T) {
	var resp idbi.CreateLeadResponse
	loadResp(t, "Development_createLeadtest__sample1alreadyexitlead", &resp)

	fp := &fakeProv{resp: &resp}
	s := New(fp, Config{DefaultSolID: "0183", LeadChannel: "Online", LeadSource: "Website"})

	out, err := s.Submit(context.Background(), Lead{
		Applicant: Applicant{
			FirstName: "priya", LastName: "patil", Mobile: "9988776655",
			Email: "priya@gmail", PAN: "fghpp4567t", Address: "142 Lake View",
			Pincode: "411001", State: "MH",
		},
		Product: "Home Loan", Category: "Loans", SubCategory: "Housing Loan",
		EstimatedAmount: 5000000,
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if !out.AlreadyExist {
		t.Errorf("expected already_exists=true for %q", out.Message)
	}

	g := fp.got.Input
	if g.FirstName != "PRIYA" || g.LastName != "PATIL" {
		t.Errorf("name not upper-cased: %q %q", g.FirstName, g.LastName)
	}
	if g.Pancard != "FGHPP4567T" {
		t.Errorf("pan = %q", g.Pancard)
	}
	if g.EstimatedAmount != "5000000" {
		t.Errorf("estimatedAmount = %q, want 5000000", g.EstimatedAmount)
	}
	if g.Solid != "0183" || g.LeadChannel != "Online" || g.LeadSource != "Website" {
		t.Errorf("config identity not applied: %+v", g)
	}
	if g.LeadType != "NEW" || g.CustomerType != "INDIVIDUAL" {
		t.Errorf("lead defaults wrong: %+v", g)
	}
	if !strings.HasPrefix(g.LeadID, "LD") {
		t.Errorf("leadId = %q, want LD-prefixed", g.LeadID)
	}
}

func TestSubmit_Validation(t *testing.T) {
	s := New(&fakeProv{resp: &idbi.CreateLeadResponse{}}, Config{})
	if _, err := s.Submit(context.Background(), Lead{Product: "", Applicant: Applicant{Mobile: "9"}}); err == nil {
		t.Error("expected error for missing product")
	}
	if _, err := s.Submit(context.Background(), Lead{Product: "Car Loan"}); err == nil {
		t.Error("expected error for missing mobile")
	}
}

func TestSubmit_PerLeadSolOverride(t *testing.T) {
	fp := &fakeProv{resp: &idbi.CreateLeadResponse{}}
	s := New(fp, Config{DefaultSolID: "0183"})
	_, err := s.Submit(context.Background(), Lead{
		Product: "Car Loan", Applicant: Applicant{Mobile: "9988776655"}, SolID: "0999",
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if fp.got.Input.Solid != "0999" {
		t.Errorf("solid = %q, want per-lead override 0999", fp.got.Input.Solid)
	}
}
