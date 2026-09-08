package idbimap

import (
	"strconv"
	"strings"

	"github.com/yourusername/astra-backend/internal/provider/idbi"
)

// Loan is the normalised view of an IDBI loan account, assembled from
// 391 getLoanAccountDetails (master) and 402 getLoanOverdueDetails (position).
type Loan struct {
	LoanAccountID      string
	CustID             string
	HolderName         string
	SanctionedAmount   float64
	DisbursedAmount    float64
	AvailableAmount    float64
	InterestRate       float64
	TenureMonths       int
	RepaymentMethod    string
	InterestMethod     string
	OpenedAt           string // raw IDBI timestamp
	OutstandingBalance float64
	OverdueAmount      float64
	DPD                int
	NpaStatus          string
	NpaDate            string
}

// LoanFromDetails maps a 391 response to a Loan (master fields only).
func LoanFromDetails(r *idbi.GetLoanAccountDetailsResponse) Loan {
	if r == nil {
		return Loan{}
	}
	res := r.Result
	rate, _ := strconv.ParseFloat(strings.TrimSpace(res.NetIntRate.Value), 64)
	tenure, _ := strconv.Atoi(strings.TrimSpace(res.LoanGenDetails.LoanPeriodMonths))
	return Loan{
		LoanAccountID:    res.LoanAcctID.AcctID,
		CustID:           res.CustID.CustID,
		HolderName:       displayName(res.CustID.PersonName),
		SanctionedAmount: MoneyF(res.LoanGenDetails.LoanAmt),
		DisbursedAmount:  MoneyF(res.AmtAlreadyDisb),
		AvailableAmount:  MoneyF(res.AmtAvailForDisb),
		InterestRate:     rate,
		TenureMonths:     tenure,
		RepaymentMethod:  res.LoanGenDetails.RePmtMethod,
		OpenedAt:         res.AcctOpenDt,
	}
}

// OverdueByAccount indexes a 402 response by loan accountId for merging.
func OverdueByAccount(r *idbi.GetLoanOverdueDetailsResponse) map[string]idbi.LoanOverdueDetail {
	out := map[string]idbi.LoanOverdueDetail{}
	if r == nil {
		return out
	}
	for _, d := range r.OverdueDetails {
		out[d.AccountID] = d
	}
	return out
}

// MergeOverdue overlays a 402 overdue record onto a Loan.
func MergeOverdue(l Loan, d idbi.LoanOverdueDetail) Loan {
	out := l
	if out.LoanAccountID == "" {
		out.LoanAccountID = d.AccountID
	}
	if out.CustID == "" {
		out.CustID = d.CustomerID
	}
	out.OutstandingBalance, _ = ParseAmount(d.OutstandingBal)
	out.OverdueAmount, _ = ParseAmount(d.TotalOverdueAmt)
	if v, err := strconv.Atoi(strings.TrimSpace(d.Dpd)); err == nil {
		out.DPD = v
	}
	out.NpaStatus = d.NpaStatus
	out.NpaDate = string(d.NpaDate)
	return out
}

// PayoffQuote is the normalised 538 inquireHPPayoff result.
type PayoffQuote struct {
	NetPayoffAmount        float64
	PendingPrincipal       float64
	PendingNormalInterest  float64
	PendingPenalInterest   float64
	PendingOverdueInterest float64
	InterestRate           float64
}

// PayoffFromInquiry maps a 538 response.
func PayoffFromInquiry(r *idbi.InquireHPPayoffResponse) PayoffQuote {
	if r == nil {
		return PayoffQuote{}
	}
	d := r.ExecuteFinacleScriptCustomData
	f := func(s string) float64 { v, _ := ParseAmount(s); return v }
	return PayoffQuote{
		NetPayoffAmount:        f(d.NetPayofamt),
		PendingPrincipal:       f(d.PendingPrincipal),
		PendingNormalInterest:  f(d.PendingNormalInterest),
		PendingPenalInterest:   f(d.PendingPenalInterest),
		PendingOverdueInterest: f(d.PendingOverdueInterest),
		InterestRate:           f(d.InterestRate),
	}
}
