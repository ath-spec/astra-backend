package analytics

import (
	"context"
	"time"

	"github.com/google/uuid"

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
	source   analyticsprovider.TransactionSource
	userRepo repository.UserRepository
}

func NewService(source analyticsprovider.TransactionSource, userRepo repository.UserRepository) *Service {
	return &Service{source: source, userRepo: userRepo}
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
