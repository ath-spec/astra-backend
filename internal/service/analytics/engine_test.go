package analytics

import (
	"testing"
	"time"

	analyticsdomain "github.com/yourusername/astra-backend/internal/domain/analytics"
)

func mkTxn(daysAgo int, hour int, amount float64, typ analyticsdomain.TxnType, category, merchant string, now time.Time) txn {
	d := now.AddDate(0, 0, -daysAgo)
	return txn{
		ID: "t", Amount: amount, Type: typ, Category: category, Merchant: merchant,
		OccurredAt: time.Date(d.Year(), d.Month(), d.Day(), hour, 0, 0, 0, time.UTC),
	}
}

// fixedNow is a Monday, so weekday/weekend day counts in a 30-day window are
// deterministic across test runs regardless of when the test executes.
var fixedNow = time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)

func TestWeekdayVsWeekend_SplitsCorrectly(t *testing.T) {
	var txns []txn
	txns = append(txns, mkTxn(2, 12, 1000, analyticsdomain.TxnDebit, "Shopping", "Amazon", fixedNow))
	txns = append(txns, mkTxn(1, 12, 500, analyticsdomain.TxnDebit, "Food & Dining", "Swiggy", fixedNow))

	res := WeekdayVsWeekend(txns, fixedNow)

	total := res.WeekdayTotal + res.WeekendTotal
	if total != 1500 {
		t.Fatalf("expected total 1500, got %v (weekday=%v weekend=%v)", total, res.WeekdayTotal, res.WeekendTotal)
	}
	if got := res.WeekdaySharePct + res.WeekendSharePct; got < 99.9 || got > 100.1 {
		t.Fatalf("shares should sum to ~100%%, got %v", got)
	}
}

func TestTrendAnalytics_DetectsIncreasingDirection(t *testing.T) {
	var txns []txn
	// "daily" trend covers the last 14 buckets (windowStart = now-14d); put
	// small spend right at the window start (bucket 0) and large spend today
	// (the last bucket) for a clear, unambiguous increase.
	for i := 0; i < 5; i++ {
		txns = append(txns, mkTxn(14, 12, 100, analyticsdomain.TxnDebit, "Groceries", "DMart", fixedNow))
	}
	for i := 0; i < 5; i++ {
		txns = append(txns, mkTxn(0, 12, 1000, analyticsdomain.TxnDebit, "Groceries", "DMart", fixedNow))
	}

	res := TrendAnalytics(txns, fixedNow, "daily")
	if res.Direction != "INCREASING" && res.Direction != "VOLATILE" {
		t.Fatalf("expected INCREASING or VOLATILE direction for a sharp rise, got %s", res.Direction)
	}
	if res.FirstVsLastPct <= 0 {
		t.Fatalf("expected positive first-vs-last change, got %v", res.FirstVsLastPct)
	}
}

func TestTrendAnalytics_LowVolatilityForFlatSpend(t *testing.T) {
	var txns []txn
	// "monthly" trend covers 6 x 30-day buckets over the last 180 days; put
	// one identical-amount transaction squarely in the middle of each bucket.
	for i := 0; i < 6; i++ {
		daysAgo := 180 - (i*30 + 15)
		txns = append(txns, mkTxn(daysAgo, 12, 300, analyticsdomain.TxnDebit, "Groceries", "DMart", fixedNow))
	}
	res := TrendAnalytics(txns, fixedNow, "monthly")
	if res.VolatilityLevel != "LOW" {
		t.Fatalf("expected LOW volatility for near-identical monthly buckets, got %s (CoV=%v)", res.VolatilityLevel, res.CoefficientOfVariation)
	}
}

func TestCategoryTrend_FlagsAnomalyOnSpike(t *testing.T) {
	var txns []txn
	// Prior 30-60d: modest spend. Current 30d: more than double -> anomaly.
	txns = append(txns, mkTxn(45, 12, 1000, analyticsdomain.TxnDebit, "Shopping", "Amazon", fixedNow))
	txns = append(txns, mkTxn(5, 12, 3000, analyticsdomain.TxnDebit, "Shopping", "Amazon", fixedNow))

	res := CategoryTrend(txns, fixedNow)
	var shopping *analyticsdomain.CategoryStat
	for i := range res.Categories {
		if res.Categories[i].Category == "Shopping" {
			shopping = &res.Categories[i]
		}
	}
	if shopping == nil {
		t.Fatal("expected a Shopping category stat")
	}
	if !shopping.IsAnomaly {
		t.Fatalf("expected Shopping to be flagged as an anomaly (cur=%v prior=%v)", shopping.CurrentTotal, shopping.PriorTotal)
	}
	if shopping.Direction != "UP" {
		t.Fatalf("expected UP direction, got %s", shopping.Direction)
	}
}

func TestCategoryTrend_NoAnomalyForStableSpend(t *testing.T) {
	var txns []txn
	txns = append(txns, mkTxn(45, 12, 1000, analyticsdomain.TxnDebit, "Groceries", "DMart", fixedNow))
	txns = append(txns, mkTxn(5, 12, 1050, analyticsdomain.TxnDebit, "Groceries", "DMart", fixedNow))

	res := CategoryTrend(txns, fixedNow)
	for _, c := range res.Categories {
		if c.Category == "Groceries" && c.IsAnomaly {
			t.Fatalf("did not expect an anomaly flag for a ~5%% change, got change_pct=%v", c.ChangePct)
		}
	}
}

func TestRecurringDetection_FindsConsistentMonthlyMerchant(t *testing.T) {
	var txns []txn
	for i := 0; i < 4; i++ {
		txns = append(txns, mkTxn(i*30, 8, 599, analyticsdomain.TxnDebit, "Subscriptions", "Netflix", fixedNow))
	}
	res := RecurringDetection(txns, fixedNow)

	var found *analyticsdomain.RecurringExpense
	for i := range res.Recurring {
		if res.Recurring[i].Merchant == "Netflix" {
			found = &res.Recurring[i]
		}
	}
	if found == nil {
		t.Fatal("expected Netflix to be detected as recurring")
	}
	if found.Frequency != "MONTHLY" {
		t.Fatalf("expected MONTHLY frequency, got %s (avg_interval=%v)", found.Frequency, found.AvgIntervalDays)
	}
	if found.Confidence != "HIGH" {
		t.Fatalf("expected HIGH confidence for 4 evenly-spaced occurrences, got %s", found.Confidence)
	}
}

func TestRecurringDetection_IgnoresInconsistentAmounts(t *testing.T) {
	var txns []txn
	amounts := []float64{200, 900, 150, 1200}
	for i, amt := range amounts {
		txns = append(txns, mkTxn(i*30, 8, amt, analyticsdomain.TxnDebit, "Shopping", "Amazon", fixedNow))
	}
	res := RecurringDetection(txns, fixedNow)
	for _, r := range res.Recurring {
		if r.Merchant == "Amazon" {
			t.Fatalf("did not expect Amazon (wildly inconsistent amounts) to be flagged recurring")
		}
	}
}

func TestRecurringDetection_RequiresAtLeastThreeOccurrences(t *testing.T) {
	var txns []txn
	txns = append(txns, mkTxn(30, 8, 599, analyticsdomain.TxnDebit, "Subscriptions", "Spotify", fixedNow))
	txns = append(txns, mkTxn(60, 8, 599, analyticsdomain.TxnDebit, "Subscriptions", "Spotify", fixedNow))

	res := RecurringDetection(txns, fixedNow)
	for _, r := range res.Recurring {
		if r.Merchant == "Spotify" {
			t.Fatalf("did not expect a merchant with only 2 occurrences to be flagged recurring")
		}
	}
}

func TestMerchantAnalysis_FlagsFrequencySpike(t *testing.T) {
	var txns []txn
	// Prior 60-90d: 1 visit (so ~0.5/month baseline). Current 30d: 5 visits -> spike.
	txns = append(txns, mkTxn(75, 12, 300, analyticsdomain.TxnDebit, "Food & Dining", "Starbucks", fixedNow))
	for i := 0; i < 5; i++ {
		txns = append(txns, mkTxn(i*2, 12, 300, analyticsdomain.TxnDebit, "Food & Dining", "Starbucks", fixedNow))
	}

	res := MerchantAnalysis(txns, fixedNow)
	found := false
	for _, m := range res.FrequencySpikes {
		if m.Merchant == "Starbucks" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected Starbucks to be flagged as a frequency spike, got spikes=%+v", res.FrequencySpikes)
	}
}

func TestNightAndImpulse_ClassifiesNightHours(t *testing.T) {
	var txns []txn
	txns = append(txns, mkTxn(1, 23, 400, analyticsdomain.TxnDebit, "Food & Dining", "Zomato", fixedNow)) // night
	txns = append(txns, mkTxn(1, 13, 400, analyticsdomain.TxnDebit, "Food & Dining", "Zomato", fixedNow)) // daytime

	res := NightAndImpulse(txns, fixedNow)
	if res.NightSpendTotal != 400 {
		t.Fatalf("expected night spend total of 400, got %v", res.NightSpendTotal)
	}
	if res.NightSpendSharePct < 49 || res.NightSpendSharePct > 51 {
		t.Fatalf("expected ~50%% night share, got %v", res.NightSpendSharePct)
	}
}

func TestAverageStats_ComputesCorrectAverages(t *testing.T) {
	var txns []txn
	txns = append(txns, mkTxn(1, 12, 100, analyticsdomain.TxnDebit, "Groceries", "DMart", fixedNow))
	txns = append(txns, mkTxn(2, 12, 300, analyticsdomain.TxnDebit, "Groceries", "DMart", fixedNow))
	txns = append(txns, mkTxn(1, 12, 5000, analyticsdomain.TxnCredit, "Salary", "Employer", fixedNow))

	res := AverageStats(txns, fixedNow)
	if res.AvgDebit != 200 {
		t.Fatalf("expected avg debit 200, got %v", res.AvgDebit)
	}
	if res.MaxDebit != 300 || res.MinDebit != 100 {
		t.Fatalf("expected max/min debit 300/100, got %v/%v", res.MaxDebit, res.MinDebit)
	}
	if res.AvgCredit != 5000 {
		t.Fatalf("expected avg credit 5000, got %v", res.AvgCredit)
	}
}

func TestSnapshot_HandlesNilBalanceGracefully(t *testing.T) {
	var txns []txn
	txns = append(txns, mkTxn(0, 12, 100, analyticsdomain.TxnDebit, "Groceries", "DMart", fixedNow))

	res := Snapshot(txns, fixedNow, nil)
	if res.CurrentBalance != nil {
		t.Fatal("expected nil CurrentBalance when none supplied")
	}
	if res.BalanceDrawdownTodayPct != nil {
		t.Fatal("expected nil drawdown fields when balance is unavailable")
	}
	if res.TodaySpend != 100 {
		t.Fatalf("expected today spend 100, got %v", res.TodaySpend)
	}
}
