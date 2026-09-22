package service

import (
	"context"
	"math"

	"github.com/google/uuid"

	catalogdomain "github.com/yourusername/astra-backend/internal/domain/catalog"
	paDomain "github.com/yourusername/astra-backend/internal/domain/portfolioanalysis"
	catalogprovider "github.com/yourusername/astra-backend/internal/provider/catalog"
	mfprovider "github.com/yourusername/astra-backend/internal/provider/mf"
	watchlistprovider "github.com/yourusername/astra-backend/internal/provider/watchlist"
)

type CatalogService struct {
	provider  catalogprovider.Provider
	mf        mfprovider.Provider
	watchlist *watchlistprovider.PostgresProvider
	analysis  *PortfolioAnalysisService
}

func NewCatalogService(
	provider catalogprovider.Provider,
	mf mfprovider.Provider,
	watchlist *watchlistprovider.PostgresProvider,
	analysis *PortfolioAnalysisService,
) *CatalogService {
	return &CatalogService{
		provider:  provider,
		mf:        mf,
		watchlist: watchlist,
		analysis:  analysis,
	}
}

func (s *CatalogService) SearchFunds(ctx context.Context, params catalogdomain.SearchParams) ([]catalogdomain.Fund, error) {
	return s.provider.SearchFunds(ctx, params)
}

func (s *CatalogService) GetFund(ctx context.Context, schemeCode string) (*catalogdomain.Fund, error) {
	return s.provider.GetFund(ctx, schemeCode)
}

func (s *CatalogService) ListNFOs(ctx context.Context) ([]catalogdomain.NFO, error) {
	return s.provider.ListNFOs(ctx)
}

// GetFundProfile returns a fund's full profile, cross-referencing the MF
// domain to fill in UserHolding when userID currently holds this fund (the
// catalog provider itself has no notion of per-user holdings), and overlaying
// the user's real live portfolio DNA and mathematically computed post-purchase projected DNA.
func (s *CatalogService) GetFundProfile(ctx context.Context, userID uuid.UUID, schemeCode string) (*catalogdomain.FundProfile, error) {
	profile, err := s.provider.GetFundProfile(ctx, schemeCode)
	if err != nil {
		return nil, err
	}

	holding, err := s.mf.GetHolding(ctx, userID, schemeCode)
	if err != nil {
		return nil, err
	}
	if holding != nil {
		profile.UserHolding = &catalogdomain.UserHolding{
			UnitsHeld:     holding.UnitsHeld,
			InvestedValue: holding.InvestedValue,
			CurrentValue:  holding.CurrentValue,
			ReturnsPct:    holding.ReturnsPct,
		}
	}

	watched, err := s.watchlist.IsWatched(ctx, userID, schemeCode)
	if err != nil {
		return nil, err
	}
	profile.IsWatched = watched

	// Live Portfolio DNA unification:
	// Overlay the user's real portfolio DNA matching the Allocation tab, and compute
	// the projected DNA after adding this fund.
	if s.analysis != nil {
		if alloc, aerr := s.analysis.Allocation(ctx, userID); aerr == nil && alloc != nil {
			s.enrichFundInsightsWithLiveDNA(profile, alloc)
		}
	}

	return profile, nil
}

// enrichFundInsightsWithLiveDNA unifies the fund profile's radar DNA with the user's actual portfolio DNA.
func (s *CatalogService) enrichFundInsightsWithLiveDNA(profile *catalogdomain.FundProfile, alloc *paDomain.AllocationResult) {
	if profile == nil || alloc == nil {
		return
	}

	fundVector := catalogprovider.ComputeFundVector(
		profile.Fund.Category,
		profile.Fund.SchemeName,
		profile.Allocation.EquityPct,
		profile.Allocation.DebtPct,
		profile.Allocation.OtherPct,
	)

	// If the user has zero holdings (new user), Current DNA is 0s and Projected is the fund's intrinsic vector.
	if alloc.TotalValue <= 0 {
		profile.Insights.CurrentValues = []float64{0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0}
		profile.Insights.ProjectedValues = fundVector
		return
	}

	// 1. Live Current DNA strictly matching Allocation screen
	currentValues := make([]float64, 7)
	if len(alloc.Genome.Values) == 7 {
		copy(currentValues, alloc.Genome.Values)
	} else {
		currentValues = []float64{
			alloc.Genome.Growth,
			alloc.Genome.Income,
			alloc.Genome.CapitalPreservation,
			alloc.Genome.InflationDefense,
			alloc.Genome.Liquidity,
			alloc.Genome.Sustainability,
			alloc.Genome.RealAssets,
		}
	}
	profile.Insights.CurrentValues = currentValues

	// 2. Mathematically simulate addition of fund into user's existing portfolio
	// Use fund's min_investment or standard ₹10,000 baseline
	simAmount := profile.Fund.MinInvestment
	if simAmount <= 0 {
		simAmount = 10000.0
	}
	// Weight between 15% and 35% for clear Aladdin what-if visualization
	weight := simAmount / (alloc.TotalValue + simAmount)
	if weight < 0.15 {
		weight = 0.20
	} else if weight > 0.35 {
		weight = 0.35
	}

	projValues := make([]float64, 7)
	for i := 0; i < 7; i++ {
		blended := (1.0-weight)*currentValues[i] + weight*fundVector[i]
		projValues[i] = math.Round(math.Max(0.0, math.Min(1.0, blended))*100) / 100
	}
	profile.Insights.ProjectedValues = projValues
}
