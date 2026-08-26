// Package analytics implements the spend analytics engine: pure functions
// over a user's transaction history (internal/domain/analytics.Transaction)
// plus a thin service that fetches via the swappable
// internal/provider/analytics.TransactionSource and calls them. Every
// analyzer here is deterministic and DB-free, so it's directly unit
// testable — see engine_test.go.
package analytics

import (
	"math"
	"sort"
	"time"

	"github.com/yourusername/astra-backend/internal/apitime"
	analyticsdomain "github.com/yourusername/astra-backend/internal/domain/analytics"
)

type txn = analyticsdomain.Transaction

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func safeDiv(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}

// inRange reports whether t falls in [from, to] — both bounds inclusive, so
// a transaction occurring at exactly "now" (the common upper bound every
// analyzer below queries with) is never silently dropped.
func inRange(t time.Time, from, to time.Time) bool {
	return !t.Before(from) && !t.After(to)
}

func debitsInRange(txns []txn, from, to time.Time) []txn {
	out := make([]txn, 0, len(txns))
	for _, t := range txns {
		if t.Type == analyticsdomain.TxnDebit && inRange(t.OccurredAt, from, to) {
			out = append(out, t)
		}
	}
	return out
}

func creditsInRange(txns []txn, from, to time.Time) []txn {
	out := make([]txn, 0, len(txns))
	for _, t := range txns {
		if t.Type == analyticsdomain.TxnCredit && inRange(t.OccurredAt, from, to) {
			out = append(out, t)
		}
	}
	return out
}

func sumAmount(txns []txn) float64 {
	var total float64
	for _, t := range txns {
		total += t.Amount
	}
	return round2(total)
}

func dateKey(t time.Time) string {
	return t.Format("2006-01-02")
}

func mean(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	var sum float64
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

func stddev(vals []float64) float64 {
	if len(vals) < 2 {
		return 0
	}
	m := mean(vals)
	var sumSq float64
	for _, v := range vals {
		d := v - m
		sumSq += d * d
	}
	return math.Sqrt(sumSq / float64(len(vals)))
}

// ============================== A: Weekday vs weekend ==============================

func WeekdayVsWeekend(txns []txn, now time.Time) analyticsdomain.WeekdayWeekendResult {
	windowStart := now.AddDate(0, 0, -30)
	current := debitsInRange(txns, windowStart, now)

	perDay := map[string]float64{}
	var weekdayTotal, weekendTotal float64
	weekdayDays, weekendDays := countDaysByKind(windowStart, now)

	for _, t := range current {
		day := t.OccurredAt.Weekday().String()
		perDay[day] = round2(perDay[day] + t.Amount)
		if isWeekend(t.OccurredAt) {
			weekendTotal += t.Amount
		} else {
			weekdayTotal += t.Amount
		}
	}
	weekdayTotal, weekendTotal = round2(weekdayTotal), round2(weekendTotal)
	total := weekdayTotal + weekendTotal

	res := analyticsdomain.WeekdayWeekendResult{
		WeekdayTotal:    weekdayTotal,
		WeekendTotal:    weekendTotal,
		WeekdaySharePct: round2(safeDiv(weekdayTotal, total) * 100),
		WeekendSharePct: round2(safeDiv(weekendTotal, total) * 100),
		WeekdayDailyAvg: round2(safeDiv(weekdayTotal, float64(weekdayDays))),
		WeekendDailyAvg: round2(safeDiv(weekendTotal, float64(weekendDays))),
		PerDay:          perDay,
	}
	res.WeekendVsWeekdayRatio = round2(safeDiv(res.WeekendDailyAvg, res.WeekdayDailyAvg))

	var peakDay, lowDay string
	var peakAmt = -1.0
	var lowAmt = math.MaxFloat64
	for day, amt := range perDay {
		if amt > peakAmt {
			peakAmt, peakDay = amt, day
		}
		if amt < lowAmt {
			lowAmt, lowDay = amt, day
		}
	}
	res.PeakDay = peakDay
	res.LowestActiveDay = lowDay

	// Prior 30-60 day period, for a "is this ratio unusual for this user" signal.
	priorStart := now.AddDate(0, 0, -60)
	priorEnd := windowStart
	prior := debitsInRange(txns, priorStart, priorEnd)
	priorWeekdayDays, priorWeekendDays := countDaysByKind(priorStart, priorEnd)
	var priorWeekday, priorWeekend float64
	for _, t := range prior {
		if isWeekend(t.OccurredAt) {
			priorWeekend += t.Amount
		} else {
			priorWeekday += t.Amount
		}
	}
	priorWeekdayAvg := safeDiv(priorWeekday, float64(priorWeekdayDays))
	priorWeekendAvg := safeDiv(priorWeekend, float64(priorWeekendDays))
	if priorWeekdayAvg > 0 {
		priorRatio := round2(safeDiv(priorWeekendAvg, priorWeekdayAvg))
		res.PriorPeriodRatio = &priorRatio
		if priorRatio > 0 {
			deviation := round2(safeDiv(res.WeekendVsWeekdayRatio-priorRatio, priorRatio) * 100)
			res.RatioDeviationPct = &deviation
		}
	}

	return res
}

func countDaysByKind(from, to time.Time) (weekdays, weekend int) {
	for d := from; d.Before(to); d = d.AddDate(0, 0, 1) {
		if isWeekend(d) {
			weekend++
		} else {
			weekdays++
		}
	}
	if weekdays == 0 {
		weekdays = 1
	}
	if weekend == 0 {
		weekend = 1
	}
	return weekdays, weekend
}

func isWeekend(t time.Time) bool {
	wd := t.Weekday()
	return wd == time.Saturday || wd == time.Sunday
}

// ============================== B: Trend analytics ==============================

func TrendAnalytics(txns []txn, now time.Time, period string) analyticsdomain.TrendResult {
	var bucketDays int
	var numBuckets int
	switch period {
	case "weekly":
		bucketDays, numBuckets = 7, 8
	case "monthly":
		bucketDays, numBuckets = 30, 6
	default:
		period, bucketDays, numBuckets = "daily", 1, 14
	}

	windowStart := now.AddDate(0, 0, -bucketDays*numBuckets)
	current := debitsInRange(txns, windowStart, now)

	points := make([]analyticsdomain.TrendPoint, numBuckets)
	totals := make([]float64, numBuckets)
	for i := 0; i < numBuckets; i++ {
		bucketStart := windowStart.AddDate(0, 0, i*bucketDays)
		points[i] = analyticsdomain.TrendPoint{PeriodStart: apitime.New(bucketStart)}
	}
	for _, t := range current {
		idx := int(t.OccurredAt.Sub(windowStart).Hours() / 24 / float64(bucketDays))
		if idx < 0 {
			idx = 0
		}
		if idx >= numBuckets {
			idx = numBuckets - 1
		}
		totals[idx] += t.Amount
	}
	for i := range points {
		points[i].Total = round2(totals[i])
	}

	res := analyticsdomain.TrendResult{Period: period, Points: points}

	peakIdx, troughIdx := 0, 0
	for i, v := range totals {
		if v > totals[peakIdx] {
			peakIdx = i
		}
		if v < totals[troughIdx] {
			troughIdx = i
		}
	}
	res.PeakPeriod, res.PeakAmount = &points[peakIdx].PeriodStart, points[peakIdx].Total
	res.TroughPeriod, res.TroughAmount = &points[troughIdx].PeriodStart, points[troughIdx].Total

	first, last := totals[0], totals[len(totals)-1]
	res.FirstVsLastPct = round2(safeDiv(last-first, math.Max(first, 1)) * 100)

	m := mean(totals)
	sd := stddev(totals)
	cov := safeDiv(sd, m)
	res.CoefficientOfVariation = round2(cov)

	switch {
	case cov > 0.5:
		res.VolatilityLevel = "HIGH"
	case cov > 0.2:
		res.VolatilityLevel = "MODERATE"
	default:
		res.VolatilityLevel = "LOW"
	}

	switch {
	case cov > 0.5:
		res.Direction = "VOLATILE"
	case res.FirstVsLastPct > 5:
		res.Direction = "INCREASING"
	case res.FirstVsLastPct < -5:
		res.Direction = "DECREASING"
	default:
		res.Direction = "FLAT"
	}

	windowTotalDays := float64(bucketDays * numBuckets)
	res.SpendVelocityPerDay = round2(safeDiv(sumAmount(current), windowTotalDays))
	res.Projected30Day = round2(res.SpendVelocityPerDay * 30)

	// Overall trend alert: most-recent 30d spend vs the 30d before that.
	recent30 := sumAmount(debitsInRange(txns, now.AddDate(0, 0, -30), now))
	prior30 := sumAmount(debitsInRange(txns, now.AddDate(0, 0, -60), now.AddDate(0, 0, -30)))
	res.OverallTrendChangePct = round2(safeDiv(recent30-prior30, math.Max(prior30, 1)) * 100)
	res.OverallTrendAlert = prior30 > 0 && recent30 >= prior30*1.3

	return res
}

// ============================== C/D: Category trend, anomalies & momentum ==============================

// categoryTotals computes total spend per category for debit transactions
// in [from, to).
func categoryTotals(txns []txn, from, to time.Time) map[string]float64 {
	totals := map[string]float64{}
	for _, t := range debitsInRange(txns, from, to) {
		totals[t.Category] += t.Amount
	}
	for k, v := range totals {
		totals[k] = round2(v)
	}
	return totals
}

func CategoryTrend(txns []txn, now time.Time) analyticsdomain.CategoryTrendResult {
	current := categoryTotals(txns, now.AddDate(0, 0, -30), now)
	prior := categoryTotals(txns, now.AddDate(0, 0, -60), now.AddDate(0, 0, -30))

	var currentTotal float64
	for _, v := range current {
		currentTotal += v
	}

	names := map[string]bool{}
	for k := range current {
		names[k] = true
	}
	for k := range prior {
		names[k] = true
	}

	stats := make([]analyticsdomain.CategoryStat, 0, len(names))
	for name := range names {
		cur, pr := current[name], prior[name]
		stat := analyticsdomain.CategoryStat{
			Category: name, CurrentTotal: cur, PriorTotal: pr,
			SharePct: round2(safeDiv(cur, currentTotal) * 100),
		}
		switch {
		case pr == 0 && cur > 0:
			stat.ChangePct, stat.Direction = 100, "UP"
		case pr == 0:
			stat.ChangePct, stat.Direction = 0, "FLAT"
		default:
			stat.ChangePct = round2(safeDiv(cur-pr, pr) * 100)
			switch {
			case stat.ChangePct > 5:
				stat.Direction = "UP"
			case stat.ChangePct < -5:
				stat.Direction = "DOWN"
			default:
				stat.Direction = "FLAT"
			}
		}
		// Anomaly: current spend more than double the prior period (the
		// single-prior-period equivalent of z-backend's rolling-average
		// threshold check).
		stat.IsAnomaly = pr > 0 && cur > pr*2
		stats = append(stats, stat)
	}

	sort.Slice(stats, func(i, j int) bool { return stats[i].SharePct > stats[j].SharePct })

	res := analyticsdomain.CategoryTrendResult{Categories: stats}
	if len(stats) > 0 {
		res.TopCategory = stats[0].Category
		res.TopCategorySharePct = stats[0].SharePct
	}
	return res
}

func CategoryMomentum(catTrend analyticsdomain.CategoryTrendResult) analyticsdomain.CategoryMomentumResult {
	withHistory := make([]analyticsdomain.CategoryStat, 0, len(catTrend.Categories))
	for _, c := range catTrend.Categories {
		if c.PriorTotal > 0 || c.CurrentTotal > 0 {
			withHistory = append(withHistory, c)
		}
	}

	rising := append([]analyticsdomain.CategoryStat(nil), withHistory...)
	sort.Slice(rising, func(i, j int) bool { return rising[i].ChangePct > rising[j].ChangePct })
	falling := append([]analyticsdomain.CategoryStat(nil), withHistory...)
	sort.Slice(falling, func(i, j int) bool { return falling[i].ChangePct < falling[j].ChangePct })

	return analyticsdomain.CategoryMomentumResult{
		Rising:  topN(rising, 5),
		Falling: topN(falling, 5),
	}
}

func topN(stats []analyticsdomain.CategoryStat, n int) []analyticsdomain.CategoryStat {
	if len(stats) > n {
		return stats[:n]
	}
	return stats
}

// ============================== E: Average transaction stats ==============================

func AverageStats(txns []txn, now time.Time) analyticsdomain.AverageStatsResult {
	windowStart := now.AddDate(0, 0, -90)
	debits := debitsInRange(txns, windowStart, now)
	credits := creditsInRange(txns, windowStart, now)

	res := analyticsdomain.AverageStatsResult{TransactionCount: len(debits) + len(credits)}
	if len(debits) > 0 {
		var sum, max, min float64
		min = debits[0].Amount
		for _, t := range debits {
			sum += t.Amount
			if t.Amount > max {
				max = t.Amount
			}
			if t.Amount < min {
				min = t.Amount
			}
		}
		res.AvgDebit = round2(sum / float64(len(debits)))
		res.MaxDebit, res.MinDebit = round2(max), round2(min)
	}
	if len(credits) > 0 {
		var sum float64
		for _, t := range credits {
			sum += t.Amount
		}
		res.AvgCredit = round2(sum / float64(len(credits)))
	}
	if res.TransactionCount > 0 {
		res.AvgOverall = round2(safeDiv(sumAmount(debits)+sumAmount(credits), float64(res.TransactionCount)))
	}
	return res
}

// ============================== F: Merchant concentration & spikes ==============================

func MerchantAnalysis(txns []txn, now time.Time) analyticsdomain.MerchantAnalysisResult {
	recent := debitsInRange(txns, now.AddDate(0, 0, -30), now)
	recentTotal := sumAmount(recent)

	type acc struct {
		total float64
		count int
	}
	byMerchant := map[string]*acc{}
	for _, t := range recent {
		a := byMerchant[t.Merchant]
		if a == nil {
			a = &acc{}
			byMerchant[t.Merchant] = a
		}
		a.total += t.Amount
		a.count++
	}

	stats := make([]analyticsdomain.MerchantStat, 0, len(byMerchant))
	for name, a := range byMerchant {
		stats = append(stats, analyticsdomain.MerchantStat{
			Merchant: name, Total: round2(a.total), VisitCount: a.count,
			SharePct: round2(safeDiv(a.total, recentTotal) * 100),
		})
	}
	sort.Slice(stats, func(i, j int) bool { return stats[i].Total > stats[j].Total })

	res := analyticsdomain.MerchantAnalysisResult{TopMerchants: topNMerchant(stats, 10)}
	for i := 0; i < len(stats) && i < 3; i++ {
		res.Top3ConcentrationPct += stats[i].SharePct
	}
	res.Top3ConcentrationPct = round2(res.Top3ConcentrationPct)

	// Prior 60-90 day window, for frequency-spike and reactivation checks.
	priorHistory := debitsInRange(txns, now.AddDate(0, 0, -90), now.AddDate(0, 0, -30))
	priorVisits := map[string]int{}
	for _, t := range priorHistory {
		priorVisits[t.Merchant]++
	}
	priorMonthTotal := map[string]float64{}
	for _, t := range debitsInRange(txns, now.AddDate(0, 0, -60), now.AddDate(0, 0, -30)) {
		priorMonthTotal[t.Merchant] += t.Amount
	}

	for _, s := range stats {
		priorMonthlyAvg := float64(priorVisits[s.Merchant]) / 2.0 // 60-day prior window ~= 2 months
		s.PriorMonthlyAvg = round2(priorMonthlyAvg)
		if priorMonthlyAvg > 0 && float64(s.VisitCount) > priorMonthlyAvg*1.5 {
			s.IsFrequencySpike = true
			res.FrequencySpikes = append(res.FrequencySpikes, s)
		}

		if pm, ok := priorMonthTotal[s.Merchant]; ok && pm > 0 {
			change := round2(safeDiv(s.Total-pm, pm) * 100)
			s.MoMChangePct = &change
			if s.Total >= pm*2 {
				s.IsMoMSpike = true
				res.MoMSpikes = append(res.MoMSpikes, s)
			}
		}
	}

	// Reactivated: active in the last 30 days, silent for the 60 days
	// before that, but had meaningful history further back (90-180 days).
	olderHistory := debitsInRange(txns, now.AddDate(0, 0, -180), now.AddDate(0, 0, -90))
	olderVisits := map[string]int{}
	for _, t := range olderHistory {
		olderVisits[t.Merchant]++
	}
	for name, a := range byMerchant {
		if priorVisits[name] == 0 && olderVisits[name] >= 3 {
			res.ReactivatedMerchants = append(res.ReactivatedMerchants, analyticsdomain.ReactivatedMerchant{
				Merchant: name, RecentTotal: round2(a.total), PriorVisitCount: olderVisits[name],
			})
		}
	}

	return res
}

func topNMerchant(stats []analyticsdomain.MerchantStat, n int) []analyticsdomain.MerchantStat {
	if len(stats) > n {
		return stats[:n]
	}
	return stats
}

// ============================== G: Recurring / subscription detection ==============================

func RecurringDetection(txns []txn, now time.Time) analyticsdomain.RecurringResult {
	windowStart := now.AddDate(0, 0, -seedWindowDaysForRecurring)
	byMerchant := map[string][]txn{}
	for _, t := range debitsInRange(txns, windowStart, now) {
		byMerchant[t.Merchant] = append(byMerchant[t.Merchant], t)
	}

	var recurring []analyticsdomain.RecurringExpense
	var totalMonthly float64

	for merchant, occurrences := range byMerchant {
		if len(occurrences) < 3 {
			continue
		}
		sort.Slice(occurrences, func(i, j int) bool { return occurrences[i].OccurredAt.Before(occurrences[j].OccurredAt) })

		amounts := make([]float64, len(occurrences))
		for i, o := range occurrences {
			amounts[i] = o.Amount
		}
		avgAmount := mean(amounts)
		amountCoV := safeDiv(stddev(amounts), avgAmount)
		if amountCoV > 0.15 {
			continue // amount too inconsistent to be a recurring bill
		}

		intervals := make([]float64, 0, len(occurrences)-1)
		for i := 1; i < len(occurrences); i++ {
			intervals = append(intervals, occurrences[i].OccurredAt.Sub(occurrences[i-1].OccurredAt).Hours()/24)
		}
		avgInterval := mean(intervals)
		intervalSD := stddev(intervals)

		var frequency string
		switch {
		case avgInterval >= 25 && avgInterval <= 35:
			frequency = "MONTHLY"
		case avgInterval >= 5 && avgInterval <= 9:
			frequency = "WEEKLY"
		default:
			frequency = "IRREGULAR"
		}
		if frequency == "IRREGULAR" && intervalSD > 7 {
			continue // neither monthly nor weekly, and not even internally consistent
		}

		confidence := "MEDIUM"
		if len(occurrences) >= 4 && intervalSD <= 4 {
			confidence = "HIGH"
		}

		recurring = append(recurring, analyticsdomain.RecurringExpense{
			Merchant: merchant, AvgAmount: round2(avgAmount), Occurrences: len(occurrences),
			AvgIntervalDays: round2(avgInterval), Frequency: frequency, Confidence: confidence,
			LastOccurredAt: apitime.New(occurrences[len(occurrences)-1].OccurredAt),
		})

		switch frequency {
		case "MONTHLY":
			totalMonthly += avgAmount
		case "WEEKLY":
			totalMonthly += avgAmount * 4.33
		}
	}

	sort.Slice(recurring, func(i, j int) bool { return recurring[i].AvgAmount > recurring[j].AvgAmount })
	return analyticsdomain.RecurringResult{Recurring: recurring, TotalMonthlyEstimate: round2(totalMonthly)}
}

const seedWindowDaysForRecurring = 120

// ============================== H: Night spend & impulse ==============================

const (
	impulseSmallAmountThreshold = 500.0
	nightHourStart              = 22
	nightHourEnd                = 5
)

var impulseCategories = map[string]bool{
	"Food & Dining": true, "Shopping": true, "Entertainment": true,
}

func isNightHour(t time.Time) bool {
	h := t.Hour()
	return h >= nightHourStart || h < nightHourEnd
}

func NightAndImpulse(txns []txn, now time.Time) analyticsdomain.NightImpulseResult {
	window := debitsInRange(txns, now.AddDate(0, 0, -30), now)
	total := sumAmount(window)

	var res analyticsdomain.NightImpulseResult
	var impulseTotal float64

	for _, t := range window {
		night := isNightHour(t.OccurredAt)
		small := t.Amount < impulseSmallAmountThreshold && impulseCategories[t.Category]

		if night {
			res.NightSpendTotal += t.Amount
		}
		if night || small {
			impulseTotal += t.Amount
			reason := "SMALL_FREQUENT"
			if night {
				reason = "NIGHT_SPEND"
			}
			if len(res.ImpulseTransactions) < 25 {
				res.ImpulseTransactions = append(res.ImpulseTransactions, analyticsdomain.ImpulseTransaction{
					ID: t.ID, Merchant: t.Merchant, Amount: t.Amount,
					OccurredAt: apitime.New(t.OccurredAt), Reason: reason,
				})
			}
		}
	}

	res.NightSpendTotal = round2(res.NightSpendTotal)
	res.NightSpendSharePct = round2(safeDiv(res.NightSpendTotal, total) * 100)
	res.ImpulseTotal = round2(impulseTotal)
	res.ImpulseScore = math.Min(100, round2(safeDiv(impulseTotal, math.Max(total, 1))*100))
	return res
}

// ============================== I: Spending pattern summary ==============================

func PatternSummary(txns []txn, now time.Time) analyticsdomain.PatternSummaryResult {
	last30 := debitsInRange(txns, now.AddDate(0, 0, -30), now)
	last90 := debitsInRange(txns, now.AddDate(0, 0, -90), now)

	res := analyticsdomain.PatternSummaryResult{
		DailyAvg:         round2(safeDiv(sumAmount(last30), 30)),
		PerWeekdayTotals: map[string]float64{},
	}
	res.WeeklyAvg = round2(res.DailyAvg * 7)
	res.MonthlyAvg = round2(safeDiv(sumAmount(last90), 3))

	if len(last90) > 0 {
		largest, smallest := last90[0], last90[0]
		for _, t := range last90 {
			if t.Amount > largest.Amount {
				largest = t
			}
			if t.Amount < smallest.Amount {
				smallest = t
			}
		}
		res.LargestTransaction = toTransactionRef(largest)
		res.SmallestTransaction = toTransactionRef(smallest)
	}

	dayTotals := map[time.Time]float64{}
	for _, t := range last30 {
		res.PerWeekdayTotals[t.OccurredAt.Weekday().String()] = round2(res.PerWeekdayTotals[t.OccurredAt.Weekday().String()] + t.Amount)
		dayTotals[t.OccurredAt.UTC().Truncate(24*time.Hour)] += t.Amount
	}
	var maxDay time.Time
	var maxTotal float64
	for day, total := range dayTotals {
		if total > maxTotal {
			maxTotal, maxDay = total, day
		}
	}
	if !maxDay.IsZero() {
		d := apitime.New(maxDay)
		res.MostExpensiveDay = &d
	}
	res.MostExpensiveDayTotal = round2(maxTotal)

	return res
}

func toTransactionRef(t txn) *analyticsdomain.TransactionRef {
	return &analyticsdomain.TransactionRef{
		ID: t.ID, Merchant: t.Merchant, Amount: round2(t.Amount), OccurredAt: apitime.New(t.OccurredAt),
	}
}

// ============================== J: Home snapshot ==============================

// Snapshot needs the user's current bank balance, which lives outside the
// transaction feed — the service layer supplies it (nil if unavailable).
func Snapshot(txns []txn, now time.Time, currentBalance *float64) analyticsdomain.SnapshotResult {
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	today := debitsInRange(txns, todayStart, now)
	week := debitsInRange(txns, now.AddDate(0, 0, -7), now)

	res := analyticsdomain.SnapshotResult{
		TodaySpend:  sumAmount(today),
		WeeklySpend: sumAmount(week),
	}

	last30Debit := sumAmount(debitsInRange(txns, now.AddDate(0, 0, -30), now))
	last30Credit := sumAmount(creditsInRange(txns, now.AddDate(0, 0, -30), now))
	res.GrowthPct = round2(safeDiv(last30Debit, math.Max(last30Credit, 1)) * 100)

	if currentBalance != nil && *currentBalance > 0 {
		res.CurrentBalance = currentBalance
		todayPct := round2(safeDiv(res.TodaySpend, *currentBalance) * 100)
		weekPct := round2(safeDiv(res.WeeklySpend, *currentBalance) * 100)
		res.BalanceDrawdownTodayPct = &todayPct
		res.BalanceDrawdownWeeklyPct = &weekPct
	}

	// 6 real 5-day buckets over the last 30 days (rather than a synthesized
	// curve) — an honest "spend shape" chart from actual data.
	windowStart := now.AddDate(0, 0, -30)
	buckets := make([]float64, 6)
	for _, t := range debitsInRange(txns, windowStart, now) {
		idx := int(t.OccurredAt.Sub(windowStart).Hours() / 24 / 5)
		if idx < 0 {
			idx = 0
		}
		if idx > 5 {
			idx = 5
		}
		buckets[idx] += t.Amount
	}
	for i, v := range buckets {
		res.SpendChart = append(res.SpendChart, analyticsdomain.SpendChartPoint{
			Label: dateKey(windowStart.AddDate(0, 0, i*5)), Value: round2(v),
		})
	}

	return res
}

// ============================== K: Comparison ==============================

func Compare(txns []txn, now time.Time, by string, names []string) analyticsdomain.ComparisonResult {
	window := debitsInRange(txns, now.AddDate(0, 0, -30), now)

	totals := map[string]float64{}
	for _, t := range window {
		key := t.Category
		if by == "merchant" {
			key = t.Merchant
		}
		totals[key] += t.Amount
	}

	var grandTotal float64
	for _, v := range totals {
		grandTotal += v
	}

	filter := map[string]bool{}
	for _, n := range names {
		filter[n] = true
	}

	entries := make([]analyticsdomain.ComparisonEntry, 0, len(totals))
	for name, total := range totals {
		if len(filter) > 0 && !filter[name] {
			continue
		}
		entries = append(entries, analyticsdomain.ComparisonEntry{
			Name: name, Total: round2(total), SharePct: round2(safeDiv(total, grandTotal) * 100),
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Total > entries[j].Total })

	if len(filter) == 0 && len(entries) > 10 {
		entries = entries[:10]
	}

	return analyticsdomain.ComparisonResult{By: by, Entries: entries, Total: round2(grandTotal)}
}

// ============================== L: Investment consistency ==============================

const investmentMonthsTracked = 12

// InvestmentConsistency buckets BUY-order activity into the trailing 12
// calendar months and reports active-month %, current streak (from the most
// recent month backward), missed months, and average monthly invested
// amount. A user with zero events (never bought anything yet) gets a valid
// all-zero result, not an error.
func InvestmentConsistency(events []analyticsdomain.InvestmentEvent, now time.Time) analyticsdomain.InvestmentConsistencyResult {
	anchor := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	monthTotals := make([]float64, investmentMonthsTracked)
	monthActive := make([]bool, investmentMonthsTracked)

	for _, e := range events {
		diff := monthsBetween(e.OccurredAt, anchor)
		idx := investmentMonthsTracked - 1 - diff
		if idx < 0 || idx >= investmentMonthsTracked {
			continue
		}
		monthTotals[idx] += e.Amount
		monthActive[idx] = true
	}

	res := analyticsdomain.InvestmentConsistencyResult{MonthsTracked: investmentMonthsTracked}
	var totalInvested float64
	for i := 0; i < investmentMonthsTracked; i++ {
		if monthActive[i] {
			res.ActiveMonths++
			totalInvested += monthTotals[i]
		}
	}
	res.MissedMonths = investmentMonthsTracked - res.ActiveMonths
	res.ActiveMonthPct = round2(safeDiv(float64(res.ActiveMonths), investmentMonthsTracked) * 100)
	if res.ActiveMonths > 0 {
		res.AvgMonthlyInvested = round2(totalInvested / float64(res.ActiveMonths))
	}

	for i := investmentMonthsTracked - 1; i >= 0; i-- {
		if !monthActive[i] {
			break
		}
		res.CurrentStreakMonths++
	}

	return res
}

func monthsBetween(t, anchor time.Time) int {
	return (anchor.Year()-t.Year())*12 + int(anchor.Month()) - int(t.Month())
}

// ============================== M: BNPL exposure ==============================

var bnplProviders = map[string]bool{
	"Simpl": true, "LazyPay": true, "ZestMoney": true, "Slice": true,
	"Amazon Pay Later": true, "Paytm Postpaid": true,
}

const (
	bnplDangerRatioPct     = 20.0
	bnplDangerHistoricMult = 1.5
)

// BNPLExposure sums repayments to known BNPL providers against an
// income proxy (credits in the same window), and flags a "danger zone" if
// the ratio is high in absolute terms or has jumped sharply vs the user's
// own recent history — mirrors z-backend's domain_features.go §9.
func BNPLExposure(txns []txn, now time.Time) analyticsdomain.BNPLExposureResult {
	current := bnplRatio(txns, now.AddDate(0, 0, -30), now)
	historical := bnplRatio(txns, now.AddDate(0, 0, -90), now.AddDate(0, 0, -30))

	res := analyticsdomain.BNPLExposureResult{
		Last30DayTotal:           current.total,
		IncomeRatioPct:           current.ratioPct,
		HistoricalIncomeRatioPct: historical.ratioPct,
		Providers:                current.providers,
	}
	res.IsDangerZone = res.IncomeRatioPct > bnplDangerRatioPct ||
		(historical.ratioPct > 0 && res.IncomeRatioPct > historical.ratioPct*bnplDangerHistoricMult)
	return res
}

type bnplWindowStats struct {
	total     float64
	ratioPct  float64
	providers []analyticsdomain.BNPLProviderStat
}

func bnplRatio(txns []txn, from, to time.Time) bnplWindowStats {
	debits := debitsInRange(txns, from, to)
	income := sumAmount(creditsInRange(txns, from, to))

	type acc struct {
		total float64
		count int
	}
	byProvider := map[string]*acc{}
	var total float64
	for _, t := range debits {
		if !bnplProviders[t.Merchant] {
			continue
		}
		a := byProvider[t.Merchant]
		if a == nil {
			a = &acc{}
			byProvider[t.Merchant] = a
		}
		a.total += t.Amount
		a.count++
		total += t.Amount
	}

	providers := make([]analyticsdomain.BNPLProviderStat, 0, len(byProvider))
	for name, a := range byProvider {
		providers = append(providers, analyticsdomain.BNPLProviderStat{
			Provider: name, Total: round2(a.total), RepaymentCount: a.count,
		})
	}
	sort.Slice(providers, func(i, j int) bool { return providers[i].Total > providers[j].Total })

	return bnplWindowStats{
		total: round2(total), ratioPct: round2(safeDiv(total, math.Max(income, 1)) * 100), providers: providers,
	}
}

// ============================== N: Verified subscription load ==============================

const subscriptionVerifiedWithinDays = 40

// VerifiedSubscriptionLoad filters the recurring-detection output down to
// subscriptions that are still actually being paid (most recent occurrence
// within the last ~40 days), so a cancelled subscription that merely looked
// recurring historically doesn't inflate the reported monthly load.
func VerifiedSubscriptionLoad(txns []txn, now time.Time) analyticsdomain.SubscriptionLoadResult {
	cutoff := now.AddDate(0, 0, -subscriptionVerifiedWithinDays)

	var active []analyticsdomain.RecurringExpense
	var total float64
	for _, r := range RecurringDetection(txns, now).Recurring {
		if r.LastOccurredAt.Time().Before(cutoff) {
			continue
		}
		active = append(active, r)
		switch r.Frequency {
		case "MONTHLY":
			total += r.AvgAmount
		case "WEEKLY":
			total += r.AvgAmount * 4.33
		}
	}

	return analyticsdomain.SubscriptionLoadResult{ActiveSubscriptions: active, TotalVerifiedMonthly: round2(total)}
}

// ============================== O: Income analysis ==============================

const incomeWindowDays = 180

// IncomeAnalysis looks at CREDIT transactions to predict the next payday and
// classify income stability/frequency, mirroring z-backend's
// income_analyzer.go and insight_service.go payday-prediction logic.
func IncomeAnalysis(txns []txn, now time.Time) analyticsdomain.IncomeResult {
	credits := creditsInRange(txns, now.AddDate(0, 0, -incomeWindowDays), now)
	sort.Slice(credits, func(i, j int) bool { return credits[i].OccurredAt.Before(credits[j].OccurredAt) })

	res := analyticsdomain.IncomeResult{CreditCount: len(credits)}
	if len(credits) == 0 {
		res.StabilityLabel, res.FrequencyLabel = "IRREGULAR", "IRREGULAR"
		return res
	}

	var sumAmt float64
	bySource := map[string]float64{}
	for _, c := range credits {
		sumAmt += c.Amount
		bySource[c.Merchant] += c.Amount
	}
	res.AvgCreditAmount = round2(sumAmt / float64(len(credits)))

	var topSource string
	var topTotal float64
	for src, t := range bySource {
		if t > topTotal {
			topTotal, topSource = t, src
		}
	}
	res.PrimarySource = topSource

	if len(credits) < 2 {
		res.StabilityLabel, res.FrequencyLabel = "IRREGULAR", "OCCASIONAL"
		return res
	}

	intervals := make([]float64, 0, len(credits)-1)
	amounts := make([]float64, len(credits))
	for i, c := range credits {
		amounts[i] = c.Amount
		if i > 0 {
			intervals = append(intervals, c.OccurredAt.Sub(credits[i-1].OccurredAt).Hours()/24)
		}
	}
	avgInterval := mean(intervals)
	intervalCoV := safeDiv(stddev(intervals), avgInterval)
	amountCoV := safeDiv(stddev(amounts), mean(amounts))
	res.TypicalIntervalDays = round2(avgInterval)

	switch {
	case intervalCoV < 0.15 && amountCoV < 0.15:
		res.StabilityLabel = "STABLE"
	case intervalCoV < 0.4:
		res.StabilityLabel = "VARIABLE"
	default:
		res.StabilityLabel = "IRREGULAR"
	}

	switch {
	case avgInterval >= 25 && avgInterval <= 35:
		res.FrequencyLabel = "MONTHLY"
	case avgInterval < 25:
		res.FrequencyLabel = "OCCASIONAL"
	default:
		res.FrequencyLabel = "IRREGULAR"
	}

	predicted := apitime.New(credits[len(credits)-1].OccurredAt.AddDate(0, 0, int(math.Round(avgInterval))))
	res.NextPredictedPayday = &predicted

	return res
}
