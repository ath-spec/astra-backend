package rmbff

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/google/uuid"

	rmbffdomain "github.com/yourusername/astra-backend/internal/domain/rmbff"
)

// Customer360 builds the Customer360Data view-model. Cash-flow, risk-score
// and health figures are derived from the client's spend history; the credit
// profile comes from the IDBI credit-exposure snapshot and the bureau score
// service. Fields with no source (gender, age, e-mail, address, occupation,
// per-loan terms, period-over-period deltas) are omitted or shown as "—".
func (s *Service) Customer360(ctx context.Context, rmID uuid.UUID, isAdmin bool, userID uuid.UUID) (*rmbffdomain.Customer360Data, error) {
	c, err := s.loadClient(ctx, rmID, isAdmin, userID)
	if err != nil {
		return nil, err
	}
	name, phone, joined, ex, hasExposure, tier := c.name, c.phone, c.joined, c.ex, c.hasExposure, c.tier
	months := c.months
	if len(months) > 6 {
		months = months[len(months)-6:]
	}
	income, expenses, emi, salary := c.recent.income, c.recent.expenses, c.recent.emi+c.recent.card, c.recent.salary
	ratio := c.recent.ratio()
	recentLen := len(c.months)
	if recentLen > 3 {
		recentLen = 3
	}
	cibil, cibilBand := c.cibil, c.cibilBand

	// Bank-balance series from the daily portfolio snapshots (last 90 days).
	balance := rmbffdomain.BalanceTrend{RangeLabel: "Last 3 Months", Points: []rmbffdomain.TrendPoint{}, AvgBalanceLabel: "—"}
	if hist, herr := s.rm.PortfolioHistory(ctx, rmID, isAdmin, userID, 90); herr == nil && len(hist.AllocationSeries) > 0 {
		series := hist.AllocationSeries
		sort.Slice(series, func(i, j int) bool { return series[i].Date.Time().Before(series[j].Date.Time()) })
		step := (len(series) + 11) / 12 // at most ~12 points
		var sum float64
		for _, p := range series {
			sum += p.BankValue
		}
		for i := 0; i < len(series); i += step {
			balance.Points = append(balance.Points, rmbffdomain.TrendPoint{
				Label: series[i].Date.Time().Format("02 Jan"), Value: math.Round(series[i].BankValue),
			})
		}
		balance.AvgBalanceLabel = inrFull(sum / float64(len(series)))
		if half := len(series) / 2; half > 0 {
			var first, second float64
			for _, p := range series[:half] {
				first += p.BankValue
			}
			for _, p := range series[half:] {
				second += p.BankValue
			}
			first, second = first/float64(half), second/float64(len(series)-half)
			if first > 0 {
				d := math.Round((second-first)/first*100*10) / 10
				balance.DeltaPct = &d
				balance.DeltaLabel = "vs previous half of the period"
			}
		}
	}

	// Recent activity: latest transactions.
	arows, err := s.pool.Query(ctx, `
		SELECT id, occurred_at, type, category, merchant, amount::float8
		FROM spend_transactions WHERE user_id = $1
		ORDER BY occurred_at DESC LIMIT 6`, userID)
	if err != nil {
		return nil, fmt.Errorf("rmbff activity: %w", err)
	}
	activity := []rmbffdomain.ActivityItem{}
	for arows.Next() {
		var id uuid.UUID
		var at time.Time
		var typ, cat, merch string
		var amt float64
		if err := arows.Scan(&id, &at, &typ, &cat, &merch, &amt); err != nil {
			arows.Close()
			return nil, err
		}
		tone, desc := "neutral", merch
		if typ == "CREDIT" {
			tone, desc = "good", cat+" credited"
		} else if cat == "EMI" || cat == "Credit Card Bill" {
			desc = cat + " paid — " + merch
		}
		activity = append(activity, rmbffdomain.ActivityItem{
			ID: id.String(), Date: at.Format("02 Jan 2006"), Description: desc, AmountLabel: inrFull(amt), Tone: tone,
		})
	}
	arows.Close()

	// Cash flow and risk-score trend (score: higher is healthier).
	cash := make([]rmbffdomain.CashFlowMonth, 0, len(months))
	risk := make([]rmbffdomain.TrendPoint, 0, len(months))
	for _, m := range months {
		cash = append(cash, rmbffdomain.CashFlowMonth{
			Label: m.month.Format("Jan"), Inflow: math.Round(m.inflow), Outflow: math.Round(m.outflow), Net: math.Round(m.inflow - m.outflow),
		})
		risk = append(risk, rmbffdomain.TrendPoint{
			Label: m.month.Format("Jan"), Value: float64(healthyScore(m.pressure())),
		})
	}

	score, band, factors := c.health()

	insights := []rmbffdomain.RiskInsightPoint{}
	if income > 0 {
		insights = append(insights, rmbffdomain.RiskInsightPoint{
			Headline: fmt.Sprintf("Spending is %.0f%% of income", ratio*100),
			Detail:   fmt.Sprintf("Average monthly outflow %s against inflow %s over the last %d months.", inrFull(expenses), inrFull(income), recentLen),
		})
	}
	if emi > 0 {
		insights = append(insights, rmbffdomain.RiskInsightPoint{
			Headline: fmt.Sprintf("Debt payments take %.0f%% of income", emi/income*100),
			Detail:   fmt.Sprintf("EMI and card bills average %s a month.", inrFull(emi)),
		})
	}
	if hasExposure && ex.utilisation >= 70 {
		insights = append(insights, rmbffdomain.RiskInsightPoint{
			Headline: fmt.Sprintf("Credit utilisation at %.0f%%", ex.utilisation),
			Detail:   fmt.Sprintf("%s outstanding against a %s limit.", inrCr(ex.outstanding), inrCr(ex.limit)),
		})
	}
	if ex.dpd > 0 {
		insights = append(insights, rmbffdomain.RiskInsightPoint{
			Headline: fmt.Sprintf("%d days past due", ex.dpd),
			Detail:   fmt.Sprintf("%s overdue across %d %s.", inrCr(ex.overdue), ex.loans, plural(ex.loans, "facility", "facilities")),
		})
	}

	tag, body := "Stable", "No material stress signals in the last few months."
	if tier != tierHealthy {
		tag = riskLabel(tier)
		body = fmt.Sprintf("Spending is %.0f%% of income with %.0f%% credit utilisation and %d days past due. Review repayment capacity before the next EMI cycle.", ratio*100, ex.utilisation, ex.dpd)
	}

	utilBand := "Low"
	if ex.utilisation >= 70 {
		utilBand = "High"
	} else if ex.utilisation >= 40 {
		utilBand = "Moderate"
	}
	custID := ex.cif
	if custID == "" {
		custID = userID.String()[:8]
	}
	tags := []string{riskLabel(tier)}
	if tier == tierHealthy {
		tags = []string{"Healthy"}
	}
	segment := "Retail"
	if salary > 0 {
		segment = "Salaried"
		tags = append(tags, "Salaried")
	}

	var loansRows []rmbffdomain.ActiveLoanRow
	if hasExposure && ex.loans > 0 {
		loansRows = append(loansRows, rmbffdomain.ActiveLoanRow{
			Product: fmt.Sprintf("Credit facilities (%d)", ex.loans), Sanctioned: inrCr(ex.limit),
			Outstanding: inrCr(ex.outstanding), EmiMonthly: inrFull(emi), Roi: "—", NextDue: "—",
		})
	}
	if loansRows == nil {
		loansRows = []rmbffdomain.ActiveLoanRow{}
	}

	return &rmbffdomain.Customer360Data{
		Breadcrumb: []string{"Customers", "Customer 360"}, BusinessUnit: "Retail Banking",
		AsOfLabel: time.Now().Format("Mon, 02 Jan 2006"),
		Profile: rmbffdomain.CustomerProfile{
			Name: name, Initials: initials(name), RiskTag: riskLabel(tier), Segment: segment,
			Products:   fmt.Sprintf("%d %s", ex.loans, plural(ex.loans, "facility", "facilities")),
			CustomerID: custID, SinceYear: fmt.Sprint(joined.Year()),
			Phone: phone, Email: "—", Address: "—", Occupation: "—",
			MonthlyIncomeLabel: inrFull(income), CustomerSinceLabel: joined.Format("Jan 2006"), Tags: tags,
		},
		HealthScore: rmbffdomain.FinancialHealthScore{Score: score, Band: band, Factors: factors},
		AiInsight:   rmbffdomain.CustomerAiInsight{Tag: tag, Body: body, CtaLabel: "View Detailed Analysis"},
		Stats: []rmbffdomain.CustomerStat{
			{Label: "Total Relationship Exposure", Value: inrCr(ex.limit), Icon: "exposure"},
			{Label: "Outstanding Loans", Value: inrCr(ex.outstanding), Icon: "loans"},
			{Label: "Total EMI (Monthly)", Value: inrFull(emi), Icon: "emi"},
			{Label: "Average Account Balance", Value: balance.AvgBalanceLabel, Icon: "balance"},
			{Label: "Monthly Income", Value: inrFull(income), Icon: "income"},
			{Label: "Monthly Expenses", Value: inrFull(c.recent.household), Icon: "expenses"},
		},
		ActiveLoans: loansRows,
		CreditProfile: rmbffdomain.CreditProfile{
			CibilScore: cibil, CibilBand: cibilBand,
			UtilizationPct: int(math.Round(ex.utilisation)), UtilizationBand: utilBand,
			TotalActiveAccounts: ex.loans, TotalEmiObligations: inrFull(emi),
			// The snapshot has no DPD history, so the 12-month high is the current figure.
			CurrentDpd: ex.dpd, HighestDpd12m: ex.dpd,
		},
		RecentActivity: activity,
		BalanceTrend:   balance,
		CashFlow:       rmbffdomain.CashFlowSeries{RangeLabel: "Last 6 Months", Months: cash},
		RiskScoreTrend: rmbffdomain.RiskScoreTrend{RangeLabel: "Last 6 Months", Points: risk},
		RiskInsights:   insights,
	}, nil
}
