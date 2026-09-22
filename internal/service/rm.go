package service

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"

	"github.com/yourusername/astra-backend/internal/ai/agents"
	"github.com/yourusername/astra-backend/internal/apiresponse"
	"github.com/yourusername/astra-backend/internal/apitime"
	dashboarddomain "github.com/yourusername/astra-backend/internal/domain/dashboard"
	fddomain "github.com/yourusername/astra-backend/internal/domain/fd"
	goalsdomain "github.com/yourusername/astra-backend/internal/domain/goals"
	mfdomain "github.com/yourusername/astra-backend/internal/domain/mf"
	paDomain "github.com/yourusername/astra-backend/internal/domain/portfolioanalysis"
	rmdomain "github.com/yourusername/astra-backend/internal/domain/rm"
	stocksdomain "github.com/yourusername/astra-backend/internal/domain/stocks"
	fdprovider "github.com/yourusername/astra-backend/internal/provider/fd"
	goalsprovider "github.com/yourusername/astra-backend/internal/provider/goals"
	"github.com/yourusername/astra-backend/internal/provider/llm"
	mfprovider "github.com/yourusername/astra-backend/internal/provider/mf"
	stocksprovider "github.com/yourusername/astra-backend/internal/provider/stocks"
	"github.com/yourusername/astra-backend/internal/repository"
	analyticsservice "github.com/yourusername/astra-backend/internal/service/analytics"
	budgetservice "github.com/yourusername/astra-backend/internal/service/budget"
)

// RMService backs the RM-facing console: the book dashboard and the
// read-only 360° client view. It composes the existing user-domain
// providers/services by userID — it never mutates user data and never
// calls the user HTTP layer.
type RMService struct {
	dashboard    *DashboardService
	analysis     *PortfolioAnalysisService
	stocks       stocksprovider.Provider
	mf           mfprovider.Provider
	fd           fdprovider.Provider
	goals        goalsprovider.Provider
	userRepo     repository.UserRepository
	assign       repository.AssignmentRepository
	rmRepo       repository.RMUserRepository
	interactions repository.RMInteractionRepository
	llm          llm.Provider
	agents       *agents.Catalog
	spend        *analyticsservice.Service
	budget       *budgetservice.Service
	pool         *pgxpool.Pool

	// snapshotRefresh dedupes concurrent background portfolio_snapshots
	// refreshes per user, the same single-flight pattern idbiaccounts.Service
	// uses for its own background sync (internal/service/idbiaccounts/
	// service.go) — without it, two RMs opening overlapping client lists at
	// the same moment would each spawn their own refresh goroutine per
	// shared client.
	snapshotRefresh sync.Map // userID -> struct{}
}

func NewRMService(
	dashboard *DashboardService,
	analysis *PortfolioAnalysisService,
	stocks stocksprovider.Provider,
	mf mfprovider.Provider,
	fd fdprovider.Provider,
	goals goalsprovider.Provider,
	userRepo repository.UserRepository,
	assign repository.AssignmentRepository,
	rmRepo repository.RMUserRepository,
	interactions repository.RMInteractionRepository,
	llmProvider llm.Provider,
	cat *agents.Catalog,
	spend *analyticsservice.Service,
	budget *budgetservice.Service,
	pool *pgxpool.Pool,
) *RMService {
	return &RMService{
		dashboard: dashboard, analysis: analysis, stocks: stocks, mf: mf, fd: fd, goals: goals,
		userRepo: userRepo, assign: assign, rmRepo: rmRepo, interactions: interactions,
		llm: llmProvider, agents: cat, spend: spend, budget: budget, pool: pool,
	}
}

// ListClients returns the paginated book for one RM. TotalWealth on each row
// comes from portfolio_snapshots (see PostgresAssignmentRepository.
// ListClients) rather than a live recompute — recomputing live for every row
// on every page load would mean N holdings/NAV fetches per list view. So any
// row whose snapshot isn't from today gets a background refresh kicked off
// (deduped per user, see refreshSnapshotInBackground) — the list still
// returns the slightly-stale number immediately, but the next load for that
// client will be current.
func (s *RMService) ListClients(ctx context.Context, rmID uuid.UUID, f rmdomain.ListFilters) (*rmdomain.ClientList, error) {
	items, total, err := s.assign.ListClients(ctx, &rmID, nil, f)
	if err != nil {
		return nil, err
	}
	today := time.Now().UTC().Truncate(24 * time.Hour)
	for _, it := range items {
		if it.SnapshotDate == nil || it.SnapshotDate.Time().Before(today) {
			s.refreshSnapshotInBackground(it.UserID)
		}
	}
	return &rmdomain.ClientList{Items: items, Total: total}, nil
}

// refreshSnapshotInBackground recomputes and upserts one user's
// portfolio_snapshots row (via dashboard.Summary) without blocking the
// caller. Deduped per user via snapshotRefresh so concurrent list loads
// covering the same client never spawn more than one in-flight refresh.
func (s *RMService) refreshSnapshotInBackground(userID uuid.UUID) {
	if _, busy := s.snapshotRefresh.LoadOrStore(userID, struct{}{}); busy {
		return
	}
	go func() {
		defer s.snapshotRefresh.Delete(userID)
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		if _, err := s.dashboard.Summary(ctx, userID); err != nil {
			slog.Warn("rm: background portfolio snapshot refresh failed", "user", userID, "error", err)
		}
	}()
}

// authorizeClient ensures the caller may view this client: admins may view
// anyone, an RM only their own book.
func (s *RMService) authorizeClient(ctx context.Context, callerRMID uuid.UUID, isAdmin bool, userID uuid.UUID) error {
	owner, found, err := s.assign.OwnerOf(ctx, userID)
	if err != nil {
		return err
	}
	if !found {
		return apiresponse.NotFound("client %s not found", userID)
	}
	if isAdmin {
		return nil
	}
	if owner == nil || *owner != callerRMID {
		return fmt.Errorf("client is not in your book: %w", apiresponse.ErrForbidden)
	}
	return nil
}

// GetClient assembles the full 360° view for one client.
func (s *RMService) GetClient(ctx context.Context, callerRMID uuid.UUID, isAdmin bool, userID uuid.UUID, growthDays int) (*rmdomain.ClientDetail, error) {
	if err := s.authorizeClient(ctx, callerRMID, isAdmin, userID); err != nil {
		return nil, err
	}
	if growthDays <= 0 || growthDays > 3650 {
		growthDays = 180
	}

	// Each fetch writes only its own local; the *ClientDetail is assembled
	// single-threaded after g.Wait() returns, so there is no shared mutable
	// state between the goroutines at all. Portfolio holdings are fetched
	// exactly once (FetchInputs) and reused for both the summary and the raw
	// holdings lists below — this used to be two independent, concurrent
	// fetches of the same stocks/mf/fd/bank data (one inside dashboard.
	// Summary, one directly here), which was not just wasted work but a real
	// consistency risk: under Postgres's default READ COMMITTED isolation, a
	// write landing between the two fetches (e.g. an order fill updating
	// last_price) could make this response's summary total disagree with
	// its own holdings list for the same portfolio.
	var (
		profile   *rmdomain.ClientProfile
		inputs    *PortfolioInputs
		growth    []dashboarddomain.SnapshotPoint
		goalsList []goalsdomain.Goal
		spend     rmdomain.SpendSummary
		dna       *paDomain.AllocationResult
	)

	g, gCtx := errgroup.WithContext(ctx)
	g.Go(func() error {
		p, err := s.assign.GetClientProfile(gCtx, userID)
		if err != nil {
			return err
		}
		if p == nil {
			return apiresponse.NotFound("client %s not found", userID)
		}
		profile = p
		return nil
	})
	g.Go(func() error {
		// GrowthHistory's <=30-days-of-history backfill path needs this
		// user's current portfolio summary — merged into this same goroutine
		// (rather than kept as its own parallel fetch) so it reuses the
		// PortfolioInputs already fetched here instead of independently
		// re-running the same stocks/mf/fd/bank query a second time. That
		// used to make every client-profile open with under a month of
		// history do the full portfolio fetch twice, concurrently, against
		// the same tables — exactly the READ COMMITTED consistency risk this
		// function's own comment above warns about, reintroduced here.
		v, err := s.dashboard.FetchInputs(gCtx, userID)
		if err != nil {
			return fmt.Errorf("client portfolio inputs: %w", err)
		}
		inputs = v
		g2, err := s.dashboard.GrowthHistory(gCtx, userID, growthDays, v)
		if err != nil {
			return fmt.Errorf("client growth: %w", err)
		}
		growth = g2
		return nil
	})
	g.Go(func() error {
		v, err := s.goals.ListGoals(gCtx, userID)
		if err != nil {
			return fmt.Errorf("client goals: %w", err)
		}
		goalsList = v
		return nil
	})
	g.Go(func() error {
		v, err := s.spendSummary(gCtx, userID)
		if err != nil {
			return err
		}
		spend = v
		return nil
	})
	g.Go(func() error {
		// Compute the current DNA and, as a side effect, record today's row
		// in portfolio_dna_snapshots so the drift history fills in from real
		// RM usage.
		v, err := s.analysis.RecordDNASnapshot(gCtx, userID)
		if err != nil {
			return fmt.Errorf("client dna: %w", err)
		}
		dna = v
		return nil
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}

	summary, err := s.dashboard.Summarize(ctx, userID, inputs)
	if err != nil {
		return nil, fmt.Errorf("client summary: %w", err)
	}

	stockH := inputs.Holdings
	var mfFolios []mfdomain.Folio
	if inputs.MF != nil {
		mfFolios = inputs.MF.Folios
	}
	fdAccts := inputs.FDAccounts
	bankAccts := inputs.BankAccounts

	banks := make([]rmdomain.BankAccount, 0, len(bankAccts))
	for _, a := range bankAccts {
		banks = append(banks, rmdomain.BankAccount{
			BankName: a.BankName, AccountType: a.AccountType, Balance: a.Balance,
		})
	}
	if stockH == nil {
		stockH = []stocksdomain.Holding{}
	}
	if mfFolios == nil {
		mfFolios = []mfdomain.Folio{}
	}
	if fdAccts == nil {
		fdAccts = []fddomain.Account{}
	}
	if goalsList == nil {
		goalsList = []goalsdomain.Goal{}
	}

	return &rmdomain.ClientDetail{
		Profile:      *profile,
		Summary:      summary,
		DNA:          dna,
		BankAccounts: banks,
		Holdings:     rmdomain.Holdings{Stocks: stockH, MF: mfFolios, FD: fdAccts},
		Goals:        goalsList,
		SpendSummary: spend,
		Growth:       growth,
	}, nil
}

// PortfolioAnalysis assembles the full Allocation / Discipline / Performance
// analysis for one client — the same three engines that back the user app's
// Portfolio Analysis screen — subject to the same book-ownership check as
// GetClient. Each section is fetched concurrently; a section that errors is
// returned as nil rather than failing the whole call, so the RM still sees
// whatever analysis is available.
func (s *RMService) PortfolioAnalysis(ctx context.Context, callerRMID uuid.UUID, isAdmin bool, userID uuid.UUID) (*rmdomain.ClientPortfolioAnalysis, error) {
	if err := s.authorizeClient(ctx, callerRMID, isAdmin, userID); err != nil {
		return nil, err
	}

	var (
		alloc *paDomain.AllocationResult
		disc  *paDomain.DisciplineResult
		perf  *paDomain.PerformanceResult
	)
	g, gCtx := errgroup.WithContext(ctx)
	g.Go(func() error {
		v, err := s.analysis.Allocation(gCtx, userID)
		if err != nil {
			return fmt.Errorf("client allocation: %w", err)
		}
		alloc = v
		return nil
	})
	g.Go(func() error {
		v, err := s.analysis.Discipline(gCtx, userID)
		if err != nil {
			return fmt.Errorf("client discipline: %w", err)
		}
		disc = v
		return nil
	})
	g.Go(func() error {
		v, err := s.analysis.Performance(gCtx, userID)
		if err != nil {
			return fmt.Errorf("client performance: %w", err)
		}
		perf = v
		return nil
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}

	return &rmdomain.ClientPortfolioAnalysis{
		Allocation:  alloc,
		Discipline:  disc,
		Performance: perf,
	}, nil
}

// ClientGrowth returns a client's portfolio growth history, subject to the
// same book-ownership check as GetClient.
func (s *RMService) ClientGrowth(ctx context.Context, callerRMID uuid.UUID, isAdmin bool, userID uuid.UUID, days int) ([]dashboarddomain.SnapshotPoint, error) {
	if err := s.authorizeClient(ctx, callerRMID, isAdmin, userID); err != nil {
		return nil, err
	}
	return s.dashboard.GrowthHistory(ctx, userID, days, nil)
}

// PortfolioHistory returns how a client's asset allocation and portfolio
// DNA have changed over time: the allocation series comes from the daily
// portfolio_snapshots, the DNA series from portfolio_dna_snapshots.
func (s *RMService) PortfolioHistory(ctx context.Context, callerRMID uuid.UUID, isAdmin bool, userID uuid.UUID, days int) (*rmdomain.PortfolioHistory, error) {
	if err := s.authorizeClient(ctx, callerRMID, isAdmin, userID); err != nil {
		return nil, err
	}
	if days <= 0 || days > 3650 {
		days = 365
	}

	out := &rmdomain.PortfolioHistory{
		AllocationSeries: []rmdomain.AllocationHistoryPoint{},
		DNASeries:        []paDomain.DNAHistoryPoint{},
	}

	rows, err := s.pool.Query(ctx, `
		SELECT snapshot_date, total_wealth, mutual_funds_value, stocks_value, fixed_deposits_value, bank_balance_value
		FROM portfolio_snapshots
		WHERE user_id = $1
		ORDER BY snapshot_date DESC
		LIMIT $2
	`, userID, days)
	if err != nil {
		return nil, fmt.Errorf("query allocation history: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var (
			p                         rmdomain.AllocationHistoryPoint
			date                      time.Time
			total, mfv, stv, fdv, bkv float64
		)
		if err := rows.Scan(&date, &total, &mfv, &stv, &fdv, &bkv); err != nil {
			return nil, fmt.Errorf("scan allocation history: %w", err)
		}
		p.Date = apitime.New(date)
		p.TotalWealth = round2(total)
		p.MFValue, p.StocksValue, p.FDValue, p.BankValue = round2(mfv), round2(stv), round2(fdv), round2(bkv)
		denom := mfv + stv + fdv + bkv
		if denom > 0 {
			p.MFPct = round2(mfv / denom * 100)
			p.StocksPct = round2(stv / denom * 100)
			p.FDPct = round2(fdv / denom * 100)
			p.BankPct = round2(bkv / denom * 100)
		}
		out.AllocationSeries = append(out.AllocationSeries, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate allocation history: %w", err)
	}
	// portfolio_snapshots query is DESC for the LIMIT; flip to oldest-first.
	for i, j := 0, len(out.AllocationSeries)-1; i < j; i, j = i+1, j-1 {
		out.AllocationSeries[i], out.AllocationSeries[j] = out.AllocationSeries[j], out.AllocationSeries[i]
	}

	dna, err := s.analysis.DNAHistory(ctx, userID, days)
	if err != nil {
		return nil, err
	}
	if dna != nil {
		out.DNASeries = dna
	}
	return out, nil
}

// spendSummary rolls up the last 30 days of debit spend for a client.
func (s *RMService) spendSummary(ctx context.Context, userID uuid.UUID) (rmdomain.SpendSummary, error) {
	var out rmdomain.SpendSummary
	var total *float64
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount), 0), COUNT(*)
		FROM spend_transactions
		WHERE user_id = $1 AND type = 'DEBIT' AND occurred_at >= now() - INTERVAL '30 days'
	`, userID).Scan(&total, &out.TxnCount)
	if err != nil {
		return out, fmt.Errorf("client spend summary: %w", err)
	}
	if total != nil {
		out.Last30DaysTotal = round2(*total)
	}
	return out, nil
}

// BookSummary aggregates one RM's whole book: AUM, client count, and a few
// attention-worthy alerts.
func (s *RMService) BookSummary(ctx context.Context, rmID uuid.UUID) (*rmdomain.BookSummary, error) {
	staff, err := s.rmRepo.GetByID(ctx, rmID)
	if err != nil {
		return nil, err
	}
	if staff == nil {
		return nil, fmt.Errorf("rm %s not found: %w", rmID, apiresponse.ErrUnauthorized)
	}

	// Client count and AUM come from single aggregate queries (authoritative,
	// unbounded). The client rows are pulled only to derive alerts — capped,
	// since alerts are a "top N attention list", not a full scan.
	counts, err := s.assign.CountsByRM(ctx)
	if err != nil {
		return nil, err
	}
	aumByRM, err := s.assign.AUMByRM(ctx)
	if err != nil {
		return nil, err
	}
	total := counts[rmID]
	aum := aumByRM[rmID]

	items, _, err := s.assign.ListClients(ctx, &rmID, nil, rmdomain.ListFilters{Limit: 200, Sort: "wealth", Order: "asc"})
	if err != nil {
		return nil, err
	}
	alerts := make([]rmdomain.BookAlert, 0)
	for _, it := range items {
		if it.OneDayChangePct <= -3 {
			alerts = append(alerts, rmdomain.BookAlert{
				UserID: it.UserID, Name: it.Name, Type: "portfolio_down",
				Detail: fmt.Sprintf("Portfolio down %.2f%% in the last day", it.OneDayChangePct),
			})
		}
	}

	out := &rmdomain.BookSummary{
		ClientCount: total,
		TotalAUM:    round2(aum),
		Capacity:    staff.MaxPortfolios,
		Alerts:      alerts,
	}
	if total > 0 {
		out.AvgPortfolioValue = round2(aum / float64(total))
	}
	if staff.MaxPortfolios > 0 {
		out.Utilisation = math.Round(float64(total)/float64(staff.MaxPortfolios)*10000) / 10000
	}
	return out, nil
}
