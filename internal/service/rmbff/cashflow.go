package rmbff

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"

	rmbffdomain "github.com/yourusername/astra-backend/internal/domain/rmbff"
)

// kpiFor builds one cash-flow KPI tile with a 3-month-over-3-month delta when
// a prior window exists. higherIsGood says whether an increase is favourable.
func kpiFor(label, icon string, recent, prior float64, havePrior, higherIsGood bool) rmbffdomain.CashFlowKpi {
	k := rmbffdomain.CashFlowKpi{Label: label, Value: inrFull(recent), Icon: icon, Tone: "good", DeltaLabel: "vs previous 3 months"}
	if !havePrior {
		k.DeltaLabel = "no prior period"
		return k
	}
	d, ok := deltaPct(recent, prior)
	if !ok {
		return k
	}
	dir := "up"
	if d < 0 {
		dir = "down"
	}
	abs := math.Abs(d)
	k.DeltaPct, k.DeltaDirection = &abs, dir
	worse := (dir == "down") == higherIsGood
	switch {
	case abs < 5:
	case worse && abs >= 15:
		k.Tone = "risk"
	case worse:
		k.Tone = "watch"
	}
	return k
}

// CashFlow builds the CashFlowIntelligenceData view-model from the client's
// spend history. The forecast is a trailing 3-month average projection, not a
// model: it shows what the next months look like if recent behaviour holds.
func (s *Service) CashFlow(ctx context.Context, rmID uuid.UUID, isAdmin bool, userID uuid.UUID) (*rmbffdomain.CashFlowIntelligenceData, error) {
	c, err := s.loadClient(ctx, rmID, isAdmin, userID)
	if err != nil {
		return nil, err
	}
	r, p, hp := c.recent, c.prior, c.hasPrior

	trend := make([]rmbffdomain.CashFlowTrendMonth, 0, len(c.months))
	for _, m := range c.months {
		trend = append(trend, rmbffdomain.CashFlowTrendMonth{
			Label: m.month.Format("Jan"), SalaryInflow: math.Round(m.salary), OtherInflow: math.Round(m.otherInflow()),
			Expenses: math.Round(m.household()), EmiPayments: math.Round(m.emi), CreditCardPayments: math.Round(m.card),
			Investments: math.Round(m.invest), NetCashFlow: math.Round(m.net()),
		})
	}

	oblig := r.emi + r.card
	net := r.income - r.expenses
	kpis := []rmbffdomain.CashFlowKpi{
		kpiFor("Monthly Income", "income", r.income, p.income, hp, true),
		kpiFor("Monthly Expenses", "expenses", r.household, p.household, hp, false),
		kpiFor("EMI Payments", "emi", r.emi, p.emi, hp, false),
		kpiFor("Credit Card Payments", "creditCard", r.card, p.card, hp, false),
		kpiFor("Total Obligations", "obligations", oblig, p.emi+p.card, hp, false),
		kpiFor("Net Monthly Cash Flow", "netCashFlow", net, p.income-p.expenses, hp, true),
	}

	// Breakdown of the latest complete month, as a share of that month's inflow.
	breakdown := rmbffdomain.CashFlowBreakdown{MonthLabel: "Latest Month", Rows: []rmbffdomain.BreakdownRow{}}
	if n := len(c.months); n > 0 {
		m := c.months[n-1]
		breakdown.MonthLabel = m.month.Format("January 2006")
		share := func(v float64) float64 { return pctOf(v, m.inflow) }
		row := func(label string, v float64, tone string, bold bool) rmbffdomain.BreakdownRow {
			return rmbffdomain.BreakdownRow{Label: label, AmountLabel: inrFull(v), Pct: share(v), Tone: tone, Bold: bold}
		}
		breakdown.Rows = []rmbffdomain.BreakdownRow{
			row("Salary Inflow", m.salary, "inflow", false),
			row("Other Inflows", m.otherInflow(), "inflow", false),
			row("Total Inflows", m.inflow, "total", true),
			row("Household Expenses", m.household(), "outflow", false),
			row("EMI Payments", m.emi, "outflow", false),
			row("Credit Card Payments", m.card, "outflow", false),
			row("Investments (SIP/others)", m.invest, "investment", false),
			row("Total Outflows", m.outflow, "total", true),
			row("Net Monthly Cash Flow", m.net(), "total", true),
		}
	}

	// Trailing-average forecast for the next three months.
	fc := rmbffdomain.CashFlowForecast{RangeLabel: "Next 90 Days", Months: []rmbffdomain.ForecastMonth{}}
	free := r.income - r.expenses
	first := time.Now()
	for i := 1; i <= 3; i++ {
		fc.Months = append(fc.Months, rmbffdomain.ForecastMonth{
			Label: first.AddDate(0, i, 0).Format("Jan 2006"), Sublabel: fmt.Sprintf("(Next %d Days)", i*30),
			ExpectedIncome: math.Round(r.income), ExpectedObligations: math.Round(oblig), ProjectedFreeCashFlow: math.Round(free),
		})
	}

	ins := func(icon, value, label string, recent, prior float64) rmbffdomain.TransactionInsightRow {
		d := 0.0
		if hp {
			d, _ = deltaPct(recent, prior)
		}
		return rmbffdomain.TransactionInsightRow{Icon: icon, Value: value, Label: label, DeltaPct: d, DeltaLabel: "vs previous 3 months"}
	}
	insights := []rmbffdomain.TransactionInsightRow{
		ins("salary", inrFull(r.salary), "Average Salary Credit", r.salary, p.salary),
		ins("card", inrFull(r.card), "Average Card Bill", r.card, p.card),
		ins("upi", fmt.Sprintf("%.0f", r.debitCount), "Debit Transactions (monthly avg)", r.debitCount, p.debitCount),
		ins("sip", inrFull(r.invest), "Monthly Investments (SIP)", r.invest, p.invest),
	}

	factors := []string{}
	if d, ok := deltaPct(r.income, p.income); hp && ok && d <= -5 {
		factors = append(factors, fmt.Sprintf("Income fell %.0f%% compared with the previous 3 months.", -d))
	}
	if d, ok := deltaPct(r.expenses, p.expenses); hp && ok && d >= 5 {
		factors = append(factors, fmt.Sprintf("Total outflow rose %.0f%% compared with the previous 3 months.", d))
	}
	if r.income > 0 && oblig/r.income >= 0.3 {
		factors = append(factors, fmt.Sprintf("EMI and card bills take %.0f%% of income.", oblig/r.income*100))
	}
	if net < 0 {
		factors = append(factors, fmt.Sprintf("Outflow exceeds inflow by %s a month.", inrFull(-net)))
	}
	body := "Cash flow is positive and stable on recent history."
	switch {
	case net < 0:
		body = fmt.Sprintf("Spending runs %s a month above income; if this holds, buffers will keep shrinking.", inrFull(-net))
	case r.income > 0 && r.ratio() >= 0.9:
		body = "Cash flow is positive but thin; a small drop in income would turn it negative."
	}
	if len(factors) == 0 {
		factors = append(factors, "No material stress signals in the last 3 months.")
	}

	return &rmbffdomain.CashFlowIntelligenceData{
		Breadcrumb: []string{"Customers", c.name, "Cash Flow Intelligence"},
		Title:      "Cash Flow Intelligence", Subtitle: "Understand income, expenses and cash flow patterns with AI-powered insights.",
		Quote: "Better financial wellness leads to stronger futures.", QuoteAttribution: "IDBI Bank",
		AsOfLabel: time.Now().Format("Mon, 02 Jan 2006"),
		Customer:  c.summary(), Kpis: kpis,
		Trend:     rmbffdomain.CashFlowTrendSeries{RangeLabel: fmt.Sprintf("Last %d Months", len(trend)), Months: trend},
		Breakdown: breakdown,
		AiInsight: rmbffdomain.CashFlowAiInsight{Body: body, Factors: factors, CtaLabel: "View Recommended Actions"},
		Forecast:  fc,
		Next30Days: rmbffdomain.Next30Days{
			ExpectedIncomeLabel: inrFull(r.income), ExpectedObligationsLabel: inrFull(oblig), ProjectedFreeCashFlowLabel: inrFull(free),
		},
		TransactionInsights: insights,
	}, nil
}
