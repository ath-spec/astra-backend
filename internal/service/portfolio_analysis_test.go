package service

import (
	"testing"

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
