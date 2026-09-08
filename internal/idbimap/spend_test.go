package idbimap

import (
	"testing"

	analytics "github.com/yourusername/astra-backend/internal/domain/analytics"
	"github.com/yourusername/astra-backend/internal/provider/idbi"
)

func TestFullStatementSpendRows(t *testing.T) {
	var resp idbi.FullStatementResponse
	loadResponse(t, "Development_getFullAccountStatementWithPaginationtest", &resp)

	rows := FullStatementSpendRows("660100100003", &resp)
	if len(rows) == 0 {
		t.Fatal("no rows")
	}
	for _, r := range rows {
		if r.ExternalID == "" {
			t.Errorf("row missing ExternalID: %+v", r)
		}
		if r.AccountRef != "660100100003" {
			t.Errorf("AccountRef = %q", r.AccountRef)
		}
		if r.Amount <= 0 {
			t.Errorf("row %s amount %v", r.ExternalID, r.Amount)
		}
		if r.OccurredAt.IsZero() {
			t.Errorf("row %s zero time", r.ExternalID)
		}
		if r.Type != analytics.TxnDebit && r.Type != analytics.TxnCredit {
			t.Errorf("row %s bad type %q", r.ExternalID, r.Type)
		}
		if r.Category == "" {
			t.Errorf("row %s empty category", r.ExternalID)
		}
	}
}

func TestAAStatementAllSpendRows(t *testing.T) {
	var resp idbi.GetAAStatementResponse
	loadResponse(t, "Development_getAccountStatementtest", &resp)

	rows := AAStatementAllSpendRows(&resp)
	if len(rows) == 0 {
		t.Fatal("no rows")
	}
	sawRef := false
	for _, r := range rows {
		if r.AccountRef != "" {
			sawRef = true
		}
		if r.ExternalID == "" || r.OccurredAt.IsZero() {
			t.Errorf("bad row %+v", r)
		}
	}
	if !sawRef {
		t.Error("expected at least one row with an AccountRef (linkReferenceNumber)")
	}
}

func TestCategorise(t *testing.T) {
	cases := map[string]string{
		"UPI/CR/22162/PAYEE0/ABCD":       "Transfers",
		"Sms Charges For April Qtr,2024": "Bills & Utilities",
		"SWIGGY ORDER 8891":              "Food & Dining",
		"UBER TRIP":                      "Transport",
		"AMAZON PAY":                     "Shopping",
		"SALARY CREDIT":                  "Income",
		"S1 TXN 1":                       "Other",
	}
	for in, want := range cases {
		if got := Categorise(in); got != want {
			t.Errorf("Categorise(%q) = %q, want %q", in, got, want)
		}
	}
}
