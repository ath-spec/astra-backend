package idbimap

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	analytics "github.com/yourusername/astra-backend/internal/domain/analytics"
	"github.com/yourusername/astra-backend/internal/provider/idbi"
)

// fixtureDir holds the live-captured sandbox responses
// (scripts/idbi-capture-fixtures.py).
const fixtureDir = "../provider/idbi/testdata"

// loadResponse reads testdata/<name>.json and unmarshals its "response" field
// into dst. These files are {spec_file, method, url, request, status, response}.
func loadResponse(t *testing.T, name string, dst any) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(fixtureDir, name+".json"))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	var env struct {
		Status   int             `json:"status"`
		Response json.RawMessage `json:"response"`
	}
	if err := json.Unmarshal(b, &env); err != nil {
		t.Fatalf("decode fixture envelope %s: %v", name, err)
	}
	if env.Status != 200 {
		t.Fatalf("fixture %s has status %d, expected 200", name, env.Status)
	}
	if err := json.Unmarshal(env.Response, dst); err != nil {
		t.Fatalf("decode fixture response %s into %T: %v", name, dst, err)
	}
}

func TestParseAmount(t *testing.T) {
	cases := map[string]float64{
		"56780.25":  56780.25,
		"0":         0,
		"":          0,
		"NULL":      0,
		"1,20,000":  120000,
		"400537.00": 400537,
	}
	for in, want := range cases {
		got, err := ParseAmount(in)
		if err != nil {
			t.Errorf("ParseAmount(%q) error: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParseAmount(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestParseTime(t *testing.T) {
	for _, s := range []string{
		"2025-05-01T00:00:00.000",
		"2025-05-31T10:21:03.000Z",
		"2018-04-18",
		"22-11-2018",
		"23-Jul-2019",
	} {
		if _, err := ParseTime(s); err != nil {
			t.Errorf("ParseTime(%q): %v", s, err)
		}
	}
	for _, s := range []string{"", "NULL", "null"} {
		got, err := ParseTime(s)
		if err != nil || !got.IsZero() {
			t.Errorf("ParseTime(%q) = %v, %v; want zero, nil", s, got, err)
		}
	}
}

func TestAccountFromEnquiry(t *testing.T) {
	var resp idbi.PerformAccountEnquiryResponse
	loadResponse(t, "Development_performAccountEnquirytest__Sample1", &resp)

	a := AccountFromEnquiry(&resp)
	if a.AccountNumber != "660100100003" {
		t.Errorf("AccountNumber = %q", a.AccountNumber)
	}
	if a.CustID != "68453002" {
		t.Errorf("CustID = %q", a.CustID)
	}
	if a.HolderName != "PRIYA PATIL" {
		t.Errorf("HolderName = %q, want 'PRIYA PATIL'", a.HolderName)
	}
	if a.AccountType != "SAVINGS" {
		t.Errorf("AccountType = %q", a.AccountType)
	}
	if a.LedgerBalance != 56780.25 {
		t.Errorf("LedgerBalance = %v, want 56780.25", a.LedgerBalance)
	}
	if a.AvailableBalance != 55780.25 {
		t.Errorf("AvailableBalance = %v, want 55780.25", a.AvailableBalance)
	}
	if a.LienAmount != 5000 {
		t.Errorf("LienAmount = %v, want 5000", a.LienAmount)
	}
	if a.Status != "A" {
		t.Errorf("Status = %q, want A", a.Status)
	}
}

func TestAccountsFromCustomerList(t *testing.T) {
	var resp idbi.GetCustomerAccountsByCustIDResponse
	loadResponse(t, "Development_getCustomerAccountsByCustIdtest01", &resp)

	accs := AccountsFromCustomerList(&resp)
	if len(accs) != 4 {
		t.Fatalf("got %d accounts, want 4", len(accs))
	}
	byType := map[string]float64{}
	for _, a := range accs {
		byType[a.AccountType] = a.LedgerBalance
		if a.CifID != "88234567" {
			t.Errorf("account %s CifID = %q, want 88234567", a.AccountNumber, a.CifID)
		}
	}
	if byType["TERM_DEPOSIT"] != 500000 {
		t.Errorf("TERM_DEPOSIT balance = %v, want 500000", byType["TERM_DEPOSIT"])
	}
	if byType["SALARY"] != 118000 {
		t.Errorf("SALARY balance = %v, want 118000", byType["SALARY"])
	}
}

func TestFullStatementTransactions(t *testing.T) {
	var resp idbi.FullStatementResponse
	loadResponse(t, "Development_getFullAccountStatementWithPaginationtest", &resp)

	txns := FullStatementTransactions(&resp)
	if len(txns) == 0 {
		t.Fatal("no transactions mapped")
	}
	var debits, credits int
	for _, tx := range txns {
		if tx.ID == "" {
			t.Errorf("txn with empty ID: %+v", tx)
		}
		if tx.Amount <= 0 {
			t.Errorf("txn %s amount = %v, want > 0", tx.ID, tx.Amount)
		}
		if tx.OccurredAt.IsZero() {
			t.Errorf("txn %s has zero OccurredAt", tx.ID)
		}
		switch tx.Type {
		case analytics.TxnDebit:
			debits++
		case analytics.TxnCredit:
			credits++
		default:
			t.Errorf("txn %s unknown type %q", tx.ID, tx.Type)
		}
	}
	if debits == 0 || credits == 0 {
		t.Errorf("expected both debits and credits, got %d/%d", debits, credits)
	}
}

func TestAAStatementAllTransactions(t *testing.T) {
	for _, name := range []string{
		"Development_getAccountStatementFromFinProtest",
		"Development_getAccountStatementtest",
	} {
		var resp idbi.GetAAStatementResponse
		loadResponse(t, name, &resp)
		txns := AAStatementAllTransactions(&resp)
		if len(txns) == 0 {
			t.Fatalf("%s: no transactions mapped", name)
		}
		for _, tx := range txns {
			if tx.Amount <= 0 {
				t.Errorf("%s: txn %s amount = %v", name, tx.ID, tx.Amount)
			}
			if tx.Type != analytics.TxnDebit && tx.Type != analytics.TxnCredit {
				t.Errorf("%s: txn %s bad type %q", name, tx.ID, tx.Type)
			}
			if tx.OccurredAt.IsZero() {
				t.Errorf("%s: txn %s zero time", name, tx.ID)
			}
		}
	}
}
