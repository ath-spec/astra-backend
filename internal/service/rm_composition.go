package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/yourusername/astra-backend/internal/apiresponse"
	rmdomain "github.com/yourusername/astra-backend/internal/domain/rm"
)

// BookComposition breaks the RM's whole book down by asset class, risk level
// and wealth band, sizes key-client concentration, tracks net flows over the
// last six months, and buckets clients into behavioural segments — all from
// live data.
func (s *RMService) BookComposition(ctx context.Context, rmID uuid.UUID) (*rmdomain.BookComposition, error) {
	staff, err := s.rmRepo.GetByID(ctx, rmID)
	if err != nil {
		return nil, err
	}
	if staff == nil {
		return nil, fmt.Errorf("rm %s not found: %w", rmID, apiresponse.ErrUnauthorized)
	}

	out := &rmdomain.BookComposition{
		ByAssetClass: []rmdomain.CompositionSlice{},
		ByRiskLevel:  []rmdomain.CompositionSlice{},
		ByWealthBand: []rmdomain.CompositionSlice{},
		TopClients:   []rmdomain.CompositionSlice{},
		Flows:        []rmdomain.FlowPoint{},
		Segments:     []rmdomain.SegmentCount{},
	}

	// ---- Per-client latest snapshot: wealth + asset-class values ----
	rows, err := s.pool.Query(ctx, `
		WITH latest AS (
			SELECT DISTINCT ON (user_id) user_id, total_wealth,
			       mutual_funds_value, stocks_value, fixed_deposits_value, bank_balance_value
			FROM portfolio_snapshots ORDER BY user_id, snapshot_date DESC
		)
		SELECT u.id, COALESCE(u.name, 'Client'),
		       COALESCE(l.total_wealth, 0), COALESCE(l.mutual_funds_value, 0), COALESCE(l.stocks_value, 0),
		       COALESCE(l.fixed_deposits_value, 0), COALESCE(l.bank_balance_value, 0)
		FROM users u
		LEFT JOIN latest l ON l.user_id = u.id
		WHERE u.assigned_rm_id = $1
	`, rmID)
	if err != nil {
		return nil, fmt.Errorf("book composition snapshots: %w", err)
	}

	type client struct {
		id                     uuid.UUID
		name                   string
		wealth, mf, st, fd, bk float64
	}
	var clients []client
	var mfSum, stSum, fdSum, bkSum float64
	for rows.Next() {
		var c client
		if err := rows.Scan(&c.id, &c.name, &c.wealth, &c.mf, &c.st, &c.fd, &c.bk); err != nil {
			rows.Close()
			return nil, err
		}
		clients = append(clients, c)
		mfSum += c.mf
		stSum += c.st
		fdSum += c.fd
		bkSum += c.bk
		out.TotalAUM += c.wealth
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out.TotalAUM = round2(out.TotalAUM)

	pct := func(v float64) float64 {
		if out.TotalAUM <= 0 {
			return 0
		}
		return round2(v / out.TotalAUM * 100)
	}
	out.ByAssetClass = []rmdomain.CompositionSlice{
		{Label: "Mutual Funds", Value: round2(mfSum), Pct: pct(mfSum)},
		{Label: "Stocks", Value: round2(stSum), Pct: pct(stSum)},
		{Label: "Fixed Deposits", Value: round2(fdSum), Pct: pct(fdSum)},
		{Label: "Bank", Value: round2(bkSum), Pct: pct(bkSum)},
	}

	// Wealth bands.
	bands := []struct {
		label    string
		lo, hi   float64
		val      float64
		count    int
	}{
		{label: "Under ₹5L", lo: 0, hi: 5e5},
		{label: "₹5L–25L", lo: 5e5, hi: 25e5},
		{label: "₹25L–1Cr", lo: 25e5, hi: 1e7},
		{label: "Over ₹1Cr", lo: 1e7, hi: 1e18},
	}
	for _, c := range clients {
		for i := range bands {
			if c.wealth >= bands[i].lo && c.wealth < bands[i].hi {
				bands[i].val += c.wealth
				bands[i].count++
				break
			}
		}
	}
	for _, b := range bands {
		out.ByWealthBand = append(out.ByWealthBand, rmdomain.CompositionSlice{
			Label: b.label, Value: round2(b.val), Pct: pct(b.val), Count: b.count,
		})
	}

	// Top-5 client concentration.
	top := append([]client{}, clients...)
	for i := 0; i < len(top); i++ {
		for j := i + 1; j < len(top); j++ {
			if top[j].wealth > top[i].wealth {
				top[i], top[j] = top[j], top[i]
			}
		}
	}
	var top5 float64
	for i := 0; i < len(top) && i < 5; i++ {
		top5 += top[i].wealth
		out.TopClients = append(out.TopClients, rmdomain.CompositionSlice{
			Label: top[i].name, Value: round2(top[i].wealth), Pct: pct(top[i].wealth),
		})
	}
	out.Top5SharePct = pct(top5)

	// ---- AUM by DNA risk level (from the most recent DNA snapshot per client) ----
	lvlRows, err := s.pool.Query(ctx, `
		WITH latest AS (
			SELECT DISTINCT ON (user_id) user_id, level, total_value
			FROM portfolio_dna_snapshots ORDER BY user_id, snapshot_date DESC
		)
		SELECT COALESCE(l.level, 'UNKNOWN'), COUNT(*), COALESCE(SUM(l.total_value), 0)
		FROM users u
		LEFT JOIN latest l ON l.user_id = u.id
		WHERE u.assigned_rm_id = $1
		GROUP BY 1
	`, rmID)
	if err == nil {
		for lvlRows.Next() {
			var lvl string
			var cnt int
			var val float64
			if err := lvlRows.Scan(&lvl, &cnt, &val); err != nil {
				continue
			}
			out.ByRiskLevel = append(out.ByRiskLevel, rmdomain.CompositionSlice{
				Label: titleCaseWords(lvl), Value: round2(val), Pct: pct(val), Count: cnt,
			})
		}
		lvlRows.Close()
	}

	// ---- Net flows, last 6 calendar months ----
	flowRows, err := s.pool.Query(ctx, `
		WITH mv AS (
			SELECT date_trunc('month', t.transaction_date) AS m,
			       SUM(CASE WHEN t.transaction_type IN ('PURCHASE','SIP') THEN t.amount ELSE 0 END) AS inflow,
			       SUM(CASE WHEN t.transaction_type = 'REDEEM' THEN t.amount ELSE 0 END) AS outflow
			FROM mf_transactions t
			JOIN mf_folios f ON f.id = t.folio_id
			JOIN users u ON u.id = f.user_id
			WHERE u.assigned_rm_id = $1 AND t.transaction_date >= date_trunc('month', CURRENT_DATE) - INTERVAL '5 months'
			GROUP BY 1
		), sv AS (
			SELECT date_trunc('month', o.order_timestamp) AS m,
			       SUM(CASE WHEN o.transaction_type = 'BUY'  THEN o.quantity * COALESCE(o.average_price, o.price, 0) ELSE 0 END) AS inflow,
			       SUM(CASE WHEN o.transaction_type = 'SELL' THEN o.quantity * COALESCE(o.average_price, o.price, 0) ELSE 0 END) AS outflow
			FROM stock_orders o
			JOIN users u ON u.id = o.user_id
			WHERE u.assigned_rm_id = $1 AND o.status = 'COMPLETE'
			  AND o.order_timestamp >= date_trunc('month', CURRENT_DATE) - INTERVAL '5 months'
			GROUP BY 1
		)
		SELECT m, SUM(inflow), SUM(outflow) FROM (
			SELECT m, inflow, outflow FROM mv
			UNION ALL SELECT m, inflow, outflow FROM sv
		) x GROUP BY m ORDER BY m ASC
	`, rmID)
	if err == nil {
		for flowRows.Next() {
			var m time.Time
			var in, outf float64
			if err := flowRows.Scan(&m, &in, &outf); err != nil {
				continue
			}
			out.Flows = append(out.Flows, rmdomain.FlowPoint{
				Label: m.Format("Jan '06"), Inflow: round2(in), Outflow: round2(outf), Net: round2(in - outf),
			})
		}
		flowRows.Close()
	}

	// ---- Behavioural segments ----
	segRows, err := s.pool.Query(ctx, `
		WITH latest AS (
			SELECT DISTINCT ON (user_id) user_id, total_wealth, bank_balance_value
			FROM portfolio_snapshots ORDER BY user_id, snapshot_date DESC
		), buys AS (
			SELECT f.user_id, COUNT(*) AS n, MAX(t.transaction_date) AS last_buy
			FROM mf_transactions t JOIN mf_folios f ON f.id = t.folio_id
			WHERE t.transaction_type IN ('PURCHASE','SIP') AND t.transaction_date >= CURRENT_DATE - 180
			GROUP BY 1
		), sells AS (
			SELECT f.user_id, COALESCE(SUM(t.amount),0) AS v
			FROM mf_transactions t JOIN mf_folios f ON f.id = t.folio_id
			WHERE t.transaction_type = 'REDEEM' AND t.transaction_date >= CURRENT_DATE - 180
			GROUP BY 1
		)
		SELECT u.id, u.created_at,
		       COALESCE(l.total_wealth, 0), COALESCE(l.bank_balance_value, 0),
		       COALESCE(b.n, 0), b.last_buy, COALESCE(s.v, 0),
		       EXISTS (SELECT 1 FROM mf_folios f WHERE f.user_id = u.id AND f.is_sip = true)
		FROM users u
		LEFT JOIN latest l ON l.user_id = u.id
		LEFT JOIN buys   b ON b.user_id = u.id
		LEFT JOIN sells  s ON s.user_id = u.id
		WHERE u.assigned_rm_id = $1
	`, rmID)
	if err == nil {
		seg := map[string]*rmdomain.SegmentCount{}
		add := func(name string, aum float64) {
			sc := seg[name]
			if sc == nil {
				sc = &rmdomain.SegmentCount{Segment: name}
				seg[name] = sc
			}
			sc.Count++
			sc.AUM = round2(sc.AUM + aum)
		}
		now := time.Now()
		for segRows.Next() {
			var id uuid.UUID
			var createdAt time.Time
			var wealth, bank, buyN, sellV float64
			var lastBuy *time.Time
			var hasSIP bool
			if err := segRows.Scan(&id, &createdAt, &wealth, &bank, &buyN, &lastBuy, &sellV, &hasSIP); err != nil {
				continue
			}
			switch {
			case now.Sub(createdAt).Hours()/24 <= 30:
				add("New (<30d)", wealth)
			case lastBuy == nil || now.Sub(*lastBuy).Hours()/24 > 90:
				add("Dormant (no buy 90d)", wealth)
			case sellV >= 50000:
				add("Withdrawer", wealth)
			case wealth > 0 && bank/wealth >= 0.4:
				add("Cash hoarder", wealth)
			case hasSIP && buyN >= 4:
				add("Disciplined accumulator", wealth)
			default:
				add("Lump-sum / sporadic", wealth)
			}
		}
		segRows.Close()
		for _, v := range seg {
			out.Segments = append(out.Segments, *v)
		}
	}

	return out, nil
}
