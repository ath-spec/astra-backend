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

// LoanLimits is the normalised 441 fetchLoanAccountLimits result: the current
// sanctioned limit and drawing power, plus their history (newest first).
type LoanLimits struct {
	SanctionedLimit float64      `json:"sanctioned_limit"`
	DrawingPower    float64      `json:"drawing_power"`
	SanctionHistory []LimitPoint `json:"sanction_history"`
	DrawingHistory  []LimitPoint `json:"drawing_history"`
}

// LimitPoint is one dated entry in a limit history.
type LimitPoint struct {
	EffectiveDate string  `json:"effective_date"`
	ExpiryDate    string  `json:"expiry_date,omitempty"`
	Amount        float64 `json:"amount"`
	Percent       float64 `json:"percent"`
}

// LoanLimitsFromResponse maps a 441 response. The "current" values are the
// entry with the latest applicableDate in each list.
func LoanLimitsFromResponse(r *idbi.FetchLoanAccountLimitsResponse) LoanLimits {
	if r == nil {
		return LoanLimits{}
	}
	toPoints := func(in []idbi.LimitHistItem, useSanct bool) []LimitPoint {
		out := make([]LimitPoint, 0, len(in))
		for _, it := range in {
			amt := MoneyF(it.DrwngPower)
			if useSanct {
				amt = MoneyF(it.SanctLimit)
			}
			pct, _ := strconv.ParseFloat(strings.TrimSpace(it.DrwngPowerPcnt.Value), 64)
			out = append(out, LimitPoint{
				EffectiveDate: it.ApplicableDate,
				ExpiryDate:    it.ExpiryDate,
				Amount:        amt,
				Percent:       pct,
			})
		}
		sortByDateDesc(out)
		return out
	}
	dp := toPoints(r.AccountLimitDetails.AcctDrwngPowerLimitHistMsgInq.OlimitLL, false)
	sl := toPoints(r.AccountLimitDetails.AcctSanctLimitHistMsg.OlimitLL, true)

	out := LoanLimits{DrawingHistory: dp, SanctionHistory: sl}
	if len(dp) > 0 {
		out.DrawingPower = dp[0].Amount
	}
	if len(sl) > 0 {
		out.SanctionedLimit = sl[0].Amount
	}
	return out
}

func sortByDateDesc(p []LimitPoint) {
	for i := 1; i < len(p); i++ {
		for j := i; j > 0 && p[j-1].EffectiveDate < p[j].EffectiveDate; j-- {
			p[j-1], p[j] = p[j], p[j-1]
		}
	}
}

// LoanOverduePosition is the normalised 404 getLoanOverduePositionEnquiry
// result for one loan account: principal vs interest, demanded / collected /
// overdue.
type LoanOverduePosition struct {
	LoanAccountID      string  `json:"loan_account_id"`
	PrincipalDemanded  float64 `json:"principal_demanded"`
	PrincipalCollected float64 `json:"principal_collected"`
	PrincipalOverdue   float64 `json:"principal_overdue"`
	InterestDemanded   float64 `json:"interest_demanded"`
	InterestCollected  float64 `json:"interest_collected"`
	InterestOverdue    float64 `json:"interest_overdue"`
}

// OverduePositionByAccount indexes a 404 response by loan accountId.
func OverduePositionByAccount(r *idbi.GetLoanOverduePositionResponse) map[string]LoanOverduePosition {
	out := map[string]LoanOverduePosition{}
	if r == nil {
		return out
	}
	for _, rec := range r.LoanOvduRec {
		id := rec.AcctID.AcctID
		out[id] = LoanOverduePosition{
			LoanAccountID:      id,
			PrincipalDemanded:  MoneyF(rec.PTotalDmd),
			PrincipalCollected: MoneyF(rec.PTotalColl),
			PrincipalOverdue:   MoneyF(rec.PTotalOvdu),
			InterestDemanded:   MoneyF(rec.TotalIntDmd),
			InterestCollected:  MoneyF(rec.TotalIntColl),
			InterestOverdue:    MoneyF(rec.TotalIntOvdu),
		}
	}
	return out
}

// RepaymentScheduleRow is one amortisation row from 473.
type RepaymentScheduleRow struct {
	Serial               int     `json:"serial"`
	DueDate              string  `json:"due_date"`
	InstalmentAmount     float64 `json:"instalment_amount"`
	PrincipalComponent   float64 `json:"principal_component"`
	InterestComponent    float64 `json:"interest_component"`
	CumulativePrincipal  float64 `json:"cumulative_principal"`
	CumulativeInterest   float64 `json:"cumulative_interest"`
	PrincipalOutstanding float64 `json:"principal_outstanding"`
}

// RepaymentSchedule is the normalised 473 result.
type RepaymentSchedule struct {
	InstalmentAmount float64                `json:"instalment_amount"`
	InstalmentCount  int                    `json:"instalment_count"`
	Frequency        string                 `json:"frequency"`
	Rows             []RepaymentScheduleRow `json:"rows"`
}

// RepaymentScheduleFromResponse maps a 473 response.
func RepaymentScheduleFromResponse(r *idbi.GenerateRepaymentScheduleResponse) RepaymentSchedule {
	if r == nil {
		return RepaymentSchedule{}
	}
	vo := r.LoanModellingSchOutputVO
	out := RepaymentSchedule{Rows: make([]RepaymentScheduleRow, 0, len(vo.OamortLL))}
	if len(vo.LamodRepaymentLL) > 0 {
		h := vo.LamodRepaymentLL[0]
		out.InstalmentAmount = MoneyF(h.FlowAmt)
		out.InstalmentCount, _ = strconv.Atoi(strings.TrimSpace(h.NoOfInstalments))
		out.Frequency = h.Freq
	}
	for _, a := range vo.OamortLL {
		sn, _ := strconv.Atoi(strings.TrimSpace(a.Key.SerialNum))
		out.Rows = append(out.Rows, RepaymentScheduleRow{
			Serial:               sn,
			DueDate:              a.AmortStruct.FlowDate,
			InstalmentAmount:     MoneyF(a.AmortStruct.InstlAmt),
			PrincipalComponent:   MoneyF(a.AmortStruct.PrincAmt),
			InterestComponent:    MoneyF(a.AmortStruct.IntAmt),
			CumulativePrincipal:  MoneyF(a.AmortStruct.CummPrincAmt),
			CumulativeInterest:   MoneyF(a.AmortStruct.CummIntAmt),
			PrincipalOutstanding: MoneyF(a.AmortStruct.PrincOutStanding),
		})
	}
	return out
}
