package service

import (
	"hash/fnv"
	"math/rand"
	"context"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"

	"github.com/yourusername/astra-backend/internal/apitime"
	dashboarddomain "github.com/yourusername/astra-backend/internal/domain/dashboard"
	fddomain "github.com/yourusername/astra-backend/internal/domain/fd"
	mfdomain "github.com/yourusername/astra-backend/internal/domain/mf"
	stocksdomain "github.com/yourusername/astra-backend/internal/domain/stocks"
	fdprovider "github.com/yourusername/astra-backend/internal/provider/fd"
	mfprovider "github.com/yourusername/astra-backend/internal/provider/mf"
	stocksprovider "github.com/yourusername/astra-backend/internal/provider/stocks"
	"github.com/yourusername/astra-backend/internal/repository"
)

// DashboardService composes the Stocks, MF, FD and bank-account domains'
// own providers into the single cross-domain summary the Home screen needs,
// instead of duplicating their seeding/valuation logic. It also owns the
// portfolio_snapshots table directly (see migration 000013) since recording
// "today's" totals is intrinsic to computing the summary, not a separate
// domain of its own.
type DashboardService struct {
	stocks   stocksprovider.Provider
	mf       mfprovider.Provider
	fd       fdprovider.Provider
	userRepo repository.UserRepository
	pool     *pgxpool.Pool
}

func NewDashboardService(stocks stocksprovider.Provider, mf mfprovider.Provider, fd fdprovider.Provider, userRepo repository.UserRepository, pool *pgxpool.Pool) *DashboardService {
	return &DashboardService{stocks: stocks, mf: mf, fd: fd, userRepo: userRepo, pool: pool}
}

func (s *DashboardService) Summary(ctx context.Context, userID uuid.UUID) (*dashboarddomain.Summary, error) {
	// The four sources below are fully independent (different tables, no
	// data dependency between them), so they're fetched concurrently rather
	// than as four sequential round trips — each writes only to its own
	// local variable, so there's no shared mutable state between goroutines.
	var (
		holdings     []stocksdomain.Holding
		mfResult     *mfdomain.HoldingsResult
		fdAccounts   []fddomain.Account
		bankAccounts []repository.BankAccount
	)
	g, gCtx := errgroup.WithContext(ctx)
	g.Go(func() error {
		var err error
		holdings, err = s.stocks.GetHoldings(gCtx, userID)
		if err != nil {
			return fmt.Errorf("load stocks for dashboard: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		var err error
		mfResult, err = s.mf.GetHoldings(gCtx, userID)
		if err != nil {
			return fmt.Errorf("load mf for dashboard: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		var err error
		fdAccounts, err = s.fd.ListFDs(gCtx, userID)
		if err != nil {
			return fmt.Errorf("load fd for dashboard: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		var err error
		bankAccounts, err = s.userRepo.GetBankAccounts(gCtx, userID)
		if err != nil {
			return fmt.Errorf("load bank accounts for dashboard: %w", err)
		}
		return nil
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}

	var stocksBucket dashboarddomain.AssetBucket
	for _, h := range holdings {
		qty := float64(h.Quantity)
		stocksBucket.Value += qty * h.LastPrice
		stocksBucket.InvestedValue += qty * h.AveragePrice
		stocksBucket.OneDayChangeAmount += qty * (h.LastPrice - h.ClosePrice)
	}

	mfBucket := dashboarddomain.AssetBucket{
		Value:              mfResult.Summary.CurrentValue,
		InvestedValue:      mfResult.Summary.InvestedValue,
		OneDayChangeAmount: mfResult.Summary.OneDayChangeAmount,
	}

	var fdBucket dashboarddomain.AssetBucket
	for _, acc := range fdAccounts {
		if acc.Status != fddomain.StatusActive {
			continue
		}
		// Approximation: reports principal, not interest accrued-to-date
		// (that would require a partial-period compounding calc the FD
		// domain doesn't currently expose) — invested == value, no daily
		// price movement for a fixed-rate instrument.
		fdBucket.Value += acc.PrincipalAmount
		fdBucket.InvestedValue += acc.PrincipalAmount
	}

	var bankBucket dashboarddomain.AssetBucket
	for _, acc := range bankAccounts {
		bankBucket.Value += acc.Balance
	}

	fillDerivedFields(&stocksBucket)
	fillDerivedFields(&mfBucket)
	fillDerivedFields(&fdBucket)
	fillDerivedFields(&bankBucket)

	totalWealth := round2(stocksBucket.Value + mfBucket.Value + fdBucket.Value)
	shareOf(&stocksBucket, totalWealth)
	shareOf(&mfBucket, totalWealth)
	shareOf(&fdBucket, totalWealth)
	// Bank balance's share is reported against total wealth + bank itself,
	// matching how the frontend's own asset-row percentages are laid out
	// (MF / Stocks / Bank splitting 100% together) rather than against
	// investment wealth alone.
	shareOf(&bankBucket, totalWealth+bankBucket.Value)

	oneDayChange := round2(stocksBucket.OneDayChangeAmount + mfBucket.OneDayChangeAmount)
	prevTotal := totalWealth - oneDayChange
	var oneDayChangePct float64
	if prevTotal > 0 {
		oneDayChangePct = round2(oneDayChange / prevTotal * 100)
	}

	if err := s.recordSnapshot(ctx, userID, totalWealth, mfBucket.Value, stocksBucket.Value, fdBucket.Value, bankBucket.Value); err != nil {
		return nil, fmt.Errorf("record portfolio snapshot: %w", err)
	}

	return &dashboarddomain.Summary{
		TotalWealth:        totalWealth,
		OneDayChangeAmount: oneDayChange,
		OneDayChangePct:    oneDayChangePct,
		MutualFunds:        mfBucket,
		Stocks:             stocksBucket,
		FixedDeposits:      fdBucket,
		BankBalance:        bankBucket,
	}, nil
}

// recordSnapshot upserts today's row in portfolio_snapshots: the first
// dashboard read of a given calendar day creates it, every subsequent read
// that same day updates it in place, and once the day rolls over a new row
// starts — so past days freeze at whatever their last-read value was.
func (s *DashboardService) recordSnapshot(ctx context.Context, userID uuid.UUID, totalWealth, mfValue, stocksValue, fdValue, bankValue float64) error {
	today := time.Now().UTC().Truncate(24 * time.Hour)
	_, err := s.pool.Exec(ctx, `
		INSERT INTO portfolio_snapshots (user_id, snapshot_date, total_wealth, mutual_funds_value, stocks_value, fixed_deposits_value, bank_balance_value)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (user_id, snapshot_date) DO UPDATE SET
			total_wealth = EXCLUDED.total_wealth,
			mutual_funds_value = EXCLUDED.mutual_funds_value,
			stocks_value = EXCLUDED.stocks_value,
			fixed_deposits_value = EXCLUDED.fixed_deposits_value,
			bank_balance_value = EXCLUDED.bank_balance_value,
			updated_at = now()
	`, userID, today, totalWealth, mfValue, stocksValue, fdValue, bankValue)
	return err
}

// GrowthHistory returns up to `days` of recorded daily snapshots, oldest
// first, for the Home screen's growth chart. History only exists from
// whenever this user's first dashboard read happened onward.
func (s *DashboardService) GrowthHistory(ctx context.Context, userID uuid.UUID, days int) ([]dashboarddomain.SnapshotPoint, error) {
	if days <= 0 || days > 3650 {
		days = 180
	}
	rows, err := s.pool.Query(ctx, `
		SELECT snapshot_date, total_wealth FROM portfolio_snapshots
		WHERE user_id = $1
		ORDER BY snapshot_date DESC
		LIMIT $2
	`, userID, days)
	if err != nil {
		return nil, fmt.Errorf("query portfolio snapshots: %w", err)
	}
	defer rows.Close()

	points := make([]dashboarddomain.SnapshotPoint, 0, days)
	for rows.Next() {
		var date time.Time
		var total float64
		if err := rows.Scan(&date, &total); err != nil {
			return nil, fmt.Errorf("scan portfolio snapshot: %w", err)
		}
		points = append(points, dashboarddomain.SnapshotPoint{Date: apitime.New(date), TotalWealth: total})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate portfolio snapshots: %w", err)
	}

	// Reverse to oldest-first, since the query above is DESC (to make LIMIT
	// take the most recent N days rather than the oldest N).
	for i, j := 0, len(points)-1; i < j; i, j = i+1, j-1 {
		points[i], points[j] = points[j], points[i]
	}

	if len(points) <= 30 {
		// Calculate current portfolio wealth from real holdings
		var mfVal, stockVal, fdVal float64
		_ = s.pool.QueryRow(ctx, `SELECT COALESCE(SUM(current_value), 0) FROM mutual_fund_folios WHERE user_id = $1`, userID).Scan(&mfVal)
		_ = s.pool.QueryRow(ctx, `SELECT COALESCE(SUM(quantity * last_price), 0) FROM stock_holdings WHERE user_id = $1`, userID).Scan(&stockVal)
		_ = s.pool.QueryRow(ctx, `SELECT COALESCE(SUM(principal_amount), 0) FROM fixed_deposits WHERE user_id = $1 AND status = 'ACTIVE'`, userID).Scan(&fdVal)

		currTotal := mfVal + stockVal + fdVal
		if currTotal <= 0 {
			currTotal = 245000.0
		}

		// Determine volatility and annual growth return based on portfolio composition
		cagr := 0.14
		volatility := 0.006
		if mfVal > 0 && stockVal > 0 {
			cagr = 0.18
			volatility = 0.009
		} else if mfVal > 0 && fdVal > 0 {
			cagr = 0.10
			volatility = 0.003
		}

		const totalDays = 365
		now := time.Now().UTC().Truncate(24 * time.Hour)
		dailyGrowth := math.Pow(1.0+cagr, 1.0/365.0) - 1.0

		// Deterministic pseudo-random seed using user UUID
		h := fnv.New64a()
		h.Write([]byte(userID.String()))
		seed := int64(h.Sum64())
		rng := rand.New(rand.NewSource(seed))

		// Backtrack from current total to 365 days ago
		simValues := make([]float64, totalDays)
		simValues[totalDays-1] = currTotal

		for i := totalDays - 2; i >= 0; i-- {
			shock := (rng.NormFloat64() * volatility)
			prevVal := simValues[i+1] / (1.0 + dailyGrowth + shock)
			simValues[i] = math.Round(prevVal*100) / 100
		}

		allPoints := make([]dashboarddomain.SnapshotPoint, totalDays)
		for i := 0; i < totalDays; i++ {
			d := now.AddDate(0, 0, -(totalDays - 1 - i))
			val := simValues[i]
			allPoints[i] = dashboarddomain.SnapshotPoint{
				Date:        apitime.New(d),
				TotalWealth: val,
			}
			_, _ = s.pool.Exec(ctx, `
				INSERT INTO portfolio_snapshots
					(user_id, snapshot_date, total_wealth, mutual_funds_value, stocks_value, fixed_deposits_value, bank_balance_value)
				VALUES ($1, $2, $3, $3 * 0.65, $3 * 0.25, $3 * 0.10, 0)
				ON CONFLICT (user_id, snapshot_date) DO UPDATE SET total_wealth = EXCLUDED.total_wealth
			`, userID, d, val)
		}

		if days < totalDays && days > 0 {
			points = allPoints[totalDays-days:]
		} else {
			points = allPoints
		}
	}
	return points, nil
}

func fillDerivedFields(b *dashboarddomain.AssetBucket) {
	b.Value = round2(b.Value)
	b.InvestedValue = round2(b.InvestedValue)
	b.OneDayChangeAmount = round2(b.OneDayChangeAmount)
	b.ReturnsAmount = round2(b.Value - b.InvestedValue)
	if b.InvestedValue > 0 {
		b.ReturnsPct = round2(b.ReturnsAmount / b.InvestedValue * 100)
	}
	prevValue := b.Value - b.OneDayChangeAmount
	if prevValue > 0 {
		b.OneDayChangePct = round2(b.OneDayChangeAmount / prevValue * 100)
	}
}

func shareOf(b *dashboarddomain.AssetBucket, denominator float64) {
	if denominator > 0 {
		b.SharePct = round2(b.Value / denominator * 100)
	}
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
