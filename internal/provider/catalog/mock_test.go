package catalog

import (
	"testing"
	"time"
)

func TestChartPoints_LastPointMatchesCurrentNAV(t *testing.T) {
	points := chartPoints("HDFC-MC-G", 142.38, 18.4)
	if len(points) != 13 {
		t.Fatalf("expected 13 points (12 months + current), got %d", len(points))
	}
	last := points[len(points)-1]
	if last.NAV != 142.38 {
		t.Errorf("last chart point NAV = %v, want 142.38 (must match current NAV exactly)", last.NAV)
	}
}

func TestChartPoints_ZeroReturnDoesNotDivideByZero(t *testing.T) {
	// returns1YPct of -100 would make (1 + returns/100) == 0; must not panic/Inf.
	points := chartPoints("HDFC-LIQ-G", 4521.66, -100)
	if len(points) != 13 {
		t.Fatalf("expected 13 points, got %d", len(points))
	}
	for _, p := range points {
		if p.NAV <= 0 {
			t.Errorf("chart point NAV must stay positive, got %v", p.NAV)
		}
	}
}

func TestDayJitter_DeterministicAndBounded(t *testing.T) {
	base := 100.0
	date := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)

	a := dayJitter("HDFC-MC-G", base, date)
	b := dayJitter("HDFC-MC-G", base, date)
	if a != b {
		t.Errorf("dayJitter is not deterministic for the same scheme/date: %v != %v", a, b)
	}
	if a < base*0.98 || a > base*1.02 {
		t.Errorf("dayJitter(%v) = %v, outside expected +/-2%% band", base, a)
	}
}
