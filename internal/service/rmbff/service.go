// Package rmbff assembles the RM console's view-model payloads (one HTTP call
// per screen) from the existing RMService reads and the IDBI credit-exposure
// snapshot. It adds no new data sources — only aggregation and formatting.
package rmbff

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	rmbffdomain "github.com/yourusername/astra-backend/internal/domain/rmbff"
	"github.com/yourusername/astra-backend/internal/service"
	"github.com/yourusername/astra-backend/internal/service/creditscore"
)

// ScoreSource supplies a client's bureau score; optional (nil leaves scores 0).
type ScoreSource interface {
	Get(ctx context.Context, userID uuid.UUID) (creditscore.Score, error)
}

type Service struct {
	rm     *service.RMService
	pool   *pgxpool.Pool
	scores ScoreSource
}

func New(rm *service.RMService, pool *pgxpool.Pool, scores ScoreSource) *Service {
	return &Service{rm: rm, pool: pool, scores: scores}
}

// exposureDonut shows funded vs non-funded limits: the exposure snapshot has
// no product-level split.
func exposureDonut(funded, nonFunded float64) rmbffdomain.LoanProductExposure {
	t := funded + nonFunded
	return rmbffdomain.LoanProductExposure{TotalCr: toCr(t), Products: []rmbffdomain.LoanProductSlice{
		{Label: "Funded", ValueCr: toCr(funded), Pct: pctOf(funded, t), Color: "#0F5C41"},
		{Label: "Non-funded", ValueCr: toCr(nonFunded), Pct: pctOf(nonFunded, t), Color: "#7FB8A0"},
	}}
}

const (
	tierHealthy   = "healthy"
	tierWatchlist = "watchlist"
	tierAtRisk    = "atRisk"
	tierCritical  = "critical"

	// criticalDPD is the days-past-due at which a HIGH-band client is
	// treated as Critical (RBI NPA threshold).
	criticalDPD = 90
)

// bookClient is one client of the RM's book joined to its credit-exposure
// snapshot. Clients with no snapshot carry no risk signal and count as healthy.
type bookClient struct {
	id          uuid.UUID
	name        string
	loanCount   int
	outstanding float64
	funded      float64
	nonFunded   float64
	overdue     float64
	maxDPD      int
	utilisation float64
	riskBand    string
	tier        string
	cif         string
	syncedAt    *time.Time
}

func tierFor(band string, dpd int) string {
	switch band {
	case "HIGH":
		if dpd >= criticalDPD {
			return tierCritical
		}
		return tierAtRisk
	case "WATCH":
		return tierWatchlist
	default:
		return tierHealthy
	}
}

func (s *Service) loadBook(ctx context.Context, rmID uuid.UUID) ([]bookClient, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT u.id, COALESCE(u.name, 'Client'),
		       COALESCE(e.loan_count, 0), COALESCE(e.total_outstanding, 0)::float8,
		       COALESCE(e.funded_limit, 0)::float8, COALESCE(e.non_funded_limit, 0)::float8,
		       COALESCE(e.total_overdue, 0)::float8, COALESCE(e.max_dpd, 0),
		       COALESCE(e.utilisation_pct, 0)::float8, COALESCE(e.risk_band, ''),
		       COALESCE(e.cif_id, ''), e.synced_at
		FROM users u
		LEFT JOIN idbi_credit_exposure e ON e.user_id = u.id
		WHERE u.assigned_rm_id = $1
	`, rmID)
	if err != nil {
		return nil, fmt.Errorf("rmbff book: %w", err)
	}
	defer rows.Close()
	var out []bookClient
	for rows.Next() {
		var c bookClient
		if err := rows.Scan(&c.id, &c.name, &c.loanCount, &c.outstanding, &c.funded, &c.nonFunded,
			&c.overdue, &c.maxDPD, &c.utilisation, &c.riskBand, &c.cif, &c.syncedAt); err != nil {
			return nil, fmt.Errorf("rmbff book scan: %w", err)
		}
		c.tier = tierFor(c.riskBand, c.maxDPD)
		out = append(out, c)
	}
	return out, rows.Err()
}

func keyIssue(c bookClient) string {
	switch {
	case c.maxDPD > 0 && c.overdue > 0:
		return fmt.Sprintf("%d days past due, %s overdue", c.maxDPD, inrCr(c.overdue))
	case c.maxDPD > 0:
		return fmt.Sprintf("%d days past due", c.maxDPD)
	case c.utilisation >= 80:
		return fmt.Sprintf("Limit utilisation at %.0f%%", c.utilisation)
	default:
		return "Elevated risk band"
	}
}

func riskLabel(tier string) string {
	switch tier {
	case tierCritical:
		return "Critical"
	case tierAtRisk:
		return "High"
	default:
		return "Watchlist"
	}
}

// Dashboard builds the DashboardData view-model for one RM.
func (s *Service) Dashboard(ctx context.Context, rmID uuid.UUID) (*rmbffdomain.DashboardData, error) {
	var name string
	if err := s.pool.QueryRow(ctx, `SELECT name FROM rm_users WHERE id = $1`, rmID).Scan(&name); err != nil {
		return nil, fmt.Errorf("rmbff rm lookup: %w", err)
	}
	book, err := s.loadBook(ctx, rmID)
	if err != nil {
		return nil, err
	}
	followUps, err := s.rm.PendingFollowUps(ctx, rmID)
	if err != nil {
		return nil, err
	}
	insights, err := s.rm.BookInsights(ctx, rmID)
	if err != nil {
		return nil, err
	}

	var total, funded, nonFunded float64
	var loans int
	counts := map[string]int{}
	for _, c := range book {
		total += c.outstanding
		funded += c.funded
		nonFunded += c.nonFunded
		loans += c.loanCount
		counts[c.tier]++
	}
	n := len(book)
	kpi := func(label, tier, tone string) rmbffdomain.KpiFigure {
		return rmbffdomain.KpiFigure{
			Label: label, Value: fmt.Sprint(counts[tier]), Tone: tone,
			SubLabel: fmt.Sprintf("%.0f%% of portfolio", pctOf(float64(counts[tier]), float64(n))),
		}
	}

	// Priority customers: worst tier first, then highest DPD, then exposure.
	var flagged []bookClient
	for _, c := range book {
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
	priority := make([]rmbffdomain.PriorityCustomer, 0, 5)
	for i, c := range flagged {
		if i == 5 {
			break
		}
		priority = append(priority, rmbffdomain.PriorityCustomer{
			ID: c.id.String(), Name: c.name,
			Product:          fmt.Sprintf("%d %s", c.loanCount, plural(c.loanCount, "loan", "loans")),
			OutstandingLabel: inrCr(c.outstanding), DaysPastDue: c.maxDPD,
			Risk: riskLabel(c.tier), KeyIssue: keyIssue(c),
		})
	}

	segment := func(label, tier, color string) rmbffdomain.PortfolioHealthSegment {
		return rmbffdomain.PortfolioHealthSegment{
			Label: label, Count: counts[tier], Pct: pctOf(float64(counts[tier]), float64(n)), Color: color,
		}
	}
	band := func(label, tier, tone string) rmbffdomain.RiskBand {
		return rmbffdomain.RiskBand{
			Label: label, Count: counts[tier], Pct: pctOf(float64(counts[tier]), float64(n)), Tone: tone,
		}
	}

	overdueCount := 0
	for _, f := range followUps {
		if f.Overdue {
			overdueCount++
		}
	}
	atRiskTotal := counts[tierAtRisk] + counts[tierCritical]

	summary := rmbffdomain.AiPortfolioSummary{
		Badge:    "Zeyro AI",
		Headline: fmt.Sprintf("%d of %d clients need attention", counts[tierWatchlist]+atRiskTotal, n),
		Body: fmt.Sprintf("%d at-risk or critical and %d on watchlist across %s outstanding.",
			atRiskTotal, counts[tierWatchlist], inrCr(total)),
		Insights: []rmbffdomain.AiInsight{},
		CtaLabel: "Review priority customers",
	}
	if counts[tierCritical] > 0 {
		summary.Insights = append(summary.Insights, rmbffdomain.AiInsight{
			Severity: "critical", Text: fmt.Sprintf("%d %s 90+ days past due.", counts[tierCritical], plural(counts[tierCritical], "client is", "clients are")),
		})
	}
	if overdueCount > 0 {
		summary.Insights = append(summary.Insights, rmbffdomain.AiInsight{
			Severity: "warning", Text: fmt.Sprintf("%d follow-up %s overdue.", overdueCount, plural(overdueCount, "is", "are")),
		})
	}

	return &rmbffdomain.DashboardData{
		GreetingName:     firstName(name),
		BusinessUnit:     "Retail Banking",
		AsOfLabel:        time.Now().Format("Mon, 02 Jan 2006"),
		Quote:            "Relationships are built one conversation at a time.",
		QuoteAttribution: "Zeyro",
		Kpis: rmbffdomain.DashboardKpis{
			TotalExposure: rmbffdomain.KpiFigure{Label: "Total Exposure", Value: inrCr(total), Tone: "neutral"},
			Customers:     rmbffdomain.KpiFigure{Label: "Customers", Value: fmt.Sprint(n), Tone: "neutral"},
			ActiveLoans:   rmbffdomain.KpiFigure{Label: "Active Loans", Value: fmt.Sprint(loans), Tone: "neutral"},
			Healthy:       kpi("Healthy", tierHealthy, "good"),
			Watchlist:     kpi("Watchlist", tierWatchlist, "watch"),
			AtRisk:        kpi("At Risk", tierAtRisk, "risk"),
			Critical:      kpi("Critical", tierCritical, "critical"),
		},
		AiSummary: summary,
		PortfolioHealth: rmbffdomain.PortfolioHealth{TotalCustomers: n, Segments: []rmbffdomain.PortfolioHealthSegment{
			segment("Healthy", tierHealthy, "#0F5C41"),
			segment("Watchlist", tierWatchlist, "#D9A21B"),
			segment("At Risk", tierAtRisk, "#E8742A"),
			segment("Critical", tierCritical, "#C8382E"),
		}},
		// The exposure snapshot has no product-level split, so the donut
		// shows funded vs non-funded limits instead.
		LoanExposure: exposureDonut(funded, nonFunded),
		// No repayment history is stored yet; empty months, zeroed headline.
		Repayment: rmbffdomain.RepaymentPerformance{RangeLabel: "Last 6 months", Months: []rmbffdomain.RepaymentMonth{}},
		RiskDistribution: []rmbffdomain.RiskBand{
			band("Healthy", tierHealthy, "excellent"),
			band("Watchlist", tierWatchlist, "fair"),
			band("At Risk", tierAtRisk, "poor"),
			band("Critical", tierCritical, "high-risk"),
		},
		PriorityCustomers: priority,
		WorkItems: []rmbffdomain.WorkItem{
			{ID: "review", Label: "At-risk accounts to review", Count: atRiskTotal, Icon: "review"},
			{ID: "followup", Label: "Pending follow-ups", Count: len(followUps), Icon: "followup"},
			{ID: "crosssell", Label: "Suggested next actions", Count: len(insights.NextBestActions), Icon: "crosssell"},
		},
		WorkFocusQuote: "Start with the accounts that can't wait.",
	}, nil
}
