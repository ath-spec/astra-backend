package service

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/yourusername/astra-backend/internal/apitime"
	rmdomain "github.com/yourusername/astra-backend/internal/domain/rm"
)

// ClientAnalytics assembles the research-grade views for one client: risk
// (drawdown/volatility), fee drag, concentration, cohort percentiles, "what
// changed since last review", and a tax-lot lens. Every value is derived from
// live data and each section ships with its own methodology note.
func (s *RMService) ClientAnalytics(ctx context.Context, callerRMID uuid.UUID, isAdmin bool, userID uuid.UUID) (*rmdomain.ClientAnalytics, error) {
	if err := s.authorizeClient(ctx, callerRMID, isAdmin, userID); err != nil {
		return nil, err
	}

	out := &rmdomain.ClientAnalytics{Methodology: analyticsMethodology()}
	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		v, err := s.computeRiskStats(gCtx, userID, 365)
		if err != nil {
			return fmt.Errorf("risk stats: %w", err)
		}
		out.Risk = v
		return nil
	})
	g.Go(func() error {
		v, err := s.computeCostSummary(gCtx, userID)
		if err != nil {
			return fmt.Errorf("cost summary: %w", err)
		}
		out.Cost = v
		return nil
	})
	g.Go(func() error {
		v, err := s.computeConcentration(gCtx, userID)
		if err != nil {
			return fmt.Errorf("concentration: %w", err)
		}
		out.Concentration = v
		return nil
	})
	g.Go(func() error {
		v, err := s.computeCohort(gCtx, userID)
		if err != nil {
			return fmt.Errorf("cohort: %w", err)
		}
		out.Cohort = v
		return nil
	})
	g.Go(func() error {
		v, err := s.computeWhatChanged(gCtx, userID)
		if err != nil {
			return fmt.Errorf("what changed: %w", err)
		}
		out.WhatChanged = v
		return nil
	})
	g.Go(func() error {
		v, err := s.computeTaxLens(gCtx, userID)
		if err != nil {
			return fmt.Errorf("tax lens: %w", err)
		}
		out.Tax = v
		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, err
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Risk: drawdown, volatility, monthly return distribution
// ---------------------------------------------------------------------------

func (s *RMService) computeRiskStats(ctx context.Context, userID uuid.UUID, days int) (*rmdomain.RiskStats, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT snapshot_date, total_wealth
		FROM portfolio_snapshots
		WHERE user_id = $1 AND snapshot_date >= CURRENT_DATE - $2::int
		ORDER BY snapshot_date ASC
	`, userID, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dates []time.Time
	var w []float64
	for rows.Next() {
		var d time.Time
		var v float64
		if err := rows.Scan(&d, &v); err != nil {
			return nil, err
		}
		if v <= 0 {
			continue
		}
		dates = append(dates, d)
		w = append(w, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rs := &rmdomain.RiskStats{Days: days, Points: len(w), RecoveryDays: -1}
	if len(w) < 3 {
		return rs, nil
	}

	// Daily returns + volatility (annualised).
	var rets []float64
	for i := 1; i < len(w); i++ {
		rets = append(rets, w[i]/w[i-1]-1)
	}
	var mean float64
	for _, r := range rets {
		mean += r
	}
	mean /= float64(len(rets))
	var varSum float64
	for _, r := range rets {
		varSum += (r - mean) * (r - mean)
	}
	if len(rets) > 1 {
		rs.VolatilityPct = round2(math.Sqrt(varSum/float64(len(rets)-1)) * math.Sqrt(252) * 100)
	}

	// Drawdown curve + max drawdown + recovery.
	peak := w[0]
	troughIdx, peakAtTrough := 0, w[0]
	rs.DrawdownCurve = make([]float64, len(w))
	rs.DrawdownDates = make([]int64, len(w))
	for i, v := range w {
		if v > peak {
			peak = v
		}
		dd := v/peak - 1
		rs.DrawdownCurve[i] = round2(dd * 100)
		rs.DrawdownDates[i] = dates[i].Unix()
		if dd < rs.MaxDrawdownPct/100 || (i == 0) {
			rs.MaxDrawdownPct = round2(dd * 100)
			troughIdx = i
			peakAtTrough = peak
		}
	}
	for i := troughIdx + 1; i < len(w); i++ {
		if w[i] >= peakAtTrough {
			rs.RecoveryDays = int(dates[i].Sub(dates[troughIdx]).Hours() / 24)
			break
		}
	}

	// Monthly returns: first vs last wealth within each calendar month.
	type mv struct{ first, last float64 }
	order := []string{}
	byMonth := map[string]*mv{}
	for i, d := range dates {
		key := d.Format("Jan '06")
		m := byMonth[key]
		if m == nil {
			m = &mv{first: w[i], last: w[i]}
			byMonth[key] = m
			order = append(order, key)
		}
		m.last = w[i]
	}
	worst, best := math.MaxFloat64, -math.MaxFloat64
	for _, k := range order {
		m := byMonth[k]
		if m.first <= 0 {
			continue
		}
		pct := round2((m.last/m.first - 1) * 100)
		rs.MonthlyReturns = append(rs.MonthlyReturns, rmdomain.MonthReturn{Label: k, Pct: pct})
		if pct < worst {
			worst = pct
		}
		if pct > best {
			best = pct
		}
	}
	if worst != math.MaxFloat64 {
		rs.WorstMonthPct = worst
	}
	if best != -math.MaxFloat64 {
		rs.BestMonthPct = best
	}
	return rs, nil
}

// ---------------------------------------------------------------------------
// Cost: weighted expense ratio, annual fee, avoidable drag
// ---------------------------------------------------------------------------

func (s *RMService) computeCostSummary(ctx context.Context, userID uuid.UUID) (*rmdomain.CostSummary, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT f.units_held * COALESCE(f.nav, 0) AS val, COALESCE(c.expense_ratio, 0) AS er
		FROM mf_folios f
		LEFT JOIN fund_catalog c ON c.scheme_code = f.scheme_code
		WHERE f.user_id = $1
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var mfValue, weighted, avoidable float64
	highCost := 0
	for rows.Next() {
		var val, er float64
		if err := rows.Scan(&val, &er); err != nil {
			return nil, err
		}
		if val <= 0 {
			continue
		}
		mfValue += val
		weighted += val * er
		if er >= 1.5 {
			highCost++
		}
		if er > 0.6 { // vs a ~0.5% direct/index baseline
			avoidable += val * (er - 0.5) / 100
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	cs := &rmdomain.CostSummary{MFValue: round2(mfValue), HighCostCount: highCost, AvoidableAnnualFee: round2(avoidable)}
	if mfValue > 0 {
		cs.WeightedExpenseRatio = round2(weighted / mfValue)
		cs.AnnualFee = round2(mfValue * cs.WeightedExpenseRatio / 100)
	}

	var wealth float64
	_ = s.pool.QueryRow(ctx,
		`SELECT total_wealth FROM portfolio_snapshots WHERE user_id = $1 ORDER BY snapshot_date DESC LIMIT 1`, userID).Scan(&wealth)
	if wealth > 0 {
		cs.FeeAsPctOfWealth = round2(cs.AnnualFee / wealth * 100)
	}

	if xr, err := s.computeXIRR(ctx, userID); err == nil && xr != nil && xr.Computed {
		cs.NetReturnPct = xr.OverallXIRRPct
		cs.GrossReturnPct = round2(xr.OverallXIRRPct + cs.WeightedExpenseRatio)
	}
	return cs, nil
}

// ---------------------------------------------------------------------------
// Concentration: single stock, AMC, fund, sector HHI, category overlap
// ---------------------------------------------------------------------------

func (s *RMService) computeConcentration(ctx context.Context, userID uuid.UUID) (*rmdomain.Concentration, error) {
	c := &rmdomain.Concentration{ByAMC: []rmdomain.ConcentrationItem{}, Overlaps: []rmdomain.FundOverlapPair{}}

	// Total portfolio value from the DNA / allocation engine (mf+stocks+fd+bank).
	var total float64
	if a, err := s.analysis.Allocation(ctx, userID); err == nil && a != nil {
		total = a.TotalValue
		var hhi float64
		for _, se := range a.SectorExposure {
			hhi += se.Percentage * se.Percentage
		}
		c.SectorHHI = round2(hhi)
		switch {
		case hhi >= 2500:
			c.SectorHHILabel = "Concentrated"
		case hhi >= 1500:
			c.SectorHHILabel = "Moderate"
		case hhi > 0:
			c.SectorHHILabel = "Diversified"
		}
	}
	pct := func(v float64) float64 {
		if total <= 0 {
			return 0
		}
		return round2(v / total * 100)
	}

	// Top stock.
	var stockName string
	var stockVal float64
	err := s.pool.QueryRow(ctx, `
		SELECT trading_symbol, quantity * COALESCE(last_price, 0) AS v
		FROM demat_holdings WHERE user_id = $1 ORDER BY v DESC LIMIT 1
	`, userID).Scan(&stockName, &stockVal)
	if err == nil && stockVal > 0 {
		c.TopStock = &rmdomain.ConcentrationItem{Label: stockName, Value: round2(stockVal), Pct: pct(stockVal)}
	}

	// MF by AMC + top fund.
	rows, err := s.pool.Query(ctx, `
		SELECT COALESCE(NULLIF(f.amc_name, ''), 'Unknown AMC') AS amc,
		       SUM(f.units_held * COALESCE(f.nav, 0)) AS v
		FROM mf_folios f WHERE f.user_id = $1 GROUP BY 1 ORDER BY v DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var amc string
		var v float64
		if err := rows.Scan(&amc, &v); err != nil {
			rows.Close()
			return nil, err
		}
		if v <= 0 {
			continue
		}
		c.ByAMC = append(c.ByAMC, rmdomain.ConcentrationItem{Label: amc, Value: round2(v), Pct: pct(v)})
	}
	rows.Close()
	if len(c.ByAMC) > 0 {
		c.TopAMC = &c.ByAMC[0]
	}

	frows, err := s.pool.Query(ctx, `
		SELECT f.scheme_name, COALESCE(NULLIF(f.category, ''), 'Other'), f.units_held * COALESCE(f.nav, 0) AS v
		FROM mf_folios f WHERE f.user_id = $1 ORDER BY v DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	type fund struct {
		name, cat string
		val       float64
	}
	var funds []fund
	for frows.Next() {
		var f fund
		if err := frows.Scan(&f.name, &f.cat, &f.val); err != nil {
			frows.Close()
			return nil, err
		}
		funds = append(funds, f)
	}
	frows.Close()
	c.HoldingsCount = len(funds)
	if len(funds) > 0 && funds[0].val > 0 {
		c.TopFund = &rmdomain.ConcentrationItem{Label: funds[0].name, Value: round2(funds[0].val), Pct: pct(funds[0].val)}
	}
	byCat := map[string][]string{}
	for _, f := range funds {
		byCat[f.cat] = append(byCat[f.cat], f.name)
	}
	for cat, names := range byCat {
		if len(names) < 2 {
			continue
		}
		for i := 0; i < len(names) && i < 3; i++ {
			for j := i + 1; j < len(names) && j < 4; j++ {
				c.Overlaps = append(c.Overlaps, rmdomain.FundOverlapPair{A: names[i], B: names[j], Category: cat})
			}
		}
	}
	return c, nil
}

// ---------------------------------------------------------------------------
// Cohort: this client's percentile vs the RM's other clients
// ---------------------------------------------------------------------------

func (s *RMService) computeCohort(ctx context.Context, userID uuid.UUID) ([]rmdomain.CohortPercentile, error) {
	// Peers = clients of the same RM.
	var rmID *uuid.UUID
	owner, found, err := s.assign.OwnerOf(ctx, userID)
	if err == nil && found {
		rmID = owner
	}
	if rmID == nil {
		return []rmdomain.CohortPercentile{}, nil
	}

	// Two cheap, comparable proxies per client:
	//   growth6m  = latest / (<=180d ago) wealth - 1
	//   invest90d = MF+stock buys in the last 90 days
	rows, err := s.pool.Query(ctx, `
		WITH latest AS (
			SELECT DISTINCT ON (user_id) user_id, total_wealth
			FROM portfolio_snapshots ORDER BY user_id, snapshot_date DESC
		), prior AS (
			SELECT DISTINCT ON (user_id) user_id, total_wealth
			FROM portfolio_snapshots WHERE snapshot_date <= CURRENT_DATE - 180
			ORDER BY user_id, snapshot_date DESC
		), invest AS (
			SELECT user_id, SUM(amt) AS v FROM (
				SELECT f.user_id, t.amount AS amt
				FROM mf_transactions t JOIN mf_folios f ON f.id = t.folio_id
				WHERE t.transaction_type IN ('PURCHASE','SIP') AND t.transaction_date >= CURRENT_DATE - 90
				UNION ALL
				SELECT o.user_id, o.quantity * COALESCE(o.average_price, o.price, 0)
				FROM stock_orders o
				WHERE o.transaction_type = 'BUY' AND o.status = 'COMPLETE' AND o.order_timestamp >= now() - INTERVAL '90 days'
			) x GROUP BY user_id
		)
		SELECT u.id,
		       CASE WHEN p.total_wealth > 0 THEN (l.total_wealth - p.total_wealth) / p.total_wealth * 100 ELSE NULL END,
		       COALESCE(i.v, 0)
		FROM users u
		LEFT JOIN latest l ON l.user_id = u.id
		LEFT JOIN prior  p ON p.user_id = u.id
		LEFT JOIN invest i ON i.user_id = u.id
		WHERE u.assigned_rm_id = $1
	`, *rmID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var growth, invest []float64
	var meGrowth, meInvest *float64
	for rows.Next() {
		var id uuid.UUID
		var g *float64
		var inv float64
		if err := rows.Scan(&id, &g, &inv); err != nil {
			return nil, err
		}
		if g != nil {
			growth = append(growth, *g)
		}
		invest = append(invest, inv)
		if id == userID {
			if g != nil {
				gg := *g
				meGrowth = &gg
			}
			ii := inv
			meInvest = &ii
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	pctRank := func(sample []float64, v float64) int {
		if len(sample) < 2 {
			return 0
		}
		below := 0
		for _, x := range sample {
			if x < v {
				below++
			}
		}
		return int(math.Round(float64(below) / float64(len(sample)) * 100))
	}
	median := func(sample []float64) float64 {
		if len(sample) == 0 {
			return 0
		}
		cp := append([]float64{}, sample...)
		sort.Float64s(cp)
		return round2(cp[len(cp)/2])
	}

	out := []rmdomain.CohortPercentile{}
	if meGrowth != nil && len(growth) >= 2 {
		out = append(out, rmdomain.CohortPercentile{
			Metric: "6-month wealth growth", Value: round2(*meGrowth),
			Percentile: pctRank(growth, *meGrowth), PeerMedian: median(growth),
			SampleSize: len(growth), HigherIsBetter: true,
		})
	}
	if meInvest != nil && len(invest) >= 2 {
		out = append(out, rmdomain.CohortPercentile{
			Metric: "New investment (90d)", Value: round2(*meInvest),
			Percentile: pctRank(invest, *meInvest), PeerMedian: median(invest),
			SampleSize: len(invest), HigherIsBetter: true,
		})
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// What changed since the last recorded review (~30d DNA snapshot)
// ---------------------------------------------------------------------------

func (s *RMService) computeWhatChanged(ctx context.Context, userID uuid.UUID) (*rmdomain.WhatChanged, error) {
	cur, err := s.analysis.Allocation(ctx, userID)
	if err != nil || cur == nil {
		return &rmdomain.WhatChanged{Changes: []rmdomain.PortfolioChange{}}, nil
	}

	var (
		snapDate               time.Time
		level                  string
		eqPct, totalVal        float64
	)
	err = s.pool.QueryRow(ctx, `
		SELECT snapshot_date, level, equity_pct, total_value
		FROM portfolio_dna_snapshots
		WHERE user_id = $1 AND snapshot_date <= CURRENT_DATE - 20
		ORDER BY snapshot_date DESC LIMIT 1
	`, userID).Scan(&snapDate, &level, &eqPct, &totalVal)

	wc := &rmdomain.WhatChanged{Changes: []rmdomain.PortfolioChange{}, LevelAfter: cur.Level}
	if err != nil {
		wc.Changes = append(wc.Changes, rmdomain.PortfolioChange{
			Text: "No earlier snapshot on file yet — this becomes the baseline for the next review.", Direction: "neutral",
		})
		return wc, nil
	}

	t := apitime.New(snapDate)
	wc.SinceDate = &t
	wc.SinceDays = int(time.Since(snapDate).Hours() / 24)
	wc.LevelBefore = level
	wc.EquityPctDelta = round2(cur.EquityPct - eqPct)
	if totalVal > 0 {
		wc.WealthPctDelta = round2((cur.TotalValue - totalVal) / totalVal * 100)
	}

	if level != cur.Level {
		wc.Changes = append(wc.Changes, rmdomain.PortfolioChange{
			Text:      fmt.Sprintf("Risk DNA moved from %s to %s.", titleCaseWords(level), titleCaseWords(cur.Level)),
			Direction: "neutral",
		})
	}
	if math.Abs(wc.EquityPctDelta) >= 3 {
		dir := "up"
		if wc.EquityPctDelta < 0 {
			dir = "down"
		}
		wc.Changes = append(wc.Changes, rmdomain.PortfolioChange{
			Text:      fmt.Sprintf("Equity weight %+.1f pts (now %.0f%%).", wc.EquityPctDelta, cur.EquityPct),
			Direction: dir,
		})
	}
	if math.Abs(wc.WealthPctDelta) >= 2 {
		dir := "up"
		if wc.WealthPctDelta < 0 {
			dir = "down"
		}
		wc.Changes = append(wc.Changes, rmdomain.PortfolioChange{
			Text:      fmt.Sprintf("Total wealth %+.1f%% over %d days.", wc.WealthPctDelta, wc.SinceDays),
			Direction: dir,
		})
	}
	if len(wc.Changes) == 0 {
		wc.Changes = append(wc.Changes, rmdomain.PortfolioChange{
			Text: "Allocation and risk level are essentially unchanged since the last snapshot.", Direction: "neutral",
		})
	}
	return wc, nil
}

// ---------------------------------------------------------------------------
// Tax lens: FIFO lot ageing for unrealised LTCG / STCG + ELSS lock-in
// ---------------------------------------------------------------------------

func (s *RMService) computeTaxLens(ctx context.Context, userID uuid.UUID) (*rmdomain.TaxLens, error) {
	now := time.Now().UTC()
	tl := &rmdomain.TaxLens{Lots: []rmdomain.TaxLot{}}

	// ---- Mutual funds ----
	rows, err := s.pool.Query(ctx, `
		SELECT f.id::text, f.scheme_name, f.units_held, COALESCE(f.nav, 0),
		       COALESCE(lower(f.category), ''),
		       t.transaction_type, t.transaction_date, t.units, t.amount
		FROM mf_folios f
		LEFT JOIN mf_transactions t ON t.folio_id = f.id
		WHERE f.user_id = $1
		ORDER BY f.id, t.transaction_date ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	type buy struct {
		when  time.Time
		units float64
		cost  float64 // per unit
	}
	type folio struct {
		name     string
		units    float64
		nav      float64
		isELSS   bool
		buys     []buy
		earliest time.Time
	}
	folios := map[string]*folio{}
	var order []string
	for rows.Next() {
		var fid, name, cat string
		var held, nav float64
		var ttype *string
		var tdate *time.Time
		var tunits, tamt *float64
		if err := rows.Scan(&fid, &name, &held, &nav, &cat, &ttype, &tdate, &tunits, &tamt); err != nil {
			rows.Close()
			return nil, err
		}
		f := folios[fid]
		if f == nil {
			f = &folio{name: name, units: held, nav: nav, isELSS: strings.Contains(cat, "elss") || strings.Contains(cat, "tax saver")}
			folios[fid] = f
			order = append(order, fid)
		}
		if ttype != nil && (*ttype == "PURCHASE" || *ttype == "SIP") && tdate != nil && tunits != nil && *tunits > 0 && tamt != nil {
			f.buys = append(f.buys, buy{when: *tdate, units: *tunits, cost: *tamt / *tunits})
			if f.earliest.IsZero() || tdate.Before(f.earliest) {
				f.earliest = *tdate
			}
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, fid := range order {
		f := folios[fid]
		if f.units <= 0 || f.nav <= 0 || len(f.buys) == 0 {
			continue
		}
		remaining := f.units
		var lot rmdomain.TaxLot
		lot.Name, lot.Type = f.name, "MF"
		for _, b := range f.buys {
			if remaining <= 0 {
				break
			}
			u := b.units
			if u > remaining {
				u = remaining
			}
			remaining -= u
			gain := (f.nav - b.cost) * u
			ageDays := now.Sub(b.when).Hours() / 24
			if ageDays >= 365 {
				lot.LTCGGain += gain
			} else {
				lot.STCGGain += gain
				if ageDays >= 275 {
					lot.NearingLTCG += f.nav * u
				}
			}
		}
		lot.CurrentValue = round2(f.units * f.nav)
		lot.UnrealisedGain = round2(lot.LTCGGain + lot.STCGGain)
		lot.LTCGGain, lot.STCGGain, lot.NearingLTCG = round2(lot.LTCGGain), round2(lot.STCGGain), round2(lot.NearingLTCG)
		tl.Lots = append(tl.Lots, lot)

		tl.LTCGGain += lot.LTCGGain
		tl.STCGGain += lot.STCGGain
		tl.NearingLTCGValue += lot.NearingLTCG
		if f.isELSS && !f.earliest.IsZero() && now.Sub(f.earliest).Hours()/24 < 3*365 {
			tl.ELSSLockedValue += lot.CurrentValue
		}
	}

	// ---- Direct stocks (FIFO on completed BUYs) ----
	srows, err := s.pool.Query(ctx, `
		SELECT trading_symbol, order_timestamp, quantity, COALESCE(average_price, price, 0)
		FROM stock_orders
		WHERE user_id = $1 AND transaction_type = 'BUY' AND status = 'COMPLETE'
		ORDER BY trading_symbol, order_timestamp ASC
	`, userID)
	if err == nil {
		defer srows.Close()
		type sbuy struct {
			when time.Time
			qty  float64
			cost float64
		}
		buysBySym := map[string][]sbuy{}
		for srows.Next() {
			var sym string
			var ts time.Time
			var qty, price float64
			if err := srows.Scan(&sym, &ts, &qty, &price); err != nil {
				continue
			}
			buysBySym[sym] = append(buysBySym[sym], sbuy{when: ts, qty: qty, cost: price})
		}
		// current positions
		crows, err := s.pool.Query(ctx,
			`SELECT trading_symbol, quantity, COALESCE(last_price, 0) FROM demat_holdings WHERE user_id = $1`, userID)
		if err == nil {
			for crows.Next() {
				var sym string
				var qty, last float64
				if err := crows.Scan(&sym, &qty, &last); err != nil {
					continue
				}
				bs := buysBySym[sym]
				if qty <= 0 || last <= 0 || len(bs) == 0 {
					continue
				}
				remaining := qty
				var lot rmdomain.TaxLot
				lot.Name, lot.Type = sym, "STOCK"
				for _, b := range bs {
					if remaining <= 0 {
						break
					}
					u := b.qty
					if u > remaining {
						u = remaining
					}
					remaining -= u
					gain := (last - b.cost) * u
					ageDays := now.Sub(b.when).Hours() / 24
					if ageDays >= 365 {
						lot.LTCGGain += gain
					} else {
						lot.STCGGain += gain
						if ageDays >= 275 {
							lot.NearingLTCG += last * u
						}
					}
				}
				lot.CurrentValue = round2(qty * last)
				lot.UnrealisedGain = round2(lot.LTCGGain + lot.STCGGain)
				lot.LTCGGain, lot.STCGGain, lot.NearingLTCG = round2(lot.LTCGGain), round2(lot.STCGGain), round2(lot.NearingLTCG)
				tl.Lots = append(tl.Lots, lot)
				tl.LTCGGain += lot.LTCGGain
				tl.STCGGain += lot.STCGGain
				tl.NearingLTCGValue += lot.NearingLTCG
			}
			crows.Close()
		}
	}

	tl.LTCGGain = round2(tl.LTCGGain)
	tl.STCGGain = round2(tl.STCGGain)
	tl.TotalUnrealisedGain = round2(tl.LTCGGain + tl.STCGGain)
	tl.NearingLTCGValue = round2(tl.NearingLTCGValue)
	tl.ELSSLockedValue = round2(tl.ELSSLockedValue)
	// Equity taxation (FY25+): LTCG 12.5% over a ₹1.25L exemption; STCG 20%.
	if tl.LTCGGain > 125000 {
		tl.EstLTCGTax = round2((tl.LTCGGain - 125000) * 0.125)
	}
	if tl.STCGGain > 0 {
		tl.EstSTCGTax = round2(tl.STCGGain * 0.20)
	}
	sort.SliceStable(tl.Lots, func(i, j int) bool { return tl.Lots[i].UnrealisedGain > tl.Lots[j].UnrealisedGain })
	return tl, nil
}

func analyticsMethodology() map[string]rmdomain.MethodNote {
	return map[string]rmdomain.MethodNote{
		"risk": {
			Method: "Daily wealth from portfolio_snapshots. Volatility = sample stdev of daily returns × √252. Drawdown = wealth ÷ running peak − 1; recovery = days from the deepest trough back to that prior peak. Monthly return = last ÷ first wealth in the calendar month − 1.",
			Read:   "Max drawdown is the worst peak-to-trough fall the client actually lived through. Volatility is annualised — compare against ~12–16% for a balanced portfolio, higher for equity-heavy.",
			Why:    "Framed drawdown history turns a scary red month into a 'we've seen this, it recovered in N days' conversation.",
		},
		"cost": {
			Method: "Weighted expense ratio = Σ(fund value × its expense ratio) ÷ total MF value. Annual fee = MF value × weighted ER. Avoidable = value in funds above ~0.6% ER × (ER − 0.5%), i.e. the drag over a direct/index baseline.",
			Read:   "Fee as % of wealth is the annual headwind before any market move. 'Avoidable' is what a switch to direct/index equivalents could save each year.",
			Why:    "Fees compound against the client silently; naming the rupee figure makes the switch conversation concrete.",
		},
		"concentration": {
			Method: "Shares are value ÷ total portfolio value. Sector HHI = Σ(sector %)² across the blended equity book (0–10,000). Overlap = two held funds in the same category.",
			Read:   "HHI under 1,500 is diversified, 1,500–2,500 moderate, above 2,500 concentrated. A single stock or AMC over ~15% is a single-point-of-failure.",
			Why:    "Concentration is the risk clients underestimate most; overlap is money paying two managers to do the same job.",
		},
		"cohort": {
			Method: "This client vs every other client of the same RM. Percentile = share of peers below this client on the metric. Peers with insufficient history are excluded.",
			Read:   "50th percentile is the middle of your book. A low percentile on investment flow flags disengagement; a high one on drawdown-adjusted growth is a reference client.",
			Why:    "Relative framing (\"you're in the top quartile of my clients for consistency\") lands harder than an absolute number.",
		},
		"what_changed": {
			Method: "Current DNA (live allocation engine) diffed against the most recent portfolio_dna_snapshots row at least 20 days old.",
			Read:   "The delta since the client was last reviewed — risk level shifts, equity-weight drift, and wealth change over the interval.",
			Why:    "Walk into a review knowing what moved, instead of rebuilding context from scratch.",
		},
		"tax": {
			Method: "FIFO lot ageing: held units matched to their purchase transactions oldest-first. Units held ≥ 365 days → LTCG, else STCG. 'Nearing LTCG' = value of units 275–365 days old. Estimated tax uses equity rules (LTCG 12.5% over a ₹1.25L exemption, STCG 20%). ELSS lock-in = 3 years from first purchase.",
			Read:   "Unrealised, not a bill — it is what would be taxable if liquidated today. 'Nearing LTCG' is value worth holding a little longer before booking gains.",
			Why:    "Timing a redemption a few weeks later, or harvesting within the exemption, is real money the client keeps.",
		},
	}
}
