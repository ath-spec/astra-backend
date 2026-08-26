package mf

import (
	"testing"
	"time"
)

func TestApproxAnnualizedReturnPct(t *testing.T) {
	now := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		invested float64
		current  float64
		since    time.Time
		want     float64
	}{
		{"doubled in exactly one year", 100000, 200000, now.AddDate(-1, 0, 0), 100},
		{"zero invested is not an error", 0, 100, now.AddDate(0, 0, -30), 0},
		{"zero current is not an error", 100, 0, now.AddDate(0, 0, -30), 0},
		{"less than a day held returns zero", 100, 101, now.Add(-time.Hour), 0},
		{"unchanged value is zero return", 100000, 100000, now.AddDate(0, -6, 0), 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := approxAnnualizedReturnPct(tt.invested, tt.current, tt.since, now)
			if got != tt.want {
				t.Errorf("approxAnnualizedReturnPct(%v, %v, ...) = %v, want %v", tt.invested, tt.current, got, tt.want)
			}
		})
	}
}

func TestNavOnDate_DeterministicAndBounded(t *testing.T) {
	base := 100.0
	date := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)

	a := navOnDate("HDFC-MC-G", base, date)
	b := navOnDate("HDFC-MC-G", base, date)
	if a != b {
		t.Errorf("navOnDate is not deterministic for the same scheme/date: %v != %v", a, b)
	}

	if a < base*0.98 || a > base*1.02 {
		t.Errorf("navOnDate(%v) = %v, outside expected +/-2%% band", base, a)
	}

	other := navOnDate("SBI-BLC-G", base, date)
	if other == a {
		t.Errorf("navOnDate should vary by scheme code, got the same value %v for both", a)
	}
}

func TestNavOnDate_VariesByDay(t *testing.T) {
	base := 100.0
	day1 := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	day2 := day1.AddDate(0, 0, 1)

	if navOnDate("HDFC-MC-G", base, day1) == navOnDate("HDFC-MC-G", base, day2) {
		t.Errorf("navOnDate should (almost always) differ across calendar days")
	}
}
