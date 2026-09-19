package service

import (
	"context"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	rmdomain "github.com/yourusername/astra-backend/internal/domain/rm"
)

// SpendIntelligence assembles the RM's "personal spend view" for one client —
// every analyzer in the spend-analytics engine, run against that client's own
// transaction history, plus their active budget if they have set one up. Each
// analyzer runs independently: a client with too little history for one
// metric simply omits that card rather than failing the whole call. Only a
// hard fetch failure (the transaction source itself erroring) fails the
// request — every other "no data yet" case degrades to an omitted field.
func (s *RMService) SpendIntelligence(ctx context.Context, callerRMID uuid.UUID, isAdmin bool, userID uuid.UUID) (*rmdomain.ClientSpendIntelligence, error) {
	if err := s.authorizeClient(ctx, callerRMID, isAdmin, userID); err != nil {
		return nil, err
	}
	if s.spend == nil {
		return &rmdomain.ClientSpendIntelligence{}, nil
	}

	out := &rmdomain.ClientSpendIntelligence{}
	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		v, err := s.spend.WeekdayVsWeekend(gCtx, userID)
		if err != nil {
			return err
		}
		if v.WeekdayTotal > 0 || v.WeekendTotal > 0 {
			out.WeekdayWeekend = &v
		}
		return nil
	})
	g.Go(func() error {
		v, err := s.spend.Trends(gCtx, userID, "daily")
		if err != nil {
			return err
		}
		if len(v.Points) > 0 {
			out.Trend = &v
		}
		return nil
	})
	g.Go(func() error {
		v, err := s.spend.CategoryTrend(gCtx, userID)
		if err != nil {
			return err
		}
		if len(v.Categories) > 0 {
			out.CategoryTrend = &v
		}
		return nil
	})
	g.Go(func() error {
		v, err := s.spend.CategoryMomentum(gCtx, userID)
		if err != nil {
			return err
		}
		if len(v.Rising) > 0 || len(v.Falling) > 0 {
			out.CategoryMomentum = &v
		}
		return nil
	})
	g.Go(func() error {
		v, err := s.spend.AverageStats(gCtx, userID)
		if err != nil {
			return err
		}
		if v.TransactionCount > 0 {
			out.AverageStats = &v
		}
		return nil
	})
	g.Go(func() error {
		v, err := s.spend.MerchantAnalysis(gCtx, userID)
		if err != nil {
			return err
		}
		if len(v.TopMerchants) > 0 {
			out.MerchantAnalysis = &v
		}
		return nil
	})
	g.Go(func() error {
		v, err := s.spend.RecurringDetection(gCtx, userID)
		if err != nil {
			return err
		}
		if len(v.Recurring) > 0 {
			out.Recurring = &v
		}
		return nil
	})
	g.Go(func() error {
		v, err := s.spend.NightAndImpulse(gCtx, userID)
		if err != nil {
			return err
		}
		out.NightImpulse = &v
		return nil
	})
	g.Go(func() error {
		v, err := s.spend.PatternSummary(gCtx, userID)
		if err != nil {
			return err
		}
		if v.DailyAvg > 0 || v.MonthlyAvg > 0 {
			out.PatternSummary = &v
		}
		return nil
	})
	g.Go(func() error {
		v, err := s.spend.Snapshot(gCtx, userID)
		if err != nil {
			return err
		}
		out.Snapshot = &v
		return nil
	})
	g.Go(func() error {
		v, err := s.spend.Compare(gCtx, userID, "category", nil)
		if err != nil {
			return err
		}
		if len(v.Entries) > 0 {
			out.CategoryComparison = &v
		}
		return nil
	})
	g.Go(func() error {
		v, err := s.spend.InvestmentConsistency(gCtx, userID)
		if err != nil {
			return err
		}
		if v.MonthsTracked > 0 {
			out.InvestmentConsistency = &v
		}
		return nil
	})
	g.Go(func() error {
		v, err := s.spend.BNPLExposure(gCtx, userID)
		if err != nil {
			return err
		}
		out.BNPLExposure = &v
		return nil
	})
	g.Go(func() error {
		v, err := s.spend.SubscriptionLoad(gCtx, userID)
		if err != nil {
			return err
		}
		if len(v.ActiveSubscriptions) > 0 {
			out.SubscriptionLoad = &v
		}
		return nil
	})
	g.Go(func() error {
		v, err := s.spend.IncomeAnalysis(gCtx, userID)
		if err != nil {
			return err
		}
		if v.CreditCount > 0 {
			out.Income = &v
		}
		return nil
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}

	// Budget is a sibling read, not a spend-engine analyzer: a failure here
	// (or simply no budget set) never fails the request — it just omits the
	// budget card.
	if s.budget != nil {
		if snap, err := s.budgetSnapshot(ctx, userID); err == nil {
			out.Budget = snap
		}
	}

	return out, nil
}

func (s *RMService) budgetSnapshot(ctx context.Context, userID uuid.UUID) (*rmdomain.BudgetSnapshot, error) {
	latest, err := s.budget.Latest(ctx, userID, 100)
	if err != nil {
		return nil, err
	}
	if len(latest.Budgets) == 0 && latest.TotalBudget <= 0 {
		return &rmdomain.BudgetSnapshot{HasBudget: false}, nil
	}

	cats := make([]rmdomain.BudgetCategoryRow, 0, len(latest.Budgets))
	for _, b := range latest.Budgets {
		if b.IsHidden {
			continue
		}
		cats = append(cats, rmdomain.BudgetCategoryRow{
			Name:            b.CategoryName,
			Icon:            b.CategoryIcon,
			Color:           b.CategoryColor,
			BudgetedAmount:  round2(b.BudgetedAmount),
			SpentAmount:     round2(b.SpentAmount),
			RemainingAmount: round2(b.RemainingAmount),
			PercentageUsed:  round2(b.PercentageUsed),
			Status:          b.Status,
		})
	}

	return &rmdomain.BudgetSnapshot{
		HasBudget:      true,
		ActiveMonth:    monthLabel(latest.LatestYear, latest.LatestMonth),
		TotalBudget:    round2(latest.TotalBudget),
		TotalSpent:     round2(latest.TotalSpent),
		ProjectedSpend: round2(latest.ProjectedSpend),
		DaysRemaining:  latest.DaysRemainingInMonth,
		Status:         latest.Status,
		HealthScore:    round2(latest.HealthScore),
		Categories:     cats,
	}, nil
}

func monthLabel(year, month int) string {
	if year == 0 || month < 1 || month > 12 {
		return ""
	}
	names := [...]string{"", "January", "February", "March", "April", "May", "June",
		"July", "August", "September", "October", "November", "December"}
	return names[month] + " " + itoaYear(year)
}

// itoaYear formats a 4-digit year without pulling in strconv for one call site.
func itoaYear(y int) string {
	digits := [4]byte{}
	for i := 3; i >= 0; i-- {
		digits[i] = byte('0' + y%10)
		y /= 10
	}
	return string(digits[:])
}
