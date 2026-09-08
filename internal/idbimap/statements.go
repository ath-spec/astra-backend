package idbimap

import (
	"strings"

	analytics "github.com/yourusername/astra-backend/internal/domain/analytics"
	"github.com/yourusername/astra-backend/internal/provider/idbi"
)

// txnType normalises IDBI's two conventions: "D"/"C" (393) and
// "DEBIT"/"CREDIT" (595/739).
func txnType(code string) analytics.TxnType {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case "C", "CREDIT", "CR":
		return analytics.TxnCredit
	default:
		return analytics.TxnDebit
	}
}

// FullStatementTransactions maps a 393 getFullAccountStatement page to the
// analytics engine's Transaction type. Category is left empty — IDBI's txnCat
// is an internal code ("TCI"), not a spend category; the categoriser runs
// downstream. Merchant is the raw narration (generic in sandbox).
func FullStatementTransactions(resp *idbi.FullStatementResponse) []analytics.Transaction {
	if resp == nil {
		return nil
	}
	rows := resp.Result.TransactionDetails
	out := make([]analytics.Transaction, 0, len(rows))
	for _, r := range rows {
		amt, _ := ParseAmount(r.TransactionSummary.TxnAmt.AmountValue)
		out = append(out, analytics.Transaction{
			ID:         r.TxnID,
			Amount:     amt,
			Type:       txnType(r.TransactionSummary.TxnType),
			Category:   "",
			Merchant:   strings.TrimSpace(r.TransactionSummary.TxnDesc),
			OccurredAt: MustParseTime(firstNonEmpty(r.TransactionSummary.TxnDate, r.ValueDate, r.PstdDate)),
		})
	}
	return out
}

// AAStatementTransactions maps a 595/739 AA statement account block to
// analytics Transactions. Narration here is richer than 393; mode (UPI/NEFT/…)
// is folded into the merchant string as a prefix hint for the categoriser.
func AAStatementTransactions(acc idbi.AAStatementAccount) []analytics.Transaction {
	rows := acc.Transactions.Transaction
	out := make([]analytics.Transaction, 0, len(rows))
	for _, r := range rows {
		amt, _ := ParseAmount(r.Amount)
		merchant := strings.TrimSpace(r.Narration)
		out = append(out, analytics.Transaction{
			ID:         r.TxnID,
			Amount:     amt,
			Type:       txnType(r.Type),
			Category:   "",
			Merchant:   merchant,
			OccurredAt: MustParseTime(firstNonEmpty(r.TransactionTimestamp, r.ValueDate)),
		})
	}
	return out
}

// AAStatementAllTransactions flattens every account block in a 595/739 response.
func AAStatementAllTransactions(resp *idbi.GetAAStatementResponse) []analytics.Transaction {
	if resp == nil {
		return nil
	}
	var out []analytics.Transaction
	for _, acc := range resp.Data {
		out = append(out, AAStatementTransactions(acc)...)
	}
	return out
}

func firstNonEmpty(ss ...string) string {
	for _, s := range ss {
		if strings.TrimSpace(s) != "" && s != "NULL" {
			return s
		}
	}
	return ""
}
