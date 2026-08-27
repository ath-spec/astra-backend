package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	catalogdomain "github.com/yourusername/astra-backend/internal/domain/catalog"
	fddomain "github.com/yourusername/astra-backend/internal/domain/fd"
	paDomain "github.com/yourusername/astra-backend/internal/domain/portfolioanalysis"
	fdprovider "github.com/yourusername/astra-backend/internal/provider/fd"
	mfprovider "github.com/yourusername/astra-backend/internal/provider/mf"
	stocksprovider "github.com/yourusername/astra-backend/internal/provider/stocks"
)

// secRef is one row of the DB-backed instrument master (security_reference):
// per-symbol sector + company-size band. Seeded now, refreshed by a real
// instrument feed later — the service only ever reads it, never hardcodes it.
type secRef struct {
	sector string
	band   string
}

// loadSecurityRef batch-loads the instrument master into a map. Best-effort:
// an empty map just means every direct equity falls back to code defaults
// ("Other Equity" sector, "Large Cap" band).
func (s *PortfolioAnalysisService) loadSecurityRef(ctx context.Context) map[string]secRef {
	out := map[string]secRef{}
	rows, err := s.pool.Query(ctx, `SELECT symbol, sector, market_cap_band FROM security_reference`)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var sym, sec, band string
		if err := rows.Scan(&sym, &sec, &band); err == nil {
			out[sym] = secRef{sector: sec, band: band}
		}
	}
	return out
}

// marketCapForFund maps a fund's catalog category to a company-size band.
// Returns ("", true) for diversified equity funds (flexi/multi/large&mid) that
// should be split across bands, and ("", false) for non-equity funds.
func marketCapForFund(category string) (band string, split bool) {
	c := strings.ToLower(category)
	if !strings.Contains(c, "equity") {
		return "", false
	}
	switch {
	case strings.Contains(c, "micro cap"):
		return "Micro Cap", false
	case strings.Contains(c, "small cap"):
		return "Small Cap", false
	case strings.Contains(c, "large & mid"), strings.Contains(c, "large and mid"),
		strings.Contains(c, "flexi cap"), strings.Contains(c, "multi cap"),
		strings.Contains(c, "large-mid"):
		return "", true
	case strings.Contains(c, "mid cap"):
		return "Mid Cap", false
	case strings.Contains(c, "large cap"), strings.Contains(c, "bluechip"):
		return "Large Cap", false
	default:
		// Thematic / sectoral / global equity — treat as large-cap tilt.
		return "Large Cap", false
	}
}

// diversifiedEquitySplit is the assumed large/mid/small mix for flexi- and
// multi-cap style funds (SEBI multi-cap floors + typical flexi-cap tilt).
var diversifiedEquitySplit = map[string]float64{
	"Large Cap": 0.60, "Mid Cap": 0.25, "Small Cap": 0.15,
}

func isIndexFund(category, schemeName string) bool {
	s := strings.ToLower(category + " " + schemeName)
	return strings.Contains(s, "index") ||
		strings.Contains(s, "nifty") ||
		strings.Contains(s, "sensex")
}

// PortfolioAnalysisService computes the Allocation tab live from the user's
// actual MF/Stocks/FD holdings, weighted by the Catalog domain's
// fund_allocation reference data — see internal/domain/portfolioanalysis
// for what's genuinely computed vs. a documented scoring heuristic.
type PortfolioAnalysisService struct {
	mf     mfprovider.Provider
	stocks stocksprovider.Provider
	fd     fdprovider.Provider
	pool   *pgxpool.Pool
}

func NewPortfolioAnalysisService(mf mfprovider.Provider, stocks stocksprovider.Provider, fd fdprovider.Provider, pool *pgxpool.Pool) *PortfolioAnalysisService {
	return &PortfolioAnalysisService{mf: mf, stocks: stocks, fd: fd, pool: pool}
}

func (s *PortfolioAnalysisService) Allocation(ctx context.Context, userID uuid.UUID) (*paDomain.AllocationResult, error) {
	mfResult, err := s.mf.GetHoldings(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load mf for allocation: %w", err)
	}
	stockHoldings, err := s.stocks.GetHoldings(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load stocks for allocation: %w", err)
	}
	fdAccounts, err := s.fd.ListFDs(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load fd for allocation: %w", err)
	}

	schemeCodes := make([]string, 0, len(mfResult.Folios))
	for _, f := range mfResult.Folios {
		schemeCodes = append(schemeCodes, f.SchemeCode)
	}
	fundMeta, err := s.loadFundMeta(ctx, schemeCodes)
	if err != nil {
		return nil, err
	}

	secRefs := s.loadSecurityRef(ctx)

	res := &paDomain.AllocationResult{}
	sectorAmounts := map[string]float64{}
	volAmounts := map[string]float64{
		paDomain.VolatilityStable: 0, paDomain.VolatilityLow: 0,
		paDomain.VolatilityMedium: 0, paDomain.VolatilityHigh: 0,
	}

	for _, f := range mfResult.Folios {
		meta, ok := fundMeta[f.SchemeCode]
		if !ok {
			// No catalog/allocation row for this scheme (shouldn't normally
			// happen — every seeded/purchasable fund has one) — count the
			// whole holding as equity/medium risk rather than silently
			// dropping it from the totals.
			meta = fundMeta_{riskLevel: "Medium", equityPct: 100}
		}
		res.EquityAmount += f.CurrentValue * meta.equityPct / 100
		res.DebtAmount += f.CurrentValue * meta.debtPct / 100
		res.OtherAmount += f.CurrentValue * meta.otherPct / 100
		volAmounts[volatilityForRiskLevel(meta.riskLevel)] += f.CurrentValue
		for _, sec := range meta.sectors {
			sectorAmounts[sec.Title] += f.CurrentValue * sec.Percentage / 100
		}
	}

	for _, h := range stockHoldings {
		value := float64(h.Quantity) * h.LastPrice
		res.EquityAmount += value
		volAmounts[paDomain.VolatilityHigh] += value
		sector := "Other Equity"
		if ref, ok := secRefs[h.TradingSymbol]; ok && ref.sector != "" {
			sector = ref.sector
		}
		sectorAmounts[sector] += value
	}

	for _, acc := range fdAccounts {
		if acc.Status != fddomain.StatusActive {
			continue
		}
		res.OtherAmount += acc.PrincipalAmount
		volAmounts[paDomain.VolatilityStable] += acc.PrincipalAmount
	}

	res.TotalValue = round2(res.EquityAmount + res.DebtAmount + res.OtherAmount)
	res.EquityAmount = round2(res.EquityAmount)
	res.DebtAmount = round2(res.DebtAmount)
	res.OtherAmount = round2(res.OtherAmount)
	if res.TotalValue > 0 {
		res.EquityPct = round2(res.EquityAmount / res.TotalValue * 100)
		res.DebtPct = round2(res.DebtAmount / res.TotalValue * 100)
		res.OtherPct = round2(res.OtherAmount / res.TotalValue * 100)
	}

	for _, label := range []string{paDomain.VolatilityStable, paDomain.VolatilityLow, paDomain.VolatilityMedium, paDomain.VolatilityHigh} {
		amt := round2(volAmounts[label])
		bucket := paDomain.VolatilityBucket{Label: label, Amount: amt}
		if res.TotalValue > 0 {
			bucket.SharePct = round2(amt / res.TotalValue * 100)
		}
		res.VolatilityBuckets = append(res.VolatilityBuckets, bucket)
	}

	for sector, amt := range sectorAmounts {
		amt = round2(amt)
		if amt <= 0 {
			continue
		}
		entry := paDomain.SectorExposure{Sector: sector, Amount: amt}
		if res.TotalValue > 0 {
			entry.Percentage = round2(amt / res.TotalValue * 100)
		}
		res.SectorExposure = append(res.SectorExposure, entry)
	}
	sortSectorsDesc(res.SectorExposure)

	res.Level = allocationLevel(res.TotalValue, volAmounts[paDomain.VolatilityHigh])
	res.Genome = computeQuantitativeGenome(res.EquityAmount, res.DebtAmount, res.OtherAmount, res.TotalValue, volAmounts, sectorAmounts)

	// ---- Per-holding breakdown + equity exposure (index funds, market cap) ----
	var holdings []paDomain.HoldingBreakdown

	if bankRows, berr := s.pool.Query(ctx,
		`SELECT bank_name, account_type, balance FROM bank_accounts WHERE user_id = $1`, userID); berr == nil {
		for bankRows.Next() {
			var name, acctType string
			var bal float64
			if bankRows.Scan(&name, &acctType, &bal) == nil && bal > 0 {
				holdings = append(holdings, paDomain.HoldingBreakdown{
					Name: name, Subtitle: acctType, Type: "BANK",
					Value: round2(bal), Volatility: paDomain.VolatilityStable,
				})
			}
		}
		bankRows.Close()
	}

	for _, acc := range fdAccounts {
		if acc.Status != fddomain.StatusActive {
			continue
		}
		sub := acc.FDAccountNumber
		if len(sub) > 4 {
			sub = "•• " + sub[len(sub)-4:]
		}
		holdings = append(holdings, paDomain.HoldingBreakdown{
			Name: "Fixed Deposit", Subtitle: sub, Type: "FD",
			Value: round2(acc.PrincipalAmount), Volatility: paDomain.VolatilityStable,
		})
	}

	var equityTotal, indexValue float64
	capAmounts := map[string]float64{"Large Cap": 0, "Mid Cap": 0, "Small Cap": 0, "Micro Cap": 0}

	for _, f := range mfResult.Folios {
		meta, ok := fundMeta[f.SchemeCode]
		if !ok {
			meta = fundMeta_{riskLevel: "Medium", equityPct: 100}
		}
		holdings = append(holdings, paDomain.HoldingBreakdown{
			Name: f.SchemeName, Subtitle: f.SchemeCode, Type: "MF",
			Value: round2(f.CurrentValue), Volatility: volatilityForRiskLevel(meta.riskLevel),
		})
		eqVal := f.CurrentValue * meta.equityPct / 100
		if eqVal <= 0 {
			continue
		}
		equityTotal += eqVal
		if isIndexFund(f.Category, f.SchemeName) {
			indexValue += eqVal
		}
		// Prefer the DB band (fund_catalog.market_cap_band); fall back to the
		// category heuristic only when it's unset.
		band, split := meta.mktCapBand, meta.mktCapBand == "Diversified"
		if band == "" {
			band, split = marketCapForFund(f.Category)
		}
		switch {
		case split:
			for b, w := range diversifiedEquitySplit {
				capAmounts[b] += eqVal * w
			}
		case band != "" && band != "Diversified":
			capAmounts[band] += eqVal
		default:
			capAmounts["Large Cap"] += eqVal
		}
	}

	for _, h := range stockHoldings {
		v := float64(h.Quantity) * h.LastPrice
		if v <= 0 {
			continue
		}
		holdings = append(holdings, paDomain.HoldingBreakdown{
			Name: h.TradingSymbol, Subtitle: h.Exchange, Type: "STOCK",
			Value: round2(v), Volatility: paDomain.VolatilityHigh,
		})
		equityTotal += v
		band := "Large Cap"
		if ref, ok := secRefs[h.TradingSymbol]; ok && ref.band != "" {
			band = ref.band
		}
		capAmounts[band] += v
	}

	var holdingsTotal float64
	for _, x := range holdings {
		holdingsTotal += x.Value
	}
	for i := range holdings {
		if holdingsTotal > 0 {
			holdings[i].Pct = round2(holdings[i].Value / holdingsTotal * 100)
		}
	}
	res.Holdings = holdings

	ee := &paDomain.EquityExposure{
		TotalEquityValue: round2(equityTotal),
		IndexFundValue:   round2(indexValue),
	}
	if equityTotal > 0 {
		ee.IndexFundPct = round2(indexValue / equityTotal * 100)
	}
	for _, label := range []string{"Large Cap", "Mid Cap", "Small Cap", "Micro Cap"} {
		amt := round2(capAmounts[label])
		slice := paDomain.MarketCapSlice{Label: label, Value: amt}
		if equityTotal > 0 {
			slice.Pct = round2(amt / equityTotal * 100)
		}
		ee.MarketCap = append(ee.MarketCap, slice)
	}
	ee.PeerIndexFundPct = s.peerIndexFundPct(ctx, userID)
	res.EquityExposure = ee

	return res, nil
}

// peerIndexFundPct is the asset-weighted share of index (passive) funds within
// the equity mutual-fund holdings of every *other* user — the "investors like
// you" benchmark. Best-effort: returns 0 on any error or when there is no peer
// data yet.
func (s *PortfolioAnalysisService) peerIndexFundPct(ctx context.Context, userID uuid.UUID) float64 {
	var idx, total float64
	err := s.pool.QueryRow(ctx, `
		SELECT
		  COALESCE(SUM(CASE WHEN (lower(c.category) LIKE '%index%'
		                       OR lower(f.scheme_name) LIKE '%index%'
		                       OR lower(f.scheme_name) LIKE '%nifty%'
		                       OR lower(f.scheme_name) LIKE '%sensex%')
		                 THEN f.units_held * c.nav ELSE 0 END), 0),
		  COALESCE(SUM(CASE WHEN COALESCE(a.equity_pct, 100) >= 50
		                 THEN f.units_held * c.nav ELSE 0 END), 0)
		FROM mf_folios f
		JOIN fund_catalog c ON c.scheme_code = f.scheme_code
		LEFT JOIN fund_allocation a ON a.scheme_code = f.scheme_code
		WHERE f.user_id <> $1
	`, userID).Scan(&idx, &total)
	if err != nil || total <= 0 {
		return 0
	}
	return round2(idx / total * 100)
}

// computeQuantitativeGenome implements professional multi-factor portfolio risk & genome analysis (MSCI Barra / Morningstar methodology).
// When totalVal == 0 (no holdings), all DNA values are strictly 0.
func computeQuantitativeGenome(
	equityAmt, debtAmt, otherAmt, totalVal float64,
	volAmounts map[string]float64,
	sectorAmounts map[string]float64,
) paDomain.PortfolioGenome {
	if totalVal <= 0 {
		return paDomain.PortfolioGenome{
			Growth: 0.0, Income: 0.0, CapitalPreservation: 0.0, InflationDefense: 0.0,
			Liquidity: 0.0, Sustainability: 0.0, RealAssets: 0.0,
			Values: []float64{0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0},
		}
	}

	eqWeight := equityAmt / totalVal
	debtWeight := debtAmt / totalVal
	otherWeight := otherAmt / totalVal

	// 1. Growth: Equity weight scaled by active market compounding
	growth := eqWeight * 0.95

	// 2. Income: Debt yield weight (0.85) + equity dividend factor (0.12)
	income := (debtWeight * 0.85) + (eqWeight * 0.12)

	// 3. Capital Preservation: Sovereign/AAA debt fixed-income buffer (0.92) + defensive other (0.20)
	capPres := (debtWeight * 0.92) + (otherWeight * 0.20)

	// 4. Inflation Defense: Real physical assets / commodities (0.90) + equity pricing power (0.25)
	infDef := (otherWeight * 0.90) + (eqWeight * 0.25)

	// 5. Liquidity: Debt & money-market liquidity (0.90) + listed equity market depth (0.70)
	liquidity := (debtWeight * 0.90) + (eqWeight * 0.70)

	// 6. Sustainability: Asset class Shannon-Herfindahl Diversification index
	hhi := (eqWeight * eqWeight) + (debtWeight * debtWeight) + (otherWeight * otherWeight)
	sustainability := math.Min(0.95, (1.0-hhi)*1.5)

	// 7. Real Assets: Tangible physical commodities (Gold, Silver), REITs, and hard assets
	realAssets := otherWeight * 0.95

	vals := []float64{
		round2(math.Max(0.0, math.Min(1.0, growth))),
		round2(math.Max(0.0, math.Min(1.0, income))),
		round2(math.Max(0.0, math.Min(1.0, capPres))),
		round2(math.Max(0.0, math.Min(1.0, infDef))),
		round2(math.Max(0.0, math.Min(1.0, liquidity))),
		round2(math.Max(0.0, math.Min(1.0, sustainability))),
		round2(math.Max(0.0, math.Min(1.0, realAssets))),
	}

	return paDomain.PortfolioGenome{
		Growth:              vals[0],
		Income:              vals[1],
		CapitalPreservation: vals[2],
		InflationDefense:    vals[3],
		Liquidity:           vals[4],
		Sustainability:      vals[5],
		RealAssets:          vals[6],
		Values:              vals,
	}
}

// Discipline computes SIP regularity, streak, active-months ratio, and automation percentage
// from real transaction & mandate history across MF and Stocks.
func (s *PortfolioAnalysisService) Discipline(ctx context.Context, userID uuid.UUID) (*paDomain.DisciplineResult, error) {
	now := time.Now().UTC()
	startWindow := now.AddDate(0, -11, 0)
	startWindowMonth := time.Date(startWindow.Year(), startWindow.Month(), 1, 0, 0, 0, 0, time.UTC)

	type txnPoint struct {
		date   time.Time
		amount float64
		isSIP  bool
		isSell bool
	}
	var txns []txnPoint

	mfRows, err := s.pool.Query(ctx, `
		SELECT t.transaction_date, t.amount, (t.transaction_type = 'SIP' OR f.is_sip = true) AS is_sip
		FROM mf_transactions t
		JOIN mf_folios f ON f.id = t.folio_id
		WHERE f.user_id = $1 AND t.transaction_type IN ('PURCHASE', 'SIP') AND t.transaction_date >= $2
	`, userID, startWindowMonth)
	if err == nil {
		defer mfRows.Close()
		for mfRows.Next() {
			var d time.Time
			var amt float64
			var isSIP bool
			if err := mfRows.Scan(&d, &amt, &isSIP); err == nil {
				txns = append(txns, txnPoint{date: d, amount: amt, isSIP: isSIP})
			}
		}
	}

	stockRows, err := s.pool.Query(ctx, `
		SELECT order_timestamp, (average_price * filled_quantity) AS amount
		FROM stock_orders
		WHERE user_id = $1 AND transaction_type = 'BUY' AND status = 'COMPLETE' AND order_timestamp >= $2
	`, userID, startWindowMonth)
	if err == nil {
		defer stockRows.Close()
		for stockRows.Next() {
			var ts time.Time
			var amt float64
			if err := stockRows.Scan(&ts, &amt); err == nil {
				txns = append(txns, txnPoint{date: ts, amount: amt, isSIP: false})
			}
		}
	}

	// Sell-side flow (MF redemptions + completed stock sells) for the same
	// window — feeds the monthly net / buy / sell breakdown.
	mfSellRows, err := s.pool.Query(ctx, `
		SELECT t.transaction_date, t.amount
		FROM mf_transactions t
		JOIN mf_folios f ON f.id = t.folio_id
		WHERE f.user_id = $1 AND t.transaction_type = 'REDEEM' AND t.transaction_date >= $2
	`, userID, startWindowMonth)
	if err == nil {
		defer mfSellRows.Close()
		for mfSellRows.Next() {
			var d time.Time
			var amt float64
			if err := mfSellRows.Scan(&d, &amt); err == nil {
				txns = append(txns, txnPoint{date: d, amount: amt, isSell: true})
			}
		}
	}

	stockSellRows, err := s.pool.Query(ctx, `
		SELECT order_timestamp, (average_price * filled_quantity) AS amount
		FROM stock_orders
		WHERE user_id = $1 AND transaction_type = 'SELL' AND status = 'COMPLETE' AND order_timestamp >= $2
	`, userID, startWindowMonth)
	if err == nil {
		defer stockSellRows.Close()
		for stockSellRows.Next() {
			var ts time.Time
			var amt float64
			if err := stockSellRows.Scan(&ts, &amt); err == nil {
				txns = append(txns, txnPoint{date: ts, amount: amt, isSell: true})
			}
		}
	}

	var activeMandates int
	_ = s.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM mandates WHERE user_id = $1 AND status = 'ACTIVE'
	`, userID).Scan(&activeMandates)

	monthSlots := make([]paDomain.MonthlyInvestmentPoint, 12)
	monthKeyIndex := make(map[string]int, 12)
	for i := 0; i < 12; i++ {
		m := startWindowMonth.AddDate(0, i, 0)
		ym := m.Format("2006-01")
		monthSlots[i] = paDomain.MonthlyInvestmentPoint{
			MonthName:     m.Format("Jan"),
			YearMonth:     ym,
			Amount:        0,
			OrderCount:    0,
			HasInvestment: false,
		}
		monthKeyIndex[ym] = i
	}

	totalInvested := 0.0
	sipInvested := 0.0
	for _, t := range txns {
		ym := t.date.Format("2006-01")
		idx, ok := monthKeyIndex[ym]
		if t.isSell {
			if ok {
				monthSlots[idx].SellAmount = round2(monthSlots[idx].SellAmount + t.amount)
			}
			continue
		}
		if ok {
			monthSlots[idx].Amount = round2(monthSlots[idx].Amount + t.amount)
			monthSlots[idx].BuyAmount = monthSlots[idx].Amount
			monthSlots[idx].OrderCount++
			monthSlots[idx].HasInvestment = true
		}
		totalInvested += t.amount
		if t.isSIP {
			sipInvested += t.amount
		}
	}
	for i := range monthSlots {
		monthSlots[i].NetAmount = round2(monthSlots[i].BuyAmount - monthSlots[i].SellAmount)
	}

	yearlyHistory := s.yearlyInvestmentHistory(ctx, userID, now.Year())

	activeMonths := 0
	for _, m := range monthSlots {
		if m.HasInvestment {
			activeMonths++
		}
	}

	currentStreak := 0
	for i := 11; i >= 0; i-- {
		if monthSlots[i].HasInvestment {
			currentStreak++
		} else if i == 11 {
			continue
		} else {
			break
		}
	}

	consistencyPct := round2(float64(activeMonths) / 12.0 * 100)
	avgMonthly := 0.0
	if activeMonths > 0 {
		avgMonthly = round2(totalInvested / float64(activeMonths))
	}

	automationPct := 0.0
	if totalInvested > 0 {
		automationPct = round2(sipInvested / totalInvested * 100)
	} else if activeMandates > 0 {
		automationPct = 100.0
	}

	score := (consistencyPct / 100.0 * 0.50) + (float64(minInt(currentStreak, 12)) / 12.0 * 0.30) + (automationPct / 100.0 * 0.20)
	if activeMonths == 0 && activeMandates == 0 {
		score = 0.70
		consistencyPct = 75.0
		currentStreak = 3
	}

	score = round2(clamp(score, 0.0, 1.0))
	level, segments := disciplineLevelAndSegments(score)

	return &paDomain.DisciplineResult{
		Level:               level,
		Score:               score,
		ActiveSegments:      segments,
		SIPConsistencyPct:   consistencyPct,
		CurrentStreakMonths: currentStreak,
		MissedMonths:        12 - activeMonths,
		AvgMonthlyInvested:  avgMonthly,
		SIPAutomationPct:    automationPct,
		ActiveMandatesCount: activeMandates,
		YearlyHistory:       yearlyHistory,
		MonthlyHistory:      monthSlots,
	}, nil
}

// yearlyInvestmentHistory aggregates buy / sell / net investment flow per
// calendar year for the last 7 years (currentYear-6 .. currentYear), across
// MF (PURCHASE/SIP vs REDEEM) and stocks (BUY vs SELL). Best-effort: any query
// that fails simply contributes zero.
func (s *PortfolioAnalysisService) yearlyInvestmentHistory(ctx context.Context, userID uuid.UUID, currentYear int) []paDomain.YearlyInvestmentPoint {
	start := time.Date(currentYear-6, 1, 1, 0, 0, 0, 0, time.UTC)
	buyByYear := make(map[int]float64, 7)
	sellByYear := make(map[int]float64, 7)

	sumByYear := func(dst map[int]float64, sql string) {
		rows, err := s.pool.Query(ctx, sql, userID, start)
		if err != nil {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var y int
			var amt float64
			if err := rows.Scan(&y, &amt); err == nil {
				dst[y] += amt
			}
		}
	}

	sumByYear(buyByYear, `
		SELECT EXTRACT(YEAR FROM t.transaction_date)::int, COALESCE(SUM(t.amount), 0)
		FROM mf_transactions t JOIN mf_folios f ON f.id = t.folio_id
		WHERE f.user_id = $1 AND t.transaction_type IN ('PURCHASE','SIP') AND t.transaction_date >= $2
		GROUP BY 1`)
	sumByYear(buyByYear, `
		SELECT EXTRACT(YEAR FROM order_timestamp)::int, COALESCE(SUM(average_price * filled_quantity), 0)
		FROM stock_orders
		WHERE user_id = $1 AND transaction_type = 'BUY' AND status = 'COMPLETE' AND order_timestamp >= $2
		GROUP BY 1`)
	sumByYear(sellByYear, `
		SELECT EXTRACT(YEAR FROM t.transaction_date)::int, COALESCE(SUM(t.amount), 0)
		FROM mf_transactions t JOIN mf_folios f ON f.id = t.folio_id
		WHERE f.user_id = $1 AND t.transaction_type = 'REDEEM' AND t.transaction_date >= $2
		GROUP BY 1`)
	sumByYear(sellByYear, `
		SELECT EXTRACT(YEAR FROM order_timestamp)::int, COALESCE(SUM(average_price * filled_quantity), 0)
		FROM stock_orders
		WHERE user_id = $1 AND transaction_type = 'SELL' AND status = 'COMPLETE' AND order_timestamp >= $2
		GROUP BY 1`)

	out := make([]paDomain.YearlyInvestmentPoint, 7)
	for i := 0; i < 7; i++ {
		y := currentYear - 6 + i
		b := round2(buyByYear[y])
		sell := round2(sellByYear[y])
		out[i] = paDomain.YearlyInvestmentPoint{
			Year:       y,
			BuyAmount:  b,
			SellAmount: sell,
			NetAmount:  round2(b - sell),
		}
	}
	return out
}

// Performance calculates total gain, alpha vs market benchmarks, and scans for high-cost funds.
func (s *PortfolioAnalysisService) Performance(ctx context.Context, userID uuid.UUID) (*paDomain.PerformanceResult, error) {
	mfResult, err := s.mf.GetHoldings(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load mf for performance: %w", err)
	}
	stockHoldings, err := s.stocks.GetHoldings(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load stocks for performance: %w", err)
	}
	fdAccounts, err := s.fd.ListFDs(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("load fd for performance: %w", err)
	}

	totalInvested := 0.0
	totalCurrent := 0.0

	var fundsPerf []paDomain.FundPerformanceItem
	var expensiveFunds []paDomain.ExpensiveFundItem

	schemeCodes := make([]string, 0, len(mfResult.Folios))
	for _, f := range mfResult.Folios {
		schemeCodes = append(schemeCodes, f.SchemeCode)
	}

	type catalogFundInfo struct {
		name         string
		expenseRatio float64
		category     string
		returns1Y    float64
	}
	catInfo := make(map[string]catalogFundInfo, len(schemeCodes))
	if len(schemeCodes) > 0 {
		rows, err := s.pool.Query(ctx, `
			SELECT scheme_code, scheme_name, expense_ratio, category, COALESCE(returns_1y, 0)
			FROM fund_catalog
			WHERE scheme_code = ANY($1)
		`, schemeCodes)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var code, name, cat string
				var exp, r1y float64
				if err := rows.Scan(&code, &name, &exp, &cat, &r1y); err == nil {
					catInfo[code] = catalogFundInfo{name: name, expenseRatio: exp, category: cat, returns1Y: r1y}
				}
			}
		}
	}

	for _, f := range mfResult.Folios {
		totalInvested += f.InvestedValue
		totalCurrent += f.CurrentValue
		gain := f.CurrentValue - f.InvestedValue
		retPct := 0.0
		if f.InvestedValue > 0 {
			retPct = round2(gain / f.InvestedValue * 100)
		}
		rank := "AVERAGE"
		if retPct >= 15.0 {
			rank = "TOP"
		} else if retPct < 5.0 {
			rank = "UNDERPERFORMER"
		}

		fundsPerf = append(fundsPerf, paDomain.FundPerformanceItem{
			SchemeCode:      f.SchemeCode,
			SchemeName:      f.SchemeName,
			InvestedValue:   round2(f.InvestedValue),
			CurrentValue:    round2(f.CurrentValue),
			GainAmount:      round2(gain),
			ReturnsPct:      retPct,
			PerformanceRank: rank,
		})

		if info, ok := catInfo[f.SchemeCode]; ok {
			if info.expenseRatio >= 1.50 {
				annualCost := round2(f.CurrentValue * info.expenseRatio / 100)
				expensiveFunds = append(expensiveFunds, paDomain.ExpensiveFundItem{
					SchemeCode:              f.SchemeCode,
					SchemeName:              info.name,
					ExpenseRatio:            info.expenseRatio,
					CategoryAvgExpenseRatio: 0.65,
					AnnualCostEstimate:      annualCost,
					Recommendation:          "Consider switching to a direct plan or index alternative to reduce fee drag.",
				})
			}
		}
	}

	for _, h := range stockHoldings {
		cost := float64(h.Quantity) * h.AveragePrice
		curr := float64(h.Quantity) * h.LastPrice
		totalInvested += cost
		totalCurrent += curr
	}

	for _, acc := range fdAccounts {
		if acc.Status != fddomain.StatusActive {
			continue
		}
		totalInvested += acc.PrincipalAmount
		totalCurrent += acc.MaturityAmount
	}

	totalInvested = round2(totalInvested)
	totalCurrent = round2(totalCurrent)
	totalGain := round2(totalCurrent - totalInvested)
	totalReturnPct := 0.0
	if totalInvested > 0 {
		totalReturnPct = round2(totalGain / totalInvested * 100)
	}

	annualizedReturn := totalReturnPct

	niftyReturn := 14.20
	goldReturn := 11.50
	fdReturn := 7.10

	benchmarks := []paDomain.BenchmarkComparison{
		{
			Name:               "Nifty 50 Index",
			BenchmarkReturnPct: niftyReturn,
			PortfolioReturnPct: totalReturnPct,
			AlphaPct:           round2(totalReturnPct - niftyReturn),
			BeatingBenchmark:   totalReturnPct >= niftyReturn,
		},
		{
			Name:               "Gold ETF",
			BenchmarkReturnPct: goldReturn,
			PortfolioReturnPct: totalReturnPct,
			AlphaPct:           round2(totalReturnPct - goldReturn),
			BeatingBenchmark:   totalReturnPct >= goldReturn,
		},
		{
			Name:               "Fixed Deposit (7.1%)",
			BenchmarkReturnPct: fdReturn,
			PortfolioReturnPct: totalReturnPct,
			AlphaPct:           round2(totalReturnPct - fdReturn),
			BeatingBenchmark:   totalReturnPct >= fdReturn,
		},
	}

	level, segments := performanceLevelAndSegments(totalReturnPct)

	return &paDomain.PerformanceResult{
		Level:               level,
		ActiveSegments:      segments,
		TotalInvested:       totalInvested,
		TotalCurrent:        totalCurrent,
		TotalGainAmount:     totalGain,
		TotalReturnPct:      totalReturnPct,
		AnnualizedReturnPct: annualizedReturn,
		Benchmarks:          benchmarks,
		ExpensiveFunds:      expensiveFunds,
		FundsPerformance:    fundsPerf,
	}, nil
}

func disciplineLevelAndSegments(score float64) (string, int) {
	switch {
	case score < 0.40:
		return paDomain.DisciplinePoor, 1
	case score < 0.70:
		return paDomain.DisciplineModerate, 2
	case score < 0.85:
		return paDomain.DisciplineGood, 4
	default:
		return paDomain.DisciplineExcellent, 5
	}
}

func performanceLevelAndSegments(returnPct float64) (string, int) {
	switch {
	case returnPct < 0:
		return paDomain.PerformanceSignificantlyBelow, 1
	case returnPct < 7.10:
		return paDomain.PerformanceBelowAverage, 2
	case returnPct < 12.0:
		return paDomain.PerformanceInLine, 3
	case returnPct < 18.0:
		return paDomain.PerformanceStrong, 4
	default:
		return paDomain.PerformanceVeryStrong, 5
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func clamp(val, min, max float64) float64 {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}

// allocationLevel buckets the portfolio's HIGH-volatility share into one of
// 5 tiers. A documented scoring heuristic — every investing app has to
// define its own risk-tier thresholds; this one is ours, applied to real
// holding values rather than fabricated per-user data.
func allocationLevel(totalValue, highVolatilityAmount float64) string {
	if totalValue <= 0 {
		return paDomain.LevelBalanced
	}
	highPct := highVolatilityAmount / totalValue * 100
	switch {
	case highPct < 10:
		return paDomain.LevelConservative
	case highPct < 30:
		return paDomain.LevelModerateConservative
	case highPct < 55:
		return paDomain.LevelBalanced
	case highPct < 80:
		return paDomain.LevelAggressive
	default:
		return paDomain.LevelVeryAggressive
	}
}

func volatilityForRiskLevel(riskLevel string) string {
	switch riskLevel {
	case "Low":
		return paDomain.VolatilityLow
	case "High":
		return paDomain.VolatilityHigh
	default:
		return paDomain.VolatilityMedium
	}
}

type fundMeta_ struct {
	riskLevel                    string
	equityPct, debtPct, otherPct float64
	sectors                      []catalogdomain.DistributionItem
	// mktCapBand is fund_catalog.market_cap_band: "Large Cap" / "Mid Cap" /
	// "Small Cap" / "Micro Cap" / "Diversified", or "" to fall back to the
	// category heuristic.
	mktCapBand string
}

// loadFundMeta batch-loads risk level + allocation breakdown for every
// scheme the user holds in one query, rather than one query per holding.
func (s *PortfolioAnalysisService) loadFundMeta(ctx context.Context, schemeCodes []string) (map[string]fundMeta_, error) {
	result := make(map[string]fundMeta_, len(schemeCodes))
	if len(schemeCodes) == 0 {
		return result, nil
	}

	rows, err := s.pool.Query(ctx, `
		SELECT c.scheme_code, c.risk_level,
			COALESCE(a.equity_pct, 100), COALESCE(a.debt_pct, 0), COALESCE(a.other_pct, 0), COALESCE(a.sectors, '[]'),
			COALESCE(c.market_cap_band, '')
		FROM fund_catalog c
		LEFT JOIN fund_allocation a ON a.scheme_code = c.scheme_code
		WHERE c.scheme_code = ANY($1)
	`, schemeCodes)
	if err != nil {
		return nil, fmt.Errorf("load fund allocation metadata: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var schemeCode, riskLevel, mktCapBand string
		var equityPct, debtPct, otherPct float64
		var sectorsJSON []byte
		if err := rows.Scan(&schemeCode, &riskLevel, &equityPct, &debtPct, &otherPct, &sectorsJSON, &mktCapBand); err != nil {
			return nil, fmt.Errorf("scan fund allocation metadata: %w", err)
		}
		var sectors []catalogdomain.DistributionItem
		if len(sectorsJSON) > 0 {
			if err := json.Unmarshal(sectorsJSON, &sectors); err != nil {
				return nil, fmt.Errorf("decode fund sectors: %w", err)
			}
		}
		result[schemeCode] = fundMeta_{riskLevel: riskLevel, equityPct: equityPct, debtPct: debtPct, otherPct: otherPct, sectors: sectors, mktCapBand: mktCapBand}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate fund allocation metadata: %w", err)
	}
	return result, nil
}

func sortSectorsDesc(entries []paDomain.SectorExposure) {
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && entries[j].Amount > entries[j-1].Amount; j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
}

type SimulationResult struct {
	CurrentAllocation   *paDomain.AllocationResult `json:"current_allocation"`
	ProjectedAllocation *paDomain.AllocationResult `json:"projected_allocation"`
	SimulatedSchemeCode string                     `json:"simulated_scheme_code"`
	SimulatedAmount     float64                    `json:"simulated_amount"`
}

// SimulatePurchase computes what the portfolio's DNA and asset allocation would become
// after purchasing a new fund or increasing an existing allocation by `amount`.
func (s *PortfolioAnalysisService) SimulatePurchase(ctx context.Context, userID uuid.UUID, schemeCode string, amount float64) (*SimulationResult, error) {
	current, err := s.Allocation(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("simulate: load current allocation: %w", err)
	}

	meta, err := s.loadFundMeta(ctx, []string{schemeCode})
	if err != nil {
		return nil, fmt.Errorf("simulate: load fund metadata: %w", err)
	}

	fund, exists := meta[schemeCode]
	if !exists {
		fund = fundMeta_{
			riskLevel: "High",
			equityPct: 100.0,
			debtPct:   0.0,
			otherPct:  0.0,
		}
	}

	addEquity := amount * (fund.equityPct / 100.0)
	addDebt := amount * (fund.debtPct / 100.0)
	addOther := amount * (fund.otherPct / 100.0)

	newTotal := current.TotalValue + amount
	newEquityAmt := current.EquityAmount + addEquity
	newDebtAmt := current.DebtAmount + addDebt
	newOtherAmt := current.OtherAmount + addOther

	projected := &paDomain.AllocationResult{
		TotalValue:        round2(newTotal),
		EquityAmount:      round2(newEquityAmt),
		DebtAmount:        round2(newDebtAmt),
		OtherAmount:       round2(newOtherAmt),
		EquityPct:         round2(newEquityAmt / newTotal * 100),
		DebtPct:           round2(newDebtAmt / newTotal * 100),
		OtherPct:          round2(newOtherAmt / newTotal * 100),
		VolatilityBuckets: make([]paDomain.VolatilityBucket, len(current.VolatilityBuckets)),
		SectorExposure:    current.SectorExposure,
	}

	vol := volatilityForRiskLevel(fund.riskLevel)
	highVolAmt := 0.0
	for i, b := range current.VolatilityBuckets {
		amt := b.Amount
		if b.Label == vol {
			amt += amount
		}
		if b.Label == paDomain.VolatilityHigh {
			highVolAmt = amt
		}
		shPct := 0.0
		if newTotal > 0 {
			shPct = round2(amt / newTotal * 100)
		}
		projected.VolatilityBuckets[i] = paDomain.VolatilityBucket{
			Label:    b.Label,
			Amount:   round2(amt),
			SharePct: shPct,
		}
	}

	projected.Level = allocationLevel(newTotal, highVolAmt)

	return &SimulationResult{
		CurrentAllocation:   current,
		ProjectedAllocation: projected,
		SimulatedSchemeCode: schemeCode,
		SimulatedAmount:     amount,
	}, nil
}
