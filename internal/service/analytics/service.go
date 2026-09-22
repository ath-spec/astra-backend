package analytics

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/yourusername/astra-backend/internal/apitime"
	analyticsdomain "github.com/yourusername/astra-backend/internal/domain/analytics"
	analyticsprovider "github.com/yourusername/astra-backend/internal/provider/analytics"
	"github.com/yourusername/astra-backend/internal/repository"
)

// fetchWindowDays covers every analyzer's needs: the widest lookback used
// below is MerchantAnalysis's 90-180 day "reactivated merchant" check.
const fetchWindowDays = 180

// Service is a thin orchestration layer: fetch via the swappable
// TransactionSource, then run the relevant pure analyzer(s) from engine.go.
// This is the seam a future real transaction feed plugs into — nothing here
// or above (the handler) changes when TransactionSource gets a real AA-backed
// implementation.
type Service struct {
	source     analyticsprovider.TransactionSource
	investment analyticsprovider.InvestmentSource
	userRepo   repository.UserRepository
}

func NewService(source analyticsprovider.TransactionSource, investment analyticsprovider.InvestmentSource, userRepo repository.UserRepository) *Service {
	return &Service{source: source, investment: investment, userRepo: userRepo}
}

func (s *Service) fetch(ctx context.Context, userID uuid.UUID, now time.Time) ([]analyticsdomain.Transaction, error) {
	return s.source.GetTransactions(ctx, userID, now.AddDate(0, 0, -fetchWindowDays), now)
}

func (s *Service) WeekdayVsWeekend(ctx context.Context, userID uuid.UUID) (analyticsdomain.WeekdayWeekendResult, error) {
	now := time.Now().UTC()
	txns, err := s.fetch(ctx, userID, now)
	if err != nil {
		return analyticsdomain.WeekdayWeekendResult{}, err
	}
	return WeekdayVsWeekend(txns, now), nil
}

func (s *Service) Trends(ctx context.Context, userID uuid.UUID, period string) (analyticsdomain.TrendResult, error) {
	now := time.Now().UTC()
	txns, err := s.fetch(ctx, userID, now)
	if err != nil {
		return analyticsdomain.TrendResult{}, err
	}
	return TrendAnalytics(txns, now, period), nil
}

func (s *Service) CategoryTrend(ctx context.Context, userID uuid.UUID) (analyticsdomain.CategoryTrendResult, error) {
	now := time.Now().UTC()
	txns, err := s.fetch(ctx, userID, now)
	if err != nil {
		return analyticsdomain.CategoryTrendResult{}, err
	}
	return CategoryTrend(txns, now), nil
}

func (s *Service) CategoryMomentum(ctx context.Context, userID uuid.UUID) (analyticsdomain.CategoryMomentumResult, error) {
	now := time.Now().UTC()
	txns, err := s.fetch(ctx, userID, now)
	if err != nil {
		return analyticsdomain.CategoryMomentumResult{}, err
	}
	return CategoryMomentum(CategoryTrend(txns, now)), nil
}

func (s *Service) AverageStats(ctx context.Context, userID uuid.UUID) (analyticsdomain.AverageStatsResult, error) {
	now := time.Now().UTC()
	txns, err := s.fetch(ctx, userID, now)
	if err != nil {
		return analyticsdomain.AverageStatsResult{}, err
	}
	return AverageStats(txns, now), nil
}

func (s *Service) MerchantAnalysis(ctx context.Context, userID uuid.UUID) (analyticsdomain.MerchantAnalysisResult, error) {
	now := time.Now().UTC()
	txns, err := s.fetch(ctx, userID, now)
	if err != nil {
		return analyticsdomain.MerchantAnalysisResult{}, err
	}
	return MerchantAnalysis(txns, now), nil
}

func (s *Service) RecurringDetection(ctx context.Context, userID uuid.UUID) (analyticsdomain.RecurringResult, error) {
	now := time.Now().UTC()
	txns, err := s.fetch(ctx, userID, now)
	if err != nil {
		return analyticsdomain.RecurringResult{}, err
	}
	return RecurringDetection(txns, now), nil
}

func (s *Service) NightAndImpulse(ctx context.Context, userID uuid.UUID) (analyticsdomain.NightImpulseResult, error) {
	now := time.Now().UTC()
	txns, err := s.fetch(ctx, userID, now)
	if err != nil {
		return analyticsdomain.NightImpulseResult{}, err
	}
	return NightAndImpulse(txns, now), nil
}

func (s *Service) PatternSummary(ctx context.Context, userID uuid.UUID) (analyticsdomain.PatternSummaryResult, error) {
	now := time.Now().UTC()
	txns, err := s.fetch(ctx, userID, now)
	if err != nil {
		return analyticsdomain.PatternSummaryResult{}, err
	}
	return PatternSummary(txns, now), nil
}

func (s *Service) Snapshot(ctx context.Context, userID uuid.UUID) (analyticsdomain.SnapshotResult, error) {
	now := time.Now().UTC()
	txns, err := s.fetch(ctx, userID, now)
	if err != nil {
		return analyticsdomain.SnapshotResult{}, err
	}

	var balance *float64
	if accounts, err := s.userRepo.GetBankAccounts(ctx, userID); err == nil {
		var sum float64
		for _, a := range accounts {
			sum += a.Balance
		}
		balance = &sum
	}
	// A failure to fetch the balance degrades gracefully (drawdown fields
	// omitted) rather than failing the whole snapshot — it's supplementary,
	// not the point of the endpoint.

	return Snapshot(txns, now, balance), nil
}

func (s *Service) Compare(ctx context.Context, userID uuid.UUID, by string, names []string) (analyticsdomain.ComparisonResult, error) {
	now := time.Now().UTC()
	txns, err := s.fetch(ctx, userID, now)
	if err != nil {
		return analyticsdomain.ComparisonResult{}, err
	}
	return Compare(txns, now, by, names), nil
}

func (s *Service) InvestmentConsistency(ctx context.Context, userID uuid.UUID) (analyticsdomain.InvestmentConsistencyResult, error) {
	now := time.Now().UTC()
	events, err := s.investment.GetInvestmentEvents(ctx, userID, now.AddDate(0, -investmentMonthsTracked, 0), now)
	if err != nil {
		return analyticsdomain.InvestmentConsistencyResult{}, err
	}
	return InvestmentConsistency(events, now), nil
}

func (s *Service) BNPLExposure(ctx context.Context, userID uuid.UUID) (analyticsdomain.BNPLExposureResult, error) {
	now := time.Now().UTC()
	txns, err := s.fetch(ctx, userID, now)
	if err != nil {
		return analyticsdomain.BNPLExposureResult{}, err
	}
	return BNPLExposure(txns, now), nil
}

func (s *Service) SubscriptionLoad(ctx context.Context, userID uuid.UUID) (analyticsdomain.SubscriptionLoadResult, error) {
	now := time.Now().UTC()
	txns, err := s.fetch(ctx, userID, now)
	if err != nil {
		return analyticsdomain.SubscriptionLoadResult{}, err
	}
	return VerifiedSubscriptionLoad(txns, now), nil
}

// ListTransactions is the paginated raw feed behind the Transactions screen.
// limit/offset drive infinite-scroll: default 25, max 100. Total is the
// count of all matching items so the client knows when to stop.
func (s *Service) ListTransactions(ctx context.Context, userID uuid.UUID, category, merchant string, days, limit, offset int) (analyticsdomain.TransactionPage, error) {
	if days <= 0 || days > 3650 {
		days = fetchWindowDays
	}
	const defaultLimit = 25
	const maxLimit = 100
	if limit <= 0 {
		limit = defaultLimit
	} else if limit > maxLimit {
		limit = maxLimit
	}
	if offset < 0 {
		offset = 0
	}

	now := time.Now().UTC()
	txns, err := s.source.GetTransactions(ctx, userID, now.AddDate(0, 0, -days), now)
	if err != nil {
		return analyticsdomain.TransactionPage{}, err
	}

	items := make([]analyticsdomain.TransactionListItem, 0, len(txns))
	for _, t := range txns {
		if category != "" && !strings.EqualFold(t.Category, category) {
			continue
		}
		if merchant != "" && !strings.EqualFold(t.Merchant, merchant) {
			continue
		}
		items = append(items, analyticsdomain.TransactionListItem{
			ID: t.ID, Amount: round2(t.Amount), Type: t.Type, Category: t.Category,
			Merchant: t.Merchant, OccurredAt: apitime.New(t.OccurredAt),
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].OccurredAt.Time().After(items[j].OccurredAt.Time()) })

	total := len(items)
	if offset >= total {
		return analyticsdomain.TransactionPage{Items: []analyticsdomain.TransactionListItem{}, Total: total, Limit: limit, Offset: offset}, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return analyticsdomain.TransactionPage{Items: items[offset:end], Total: total, Limit: limit, Offset: offset}, nil
}

func (s *Service) IncomeAnalysis(ctx context.Context, userID uuid.UUID) (analyticsdomain.IncomeResult, error) {
	now := time.Now().UTC()
	txns, err := s.fetch(ctx, userID, now)
	if err != nil {
		return analyticsdomain.IncomeResult{}, err
	}
	return IncomeAnalysis(txns, now), nil
}
