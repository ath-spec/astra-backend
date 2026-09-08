package idbimap

import (
	"strings"

	"github.com/yourusername/astra-backend/internal/provider/idbi"
)

// Account is the normalised view of an IDBI deposit account, assembled from
// 365 (performAccountEnquiry) and/or 394 (getCustomerAccountsByCustId). It is
// deliberately a flat, typed struct — the service layer maps it onto the
// idbi_accounts table and onto whatever the dashboard/accounts screens need.
type Account struct {
	AccountNumber string
	AccountType   string // SAVINGS, CURRENT, TERM_DEPOSIT, SALARY, SBA, ...
	Currency      string
	CustID        string
	CifID         string
	HolderName    string
	Status        string // "A" = active (from 365) or "ACTIVE"
	BranchID      string
	BranchName    string
	OpenedAt      string // raw IDBI timestamp; parse with ParseTime

	// Balances. LedgerBalance is the headline figure; AvailableBalance is
	// spendable; LienAmount is locked. Zero if the source API didn't provide
	// that particular type.
	LedgerBalance    float64
	AvailableBalance float64
	EffectiveBalance float64
	LienAmount       float64
}

// AccountFromEnquiry maps a 365 response to an Account. This is the richer
// source — it carries custId, holder name, branch, registered address, and all
// seven balance types.
func AccountFromEnquiry(r *idbi.PerformAccountEnquiryResponse) Account {
	if r == nil {
		return Account{}
	}
	a := Account{
		AccountNumber: r.AcctID,
		AccountType:   firstNonEmpty(r.AcctType.SchmType, ""),
		Currency:      r.AcctCurr,
		CustID:        r.CustID,
		HolderName:    displayName(r.PersonName),
		Status:        r.BankAcctStatusCode,
		BranchID:      r.BankInfo.BranchID,
		BranchName:    r.BankInfo.BranchName,
		OpenedAt:      r.AcctOpenDt,
	}
	a.LedgerBalance, _ = ParseAmount(r.Balance("LEDGER"))
	a.AvailableBalance, _ = ParseAmount(r.Balance("AVAIL"))
	a.EffectiveBalance, _ = ParseAmount(r.Balance("EFFAVL"))
	a.LienAmount, _ = ParseAmount(r.Balance("LIEN"))
	if a.LedgerBalance == 0 {
		// some accounts only carry ACCBAL
		a.LedgerBalance, _ = ParseAmount(r.Balance("ACCBAL"))
	}
	return a
}

// AccountsFromCustomerList maps a 394 response to a slice of Accounts. This
// source is lighter — one balance figure, a coarse type, no custId per row
// (the response is keyed by cifId, set here on every account).
func AccountsFromCustomerList(r *idbi.GetCustomerAccountsByCustIDResponse) []Account {
	if r == nil {
		return nil
	}
	out := make([]Account, 0, len(r.CustomerAccountInfo))
	for _, ai := range r.CustomerAccountInfo {
		bal, _ := ParseAmount(ai.AcctBalance.AmountValue)
		out = append(out, Account{
			AccountNumber:    ai.AcctNumber,
			AccountType:      ai.AcctType,
			Currency:         ai.AcctCurrCode,
			CifID:            r.CifID,
			LedgerBalance:    bal,
			AvailableBalance: bal,
		})
	}
	return out
}

// MergeAccount overlays the richer 365 fields onto a 394-derived account,
// keeping non-zero balances and non-empty identifiers from whichever source
// had them.
func MergeAccount(base, enrich Account) Account {
	out := base
	if enrich.CustID != "" {
		out.CustID = enrich.CustID
	}
	if enrich.CifID != "" {
		out.CifID = enrich.CifID
	}
	if enrich.HolderName != "" {
		out.HolderName = enrich.HolderName
	}
	if enrich.Status != "" {
		out.Status = enrich.Status
	}
	if enrich.BranchID != "" {
		out.BranchID = enrich.BranchID
	}
	if enrich.BranchName != "" {
		out.BranchName = enrich.BranchName
	}
	if enrich.OpenedAt != "" {
		out.OpenedAt = enrich.OpenedAt
	}
	if enrich.AccountType != "" {
		out.AccountType = enrich.AccountType
	}
	if enrich.LedgerBalance != 0 {
		out.LedgerBalance = enrich.LedgerBalance
	}
	if enrich.AvailableBalance != 0 {
		out.AvailableBalance = enrich.AvailableBalance
	}
	if enrich.EffectiveBalance != 0 {
		out.EffectiveBalance = enrich.EffectiveBalance
	}
	if enrich.LienAmount != 0 {
		out.LienAmount = enrich.LienAmount
	}
	return out
}

func displayName(n idbi.PersonName) string {
	parts := make([]string, 0, 3)
	for _, p := range []string{n.FirstName, n.MiddleName, n.LastName} {
		if s := strings.TrimSpace(p); s != "" {
			parts = append(parts, s)
		}
	}
	if len(parts) == 0 {
		return strings.TrimSpace(n.Name)
	}
	return strings.Join(parts, " ")
}
