package catalog

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourusername/astra-backend/internal/apiresponse"
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
	f.NAVDate = navDate.Format("2006-01-02")
	if launchDate != nil {
		d := launchDate.Format("2006-01-02")
		f.LaunchDate = &d
	}
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
		n.OfferOpenDate = openDate.Format("2006-01-02")
		n.OfferCloseDate = closeDate.Format("2006-01-02")
		if allotDate != nil {
			d := allotDate.Format("2006-01-02")
			n.AllotmentDate = &d
		}
		nfos = append(nfos, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate nfos: %w", err)
	}
	return nfos, nil
}
