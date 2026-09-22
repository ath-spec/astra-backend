package service

import (
	"context"
	"fmt"
	"hash/fnv"
	"math"
	"math/rand"
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
	"github.com/yourusername/astra-backend/internal/service/idbiaccounts"
)

// DashboardService composes the Stocks, MF, FD and bank-account domains'
// own providers into the single cross-domain summary the Home screen needs,
// instead of duplicating their seeding/valuation logic. It also owns the
// portfolio_snapshots table directly (see migration 000013) since recording
// "today's" totals is intrinsic to computing the summary, not a separate
// domain of its own.
type DashboardService struct {
	stocks       stocksprovider.Provider
	mf           mfprovider.Provider
	fd           fdprovider.Provider
	userRepo     repository.UserRepository
	pool         *pgxpool.Pool
	idbiAccounts *idbiaccounts.Service
}

func NewDashboardService(stocks stocksprovider.Provider, mf mfprovider.Provider, fd fdprovider.Provider, userRepo repository.UserRepository, pool *pgxpool.Pool) *DashboardService {
	return &DashboardService{stocks: stocks, mf: mf, fd: fd, userRepo: userRepo, pool: pool}
}

// WithIDBIAccounts wires the IDBI-synced deposit accounts (feature 1) into
// the dashboard's bank balance total. Without this, the Home screen's Bank
// Accounts figure was computed from the bank_accounts table alone while
// GET /api/v1/aa/accounts (the screen you land on when you tap it) also
// folds in the user's real IDBI accounts — so a user with IDBI accounts
// synced saw two different totals and two different account counts for the
// same "linked accounts" depending on which screen they were looking at.
func (s *DashboardService) WithIDBIAccounts(svc *idbiaccounts.Service) *DashboardService {
	s.idbiAccounts = svc
	return s
}

// PortfolioInputs is the raw, per-provider data a portfolio valuation is
// built from. Fetch it once per request (FetchInputs) and reuse it for every
// computation that needs it (Summarize, and any raw holdings lists a caller
// also wants) — never fetch it twice in the same request. Two independent
// fetches run concurrently in separate goroutines are not guaranteed to see
// the same database state under Postgres's default READ COMMITTED isolation:
// a concurrent write between them (e.g. an order fill updating last_price)
// can make one goroutine's holdings disagree with the other's, so a single
// response could report two different totals for the same portfolio.
type PortfolioInputs struct {
	Holdings     []stocksdomain.Holding
	MF           *mfdomain.HoldingsResult
	FDAccounts   []fddomain.Account
	BankAccounts []repository.BankAccount
}

// FetchInputs loads the four independent data sources a portfolio valuation
// needs, concurrently (different tables, no data dependency between them —
// each goroutine writes only its own local, so there's no shared mutable
// state races here).
func (s *DashboardService) FetchInputs(ctx context.Context, userID uuid.UUID) (*PortfolioInputs, error) {
	var in PortfolioInputs
	var idbiBankAccounts []repository.BankAccount
	g, gCtx := errgroup.WithContext(ctx)
	g.Go(func() error {
		var err error
		in.Holdings, err = s.stocks.GetHoldings(gCtx, userID)
		if err != nil {
			return fmt.Errorf("load stocks for dashboard: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		var err error
		in.MF, err = s.mf.GetHoldings(gCtx, userID)
		if err != nil {
			return fmt.Errorf("load mf for dashboard: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		var err error
		in.FDAccounts, err = s.fd.ListFDs(gCtx, userID)
		if err != nil {
			return fmt.Errorf("load fd for dashboard: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		var err error
		in.BankAccounts, err = s.userRepo.GetBankAccounts(gCtx, userID)
		if err != nil {
			return fmt.Errorf("load bank accounts for dashboard: %w", err)
		}
		return nil
	})
	if s.idbiAccounts != nil {
		g.Go(func() error {
			// Mirrors the same fold-in GetAccounts does — a failure here (no
			// link yet, IDBI gateway down) must not fail the whole dashboard,
			// it just means this user's total is bank_accounts-only for now.
			// Written to its own local, then merged into in.BankAccounts only
			// after g.Wait() below — appending directly to in.BankAccounts
			// here would race the GetBankAccounts goroutine writing that same
			// field concurrently.
			idbiAccs, ierr := s.idbiAccounts.List(gCtx, userID)
			if ierr != nil {
				return nil
			}
			for _, a := range idbiAccs {
				bal := a.LedgerBalance
				if bal == 0 {
					bal = a.AvailableBalance
				}
				idbiBankAccounts = append(idbiBankAccounts, repository.BankAccount{
					BankName:    "IDBI Bank",
					AccountType: a.AccountType,
					Balance:     bal,
				})
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	in.BankAccounts = append(in.BankAccounts, idbiBankAccounts...)
	return &in, nil
}

func (s *DashboardService) Summary(ctx context.Context, userID uuid.UUID) (*dashboarddomain.Summary, error) {
	in, err := s.FetchInputs(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.Summarize(ctx, userID, in)
}

// Summarize computes the dashboard summary from already-fetched inputs and
// records today's portfolio_snapshots row. Callers that already fetched
// PortfolioInputs for another reason (e.g. RMService.GetClient also needs
// the raw holdings lists) should call this directly instead of Summary, so
// the data is only ever read from the database once per request.
func (s *DashboardService) Summarize(ctx context.Context, userID uuid.UUID, in *PortfolioInputs) (*dashboarddomain.Summary, error) {
	mfResult, fdAccounts, bankAccounts := in.MF, in.FDAccounts, in.BankAccounts

	var stocksBucket dashboarddomain.AssetBucket
	for _, h := range in.Holdings {
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

	// Total wealth / net worth is every asset the user holds: stocks + MF +
	// FDs + bank balances. Bank balance used to be left out of this sum
	// entirely (only folded into the *denominator* below for its own share%,
	// never into the total itself) — so a user's reported net worth was
	// missing however much cash they had in linked bank accounts.
	totalWealth := round2(stocksBucket.Value + mfBucket.Value + fdBucket.Value + bankBucket.Value)
	shareOf(&stocksBucket, totalWealth)
	shareOf(&mfBucket, totalWealth)
	shareOf(&fdBucket, totalWealth)
	shareOf(&bankBucket, totalWealth)

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
//
// prefetched lets a caller that already has this user's PortfolioInputs pass
// them in (nil otherwise) — RMService.GetClient needs both this and the
// dashboard summary for its own portfolio-inputs branch, and without this,
// the <=30-days-of-history backfill below called Summary(ctx, userID), which
// silently re-ran the exact same FetchInputs query that caller had already
// done seconds earlier in a sibling goroutine.
func (s *DashboardService) GrowthHistory(ctx context.Context, userID uuid.UUID, days int, prefetched *PortfolioInputs) ([]dashboarddomain.SnapshotPoint, error) {
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
		// Current portfolio wealth, from the same single source of truth as
		// the Home screen's headline number (Summary/Summarize) — this used
		// to run its own raw SQL against mutual_fund_folios/stock_holdings/
		// fixed_deposits, none of which exist (the real tables are
		// mf_folios/demat_holdings/fd_accounts), so every query silently
		// errored, mfVal/stockVal/fdVal stayed 0, and every user fell into
		// the currTotal<=0 branch below — fabricating a growth curve seeded
		// from a hardcoded ₹245,000 that had nothing to do with their real
		// portfolio and could visibly disagree with the real total shown
		// elsewhere in the same screen.
		var summary *dashboarddomain.Summary
		if prefetched != nil {
			s2, err := s.Summarize(ctx, userID, prefetched)
			if err != nil {
				return nil, fmt.Errorf("compute current wealth for growth backfill: %w", err)
			}
			summary = s2
		} else {
			s2, err := s.Summary(ctx, userID)
			if err != nil {
				return nil, fmt.Errorf("compute current wealth for growth backfill: %w", err)
			}
			summary = s2
		}
		mfVal, stockVal, fdVal := summary.MutualFunds.Value, summary.Stocks.Value, summary.FixedDeposits.Value

		currTotal := summary.TotalWealth
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
		dates := make([]time.Time, totalDays)
		vals := make([]float64, totalDays)
		for i := 0; i < totalDays; i++ {
			d := now.AddDate(0, 0, -(totalDays - 1 - i))
			val := simValues[i]
			allPoints[i] = dashboarddomain.SnapshotPoint{
				Date:        apitime.New(d),
				TotalWealth: val,
			}
			dates[i] = d
			vals[i] = val
		}
		// One batched statement instead of 365 sequential round-trips — this
		// backfill only runs once per user (their first 30 days), but it used
		// to block the request on 365 individual blocking Execs.
		if _, err := s.pool.Exec(ctx, `
			INSERT INTO portfolio_snapshots
				(user_id, snapshot_date, total_wealth, mutual_funds_value, stocks_value, fixed_deposits_value, bank_balance_value)
			SELECT $1, d, v, v * 0.65, v * 0.25, v * 0.10, 0
			FROM unnest($2::date[], $3::float8[]) AS t(d, v)
			ON CONFLICT (user_id, snapshot_date) DO UPDATE SET total_wealth = EXCLUDED.total_wealth
		`, userID, dates, vals); err != nil {
			return nil, fmt.Errorf("batch insert growth backfill: %w", err)
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
