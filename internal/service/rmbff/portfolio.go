package rmbff

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	rmbffdomain "github.com/yourusername/astra-backend/internal/domain/rmbff"
)

const portfolioPageSize = 10

// PortfolioQuery carries the ?page=, ?tab= and ?search= filters.
type PortfolioQuery struct {
	Page   int
	Tab    string // all | watchlist | at-risk | critical
	Search string
}

func tabTier(tab string) (string, bool) {
	switch tab {
	case "watchlist":
		return tierWatchlist, true
	case "at-risk":
		return tierAtRisk, true
	case "critical":
		return tierCritical, true
	}
	return "", false
}

func rowRisk(tier string) string {
	switch tier {
	case tierCritical:
		return "Critical"
	case tierAtRisk:
		return "High"
	case tierWatchlist:
		return "Medium"
	default:
		return "Low"
	}
}

func timeAgo(t *time.Time) string {
	if t == nil {
		return "—"
	}
	d := time.Since(*t)
	switch {
	case d < time.Hour:
		return "Just now"
	case d < 24*time.Hour:
		h := int(d.Hours())
		return fmt.Sprintf("%d %s ago", h, plural(h, "hour", "hours"))
	default:
		days := int(math.Floor(d.Hours() / 24))
		return fmt.Sprintf("%d %s ago", days, plural(days, "day", "days"))
	}
}

// Portfolio builds the PortfolioData view-model: KPIs and tab counts over the
// whole book, and one filtered, exposure-sorted page of customer rows.
func (s *Service) Portfolio(ctx context.Context, rmID uuid.UUID, q PortfolioQuery) (*rmbffdomain.PortfolioData, error) {
	book, err := s.loadBook(ctx, rmID)
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

	filtered := make([]bookClient, 0, len(book))
	wantTier, tiered := tabTier(q.Tab)
	needle := strings.ToLower(strings.TrimSpace(q.Search))
	for _, c := range book {
		if tiered && c.tier != wantTier {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(c.name), needle) &&
			!strings.Contains(strings.ToLower(c.cif), needle) {
			continue
		}
		filtered = append(filtered, c)
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].outstanding > filtered[j].outstanding })

	pages := (len(filtered) + portfolioPageSize - 1) / portfolioPageSize
	if pages == 0 {
		pages = 1
	}
	page := q.Page
	if page < 1 {
		page = 1
	}
	if page > pages {
		page = pages
	}
	lo := (page - 1) * portfolioPageSize
	hi := lo + portfolioPageSize
	if hi > len(filtered) {
		hi = len(filtered)
	}

	rows := make([]rmbffdomain.CustomerRow, 0, hi-lo)
	for _, c := range filtered[lo:hi] {
		custID := c.cif
		if custID == "" {
			custID = strings.ToUpper(c.id.String()[:8])
		}
		score := 0
		if s.scores != nil {
			if sc, err := s.scores.Get(ctx, c.id); err == nil {
				score = sc.Score
			}
		}
		rows = append(rows, rmbffdomain.CustomerRow{
			ID: c.id.String(), Name: c.name, CustomerID: custID,
			Product:       fmt.Sprintf("%d %s", c.loanCount, plural(c.loanCount, "loan", "loans")),
			Region:        "—",
			ExposureLabel: inrCr(c.outstanding),
			CreditScore:   score,
			// No cash-flow trend is computed per client here yet.
			CashFlowTrend: rmbffdomain.CashFlowTrend{Kind: "stable"},
			DPD:           c.maxDPD, Risk: rowRisk(c.tier), LastUpdated: timeAgo(c.syncedAt),
		})
	}

	n := len(book)
	atRiskTotal := counts[tierAtRisk] + counts[tierCritical]
	obs := []rmbffdomain.AiInsight{}
	if counts[tierCritical] > 0 {
		obs = append(obs, rmbffdomain.AiInsight{Severity: "critical",
			Text: fmt.Sprintf("%d %s 90+ days past due.", counts[tierCritical], plural(counts[tierCritical], "client is", "clients are"))})
	}
	if len(insights.NextBestActions) > 0 {
		obs = append(obs, rmbffdomain.AiInsight{Severity: "warning",
			Text: fmt.Sprintf("%d suggested next %s queued.", len(insights.NextBestActions), plural(len(insights.NextBestActions), "action", "actions"))})
	}

	neutral := func(label, value, tone string) rmbffdomain.PortfolioKpi {
		return rmbffdomain.PortfolioKpi{Label: label, Value: value, Tone: tone}
	}
	return &rmbffdomain.PortfolioData{
		Breadcrumb:       []string{"Portfolio", "Retail Customers"},
		Title:            "Retail Customer Portfolio",
		Subtitle:         "View and manage your retail customer portfolio with real-time risk and financial insights.",
		Quote:            "Customer first, progress together.",
		QuoteAttribution: "IDBI Bank",
		AsOfLabel:        time.Now().Format("Mon, 02 Jan 2006"),
		Kpis: rmbffdomain.PortfolioKpis{
			TotalCustomers: neutral("Total Customers", fmt.Sprint(n), "good"),
			TotalExposure:  neutral("Total Exposure", inrCr(total), "good"),
			ActiveLoans:    neutral("Active Loans", fmt.Sprint(loans), "info"),
			Watchlist:      neutral("Watchlist", fmt.Sprint(counts[tierWatchlist]), "watch"),
			AtRisk:         neutral("At Risk", fmt.Sprint(counts[tierAtRisk]), "risk"),
			Critical:       neutral("Critical", fmt.Sprint(counts[tierCritical]), "critical"),
		},
		Filters: []rmbffdomain.FilterOption{},
		Tabs: []rmbffdomain.PortfolioTab{
			{Key: "all", Label: "All Customers", Count: n},
			{Key: "watchlist", Label: "Watchlist", Count: counts[tierWatchlist]},
			{Key: "at-risk", Label: "At Risk", Count: counts[tierAtRisk]},
			{Key: "critical", Label: "Critical", Count: counts[tierCritical]},
		},
		SortLabel:      "Exposure (High to Low)",
		Customers:      rows,
		TotalCustomers: len(filtered),
		TotalPages:     pages,
		CurrentPage:    page,
		RowsPerPage:    portfolioPageSize,
		AiInsight: rmbffdomain.PortfolioAiInsight{
			Headline:     fmt.Sprintf("%d retail %s require attention", counts[tierWatchlist]+atRiskTotal, plural(counts[tierWatchlist]+atRiskTotal, "customer", "customers")),
			Body:         fmt.Sprintf("%d at-risk or critical and %d on watchlist across %s outstanding.", atRiskTotal, counts[tierWatchlist], inrCr(total)),
			CtaLabel:     "View Priority Customers",
			Observations: obs,
		},
		LoanExposure:           exposureDonut(funded, nonFunded),
		FooterQuote:            "Identifying opportunities. Enabling better tomorrows.",
		FooterQuoteAttribution: "IDBI Bank",
	}, nil
}
