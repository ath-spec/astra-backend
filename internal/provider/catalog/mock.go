package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourusername/astra-backend/internal/apiresponse"
	"github.com/yourusername/astra-backend/internal/apitime"
	catalogdomain "github.com/yourusername/astra-backend/internal/domain/catalog"
)

const (
	defaultLimit = 20
	maxLimit     = 100
)

// MockProvider reads the fund_catalog/nfo_listings reference tables seeded
// by migration 000003. It's named "Mock" for consistency with the other
// swappable providers, even though this data is closer to "static reference
// data" than a simulated exchange — a real provider would call an AMFI/RTA
// feed here instead of Postgres.
type MockProvider struct {
	pool *pgxpool.Pool
}

func NewMockProvider(pool *pgxpool.Pool) *MockProvider {
	return &MockProvider{pool: pool}
}

// scanner is satisfied by both pgx.Row (QueryRow) and pgx.Rows (Query), so
// the same scan logic serves both a single-fund lookup and a search list.
type scanner interface {
	Scan(dest ...any) error
}

func scanFund(row scanner) (catalogdomain.Fund, error) {
	var f catalogdomain.Fund
	var navDate time.Time
	var launchDate *time.Time
	err := row.Scan(
		&f.SchemeCode, &f.SchemeName, &f.AMCName, &f.ISIN, &f.Category, &f.RiskLevel, &f.NAV, &navDate,
		&f.ExpenseRatio, &f.AUM, &f.MinInvestment, &f.MinSIPAmount, &f.Returns1Y, &f.Returns3Y, &f.Returns5Y,
		&f.FundManager, &f.BenchmarkIndex, &launchDate, &f.ExitLoadPct, &f.ExitLoadPeriodDays,
	)
	if err != nil {
		return catalogdomain.Fund{}, fmt.Errorf("scan fund: %w", err)
	}
	f.NAVDate = apitime.New(navDate)
	f.LaunchDate = apitime.NewPtr(launchDate)
	return f, nil
}

const fundColumns = `scheme_code, scheme_name, amc_name, isin, category, risk_level, nav, nav_date,
	expense_ratio, aum, min_investment, min_sip_amount, returns_1y, returns_3y, returns_5y,
	fund_manager, benchmark_index, launch_date, exit_load_pct, exit_load_period_days`

func (p *MockProvider) SearchFunds(ctx context.Context, params catalogdomain.SearchParams) ([]catalogdomain.Fund, error) {
	page := params.Page
	if page < 1 {
		page = 1
	}
	limit := params.Limit
	if limit <= 0 || limit > maxLimit {
		limit = defaultLimit
	}
	offset := (page - 1) * limit

	rows, err := p.pool.Query(ctx, `
		SELECT `+fundColumns+`
		FROM fund_catalog
		WHERE ($1 = '' OR category = $1)
		  AND ($2 = '' OR risk_level = $2)
		  AND ($3::numeric IS NULL OR min_investment <= $3)
		ORDER BY scheme_name
		LIMIT $4 OFFSET $5
	`, params.Category, params.RiskLevel, params.MinInvestmentMax, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("search funds: %w", err)
	}
	defer rows.Close()

	funds := make([]catalogdomain.Fund, 0)
	for rows.Next() {
		f, err := scanFund(rows)
		if err != nil {
			return nil, err
		}
		funds = append(funds, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate funds: %w", err)
	}
	return funds, nil
}

func (p *MockProvider) GetFund(ctx context.Context, schemeCode string) (*catalogdomain.Fund, error) {
	row := p.pool.QueryRow(ctx, `SELECT `+fundColumns+` FROM fund_catalog WHERE scheme_code = $1`, schemeCode)
	f, err := scanFund(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apiresponse.NotFound("fund %s not found", schemeCode)
		}
		return nil, err
	}
	return &f, nil
}

func deriveFundDeepDive(category, schemeName string) catalogdomain.DeepDiveInfo {
	lower := strings.ToLower(category + " " + schemeName)
	if strings.Contains(lower, "small") || strings.Contains(lower, "mid") {
		return catalogdomain.DeepDiveInfo{
			PrimaryRole:   "High Alpha Growth",
			SecondaryRole: "Capital Compounding",
			Strengths:     "High compounding runway and market-beating alpha during expansion cycles.",
			TradeOffs:     "Elevated near-term volatility and sharper market corrections.",
			Contribution:  "Targets fast-growing emerging companies to accelerate long-term capital compounding.",
		}
	} else if strings.Contains(lower, "tech") || strings.Contains(lower, "semi") || strings.Contains(lower, "energy") || strings.Contains(lower, "thematic") || strings.Contains(lower, "auto") {
		return catalogdomain.DeepDiveInfo{
			PrimaryRole:   "Thematic Growth",
			SecondaryRole: "Sector Alpha",
			Strengths:     "Concentrated capital efficiency in high-conviction secular mega-trends.",
			TradeOffs:     "Cyclical sector dependence and higher tracking risk.",
			Contribution:  fmt.Sprintf("Provides high-conviction exposure to industry tailwinds in %s.", category),
		}
	} else if strings.Contains(lower, "debt") || strings.Contains(lower, "liquid") || strings.Contains(lower, "hybrid") || strings.Contains(lower, "bond") || strings.Contains(lower, "conservative") {
		return catalogdomain.DeepDiveInfo{
			PrimaryRole:   "Income & Stability",
			SecondaryRole: "Downside Protection",
			Strengths:     "Predictable yields, low drawdown risk, and high capital security.",
			TradeOffs:     "Lower long-term compounding rate compared to pure equity.",
			Contribution:  "Provides steady yield generation and cushions overall equity portfolio drawdowns.",
		}
	} else if strings.Contains(lower, "gold") || strings.Contains(lower, "reit") || strings.Contains(lower, "silver") {
		return catalogdomain.DeepDiveInfo{
			PrimaryRole:   "Inflation Defense",
			SecondaryRole: "Real Asset Hedge",
			Strengths:     "Uncorrelated returns with fiat currency and equity market volatility.",
			TradeOffs:     "No regular corporate cash flows or dividend yields.",
			Contribution:  "Acts as an inflation hedge and provides uncorrelated asset protection during volatile markets.",
		}
	}
	return catalogdomain.DeepDiveInfo{
		PrimaryRole:   "Core Growth",
		SecondaryRole: "Capital Preservation",
		Strengths:     "High return on capital, dominant moat, and resilient free cash flows.",
		TradeOffs:     "Tracks broader benchmark returns with moderate upside beta.",
		Contribution:  "Provides stability and consistent growth by investing in established market leaders.",
	}
}

func deriveFundInsights(category, schemeName string, equityPct, debtPct, otherPct float64) catalogdomain.FundInsights {
	lower := strings.ToLower(category + " " + schemeName)
	var whyGet, suitable, avoid, impact, doesNow, buyMore string

	if strings.Contains(lower, "small") || strings.Contains(lower, "mid") {
		whyGet = "Delivers high alpha potential and outsized long-term capital compounding."
		suitable = "Long-term investors with 5+ year time horizon seeking high wealth creation."
		avoid = "Investors seeking short-term liquidity or low risk tolerance."
		impact = "Increases growth beta and expected annualized portfolio returns."
		doesNow = "Currently provides aggressive mid/small-cap compounding, maximizing long-term wealth appreciation."
		buyMore = "Adding more will tilt your portfolio towards higher alpha while slightly increasing market beta."
	} else if strings.Contains(lower, "thematic") || strings.Contains(lower, "tech") || strings.Contains(lower, "semi") {
		whyGet = fmt.Sprintf("High-conviction capture of structural expansion in %s.", category)
		suitable = "Investors wanting targeted sector upside alongside a balanced core portfolio."
		avoid = "Risk-averse investors needing steady dividend distributions."
		impact = "Concentrates capital into high-growth thematic innovators."
		doesNow = fmt.Sprintf("Currently delivers high-conviction thematic growth targeting secular tailwinds in %s.", category)
		buyMore = fmt.Sprintf("Adding more will increase sector momentum exposure in %s.", category)
	} else if strings.Contains(lower, "debt") || strings.Contains(lower, "bond") || strings.Contains(lower, "hybrid") {
		whyGet = "Generates consistent accrual yield and preserves capital across market downturns."
		suitable = "Conservative investors seeking stable cash flow and portfolio defensiveness."
		avoid = "Aggressive growth investors targeting maximum equity alpha."
		impact = "Reduces portfolio standard deviation and downside drawdown."
		doesNow = "Currently provides consistent yield and capital preservation, anchoring the portfolio against market volatility."
		buyMore = "Adding more will pull your overall portfolio towards Capital Preservation and Income."
	} else if strings.Contains(lower, "gold") || strings.Contains(lower, "reit") || strings.Contains(lower, "silver") {
		whyGet = "Protects purchasing power against currency debasement and stagflation."
		suitable = "All portfolios requiring 5-10% hard asset diversification."
		avoid = "Short-term speculative trades."
		impact = "Lowers total portfolio correlation with equity indices."
		doesNow = "Currently protects against inflation and market downturns through hard commodity diversification."
		buyMore = "Adding more will strengthen your Inflation Defense and Real Asset vectors."
	} else {
		whyGet = "Provides solid, resilient equity exposure across India's largest and most established bluechip companies."
		suitable = "Core long-term compounding with low tracking error."
		avoid = "Investors expecting triple-digit short-term returns."
		impact = "Anchors the core portfolio and maintains balanced risk exposure."
		doesNow = "Currently provides large-cap equity exposure, balancing out the volatility of higher growth holdings."
		buyMore = "Adding more will pull your overall portfolio slightly towards Capital Preservation while maintaining steady compound growth."
	}

	eqFactor := math.Min(math.Max(equityPct/100.0, 0.15), 0.95)
	debtFactor := math.Min(math.Max(debtPct/100.0, 0.10), 0.85)
	otherFactor := math.Min(math.Max(otherPct/100.0, 0.08), 0.75)

	currValues := []float64{
		math.Round(eqFactor*100) / 100,
		math.Round(debtFactor*100) / 100,
		math.Round(math.Max(debtFactor, 0.45)*100) / 100,
		math.Round(otherFactor*100) / 100,
		0.70,
		0.55,
		math.Round(otherFactor*100) / 100,
	}

	projValues := make([]float64, len(currValues))
	for i, v := range currValues {
		projValues[i] = math.Round(math.Min(v*1.15, 0.98)*100) / 100
	}

	return catalogdomain.FundInsights{
		IsPositiveImpact:     true,
		WhyGetFund:           whyGet,
		SuitableFor:          suitable,
		AvoidIf:              avoid,
		ImpactText:           impact,
		WhatItDoesRightNow:   doesNow,
		WhatBuyingMoreWillDo: buyMore,
		CurrentValues:        currValues,
		ProjectedValues:      projValues,
	}
}

func (p *MockProvider) GetFundProfile(ctx context.Context, schemeCode string) (*catalogdomain.FundProfile, error) {
	fund, err := p.GetFund(ctx, schemeCode)
	if err != nil {
		return nil, err
	}

	var equityPct, debtPct, otherPct float64
	var sectorsJSON, holdingsJSON []byte
	err = p.pool.QueryRow(ctx, `
		SELECT equity_pct, debt_pct, other_pct, sectors, top_holdings FROM fund_allocation WHERE scheme_code = $1
	`, schemeCode).Scan(&equityPct, &debtPct, &otherPct, &sectorsJSON, &holdingsJSON)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("lookup fund allocation: %w", err)
	}
	var sectors, holdings []catalogdomain.DistributionItem
	if len(sectorsJSON) > 0 {
		if err := json.Unmarshal(sectorsJSON, &sectors); err != nil {
			return nil, fmt.Errorf("decode fund sectors: %w", err)
		}
	}
	if len(holdingsJSON) > 0 {
		if err := json.Unmarshal(holdingsJSON, &holdings); err != nil {
			return nil, fmt.Errorf("decode fund top holdings: %w", err)
		}
	}

	returns1Y := 0.0
	if fund.Returns1Y != nil {
		returns1Y = *fund.Returns1Y
	}

	deepDive := deriveFundDeepDive(fund.Category, fund.SchemeName)
	insights := deriveFundInsights(fund.Category, fund.SchemeName, equityPct, debtPct, otherPct)

	return &catalogdomain.FundProfile{
		Fund: *fund,
		Allocation: catalogdomain.AllocationBreakdown{
			EquityPct: equityPct, DebtPct: debtPct, OtherPct: otherPct,
			Sectors: sectors, TopHoldings: holdings,
		},
		ChartPoints: chartPoints(schemeCode, fund.NAV, returns1Y, 180),
		DeepDive:    deepDive,
		Insights:    insights,
	}, nil
}

func (p *MockProvider) ListNFOs(ctx context.Context) ([]catalogdomain.NFO, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT nfo_id, scheme_name, amc_name, category, offer_open_date, offer_close_date, offer_price, min_investment, allotment_date
		FROM nfo_listings
		ORDER BY offer_open_date
	`)
	if err != nil {
		return nil, fmt.Errorf("list nfos: %w", err)
	}
	defer rows.Close()

	nfos := make([]catalogdomain.NFO, 0)
	for rows.Next() {
		var n catalogdomain.NFO
		var openDate, closeDate time.Time
		var allotDate *time.Time
		if err := rows.Scan(&n.NFOID, &n.SchemeName, &n.AMCName, &n.Category, &openDate, &closeDate, &n.OfferPrice, &n.MinInvestment, &allotDate); err != nil {
			return nil, fmt.Errorf("scan nfo: %w", err)
		}
		n.OfferOpenDate = apitime.New(openDate)
		n.OfferCloseDate = apitime.New(closeDate)
		n.AllotmentDate = apitime.NewPtr(allotDate)
		nfos = append(nfos, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate nfos: %w", err)
	}
	return nfos, nil
}

func dayJitter(schemeCode string, base float64, date time.Time) float64 {
	h := fnv.New64a()
	h.Write([]byte(fmt.Sprintf("%s:%s", schemeCode, date.Format("2006-01-02"))))
	rng := rand.New(rand.NewSource(int64(h.Sum64())))
	jitter := (rng.Float64() - 0.5) * 0.02 * base
	return math.Round((base+jitter)*100) / 100
}

func chartPoints(schemeCode string, currentNAV, returns1Y float64, daysOpt ...int) []catalogdomain.ChartPoint {
	days := 13
	if len(daysOpt) > 0 && daysOpt[0] > 0 {
		days = daysOpt[0]
	}
	startNAV := currentNAV / (1.0 + (returns1Y / 100.0))
	if startNAV <= 0 {
		startNAV = currentNAV * 0.8
	}

	points := make([]catalogdomain.ChartPoint, days)
	now := time.Now().UTC().Truncate(24 * time.Hour)

	for i := 0; i < days; i++ {
		t := float64(i) / float64(days-1)
		base := startNAV + (currentNAV-startNAV)*t
		date := now.AddDate(0, 0, -(days - 1 - i))
		nav := dayJitter(schemeCode, base, date)
		if i == days-1 {
			nav = currentNAV
		}
		points[i] = catalogdomain.ChartPoint{
			Date: apitime.New(date),
			NAV:  nav,
		}
	}
	return points
}
