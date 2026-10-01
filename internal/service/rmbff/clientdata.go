package rmbff

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	rmbffdomain "github.com/yourusername/astra-backend/internal/domain/rmbff"
)

// monthFlow is one complete calendar month of a client's spend history,
// split into the buckets the RM screens need.
type monthFlow struct {
	month      time.Time
	inflow     float64
	salary     float64
	outflow    float64
	emi        float64
	card       float64
	invest     float64
	debitCount int
}

func (m monthFlow) otherInflow() float64 { return m.inflow - m.salary }
func (m monthFlow) debt() float64        { return m.emi + m.card }

// household is everyday spend: outflow that is not EMI, card bill or investment.
func (m monthFlow) household() float64 { return m.outflow - m.emi - m.card - m.invest }
func (m monthFlow) net() float64       { return m.inflow - m.outflow }

// pressure is outflow / inflow for the month (0 when there is no inflow).
func (m monthFlow) pressure() float64 {
	if m.inflow <= 0 {
		return 0
	}
	return m.outflow / m.inflow
}

// healthyScore maps spend pressure to 0-100, higher is healthier.
func healthyScore(pressure float64) int {
	return clampInt(int(math.Round((1.25-pressure)/0.55*100)), 0, 100)
}

type exposureSnap struct {
	cif                                      string
	limit, outstanding, overdue, utilisation float64
	loans, dpd                               int
	band, rating                             string
	syncedAt                                 time.Time
}

// avgs are 3-month averages over the most recent complete months.
type avgs struct {
	income, expenses, emi, card, invest, salary, household float64
	debitCount                                             float64
}

func (a avgs) ratio() float64 {
	if a.income <= 0 {
		return 0
	}
	return a.expenses / a.income
}

func averageOf(ms []monthFlow) avgs {
	var a avgs
	for _, m := range ms {
		a.income += m.inflow
		a.expenses += m.outflow
		a.emi += m.emi
		a.card += m.card
		a.invest += m.invest
		a.salary += m.salary
		a.household += m.household()
		a.debitCount += float64(m.debitCount)
	}
	if n := float64(len(ms)); n > 0 {
		a.income, a.expenses, a.emi, a.card, a.invest, a.salary, a.household, a.debitCount =
			a.income/n, a.expenses/n, a.emi/n, a.card/n, a.invest/n, a.salary/n, a.household/n, a.debitCount/n
	}
	return a
}

// clientData is everything the per-client RM screens derive from.
type clientData struct {
	userID      uuid.UUID
	name, phone string
	joined      time.Time

	ex          exposureSnap
	hasExposure bool
	tier        string

	months   []monthFlow // up to 6 complete months, oldest first
	recent   avgs        // last 3 complete months
	prior    avgs        // the 3 complete months before that
	hasPrior bool

	cibil        int
	cibilBand    string
	bureauSource string
	bureauAt     time.Time
}

func (c clientData) custID() string {
	if c.ex.cif != "" {
		return c.ex.cif
	}
	return c.userID.String()[:8]
}

func (c clientData) segment() string {
	if c.recent.salary > 0 {
		return "Salaried"
	}
	return "Retail"
}

func (c clientData) summary() rmbffdomain.CashFlowCustomerSummary {
	return rmbffdomain.CashFlowCustomerSummary{
		Name: c.name, Initials: initials(c.name), CustomerID: c.custID(), Segment: c.segment(),
		City:     "",
		Products: fmt.Sprintf("%d %s", c.ex.loans, plural(c.ex.loans, "facility", "facilities")),
		RiskTag:  riskLabel(c.tier),
	}
}

// health returns the 0-100 financial health score, its band and factors.
func (c clientData) health() (int, string, []rmbffdomain.HealthScoreFactor) {
	income, ratio := c.recent.income, c.recent.ratio()
	savings := clampInt(int(math.Round((1-ratio)/0.3*100)), 0, 100)
	if income <= 0 {
		savings = 0
	}
	bureau := 0
	if c.cibil > 0 {
		bureau = clampInt(int(math.Round(float64(c.cibil-300)/600*100)), 0, 100)
	}
	util := clampInt(100-int(math.Round(c.ex.utilisation)), 0, 100)
	repay := clampInt(100-int(math.Round(float64(c.ex.dpd)*100/90)), 0, 100)
	burden := 100
	if income > 0 {
		burden = clampInt(100-int(math.Round((c.recent.emi+c.recent.card)/income*250)), 0, 100)
	}
	factors := []rmbffdomain.HealthScoreFactor{
		{Label: "Repayment Behaviour", Score: repay},
		{Label: "Cash Flow", Score: savings},
		{Label: "Credit Utilization", Score: util},
		{Label: "Bureau Health", Score: bureau},
		{Label: "Liability Burden", Score: burden},
	}
	total := 0
	for _, f := range factors {
		total += f.Score
	}
	score := total / len(factors)
	band := "Critical"
	switch {
	case score >= 75:
		band = "Healthy"
	case score >= 55:
		band = "Watch"
	case score >= 35:
		band = "At Risk"
	}
	return score, band, factors
}

// loadClient authorises the caller for the client and gathers the shared data.
func (s *Service) loadClient(ctx context.Context, rmID uuid.UUID, isAdmin bool, userID uuid.UUID) (*clientData, error) {
	if err := s.rm.AuthorizeClient(ctx, rmID, isAdmin, userID); err != nil {
		return nil, err
	}
	c := &clientData{userID: userID, cibilBand: "—"}
	if err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(name, 'Client'), COALESCE(phone_number, ''), created_at FROM users WHERE id = $1`, userID,
	).Scan(&c.name, &c.phone, &c.joined); err != nil {
		return nil, fmt.Errorf("rmbff client: %w", err)
	}

	err := s.pool.QueryRow(ctx, `
		SELECT cif_id, cust_rating, total_limit::float8, total_outstanding::float8, total_overdue::float8,
		       utilisation_pct::float8, loan_count, max_dpd, risk_band, synced_at
		FROM idbi_credit_exposure WHERE user_id = $1`, userID,
	).Scan(&c.ex.cif, &c.ex.rating, &c.ex.limit, &c.ex.outstanding, &c.ex.overdue,
		&c.ex.utilisation, &c.ex.loans, &c.ex.dpd, &c.ex.band, &c.ex.syncedAt)
	switch {
	case err == nil:
		c.hasExposure = true
	case errors.Is(err, pgx.ErrNoRows):
	default:
		return nil, fmt.Errorf("rmbff exposure: %w", err)
	}
	c.tier = tierFor(c.ex.band, c.ex.dpd)

	rows, err := s.pool.Query(ctx, `
		SELECT date_trunc('month', occurred_at) AS m,
		       COALESCE(SUM(amount) FILTER (WHERE type = 'CREDIT'), 0)::float8,
		       COALESCE(SUM(amount) FILTER (WHERE type = 'CREDIT' AND category = 'Salary'), 0)::float8,
		       COALESCE(SUM(amount) FILTER (WHERE type = 'DEBIT'), 0)::float8,
		       COALESCE(SUM(amount) FILTER (WHERE type = 'DEBIT' AND category = 'EMI'), 0)::float8,
		       COALESCE(SUM(amount) FILTER (WHERE type = 'DEBIT' AND category = 'Credit Card Bill'), 0)::float8,
		       COALESCE(SUM(amount) FILTER (WHERE type = 'DEBIT' AND (category ILIKE '%invest%' OR category ILIKE '%sip%' OR category ILIKE '%mutual%')), 0)::float8,
		       COUNT(*) FILTER (WHERE type = 'DEBIT')
		FROM spend_transactions
		WHERE user_id = $1
		  AND occurred_at >= date_trunc('month', now()) - INTERVAL '9 months'
		  AND occurred_at <  date_trunc('month', now())
		GROUP BY 1 ORDER BY 1`, userID)
	if err != nil {
		return nil, fmt.Errorf("rmbff flows: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var m monthFlow
		if err := rows.Scan(&m.month, &m.inflow, &m.salary, &m.outflow, &m.emi, &m.card, &m.invest, &m.debitCount); err != nil {
			return nil, err
		}
		c.months = append(c.months, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	n := len(c.months)
	lo := n - 3
	if lo < 0 {
		lo = 0
	}
	c.recent = averageOf(c.months[lo:])
	if n >= 6 {
		c.prior = averageOf(c.months[n-6 : n-3])
		c.hasPrior = true
	}

	if s.scores != nil {
		if sc, serr := s.scores.Get(ctx, userID); serr == nil {
			c.cibil, c.cibilBand, c.bureauSource, c.bureauAt = sc.Score, sc.Band, sc.Source, sc.GeneratedAt
		}
	}
	return c, nil
}

// deltaPct is the % change from prior to recent, one decimal; ok=false if prior is 0.
// The base is |prior| so a negative figure that gets worse (-66k to -90k) reads
// as a decrease, not an increase.
func deltaPct(recent, prior float64) (float64, bool) {
	if prior == 0 {
		return 0, false
	}
	return math.Round((recent-prior)/math.Abs(prior)*1000) / 10, true
}
