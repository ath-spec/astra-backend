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

// bookFlow is one client-month of the RM's book: inflow, outflow, debt
// payments and salary, used to derive per-client warning signals.
type bookFlow struct {
	month                         time.Time
	inflow, outflow, debt, salary float64
}

func (s *Service) loadBookFlows(ctx context.Context, rmID uuid.UUID) (map[uuid.UUID][]bookFlow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT t.user_id, date_trunc('month', t.occurred_at),
		       COALESCE(SUM(t.amount) FILTER (WHERE t.type = 'CREDIT'), 0)::float8,
		       COALESCE(SUM(t.amount) FILTER (WHERE t.type = 'DEBIT'), 0)::float8,
		       COALESCE(SUM(t.amount) FILTER (WHERE t.type = 'DEBIT' AND t.category IN ('EMI', 'Credit Card Bill')), 0)::float8,
		       COALESCE(SUM(t.amount) FILTER (WHERE t.type = 'CREDIT' AND t.category = 'Salary'), 0)::float8
		FROM spend_transactions t
		JOIN users u ON u.id = t.user_id
		WHERE u.assigned_rm_id = $1
		  AND t.occurred_at >= date_trunc('month', now()) - INTERVAL '6 months'
		  AND t.occurred_at <  date_trunc('month', now())
		GROUP BY 1, 2 ORDER BY 1, 2`, rmID)
	if err != nil {
		return nil, fmt.Errorf("rmbff book flows: %w", err)
	}
	defer rows.Close()
	out := map[uuid.UUID][]bookFlow{}
	for rows.Next() {
		var id uuid.UUID
		var f bookFlow
		if err := rows.Scan(&id, &f.month, &f.inflow, &f.outflow, &f.debt, &f.salary); err != nil {
			return nil, err
		}
		out[id] = append(out[id], f)
	}
	return out, rows.Err()
}

func sumFlows(fs []bookFlow) (inflow, outflow, debt float64) {
	for _, f := range fs {
		inflow += f.inflow
		outflow += f.outflow
		debt += f.debt
	}
	return
}

type warning struct {
	client  bookClient
	title   string
	icon    string
	desc    string
	signals []rmbffdomain.WarningSignal
}

func severityOf(tier string) string {
	switch tier {
	case tierCritical:
		return "Critical"
	case tierAtRisk:
		return "High"
	default:
		return "Medium"
	}
}

func signalFor(label string, change float64) rmbffdomain.WarningSignal {
	dir := "up"
	if change < 0 {
		dir = "down"
	}
	return rmbffdomain.WarningSignal{Label: label, Direction: dir, Value: fmt.Sprintf("%.0f%%", math.Abs(change))}
}

// buildWarning picks the most pressing signal for a flagged client.
func (s *Service) buildWarning(c bookClient, fl []bookFlow, score int) warning {
	w := warning{client: c, title: "Elevated Risk Band", icon: "emi",
		desc: "Credit exposure snapshot flags this client as elevated risk."}

	n := len(fl)
	var recIn, recOut, recDebt, priIn, priOut float64
	if n > 0 {
		lo := n - 3
		if lo < 0 {
			lo = 0
		}
		recIn, recOut, recDebt = sumFlows(fl[lo:])
		if lo >= 1 {
			plo := lo - 3
			if plo < 0 {
				plo = 0
			}
			priIn, priOut, _ = sumFlows(fl[plo:lo])
		}
	}
	ratio, debtShare := 0.0, 0.0
	if recIn > 0 {
		ratio, debtShare = recOut/recIn, recDebt/recIn
	}
	inChange, haveIn := deltaPct(recIn, priIn)
	outChange, haveOut := deltaPct(recOut, priOut)
	if haveIn && math.Abs(inChange) >= 1 {
		w.signals = append(w.signals, signalFor("Income Inflow", inChange))
	}
	if haveOut && math.Abs(outChange) >= 1 {
		w.signals = append(w.signals, signalFor("Total Outflow", outChange))
	}
	if c.utilisation > 0 {
		w.signals = append(w.signals, rmbffdomain.WarningSignal{Label: "Credit Utilization", Direction: "up", Value: fmt.Sprintf("%.0f%%", c.utilisation)})
	}
	if debtShare > 0 {
		w.signals = append(w.signals, rmbffdomain.WarningSignal{Label: "EMI-to-Income Ratio", Direction: "up", Value: fmt.Sprintf("%.0f%%", debtShare*100)})
	}

	switch {
	case c.maxDPD > 0:
		w.title, w.icon = "Overdue Payments", "delay"
		w.desc = fmt.Sprintf("%d days past due with %s overdue.", c.maxDPD, inrCr(c.overdue))
	case recIn > 0 && ratio >= 1:
		w.title, w.icon = "EMI Stress", "emi"
		w.desc = fmt.Sprintf("Spending is %.0f%% of income; EMI and card bills take %.0f%%.", ratio*100, debtShare*100)
	case n >= 2 && fl[n-1].salary == 0 && fl[n-2].salary > 0:
		w.title, w.icon = "Salary Credit Interruption", "salary"
		w.desc = "Salary credit was not received last month."
	case haveIn && inChange <= -10:
		w.title, w.icon = "Declining Income Inflows", "salary"
		w.desc = fmt.Sprintf("Inflows fell %.0f%% versus the previous 3 months.", -inChange)
	case c.utilisation >= 80:
		w.title, w.icon = "High Credit Utilization", "utilization"
		w.desc = fmt.Sprintf("%.0f%% of the credit limit is in use.", c.utilisation)
	case score > 0 && score < 650:
		w.title, w.icon = "Weak Bureau Score", "bureau"
		w.desc = fmt.Sprintf("Bureau score is %d.", score)
	}
	return w
}

// EarlyWarnings builds the EarlyWarningsData view-model for the RM's book.
func (s *Service) EarlyWarnings(ctx context.Context, rmID uuid.UUID) (*rmbffdomain.EarlyWarningsData, error) {
	book, err := s.loadBook(ctx, rmID)
	if err != nil {
		return nil, err
	}
	flows, err := s.loadBookFlows(ctx, rmID)
	if err != nil {
		return nil, err
	}

	counts := map[string]int{}
	var flagged []bookClient
	for _, c := range book {
		counts[c.tier]++
		if c.tier != tierHealthy {
			flagged = append(flagged, c)
		}
	}
	rank := map[string]int{tierCritical: 0, tierAtRisk: 1, tierWatchlist: 2}
	sort.Slice(flagged, func(i, j int) bool {
		a, b := flagged[i], flagged[j]
		if rank[a.tier] != rank[b.tier] {
			return rank[a.tier] < rank[b.tier]
		}
		if a.maxDPD != b.maxDPD {
			return a.maxDPD > b.maxDPD
		}
		return a.outstanding > b.outstanding
	})

	n := len(book)
	kpi := func(label, tier, tone string) rmbffdomain.WarningKpi {
		return rmbffdomain.WarningKpi{Label: label, Value: fmt.Sprint(counts[tier]), Tone: tone,
			SubLabel: fmt.Sprintf("%.1f%% of portfolio", pctOf(float64(counts[tier]), float64(n)))}
	}

	scoreOf := func(id uuid.UUID) int {
		if s.scores == nil {
			return 0
		}
		if sc, e := s.scores.Get(ctx, id); e == nil {
			return sc.Score
		}
		return 0
	}
	custID := func(c bookClient) string {
		if c.cif != "" {
			return c.cif
		}
		return c.id.String()[:8]
	}

	const maxOther = 6
	var warns []warning
	for i, c := range flagged {
		if i >= maxOther+1 {
			break
		}
		warns = append(warns, s.buildWarning(c, flows[c.id], scoreOf(c.id)))
	}

	data := &rmbffdomain.EarlyWarningsData{
		Breadcrumb: []string{"Portfolio", "Early Warnings"}, Title: "Early Warning Intelligence",
		Subtitle:  "Zeyro's AI detects early signs of risk before they turn into defaults.",
		AsOfLabel: time.Now().Format("Mon, 02 Jan 2006"),
		Kpis: []rmbffdomain.WarningKpi{
			kpi("Critical", tierCritical, "critical"), kpi("High", tierAtRisk, "risk"),
			kpi("Medium", tierWatchlist, "watch"), kpi("Normal", tierHealthy, "good"),
		},
		Filters:      []rmbffdomain.FilterOption{},
		OtherAlerts:  []rmbffdomain.AlertCardData{},
		RecentAlerts: []rmbffdomain.RecentAlertItem{},
	}

	atRiskTotal := counts[tierAtRisk] + counts[tierCritical]
	needs := counts[tierWatchlist] + atRiskTotal
	data.AiInsight = rmbffdomain.PortfolioAiInsight{
		Headline: fmt.Sprintf("%d retail %s require attention", needs, plural(needs, "customer", "customers")),
		Body: fmt.Sprintf("%d at-risk or critical and %d on watchlist, flagged from days past due, utilisation and cash-flow trends.",
			atRiskTotal, counts[tierWatchlist]),
		CtaLabel: "View Priority Customers", Observations: []rmbffdomain.AiInsight{},
	}

	for i, w := range warns {
		c := w.client
		if i == 0 {
			sev := severityOf(c.tier)
			data.FeaturedAlert = &rmbffdomain.FeaturedAlert{
				Tag: upper(w.title), Severity: sev, CustomerName: c.name, CustomerID: custID(c), UserID: c.id.String(),
				Product:       fmt.Sprintf("%d %s", c.loanCount, plural(c.loanCount, "loan", "loans")),
				ExposureLabel: inrCr(c.outstanding), CreditScore: scoreOf(c.id), RiskTag: riskLabel(c.tier) + " Risk",
				Narrative: w.desc, Signals: w.signals, RiskImpact: sev, DetectedLabel: timeAgo(c.syncedAt),
			}
			continue
		}
		data.OtherAlerts = append(data.OtherAlerts, rmbffdomain.AlertCardData{
			ID: "alert-" + c.id.String(), Icon: w.icon, Title: w.title, Severity: severityOf(c.tier),
			CustomerName: c.name, CustomerID: custID(c), UserID: c.id.String(),
			Product:       fmt.Sprintf("%d %s", c.loanCount, plural(c.loanCount, "loan", "loans")),
			ExposureLabel: inrCr(c.outstanding), CreditScore: scoreOf(c.id), Description: w.desc,
			DetectedLabel: "Detected " + timeAgo(c.syncedAt),
		})
	}
	if data.FeaturedAlert != nil && data.FeaturedAlert.Signals == nil {
		data.FeaturedAlert.Signals = []rmbffdomain.WarningSignal{}
	}

	// Most recently refreshed flagged clients first.
	recent := append([]warning(nil), warns...)
	sort.Slice(recent, func(i, j int) bool {
		a, b := recent[i].client.syncedAt, recent[j].client.syncedAt
		if a == nil || b == nil {
			return b == nil && a != nil
		}
		return a.After(*b)
	})
	for i, w := range recent {
		if i == 5 {
			break
		}
		data.RecentAlerts = append(data.RecentAlerts, rmbffdomain.RecentAlertItem{
			ID: "ra-" + w.client.id.String(), Label: w.title, CustomerName: w.client.name,
			TimeAgo: timeAgo(w.client.syncedAt), Severity: severityOf(w.client.tier),
		})
	}

	// Book-wide health trend: average per-client health score for each month.
	sums, cnts := map[time.Time]int{}, map[time.Time]int{}
	for _, fs := range flows {
		for _, f := range fs {
			if f.inflow <= 0 {
				continue
			}
			sums[f.month] += healthyScore(f.outflow / f.inflow)
			cnts[f.month]++
		}
	}
	var ms []time.Time
	for m := range sums {
		ms = append(ms, m)
	}
	sort.Slice(ms, func(i, j int) bool { return ms[i].Before(ms[j]) })
	data.RiskTrend.Points = []rmbffdomain.RiskTrendPoint{}
	for _, m := range ms {
		data.RiskTrend.Points = append(data.RiskTrend.Points, rmbffdomain.RiskTrendPoint{Label: m.Format("Jan"), Score: sums[m] / cnts[m]})
	}
	if l := len(data.RiskTrend.Points); l > 0 {
		data.RiskTrend.CurrentScore = data.RiskTrend.Points[l-1].Score
	}
	return data, nil
}
