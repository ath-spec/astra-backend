package service

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/yourusername/astra-backend/internal/apiresponse"
	"github.com/yourusername/astra-backend/internal/apitime"
	fddomain "github.com/yourusername/astra-backend/internal/domain/fd"
	goalsdomain "github.com/yourusername/astra-backend/internal/domain/goals"
	rmdomain "github.com/yourusername/astra-backend/internal/domain/rm"
)

// ============================================================================
// XIRR — money-weighted return from dated cash flows
// ============================================================================

type cashflow struct {
	when   time.Time
	amount float64 // negative = invested by client, positive = received by client
}

// xirr solves NPV(rate) = 0 for a set of dated cash flows. Returns ok=false
// when it can't converge (fewer than two flows, or all one sign).
func xirr(flows []cashflow) (float64, bool) {
	if len(flows) < 2 {
		return 0, false
	}
	var hasPos, hasNeg bool
	t0 := flows[0].when
	for _, f := range flows {
		if f.amount > 0 {
			hasPos = true
		} else if f.amount < 0 {
			hasNeg = true
		}
		if f.when.Before(t0) {
			t0 = f.when
		}
	}
	if !hasPos || !hasNeg {
		return 0, false
	}

	years := func(t time.Time) float64 { return t.Sub(t0).Hours() / 24 / 365.0 }
	npv := func(rate float64) float64 {
		var s float64
		for _, f := range flows {
			s += f.amount / math.Pow(1+rate, years(f.when))
		}
		return s
	}
	dNpv := func(rate float64) float64 {
		var s float64
		for _, f := range flows {
			y := years(f.when)
			s += -y * f.amount / math.Pow(1+rate, y+1)
		}
		return s
	}

	// Newton-Raphson from 10%.
	rate := 0.1
	for i := 0; i < 100; i++ {
		d := dNpv(rate)
		if math.Abs(d) < 1e-10 {
			break
		}
		next := rate - npv(rate)/d
		if math.IsNaN(next) || math.IsInf(next, 0) {
			break
		}
		if math.Abs(next-rate) < 1e-7 {
			if next > -0.9999 && next < 1e6 {
				return next, true
			}
			break
		}
		rate = next
	}

	// Bisection fallback on [-0.99, 10].
	lo, hi := -0.99, 10.0
	flo, fhi := npv(lo), npv(hi)
	if flo*fhi > 0 {
		return 0, false
	}
	for i := 0; i < 200; i++ {
		mid := (lo + hi) / 2
		fm := npv(mid)
		if math.Abs(fm) < 1e-6 {
			return mid, true
		}
		if flo*fm < 0 {
			hi = mid
		} else {
			lo, flo = mid, fm
		}
	}
	return (lo + hi) / 2, true
}

// inr renders a compact rupee string for action copy.
func inr(v float64) string {
	a := math.Abs(v)
	switch {
	case a >= 1e7:
		return fmt.Sprintf("₹%.2f Cr", v/1e7)
	case a >= 1e5:
		return fmt.Sprintf("₹%.2f L", v/1e5)
	case a >= 1e3:
		return fmt.Sprintf("₹%.1fK", v/1e3)
	default:
		return fmt.Sprintf("₹%.0f", v)
	}
}

// ============================================================================
// Client-level advisory
// ============================================================================

// ClientAdvisory computes actionable, fully-derived insights for one client:
// true XIRR, deployable idle cash, goal funding projections, maturing FDs, and
// a ranked Next-Best-Action list. Same book-ownership check as GetClient.
func (s *RMService) ClientAdvisory(ctx context.Context, callerRMID uuid.UUID, isAdmin bool, userID uuid.UUID) (*rmdomain.ClientAdvisory, error) {
	if err := s.authorizeClient(ctx, callerRMID, isAdmin, userID); err != nil {
		return nil, err
	}

	var (
		idle       *rmdomain.IdleCashResult
		xr         *rmdomain.XIRRResult
		goalsList  []goalsdomain.Goal
		fdAccts    []fddomain.Account
		assumedRet = 9.0
		monthlyInv float64
		signals    []rmdomain.AdvisoryAction
	)

	g, gCtx := errgroup.WithContext(ctx)
	g.Go(func() error {
		v, err := s.computeIdleCash(gCtx, userID)
		if err != nil {
			return err
		}
		idle = v
		return nil
	})
	g.Go(func() error {
		v, err := s.computeXIRR(gCtx, userID)
		if err != nil {
			return err
		}
		xr = v
		return nil
	})
	g.Go(func() error {
		v, err := s.goals.ListGoals(gCtx, userID)
		if err != nil {
			return fmt.Errorf("advisory goals: %w", err)
		}
		goalsList = v
		return nil
	})
	g.Go(func() error {
		v, err := s.fd.ListFDs(gCtx, userID)
		if err != nil {
			return fmt.Errorf("advisory fd: %w", err)
		}
		fdAccts = v
		return nil
	})
	g.Go(func() error {
		if a, err := s.analysis.Allocation(gCtx, userID); err == nil && a != nil {
			eq := a.EquityPct / 100
			if eq < 0 {
				eq = 0
			} else if eq > 1 {
				eq = 1
			}
			assumedRet = round2(eq*11 + (1-eq)*6.5)
		}
		if d, err := s.analysis.Discipline(gCtx, userID); err == nil && d != nil {
			monthlyInv = d.AvgMonthlyInvested
		}
		return nil
	})
	g.Go(func() error {
		v, err := s.clientRiskSignals(gCtx, userID)
		if err != nil {
			return err
		}
		signals = v
		return nil
	})
	if err := g.Wait(); err != nil {
		return nil, err
	}

	projections := s.computeGoalProjections(goalsList, assumedRet, monthlyInv)
	maturing := maturingFDs(fdAccts, 90)

	actions := append([]rmdomain.AdvisoryAction{}, signals...)
	if idle != nil && idle.IdleAmount >= 25000 {
		pri := 3
		if idle.IdleAmount >= 500000 {
			pri = 2
		}
		actions = append(actions, rmdomain.AdvisoryAction{
			Kind:     "deploy_cash",
			Priority: pri,
			Title:    "Deploy idle cash",
			Detail:   fmt.Sprintf("%s in bank — about %s over a 6-month spending buffer.", inr(idle.BankTotal), inr(idle.IdleAmount)),
			Amount:   idle.IdleAmount,
		})
	}
	for _, p := range projections {
		if p.ProjectedShortfall > 0 && p.AdditionalMonthlyRequired > 0 {
			actions = append(actions, rmdomain.AdvisoryAction{
				Kind:     "goal_gap",
				Priority: 4,
				Title:    fmt.Sprintf("Goal %q is behind plan", p.Name),
				Detail:   fmt.Sprintf("On current run-rate it reaches %s vs a %s target — needs about +%s/mo.", inr(p.ProjectedAmount), inr(p.TargetAmount), inr(p.AdditionalMonthlyRequired)),
				Amount:   p.ProjectedShortfall,
			})
		}
	}
	for _, fd := range maturing {
		pri := 3
		if fd.DaysToMaturity <= 30 {
			pri = 2
		}
		actions = append(actions, rmdomain.AdvisoryAction{
			Kind:     "fd_maturing",
			Priority: pri,
			Title:    "FD maturing soon",
			Detail:   fmt.Sprintf("%s matures in %d days — plan the reinvestment.", inr(fd.MaturityAmount), fd.DaysToMaturity),
			Amount:   fd.MaturityAmount,
		})
	}

	sortActions(actions)

	out := &rmdomain.ClientAdvisory{
		Actions:         actions,
		XIRR:            xr,
		IdleCash:        idle,
		GoalProjections: projections,
		MaturingFDs:     maturing,
	}
	if len(actions) > 0 {
		out.NextBestAction = &actions[0]
	}
	return out, nil
}

func (s *RMService) computeIdleCash(ctx context.Context, userID uuid.UUID) (*rmdomain.IdleCashResult, error) {
	var bank, debit90 float64
	if err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(balance),0) FROM bank_accounts WHERE user_id = $1`, userID,
	).Scan(&bank); err != nil {
		return nil, fmt.Errorf("idle cash bank: %w", err)
	}
	if err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(amount),0) FROM spend_transactions
		 WHERE user_id = $1 AND type = 'DEBIT' AND occurred_at >= now() - INTERVAL '90 days'`, userID,
	).Scan(&debit90); err != nil {
		return nil, fmt.Errorf("idle cash spend: %w", err)
	}

	avgMonthly := debit90 / 3.0
	buffer := avgMonthly * 6
	idle := bank - buffer
	if idle < 0 {
		idle = 0
	}
	res := &rmdomain.IdleCashResult{
		BankTotal:       round2(bank),
		AvgMonthlySpend: round2(avgMonthly),
		EmergencyBuffer: round2(buffer),
		IdleAmount:      round2(idle),
	}
	if avgMonthly > 0 {
		res.MonthsOfSpendHeld = round2(bank / avgMonthly)
	}
	return res, nil
}

func (s *RMService) computeXIRR(ctx context.Context, userID uuid.UUID) (*rmdomain.XIRRResult, error) {
	now := time.Now().UTC()
	out := &rmdomain.XIRRResult{Holdings: []rmdomain.HoldingXIRR{}}
	var all []cashflow

	// ---- Mutual funds, grouped by folio ----
	rows, err := s.pool.Query(ctx, `
		SELECT f.id::text, f.scheme_name, f.units_held * COALESCE(f.nav, 0) AS cur_val,
		       t.transaction_type, t.transaction_date, t.amount
		FROM mf_folios f
		LEFT JOIN mf_transactions t ON t.folio_id = f.id
		WHERE f.user_id = $1
		ORDER BY f.id, t.transaction_date
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("xirr mf: %w", err)
	}
	type acc struct {
		name   string
		curVal float64
		flows  []cashflow
	}
	folios := map[string]*acc{}
	var folioOrder []string
	for rows.Next() {
		var fid, name string
		var curVal float64
		var ttype *string
		var tdate *time.Time
		var amt *float64
		if err := rows.Scan(&fid, &name, &curVal, &ttype, &tdate, &amt); err != nil {
			rows.Close()
			return nil, fmt.Errorf("xirr mf scan: %w", err)
		}
		fa := folios[fid]
		if fa == nil {
			fa = &acc{name: name, curVal: curVal}
			folios[fid] = fa
			folioOrder = append(folioOrder, fid)
		}
		if ttype != nil && tdate != nil && amt != nil {
			sign := -1.0
			if *ttype == "REDEEM" {
				sign = 1.0
			}
			fa.flows = append(fa.flows, cashflow{when: *tdate, amount: sign * *amt})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("xirr mf iterate: %w", err)
	}
	for _, fid := range folioOrder {
		fa := folios[fid]
		flows := append([]cashflow{}, fa.flows...)
		if fa.curVal > 0 {
			flows = append(flows, cashflow{when: now, amount: fa.curVal})
		}
		all = append(all, flows...)
		if r, ok := xirr(flows); ok {
			out.Holdings = append(out.Holdings, rmdomain.HoldingXIRR{Name: fa.name, Type: "MF", XIRRPct: round2(r * 100), Computed: true})
		} else {
			out.Holdings = append(out.Holdings, rmdomain.HoldingXIRR{Name: fa.name, Type: "MF"})
		}
	}

	// ---- Stocks, grouped by trading symbol ----
	srows, err := s.pool.Query(ctx, `
		SELECT o.trading_symbol, o.transaction_type, o.order_timestamp,
		       o.quantity * COALESCE(o.average_price, o.price, 0) AS value
		FROM stock_orders o
		WHERE o.user_id = $1 AND o.status = 'COMPLETE' AND o.transaction_type IN ('BUY','SELL')
		ORDER BY o.trading_symbol, o.order_timestamp
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("xirr stocks: %w", err)
	}
	syms := map[string][]cashflow{}
	var symOrder []string
	for srows.Next() {
		var sym, ttype string
		var ts time.Time
		var val float64
		if err := srows.Scan(&sym, &ttype, &ts, &val); err != nil {
			srows.Close()
			return nil, fmt.Errorf("xirr stocks scan: %w", err)
		}
		if _, seen := syms[sym]; !seen {
			symOrder = append(symOrder, sym)
		}
		sign := -1.0
		if ttype == "SELL" {
			sign = 1.0
		}
		syms[sym] = append(syms[sym], cashflow{when: ts, amount: sign * val})
	}
	srows.Close()
	if err := srows.Err(); err != nil {
		return nil, fmt.Errorf("xirr stocks iterate: %w", err)
	}

	curVals := map[string]float64{}
	crows, err := s.pool.Query(ctx,
		`SELECT trading_symbol, quantity * COALESCE(last_price, 0) FROM demat_holdings WHERE user_id = $1`, userID)
	if err != nil {
		return nil, fmt.Errorf("xirr demat: %w", err)
	}
	for crows.Next() {
		var sym string
		var v float64
		if err := crows.Scan(&sym, &v); err != nil {
			crows.Close()
			return nil, fmt.Errorf("xirr demat scan: %w", err)
		}
		curVals[sym] += v
	}
	crows.Close()
	if err := crows.Err(); err != nil {
		return nil, fmt.Errorf("xirr demat iterate: %w", err)
	}

	for _, sym := range symOrder {
		flows := append([]cashflow{}, syms[sym]...)
		if cv := curVals[sym]; cv > 0 {
			flows = append(flows, cashflow{when: now, amount: cv})
		}
		all = append(all, flows...)
		if r, ok := xirr(flows); ok {
			out.Holdings = append(out.Holdings, rmdomain.HoldingXIRR{Name: sym, Type: "STOCK", XIRRPct: round2(r * 100), Computed: true})
		} else {
			out.Holdings = append(out.Holdings, rmdomain.HoldingXIRR{Name: sym, Type: "STOCK"})
		}
	}

	if r, ok := xirr(all); ok {
		out.OverallXIRRPct = round2(r * 100)
		out.Computed = true
	}
	return out, nil
}

// clientRiskSignals runs a few cheap queries for time-sensitive attention
// items: a large recent redemption, a lapsed SIP this month, a 30-day
// drawdown.
func (s *RMService) clientRiskSignals(ctx context.Context, userID uuid.UUID) ([]rmdomain.AdvisoryAction, error) {
	var out []rmdomain.AdvisoryAction

	var redeemed7d float64
	if err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(amt), 0) FROM (
			SELECT t.amount AS amt
			FROM mf_transactions t JOIN mf_folios f ON f.id = t.folio_id
			WHERE f.user_id = $1 AND t.transaction_type = 'REDEEM'
			  AND t.transaction_date >= CURRENT_DATE - 7
			UNION ALL
			SELECT o.quantity * COALESCE(o.average_price, o.price, 0)
			FROM stock_orders o
			WHERE o.user_id = $1 AND o.transaction_type = 'SELL' AND o.status = 'COMPLETE'
			  AND o.order_timestamp >= now() - INTERVAL '7 days'
		) x
	`, userID).Scan(&redeemed7d); err != nil {
		return nil, fmt.Errorf("risk signals redemptions: %w", err)
	}
	if redeemed7d >= 50000 {
		out = append(out, rmdomain.AdvisoryAction{
			Kind:     "retention_call",
			Priority: 1,
			Title:    "Large withdrawal this week",
			Detail:   fmt.Sprintf("Redeemed %s in the last 7 days — check in before more follows.", inr(redeemed7d)),
			Amount:   redeemed7d,
		})
	}

	var hasSIP, paidThisMonth bool
	if err := s.pool.QueryRow(ctx, `
		SELECT
			EXISTS (SELECT 1 FROM mf_folios WHERE user_id = $1 AND is_sip = true),
			EXISTS (
				SELECT 1 FROM mf_transactions t JOIN mf_folios f ON f.id = t.folio_id
				WHERE f.user_id = $1 AND t.transaction_type IN ('PURCHASE','SIP')
				  AND date_trunc('month', t.transaction_date) = date_trunc('month', CURRENT_DATE)
			)
	`, userID).Scan(&hasSIP, &paidThisMonth); err != nil {
		return nil, fmt.Errorf("risk signals sip: %w", err)
	}
	if hasSIP && !paidThisMonth {
		out = append(out, rmdomain.AdvisoryAction{
			Kind:     "sip_lapsed",
			Priority: 2,
			Title:    "SIP not seen this month",
			Detail:   "An active SIP mandate has no contribution recorded this calendar month.",
		})
	}

	var latest, prior float64
	err := s.pool.QueryRow(ctx, `
		SELECT
			(SELECT total_wealth FROM portfolio_snapshots WHERE user_id = $1 ORDER BY snapshot_date DESC LIMIT 1),
			(SELECT total_wealth FROM portfolio_snapshots WHERE user_id = $1 AND snapshot_date <= CURRENT_DATE - 30 ORDER BY snapshot_date DESC LIMIT 1)
	`, userID).Scan(&latest, &prior)
	if err == nil && prior > 0 {
		change := (latest - prior) / prior * 100
		if change <= -5 {
			out = append(out, rmdomain.AdvisoryAction{
				Kind:     "drawdown_call",
				Priority: 2,
				Title:    "Portfolio down over 30 days",
				Detail:   fmt.Sprintf("Total wealth is %.1f%% below where it was a month ago — a reassurance call may help.", change),
				Amount:   round2(prior - latest),
			})
		}
	}

	return out, nil
}

func (s *RMService) computeGoalProjections(goals []goalsdomain.Goal, assumedReturnPct, clientMonthlyInvested float64) []rmdomain.GoalProjection {
	now := time.Now().UTC()
	rMonthly := assumedReturnPct / 100 / 12

	type gp struct {
		g          goalsdomain.Goal
		monthsLeft int
		required   float64
	}
	var items []gp
	var sumRequired float64

	for _, g := range goals {
		if g.Status != goalsdomain.StatusActive {
			continue
		}
		monthsLeft := 0
		if g.Deadline != nil {
			ml := int(g.Deadline.Time().Sub(now).Hours() / 24 / 30.44)
			if ml > 0 {
				monthsLeft = ml
			}
		}
		required := 0.0
		if monthsLeft > 0 {
			n := float64(monthsLeft)
			if rMonthly > 0 {
				grow := math.Pow(1+rMonthly, n)
				required = (g.TargetAmount - g.CurrentAmount*grow) / ((grow - 1) / rMonthly)
			} else {
				required = (g.TargetAmount - g.CurrentAmount) / n
			}
			if required < 0 {
				required = 0
			}
		}
		items = append(items, gp{g: g, monthsLeft: monthsLeft, required: required})
		sumRequired += required
	}

	out := make([]rmdomain.GoalProjection, 0, len(items))
	for _, it := range items {
		g := it.g
		p := rmdomain.GoalProjection{
			GoalID:           g.ID,
			Name:             g.Name,
			TargetAmount:     round2(g.TargetAmount),
			CurrentAmount:    round2(g.CurrentAmount),
			TargetDate:       g.Deadline,
			MonthsLeft:       it.monthsLeft,
			AssumedReturnPct: round2(assumedReturnPct),
			RequiredMonthly:  round2(it.required),
		}

		var share float64
		switch {
		case sumRequired > 0:
			share = it.required / sumRequired * clientMonthlyInvested
		case len(items) > 0:
			share = clientMonthlyInvested / float64(len(items))
		}
		p.EstimatedMonthlyToGoal = round2(share)

		if it.monthsLeft > 0 {
			n := float64(it.monthsLeft)
			var projected float64
			if rMonthly > 0 {
				grow := math.Pow(1+rMonthly, n)
				projected = g.CurrentAmount*grow + share*((grow-1)/rMonthly)
			} else {
				projected = g.CurrentAmount + share*n
			}
			p.ProjectedAmount = round2(projected)
			if projected < g.TargetAmount {
				p.ProjectedShortfall = round2(g.TargetAmount - projected)
			}
			p.OnTrack = projected >= g.TargetAmount*0.98
			if add := it.required - share; add > 0 {
				p.AdditionalMonthlyRequired = round2(add)
			}
		} else {
			p.OnTrack = g.CurrentAmount >= g.TargetAmount
		}
		out = append(out, p)
	}
	return out
}

func maturingFDs(accts []fddomain.Account, withinDays int) []rmdomain.MaturingFD {
	now := time.Now().UTC()
	out := []rmdomain.MaturingFD{}
	for _, a := range accts {
		if a.Status != fddomain.StatusActive {
			continue
		}
		days := int(a.MaturityDate.Time().Sub(now).Hours() / 24)
		if days < 0 || days > withinDays {
			continue
		}
		out = append(out, rmdomain.MaturingFD{
			FDAccountNumber: a.FDAccountNumber,
			PrincipalAmount: round2(a.PrincipalAmount),
			MaturityAmount:  round2(a.MaturityAmount),
			MaturityDate:    a.MaturityDate,
			DaysToMaturity:  days,
			InterestRate:    a.InterestRate,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DaysToMaturity < out[j].DaysToMaturity })
	return out
}

func sortActions(a []rmdomain.AdvisoryAction) {
	sort.SliceStable(a, func(i, j int) bool {
		if a[i].Priority != a[j].Priority {
			return a[i].Priority < a[j].Priority
		}
		return a[i].Amount > a[j].Amount
	})
}

// ============================================================================
// Book-level insights
// ============================================================================

// BookInsights aggregates the whole book into an actionable panel for the RM
// dashboard: deployable idle cash, maturing FDs, retention alerts, and a
// single ranked "who to call" queue. Everything is computed from live data.
func (s *RMService) BookInsights(ctx context.Context, rmID uuid.UUID) (*rmdomain.BookInsights, error) {
	staff, err := s.rmRepo.GetByID(ctx, rmID)
	if err != nil {
		return nil, err
	}
	if staff == nil {
		return nil, fmt.Errorf("rm %s not found: %w", rmID, apiresponse.ErrUnauthorized)
	}

	out := &rmdomain.BookInsights{
		NextBestActions: []rmdomain.ClientActionItem{},
		MaturingFDs:     []rmdomain.ClientMaturingFD{},
		RetentionAlerts: []rmdomain.ClientActionItem{},
	}
	// best[userID] keeps that client's single highest-priority action.
	best := map[uuid.UUID]rmdomain.ClientActionItem{}
	consider := func(it rmdomain.ClientActionItem) {
		cur, ok := best[it.UserID]
		if !ok || it.Action.Priority < cur.Action.Priority ||
			(it.Action.Priority == cur.Action.Priority && it.Action.Amount > cur.Action.Amount) {
			best[it.UserID] = it
		}
	}

	// ---- Idle cash across the book ----
	{
		rows, err := s.pool.Query(ctx, `
			WITH spend AS (
				SELECT user_id, COALESCE(SUM(amount), 0) / 3.0 AS avg_monthly
				FROM spend_transactions
				WHERE type = 'DEBIT' AND occurred_at >= now() - INTERVAL '90 days'
				GROUP BY user_id
			), bank AS (
				SELECT user_id, COALESCE(SUM(balance), 0) AS bal
				FROM bank_accounts GROUP BY user_id
			)
			SELECT b.user_id, u.name, COALESCE(u.phone_number, ''), b.bal, COALESCE(s.avg_monthly, 0)
			FROM bank b
			JOIN users u ON u.id = b.user_id
			LEFT JOIN spend s ON s.user_id = b.user_id
			WHERE u.assigned_rm_id = $1
		`, rmID)
		if err != nil {
			return nil, fmt.Errorf("book idle cash: %w", err)
		}
		for rows.Next() {
			var uid uuid.UUID
			var name, phone string
			var bal, avgMonthly float64
			if err := rows.Scan(&uid, &name, &phone, &bal, &avgMonthly); err != nil {
				rows.Close()
				return nil, fmt.Errorf("book idle cash scan: %w", err)
			}
			idle := bal - avgMonthly*6
			threshold := avgMonthly
			if threshold < 25000 {
				threshold = 25000
			}
			if idle < threshold {
				continue
			}
			out.IdleCash.TotalIdle += idle
			out.IdleCash.ClientCount++
			pri := 3
			if idle >= 500000 {
				pri = 2
			}
			consider(rmdomain.ClientActionItem{
				UserID: uid, Name: name, Phone: phone,
				Action: rmdomain.AdvisoryAction{
					Kind: "deploy_cash", Priority: pri, Title: "Deploy idle cash",
					Detail: fmt.Sprintf("%s over a 6-month buffer, sitting in bank.", inr(idle)),
					Amount: round2(idle),
				},
			})
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("book idle cash iterate: %w", err)
		}
		out.IdleCash.TotalIdle = round2(out.IdleCash.TotalIdle)
	}

	// ---- FDs maturing in the next 90 days ----
	{
		rows, err := s.pool.Query(ctx, `
			SELECT f.user_id, u.name, COALESCE(u.phone_number, ''), COALESCE(ba.bank_name, ''),
			       f.fd_account_number, f.principal_amount, f.maturity_amount, f.maturity_date, f.interest_rate,
			       (f.maturity_date - CURRENT_DATE) AS days_to
			FROM fd_accounts f
			JOIN users u ON u.id = f.user_id
			LEFT JOIN bank_accounts ba ON ba.id = f.bank_account_id
			WHERE u.assigned_rm_id = $1 AND f.status = 'ACTIVE'
			  AND f.maturity_date >= CURRENT_DATE AND f.maturity_date <= CURRENT_DATE + 90
			ORDER BY f.maturity_date
		`, rmID)
		if err != nil {
			return nil, fmt.Errorf("book maturing fds: %w", err)
		}
		for rows.Next() {
			var uid uuid.UUID
			var name, phone, bank, fdNo string
			var principal, maturity, rate float64
			var matDate time.Time
			var daysTo int
			if err := rows.Scan(&uid, &name, &phone, &bank, &fdNo, &principal, &maturity, &matDate, &rate, &daysTo); err != nil {
				rows.Close()
				return nil, fmt.Errorf("book maturing fds scan: %w", err)
			}
			mf := rmdomain.MaturingFD{
				FDAccountNumber: fdNo, BankName: bank,
				PrincipalAmount: round2(principal), MaturityAmount: round2(maturity),
				MaturityDate: apitime.New(matDate), DaysToMaturity: daysTo, InterestRate: rate,
			}
			out.MaturingFDs = append(out.MaturingFDs, rmdomain.ClientMaturingFD{UserID: uid, Name: name, FD: mf})
			pri := 3
			if daysTo <= 30 {
				pri = 2
			}
			consider(rmdomain.ClientActionItem{
				UserID: uid, Name: name, Phone: phone,
				Action: rmdomain.AdvisoryAction{
					Kind: "fd_maturing", Priority: pri, Title: "FD maturing soon",
					Detail: fmt.Sprintf("%s matures in %d days — line up the reinvestment.", inr(maturity), daysTo),
					Amount: round2(maturity),
				},
			})
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("book maturing fds iterate: %w", err)
		}
	}

	// ---- Large redemptions in the last 7 days (retention risk) ----
	{
		rows, err := s.pool.Query(ctx, `
			SELECT x.user_id, u.name, COALESCE(u.phone_number, ''), SUM(x.amt) AS total
			FROM (
				SELECT f.user_id, t.amount AS amt
				FROM mf_transactions t JOIN mf_folios f ON f.id = t.folio_id
				WHERE t.transaction_type = 'REDEEM' AND t.transaction_date >= CURRENT_DATE - 7
				UNION ALL
				SELECT o.user_id, o.quantity * COALESCE(o.average_price, o.price, 0)
				FROM stock_orders o
				WHERE o.transaction_type = 'SELL' AND o.status = 'COMPLETE'
				  AND o.order_timestamp >= now() - INTERVAL '7 days'
			) x
			JOIN users u ON u.id = x.user_id
			WHERE u.assigned_rm_id = $1
			GROUP BY x.user_id, u.name, u.phone_number
			HAVING SUM(x.amt) >= 50000
			ORDER BY total DESC
		`, rmID)
		if err != nil {
			return nil, fmt.Errorf("book retention: %w", err)
		}
		for rows.Next() {
			var uid uuid.UUID
			var name, phone string
			var total float64
			if err := rows.Scan(&uid, &name, &phone, &total); err != nil {
				rows.Close()
				return nil, fmt.Errorf("book retention scan: %w", err)
			}
			item := rmdomain.ClientActionItem{
				UserID: uid, Name: name, Phone: phone,
				Action: rmdomain.AdvisoryAction{
					Kind: "retention_call", Priority: 1, Title: "Large withdrawal this week",
					Detail: fmt.Sprintf("Redeemed %s in the last 7 days.", inr(total)),
					Amount: round2(total),
				},
			}
			out.RetentionAlerts = append(out.RetentionAlerts, item)
			consider(item)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("book retention iterate: %w", err)
		}
	}

	// ---- Lapsed SIPs this month ----
	{
		rows, err := s.pool.Query(ctx, `
			SELECT u.id, u.name, COALESCE(u.phone_number, '')
			FROM users u
			WHERE u.assigned_rm_id = $1
			  AND EXISTS (SELECT 1 FROM mf_folios f WHERE f.user_id = u.id AND f.is_sip = true)
			  AND NOT EXISTS (
				SELECT 1 FROM mf_transactions t JOIN mf_folios f ON f.id = t.folio_id
				WHERE f.user_id = u.id AND t.transaction_type IN ('PURCHASE','SIP')
				  AND date_trunc('month', t.transaction_date) = date_trunc('month', CURRENT_DATE)
			  )
		`, rmID)
		if err != nil {
			return nil, fmt.Errorf("book lapsed sips: %w", err)
		}
		for rows.Next() {
			var uid uuid.UUID
			var name, phone string
			if err := rows.Scan(&uid, &name, &phone); err != nil {
				rows.Close()
				return nil, fmt.Errorf("book lapsed sips scan: %w", err)
			}
			consider(rmdomain.ClientActionItem{
				UserID: uid, Name: name, Phone: phone,
				Action: rmdomain.AdvisoryAction{
					Kind: "sip_lapsed", Priority: 2, Title: "SIP not seen this month",
					Detail: "Active SIP mandate with no contribution this calendar month.",
				},
			})
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("book lapsed sips iterate: %w", err)
		}
	}

	// ---- 30-day drawdowns ----
	{
		rows, err := s.pool.Query(ctx, `
			WITH latest AS (
				SELECT DISTINCT ON (user_id) user_id, total_wealth
				FROM portfolio_snapshots ORDER BY user_id, snapshot_date DESC
			), prior AS (
				SELECT DISTINCT ON (user_id) user_id, total_wealth
				FROM portfolio_snapshots WHERE snapshot_date <= CURRENT_DATE - 30
				ORDER BY user_id, snapshot_date DESC
			)
			SELECT l.user_id, u.name, COALESCE(u.phone_number, ''), l.total_wealth, p.total_wealth
			FROM latest l
			JOIN users u ON u.id = l.user_id
			JOIN prior p ON p.user_id = l.user_id
			WHERE u.assigned_rm_id = $1 AND p.total_wealth > 0
			  AND (l.total_wealth - p.total_wealth) / p.total_wealth <= -0.05
		`, rmID)
		if err != nil {
			return nil, fmt.Errorf("book drawdown: %w", err)
		}
		for rows.Next() {
			var uid uuid.UUID
			var name, phone string
			var latest, prior float64
			if err := rows.Scan(&uid, &name, &phone, &latest, &prior); err != nil {
				rows.Close()
				return nil, fmt.Errorf("book drawdown scan: %w", err)
			}
			change := (latest - prior) / prior * 100
			consider(rmdomain.ClientActionItem{
				UserID: uid, Name: name, Phone: phone,
				Action: rmdomain.AdvisoryAction{
					Kind: "drawdown_call", Priority: 2, Title: "Portfolio down over 30 days",
					Detail: fmt.Sprintf("Total wealth %.1f%% below a month ago.", change),
					Amount: round2(prior - latest),
				},
			})
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("book drawdown iterate: %w", err)
		}
	}

	for _, it := range best {
		out.NextBestActions = append(out.NextBestActions, it)
	}
	sort.SliceStable(out.NextBestActions, func(i, j int) bool {
		ai, aj := out.NextBestActions[i].Action, out.NextBestActions[j].Action
		if ai.Priority != aj.Priority {
			return ai.Priority < aj.Priority
		}
		return ai.Amount > aj.Amount
	})
	if len(out.NextBestActions) > 20 {
		out.NextBestActions = out.NextBestActions[:20]
	}
	return out, nil
}
