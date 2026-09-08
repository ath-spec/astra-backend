package idbimap

import (
	"strconv"
	"strings"

	"github.com/yourusername/astra-backend/internal/provider/idbi"
)

// CreditExposure is the normalised RM credit-risk view of a client, assembled
// from 442 fetchCustomerLimitDetails (limits) and 402 getLoanOverdueDetails
// (overdue aggregate).
type CreditExposure struct {
	CifID            string
	CustomerName     string
	AccountManager   string
	CustRating       string
	TotalLimit       float64
	FundedLimit      float64
	NonFundedLimit   float64
	TotalOutstanding float64
	UtilisationPct   float64

	LoanCount      int
	TotalOverdue   float64
	MaxDPD         int
	WorstNpaStatus string
	RiskBand       string // LOW / WATCH / HIGH
}

// ExposureFromLimitDetails maps a 442 response to the limits half.
func ExposureFromLimitDetails(r *idbi.FetchCustomerLimitDetailsResponse) CreditExposure {
	if r == nil {
		return CreditExposure{}
	}
	e := CreditExposure{
		CifID:            r.CustomerSummary.CustCifID,
		CustomerName:     r.CustomerSummary.CustomerName,
		AccountManager:   r.CustomerSummary.AccountManager,
		CustRating:       r.CustomerSummary.CustRating,
		TotalLimit:       AmountF(r.ExposureSummary.TotalLimit),
		FundedLimit:      AmountF(r.ExposureSummary.FundedLimit),
		NonFundedLimit:   AmountF(r.ExposureSummary.NonFundedLimit),
		TotalOutstanding: AmountF(r.ExposureSummary.TotalOutstanding),
	}
	if e.TotalLimit > 0 {
		e.UtilisationPct = round2(e.TotalOutstanding / e.TotalLimit * 100)
	}
	return e
}

// MergeOverdueSignal folds a 402 response's aggregate into the exposure and
// computes a risk band.
func MergeOverdueSignal(e CreditExposure, r *idbi.GetLoanOverdueDetailsResponse) CreditExposure {
	out := e
	if r != nil {
		out.LoanCount = len(r.OverdueDetails)
		for _, d := range r.OverdueDetails {
			ov, _ := ParseAmount(d.TotalOverdueAmt)
			out.TotalOverdue += ov
			if v, err := strconv.Atoi(strings.TrimSpace(d.Dpd)); err == nil && v > out.MaxDPD {
				out.MaxDPD = v
			}
			out.WorstNpaStatus = worseNPA(out.WorstNpaStatus, d.NpaStatus)
		}
	}
	out.RiskBand = riskBand(out)
	return out
}

func worseNPA(a, b string) string {
	rank := map[string]int{"": 0, "SA": 0, "STD": 0, "SMA-0": 1, "SMA-1": 2, "SMA-2": 3, "NPA": 4, "SUB": 4, "DBT": 5, "LOSS": 6}
	if rank[strings.ToUpper(b)] > rank[strings.ToUpper(a)] {
		return b
	}
	return a
}

func riskBand(e CreditExposure) string {
	switch {
	case e.MaxDPD >= 90 || rankNPA(e.WorstNpaStatus) >= 4 || e.TotalOverdue > 0 && e.MaxDPD >= 60:
		return "HIGH"
	case e.MaxDPD >= 1 || e.UtilisationPct >= 90 || rankNPA(e.WorstNpaStatus) >= 1:
		return "WATCH"
	default:
		return "LOW"
	}
}

func rankNPA(s string) int {
	return map[string]int{"": 0, "SA": 0, "STD": 0, "SMA-0": 1, "SMA-1": 2, "SMA-2": 3, "NPA": 4, "SUB": 4, "DBT": 5, "LOSS": 6}[strings.ToUpper(s)]
}
