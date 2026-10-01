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

func stageTone(tier string) (string, string) {
	switch tier {
	case tierCritical, tierAtRisk:
		return "High", "high"
	case tierWatchlist:
		return "Moderate", "moderate"
	default:
		return "Low", "low"
	}
}

// bookMedians returns the median utilisation, DPD and bureau score across the
// book of the RM that owns the client (the "peers" for the comparison card).
func (s *Service) bookMedians(ctx context.Context, userID uuid.UUID) (util, dpd float64, score int, err error) {
	if err = s.pool.QueryRow(ctx, `
		SELECT COALESCE(percentile_cont(0.5) WITHIN GROUP (ORDER BY e.utilisation_pct), 0)::float8,
		       COALESCE(percentile_cont(0.5) WITHIN GROUP (ORDER BY e.max_dpd), 0)::float8
		FROM idbi_credit_exposure e
		JOIN users u ON u.id = e.user_id
		WHERE u.assigned_rm_id = (SELECT assigned_rm_id FROM users WHERE id = $1)`, userID,
	).Scan(&util, &dpd); err != nil {
		return 0, 0, 0, fmt.Errorf("rmbff medians: %w", err)
	}
	if s.scores == nil {
		return util, dpd, 0, nil
	}
	rows, qerr := s.pool.Query(ctx, `
		SELECT id FROM users WHERE assigned_rm_id = (SELECT assigned_rm_id FROM users WHERE id = $1)`, userID)
	if qerr != nil {
		return util, dpd, 0, nil
	}
	defer rows.Close()
	var scores []int
	for rows.Next() {
		var id uuid.UUID
		if rows.Scan(&id) != nil {
			continue
		}
		if sc, e := s.scores.Get(ctx, id); e == nil && sc.Score > 0 {
			scores = append(scores, sc.Score)
		}
	}
	if len(scores) > 0 {
		sort.Ints(scores)
		score = scores[len(scores)/2]
	}
	return util, dpd, score, nil
}

// CreditRisk builds the CreditRiskIntelligenceData view-model. The exposure
// snapshot and bureau score are point-in-time, so the score/DPD/utilisation
// "trends" and the migration path carry only the current position until
// snapshot history is stored; peers are the median of the client's own RM book.
func (s *Service) CreditRisk(ctx context.Context, rmID uuid.UUID, isAdmin bool, userID uuid.UUID) (*rmbffdomain.CreditRiskIntelligenceData, error) {
	c, err := s.loadClient(ctx, rmID, isAdmin, userID)
	if err != nil {
		return nil, err
	}
	ex, r := c.ex, c.recent
	now := time.Now()
	nowLabel := now.Format("Jan")
	score, band, factors := c.health()
	util := int(math.Round(ex.utilisation))

	chips := []rmbffdomain.CreditRiskProductChip{}
	if c.hasExposure {
		chips = append(chips, rmbffdomain.CreditRiskProductChip{
			Label: fmt.Sprintf("Credit facilities (%d)", ex.loans), ValueLabel: inrCr(ex.outstanding), Icon: "personalLoan",
		})
	}

	stageLabel, stageTone_ := stageTone(c.tier)
	migration := rmbffdomain.RiskMigration{
		Stages:      []rmbffdomain.RiskMigrationStage{{Label: stageLabel, Tone: stageTone_, DateLabel: now.Format("Jan 2006")}},
		Description: fmt.Sprintf("Currently %s risk. Earlier risk bands are not stored yet, so no migration path is shown.", stageLabel),
	}

	bureauLabel := "(bureau score unavailable)"
	rows := []rmbffdomain.BureauSummaryRow{}
	if c.cibil > 0 {
		bureauLabel = fmt.Sprintf("(CIBIL, %s source)", c.bureauSource)
		rows = append(rows,
			rmbffdomain.BureauSummaryRow{Label: "Credit Score", Value: fmt.Sprint(c.cibil)},
			rmbffdomain.BureauSummaryRow{Label: "Score Band", Value: c.cibilBand},
		)
	}
	if c.hasExposure {
		rows = append(rows,
			rmbffdomain.BureauSummaryRow{Label: "Internal Rating", Value: ex.rating},
			rmbffdomain.BureauSummaryRow{Label: "Active Facilities", Value: fmt.Sprint(ex.loans)},
			rmbffdomain.BureauSummaryRow{Label: "Total Overdue", Value: inrCr(ex.overdue)},
			rmbffdomain.BureauSummaryRow{Label: "Last Updated", Value: ex.syncedAt.Format("02 Jan 2006")},
		)
	}

	drivers := []rmbffdomain.RiskDriver{}
	add := func(h, d, impact string) {
		drivers = append(drivers, rmbffdomain.RiskDriver{Headline: h, Detail: d, Impact: impact})
	}
	if ex.dpd > 0 {
		imp := "Medium Impact"
		if ex.dpd >= 30 {
			imp = "High Impact"
		}
		add("Overdue Payments", fmt.Sprintf("%d days past due with %s overdue.", ex.dpd, inrCr(ex.overdue)), imp)
	}
	if ex.utilisation >= 60 {
		imp := "Medium Impact"
		if ex.utilisation >= 80 {
			imp = "High Impact"
		}
		add("High Credit Utilization", fmt.Sprintf("%.0f%% of the %s limit is in use.", ex.utilisation, inrCr(ex.limit)), imp)
	}
	if ratio := r.ratio(); ratio >= 0.9 {
		imp := "Medium Impact"
		if ratio >= 1 {
			imp = "High Impact"
		}
		add("Spending Close to Income", fmt.Sprintf("Outflow is %.0f%% of income on a 3-month average.", ratio*100), imp)
	}
	if r.income > 0 && (r.emi+r.card)/r.income >= 0.3 {
		add("Heavy Debt Servicing", fmt.Sprintf("EMI and card bills are %.0f%% of income.", (r.emi+r.card)/r.income*100), "Medium Impact")
	}
	if d, ok := deltaPct(r.income, c.prior.income); c.hasPrior && ok && d <= -10 {
		add("Declining Income Inflows", fmt.Sprintf("Inflows fell %.0f%% versus the previous 3 months.", -d), "High Impact")
	}

	peers := rmbffdomain.PeerComparison{CustomerLegend: "Customer", IndustryLegend: "RM book median", Metrics: []rmbffdomain.PeerComparisonMetric{}}
	if mu, md, ms, merr := s.bookMedians(ctx, userID); merr == nil {
		if c.cibil > 0 && ms > 0 {
			peers.Metrics = append(peers.Metrics, rmbffdomain.PeerComparisonMetric{
				Label: "Credit Score", CustomerValue: float64(c.cibil), IndustryValue: float64(ms),
				CustomerLabel: fmt.Sprint(c.cibil), IndustryLabel: fmt.Sprint(ms), MaxScale: 900,
			})
		}
		if c.hasExposure {
			peers.Metrics = append(peers.Metrics,
				rmbffdomain.PeerComparisonMetric{
					Label: "Credit Utilization", CustomerValue: ex.utilisation, IndustryValue: math.Round(mu),
					CustomerLabel: fmt.Sprintf("%d%%", util), IndustryLabel: fmt.Sprintf("%.0f%%", mu), MaxScale: 100,
				},
				rmbffdomain.PeerComparisonMetric{
					Label: "DPD (Days)", CustomerValue: float64(ex.dpd), IndustryValue: math.Round(md),
					CustomerLabel: fmt.Sprint(ex.dpd), IndustryLabel: fmt.Sprintf("%.0f", md),
					MaxScale: math.Max(30, math.Max(float64(ex.dpd), md)),
				})
		}
	}

	body := "Credit profile is stable with no material risk drivers."
	actions := []string{"Continue routine monitoring"}
	if c.tier != tierHealthy {
		body = fmt.Sprintf("Credit profile shows %s risk: %d days past due, %.0f%% utilisation and spending at %.0f%% of income.",
			stageLabel, ex.dpd, ex.utilisation, r.ratio()*100)
		actions = []string{
			"Discuss repayment behaviour with the customer",
			"Monitor salary credits and account activity",
			"Review restructuring or limit options if stress persists",
		}
	}

	return &rmbffdomain.CreditRiskIntelligenceData{
		Breadcrumb: []string{"Customers", c.name, "Credit & Risk Intelligence"},
		Title:      "Credit & Risk Intelligence", Subtitle: "Comprehensive credit assessment with AI-powered risk analysis.",
		AsOfLabel: now.Format("Mon, 02 Jan 2006"),
		Customer:  c.summary(), ProductChips: chips,
		HealthScore:      rmbffdomain.FinancialHealthScore{Score: score, Band: band, Factors: factors},
		ScoreTrend:       rmbffdomain.TrendSeries{RangeLabel: "Current", Points: pointIf(c.cibil > 0, nowLabel, float64(c.cibil))},
		RiskMigration:    migration,
		DpdTrend:         rmbffdomain.TrendSeries{RangeLabel: "Current", Points: pointIf(c.hasExposure, nowLabel, float64(ex.dpd))},
		UtilizationTrend: rmbffdomain.TrendSeries{RangeLabel: "Current", Points: pointIf(c.hasExposure, nowLabel, ex.utilisation)},
		ActiveAccounts: rmbffdomain.ActiveAccounts{Total: ex.loans, Slices: []rmbffdomain.DonutSlice{
			{Label: "Credit facilities", Value: float64(ex.loans), Pct: 100, Color: "#0F5C41"},
		}},
		BureauSummary:  rmbffdomain.BureauSummary{SourceLabel: bureauLabel, Rows: rows},
		RiskDrivers:    drivers,
		PeerComparison: peers,
		AiInsight:      rmbffdomain.CreditAiInsight{Body: body, Actions: actions, CtaLabel: "Create RM Action Plan"},
	}, nil
}

func pointIf(ok bool, label string, v float64) []rmbffdomain.TrendPoint {
	if !ok {
		return []rmbffdomain.TrendPoint{}
	}
	return []rmbffdomain.TrendPoint{{Label: label, Value: v}}
}
