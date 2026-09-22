package service

import (
	"testing"

	catalogdomain "github.com/yourusername/astra-backend/internal/domain/catalog"
	paDomain "github.com/yourusername/astra-backend/internal/domain/portfolioanalysis"
)

func TestAllocationLevel(t *testing.T) {
	tests := []struct {
		name                 string
		totalValue           float64
		highVolatilityAmount float64
		want                 string
	}{
		{"zero total value defaults to balanced", 0, 0, paDomain.LevelBalanced},
		{"no high-risk holdings", 100000, 0, paDomain.LevelConservative},
		{"9% high is still conservative", 100000, 9000, paDomain.LevelConservative},
		{"10% high crosses into moderate-conservative", 100000, 10000, paDomain.LevelModerateConservative},
		{"29% high stays moderate-conservative", 100000, 29000, paDomain.LevelModerateConservative},
		{"30% high crosses into balanced", 100000, 30000, paDomain.LevelBalanced},
		{"54% high stays balanced", 100000, 54000, paDomain.LevelBalanced},
		{"55% high crosses into aggressive", 100000, 55000, paDomain.LevelAggressive},
		{"79% high stays aggressive", 100000, 79000, paDomain.LevelAggressive},
		{"80% high crosses into very aggressive", 100000, 80000, paDomain.LevelVeryAggressive},
		{"100% high is very aggressive", 100000, 100000, paDomain.LevelVeryAggressive},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := allocationLevel(tt.totalValue, tt.highVolatilityAmount)
			if got != tt.want {
				t.Errorf("allocationLevel(%v, %v) = %q, want %q", tt.totalValue, tt.highVolatilityAmount, got, tt.want)
			}
		})
	}
}

func TestVolatilityForRiskLevel(t *testing.T) {
	tests := map[string]string{
		"Low":    paDomain.VolatilityLow,
		"Medium": paDomain.VolatilityMedium,
		"High":   paDomain.VolatilityHigh,
		"":       paDomain.VolatilityMedium, // unrecognized falls back to medium
		"bogus":  paDomain.VolatilityMedium,
	}
	for riskLevel, want := range tests {
		if got := volatilityForRiskLevel(riskLevel); got != want {
			t.Errorf("volatilityForRiskLevel(%q) = %q, want %q", riskLevel, got, want)
		}
	}
}

func TestSortSectorsDesc(t *testing.T) {
	entries := []paDomain.SectorExposure{
		{Sector: "A", Amount: 10},
		{Sector: "B", Amount: 50},
		{Sector: "C", Amount: 30},
	}
	sortSectorsDesc(entries)
	want := []string{"B", "C", "A"}
	for i, s := range want {
		if entries[i].Sector != s {
			t.Errorf("position %d: got %q, want %q", i, entries[i].Sector, s)
		}
	}
}

func TestDisciplineLevelAndSegments(t *testing.T) {
	tests := []struct {
		score        float64
		wantLevel    string
		wantSegments int
	}{
		{0.20, paDomain.DisciplinePoor, 1},
		{0.39, paDomain.DisciplinePoor, 1},
		{0.40, paDomain.DisciplineModerate, 2},
		{0.69, paDomain.DisciplineModerate, 2},
		{0.70, paDomain.DisciplineGood, 4},
		{0.84, paDomain.DisciplineGood, 4},
		{0.85, paDomain.DisciplineExcellent, 5},
		{1.00, paDomain.DisciplineExcellent, 5},
	}
	for _, tt := range tests {
		gotLevel, gotSegments := disciplineLevelAndSegments(tt.score)
		if gotLevel != tt.wantLevel || gotSegments != tt.wantSegments {
			t.Errorf("disciplineLevelAndSegments(%v) = (%q, %d), want (%q, %d)", tt.score, gotLevel, gotSegments, tt.wantLevel, tt.wantSegments)
		}
	}
}

func TestPerformanceLevelAndSegments(t *testing.T) {
	tests := []struct {
		returnPct    float64
		wantLevel    string
		wantSegments int
	}{
		{-5.0, paDomain.PerformanceSignificantlyBelow, 1},
		{3.0, paDomain.PerformanceBelowAverage, 2},
		{7.1, paDomain.PerformanceInLine, 3},
		{10.5, paDomain.PerformanceInLine, 3},
		{14.2, paDomain.PerformanceStrong, 4},
		{17.9, paDomain.PerformanceStrong, 4},
		{18.0, paDomain.PerformanceVeryStrong, 5},
		{25.0, paDomain.PerformanceVeryStrong, 5},
	}
	for _, tt := range tests {
		gotLevel, gotSegments := performanceLevelAndSegments(tt.returnPct)
		if gotLevel != tt.wantLevel || gotSegments != tt.wantSegments {
			t.Errorf("performanceLevelAndSegments(%v) = (%q, %d), want (%q, %d)", tt.returnPct, gotLevel, gotSegments, tt.wantLevel, tt.wantSegments)
		}
	}
}

func TestComputeQuantitativeGenome(t *testing.T) {
	// Zero holdings -> all 0
	zeroGenome := ComputeQuantitativeGenome(0, 0, 0, 0, nil, nil)
	if zeroGenome.Growth != 0 || zeroGenome.Income != 0 || len(zeroGenome.Values) != 7 {
		t.Errorf("zero genome mismatch: got %+v", zeroGenome)
	}
	for i, v := range zeroGenome.Values {
		if v != 0 {
			t.Errorf("zero genome value[%d] = %v, want 0", i, v)
		}
	}

	// 100% equity holdings -> high growth, low income/capPres
	eqGenome := ComputeQuantitativeGenome(100000, 0, 0, 100000, nil, nil)
	if eqGenome.Growth != 0.95 {
		t.Errorf("equity growth = %v, want 0.95", eqGenome.Growth)
	}
	if eqGenome.CapitalPreservation != 0.0 {
		t.Errorf("equity capPres = %v, want 0.0", eqGenome.CapitalPreservation)
	}

	// 100% debt holdings -> high income, high capPres
	debtGenome := ComputeQuantitativeGenome(0, 100000, 0, 100000, nil, nil)
	if debtGenome.Income != 0.85 {
		t.Errorf("debt income = %v, want 0.85", debtGenome.Income)
	}
	if debtGenome.CapitalPreservation != 0.92 {
		t.Errorf("debt capPres = %v, want 0.92", debtGenome.CapitalPreservation)
	}
}

func TestCatalogServiceEnrichFundInsightsWithLiveDNA(t *testing.T) {
	svc := &CatalogService{}

	// Case 1: User with zero holdings
	zeroAlloc := &paDomain.AllocationResult{
		TotalValue: 0,
		Genome:     paDomain.PortfolioGenome{Values: []float64{0, 0, 0, 0, 0, 0, 0}},
	}
	profile := &catalogdomain.FundProfile{
		Fund: catalogdomain.Fund{
			Category:   "Equity: Small Cap",
			SchemeName: "Nippon India Small Cap Fund - Growth",
		},
		Allocation: catalogdomain.AllocationBreakdown{
			EquityPct: 100,
			DebtPct:   0,
			OtherPct:  0,
		},
	}
	svc.enrichFundInsightsWithLiveDNA(profile, zeroAlloc)

	if len(profile.Insights.CurrentValues) != 7 {
		t.Fatalf("expected 7 current values, got %d", len(profile.Insights.CurrentValues))
	}
	for i, v := range profile.Insights.CurrentValues {
		if v != 0 {
			t.Errorf("zero-state current value[%d] = %v, want 0", i, v)
		}
	}
	// Projected should be the fund's intrinsic vector (high growth for small cap)
	if profile.Insights.ProjectedValues[0] < 0.90 {
		t.Errorf("expected small cap projected growth to be >= 0.90, got %v", profile.Insights.ProjectedValues[0])
	}

	// Case 2: User with existing portfolio (e.g. 50k Equity, 50k Debt)
	liveAlloc := &paDomain.AllocationResult{
		TotalValue:   100000,
		EquityAmount: 50000,
		DebtAmount:   50000,
		OtherAmount:  0,
		Genome:       ComputeQuantitativeGenome(50000, 50000, 0, 100000, nil, nil),
	}
	svc.enrichFundInsightsWithLiveDNA(profile, liveAlloc)

	for i := 0; i < 7; i++ {
		if profile.Insights.CurrentValues[i] != liveAlloc.Genome.Values[i] {
			t.Errorf("current value[%d] = %v, want %v (exact match with live allocation)",
				i, profile.Insights.CurrentValues[i], liveAlloc.Genome.Values[i])
		}
	}

	// Projected should increase growth
	if profile.Insights.ProjectedValues[0] <= profile.Insights.CurrentValues[0] {
		t.Errorf("buying small cap should increase growth: current=%v, projected=%v",
			profile.Insights.CurrentValues[0], profile.Insights.ProjectedValues[0])
	}
}
