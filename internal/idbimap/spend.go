package idbimap

import (
	"strings"
	"time"

	analytics "github.com/yourusername/astra-backend/internal/domain/analytics"
	"github.com/yourusername/astra-backend/internal/provider/idbi"
)

// SpendRow is one transaction ready to upsert into spend_transactions. It
// carries the provider's own id (ExternalID) and the owning account
// (AccountRef) so a re-sync is idempotent — fields the analytics engine's
// Transaction type doesn't carry.
type SpendRow struct {
	ExternalID string
	AccountRef string
	Amount     float64
	Type       analytics.TxnType
	Category   string
	Merchant   string
	OccurredAt time.Time
}

// FullStatementSpendRows maps a 393 page to spend rows for one account.
// acctID is the IDBI account the statement was pulled for (AccountRef).
func FullStatementSpendRows(acctID string, resp *idbi.FullStatementResponse) []SpendRow {
	if resp == nil {
		return nil
	}
	rows := resp.Result.TransactionDetails
	out := make([]SpendRow, 0, len(rows))
	for _, r := range rows {
		amt, _ := ParseAmount(r.TransactionSummary.TxnAmt.AmountValue)
		merchant := strings.TrimSpace(r.TransactionSummary.TxnDesc)
		out = append(out, SpendRow{
			ExternalID: r.TxnID,
			AccountRef: acctID,
			Amount:     amt,
			Type:       txnType(r.TransactionSummary.TxnType),
			Category:   Categorise(merchant),
			Merchant:   merchant,
			OccurredAt: MustParseTime(firstNonEmpty(r.TransactionSummary.TxnDate, r.ValueDate, r.PstdDate)),
		})
	}
	return out
}

// AAStatementSpendRows maps a 595/739 account block to spend rows.
func AAStatementSpendRows(acc idbi.AAStatementAccount) []SpendRow {
	rows := acc.Transactions.Transaction
	ref := acc.LinkReferenceNumber
	out := make([]SpendRow, 0, len(rows))
	for _, r := range rows {
		amt, _ := ParseAmount(r.Amount)
		merchant := strings.TrimSpace(r.Narration)
		out = append(out, SpendRow{
			ExternalID: r.TxnID,
			AccountRef: ref,
			Amount:     amt,
			Type:       txnType(r.Type),
			Category:   Categorise(strings.TrimSpace(r.Mode + " " + merchant)),
			Merchant:   merchant,
			OccurredAt: MustParseTime(firstNonEmpty(r.TransactionTimestamp, r.ValueDate)),
		})
	}
	return out
}

// AAStatementAllSpendRows flattens every account block in a 595/739 response.
func AAStatementAllSpendRows(resp *idbi.GetAAStatementResponse) []SpendRow {
	if resp == nil {
		return nil
	}
	var out []SpendRow
	for _, acc := range resp.Data {
		out = append(out, AAStatementSpendRows(acc)...)
	}
	return out
}

// Categorise is a best-effort keyword bucket for a transaction narration.
// IDBI/AA carry no spend category (txnCat is an internal bank code), so this
// fills the gap until a dedicated categoriser is wired. Returns "Other" for
// anything unrecognised. Sandbox narrations are generic ("S1 TXN 1") so most
// land in "Other" there; real narrations categorise better.
func Categorise(narration string) string {
	n := strings.ToLower(narration)
	switch {
	case containsAny(n, "swiggy", "zomato", "restaurant", "cafe", "food", "eatery", "dominos", "mcdonald"):
		return "Food & Dining"
	case containsAny(n, "uber", "ola", "rapido", "irctc", "petrol", "fuel", "metro", "fastag", "toll"):
		return "Transport"
	case containsAny(n, "amazon", "flipkart", "myntra", "ajio", "shopping", "retail", "mall", "store"):
		return "Shopping"
	case containsAny(n, "electricity", "water bill", "gas bill", "broadband", "recharge", "dth", "bill pay", "sms charges", "acct charges", "account charges"):
		return "Bills & Utilities"
	case containsAny(n, "rent", "maintenance", "society"):
		return "Rent & Housing"
	case containsAny(n, "salary", "sal cr", "payroll"):
		return "Income"
	case containsAny(n, "sip", "mutual fund", "zerodha", "groww", "invest"):
		return "Investments"
	case containsAny(n, "pharma", "hospital", "medical", "apollo", "clinic", "chemist"):
		return "Health"
	case containsAny(n, "netflix", "spotify", "hotstar", "prime", "subscription"):
		return "Entertainment"
	case containsAny(n, "atm", "cash withdrawal", "cash wdl"):
		return "Cash"
	case containsAny(n, "upi", "imps", "neft", "rtgs", "transfer", "to transfer", "by transfer"):
		return "Transfers"
	default:
		return "Other"
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
