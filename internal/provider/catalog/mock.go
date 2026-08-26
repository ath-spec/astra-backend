package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"math/rand"
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
		&f.FundManager, &f.BenchmarkIndex, &launchDate,
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
	fund_manager, benchmark_index, launch_date`

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
	// A fund with no allocation row yet (e.g. a newly added catalog entry
	// with migration 000012 not re-run for it) still returns a valid
	// profile — just with an empty breakdown rather than an error.
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

	return &catalogdomain.FundProfile{
		Fund: *fund,
		Allocation: catalogdomain.AllocationBreakdown{
			EquityPct: equityPct, DebtPct: debtPct, OtherPct: otherPct,
			Sectors: sectors, TopHoldings: holdings,
		},
		ChartPoints: chartPoints(schemeCode, fund.NAV, returns1Y),
	}, nil
}

// chartPoints synthesizes a 12-month NAV history trending from the fund's
// disclosed 1-year return to its current catalog NAV, with a small
// deterministic day-to-day jitter layered on top for a non-linear look.
// There is no real historical NAV feed behind this mock catalog — this is
// documented reference data, not a recorded price series.
func chartPoints(schemeCode string, currentNAV, returns1YPct float64) []catalogdomain.ChartPoint {
	const months = 12
	startNAV := currentNAV
	if returns1YPct > -100 {
		startNAV = currentNAV / (1 + returns1YPct/100)
	}

	now := time.Now().UTC()
	points := make([]catalogdomain.ChartPoint, 0, months+1)
	for i := 0; i <= months; i++ {
		t := float64(i) / float64(months)
		trendNAV := startNAV + (currentNAV-startNAV)*t
		date := now.AddDate(0, -(months - i), 0)
		nav := dayJitter(schemeCode, trendNAV, date)
		if i == months {
			nav = currentNAV // last point always matches the fund's live NAV
		}
		points = append(points, catalogdomain.ChartPoint{Date: apitime.New(date), NAV: round4(nav)})
	}
	return points
}

// dayJitter applies the same deterministic +/-1% day-bucketed NAV move the
// MF investment domain uses (internal/provider/mf.navOnDate) — duplicated
// here rather than imported to keep the Catalog and MF packages independent
// (catalog reference data doesn't depend on a user ever holding anything).
func dayJitter(schemeCode string, baseNAV float64, date time.Time) float64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(schemeCode))
	bucket := date.UTC().Truncate(24*time.Hour).Unix() / 86400
	r := rand.New(rand.NewSource(int64(h.Sum64()) + bucket)) //nolint:gosec // mock market data, not security-sensitive
	pctMove := (r.Float64() - 0.5) * 0.02
	return baseNAV * (1 + pctMove)
}

func round4(v float64) float64 { return math.Round(v*10000) / 10000 }

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
