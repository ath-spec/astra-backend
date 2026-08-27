package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	rmdomain "github.com/yourusername/astra-backend/internal/domain/rm"
)

// narrativeTopics is the fixed set of report sections the copilot may write.
var narrativeTopics = []string{
	"allocation", "genome", "discipline", "performance",
	"risk", "cost", "concentration", "cohort", "drift", "tax",
}

// narrativeMaxAge forces a refresh of an otherwise-unchanged narrative after
// this long, so nothing goes stale indefinitely.
const narrativeMaxAge = 30 * 24 * time.Hour

// Token-lean system prompt. Every constraint here earns its place.
const narrativeSystemPrompt = `You write the analysis paragraphs of an internal portfolio review, for the client's own relationship manager. Input: that client's computed figures, one line per topic tag.

For each topic present in the input, write one flowing paragraph, 2-3 sentences, at most ~55 words: what the pattern is, what it means for this client, the implication for the next review. Do not simply recite the numbers.

Rules: use only the given figures, invent nothing; no fund, stock, ETF or product names; plain prose, no markdown, no lists; skip any topic not in the input.

Reply with a compact JSON object {topic: paragraph}. Nothing else.`

// ClientNarrative returns AI-written, figure-grounded report paragraphs per
// analytic topic. A cheap fingerprint query gates everything: while the
// fingerprint and cache age are unchanged, the cached JSON is returned with no
// analytics recompute and no model call.
func (s *RMService) ClientNarrative(ctx context.Context, callerRMID uuid.UUID, isAdmin bool, userID uuid.UUID, force bool) (map[string]string, error) {
	if err := s.authorizeClient(ctx, callerRMID, isAdmin, userID); err != nil {
		return nil, err
	}
	if s.groqKey == "" {
		return map[string]string{}, nil
	}

	fp, err := s.narrativeFingerprint(ctx, userID)
	if err != nil {
		return nil, err
	}

	var (
		cachedFP  string
		cachedRaw []byte
		updatedAt time.Time
	)
	_ = s.pool.QueryRow(ctx,
		`SELECT fingerprint, topics, updated_at FROM rm_client_narratives WHERE user_id = $1`, userID,
	).Scan(&cachedFP, &cachedRaw, &updatedAt)

	fresh := cachedFP == fp && time.Since(updatedAt) < narrativeMaxAge
	if !force && fresh && len(cachedRaw) > 0 {
		var m map[string]string
		if json.Unmarshal(cachedRaw, &m) == nil {
			return m, nil
		}
	}

	// Fingerprint moved (or forced / aged) — do the full recompute + one call.
	var (
		pa  *rmdomain.ClientPortfolioAnalysis
		an  *rmdomain.ClientAnalytics
		adv *rmdomain.ClientAdvisory
	)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error { v, e := s.PortfolioAnalysis(gctx, callerRMID, isAdmin, userID); pa = v; return e })
	g.Go(func() error { v, e := s.ClientAnalytics(gctx, callerRMID, isAdmin, userID); an = v; return e })
	g.Go(func() error { v, e := s.ClientAdvisory(gctx, callerRMID, isAdmin, userID); adv = v; return e })
	if err := g.Wait(); err != nil {
		return nil, err
	}

	blob := buildFiguresBlob(pa, an, adv)
	if strings.TrimSpace(blob) == "" {
		return map[string]string{}, nil
	}

	m, err := s.generateNarrative(ctx, blob)
	if err != nil {
		if len(cachedRaw) > 0 { // serve stale rather than nothing
			var old map[string]string
			if json.Unmarshal(cachedRaw, &old) == nil {
				return old, nil
			}
		}
		return nil, err
	}

	raw, _ := json.Marshal(m)
	_, _ = s.pool.Exec(ctx, `
		INSERT INTO rm_client_narratives (user_id, fingerprint, topics, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (user_id) DO UPDATE
		SET fingerprint = EXCLUDED.fingerprint, topics = EXCLUDED.topics, updated_at = now()
	`, userID, fp, raw)
	return m, nil
}

// narrativeFingerprint is a single round-trip digest of the client's material
// state. It deliberately ignores tiny NAV noise (values rounded to ₹1k) so the
// cache does not thrash, but moves on any new snapshot day, transaction, goal,
// risk-level change, or meaningful value shift.
func (s *RMService) narrativeFingerprint(ctx context.Context, userID uuid.UUID) (string, error) {
	var (
		snapDate    *time.Time
		dnaLevel    string
		mfTxns      int
		stockOrders int
		mfValue     float64
		bankBal     float64
		activeGoals int
	)
	err := s.pool.QueryRow(ctx, `
		SELECT
			(SELECT max(snapshot_date) FROM portfolio_snapshots WHERE user_id = $1),
			COALESCE((SELECT level FROM portfolio_dna_snapshots WHERE user_id = $1 ORDER BY snapshot_date DESC LIMIT 1), ''),
			(SELECT count(*) FROM mf_transactions t JOIN mf_folios f ON f.id = t.folio_id WHERE f.user_id = $1),
			(SELECT count(*) FROM stock_orders WHERE user_id = $1),
			COALESCE((SELECT sum(units_held * COALESCE(nav, 0)) FROM mf_folios WHERE user_id = $1), 0),
			COALESCE((SELECT sum(balance) FROM bank_accounts WHERE user_id = $1), 0),
			(SELECT count(*) FROM goals WHERE user_id = $1 AND status = 'ACTIVE')
	`, userID).Scan(&snapDate, &dnaLevel, &mfTxns, &stockOrders, &mfValue, &bankBal, &activeGoals)
	if err != nil {
		return "", fmt.Errorf("narrative fingerprint: %w", err)
	}
	day := ""
	if snapDate != nil {
		day = snapDate.Format("20060102")
	}
	return fmt.Sprintf("%s|%s|%d|%d|%.0f|%.0f|%d",
		day, dnaLevel, mfTxns, stockOrders,
		math.Round(mfValue/1000)*1000, math.Round(bankBal/1000)*1000, activeGoals), nil
}

func (s *RMService) generateNarrative(ctx context.Context, figures string) (map[string]string, error) {
	payload := map[string]interface{}{
		"model":       "openai/gpt-oss-120b",
		"temperature": 0.3,
		"messages": []map[string]interface{}{
			{"role": "system", "content": narrativeSystemPrompt},
			{"role": "user", "content": figures},
		},
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, "POST", groqAPIURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.groqKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{Timeout: 45 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("contacting narrative model: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("narrative model returned %d", resp.StatusCode)
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil || len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("could not parse narrative response")
	}

	content := strings.TrimSpace(parsed.Choices[0].Message.Content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	if i := strings.IndexByte(content, '{'); i > 0 {
		content = content[i:]
	}
	if i := strings.LastIndexByte(content, '}'); i >= 0 && i < len(content)-1 {
		content = content[:i+1]
	}

	var m map[string]string
	if err := json.Unmarshal([]byte(content), &m); err != nil {
		return nil, fmt.Errorf("narrative JSON invalid: %w", err)
	}
	out := map[string]string{}
	for _, t := range narrativeTopics {
		if v := strings.TrimSpace(m[t]); v != "" {
			out[t] = v
		}
	}
	return out, nil
}

// buildFiguresBlob renders the client's computed analytics as a compact,
// deterministic, one-line-per-topic block — the sole input the model may use.
// Topics with no meaningful data are omitted entirely.
func buildFiguresBlob(pa *rmdomain.ClientPortfolioAnalysis, an *rmdomain.ClientAnalytics, adv *rmdomain.ClientAdvisory) string {
	var b strings.Builder
	line := func(f string, args ...interface{}) { fmt.Fprintf(&b, f+"\n", args...) }

	if pa != nil && pa.Allocation != nil {
		a := pa.Allocation
		secs := ""
		for i, se := range a.SectorExposure {
			if i >= 4 {
				break
			}
			if i > 0 {
				secs += ", "
			}
			secs += fmt.Sprintf("%s %.0f", se.Sector, se.Percentage)
		}
		idx := ""
		if a.EquityExposure != nil {
			idx = fmt.Sprintf(" idx=%.0f peeridx=%.0f", a.EquityExposure.IndexFundPct, a.EquityExposure.PeerIndexFundPct)
		}
		line("alloc: level=%s eq=%.0f debt=%.0f other=%.0f val=%.0f%s sectors=[%s]",
			a.Level, a.EquityPct, a.DebtPct, a.OtherPct, a.TotalValue, idx, secs)

		gm := a.Genome
		line("genome: growth=%.0f income=%.0f preservation=%.0f inflation=%.0f liquidity=%.0f esg=%.0f real=%.0f",
			norm100(gm.Growth), norm100(gm.Income), norm100(gm.CapitalPreservation), norm100(gm.InflationDefense),
			norm100(gm.Liquidity), norm100(gm.Sustainability), norm100(gm.RealAssets))
	}
	if pa != nil && pa.Discipline != nil {
		d := pa.Discipline
		line("disc: level=%s streak=%d missed=%d consistency=%.0f automation=%.0f avgmo=%.0f mandates=%d",
			d.Level, d.CurrentStreakMonths, d.MissedMonths, d.SIPConsistencyPct, d.SIPAutomationPct, d.AvgMonthlyInvested, d.ActiveMandatesCount)
	}
	if pa != nil && pa.Performance != nil {
		p := pa.Performance
		bm := ""
		for _, x := range p.Benchmarks {
			bm += fmt.Sprintf("; vs %s p=%.1f b=%.1f a=%.1f", strings.ReplaceAll(x.Name, " ", "_"), x.PortfolioReturnPct, x.BenchmarkReturnPct, x.AlphaPct)
		}
		line("perf: level=%s ret=%.1f annual=%.1f gain=%.0f expensive=%d%s",
			p.Level, p.TotalReturnPct, p.AnnualizedReturnPct, p.TotalGainAmount, len(p.ExpensiveFunds), bm)
	}

	if an != nil {
		if r := an.Risk; r != nil && r.Points >= 3 {
			line("risk: maxdd=%.1f vol=%.1f worstmo=%.1f bestmo=%.1f recovery=%s n=%d",
				r.MaxDrawdownPct, r.VolatilityPct, r.WorstMonthPct, r.BestMonthPct, dayStr(r.RecoveryDays), r.Points)
		}
		if c := an.Cost; c != nil && c.MFValue > 0 {
			line("cost: er=%.2f fee=%.0f feepct=%.2f avoidable=%.0f highcost=%d net=%.1f gross=%.1f",
				c.WeightedExpenseRatio, c.AnnualFee, c.FeeAsPctOfWealth, c.AvoidableAnnualFee, c.HighCostCount, c.NetReturnPct, c.GrossReturnPct)
		}
		if cc := an.Concentration; cc != nil && cc.HoldingsCount > 0 {
			extra := ""
			if cc.TopStock != nil {
				extra += fmt.Sprintf(" topstock=\"%s %.0f\"", cc.TopStock.Label, cc.TopStock.Pct)
			}
			if cc.TopAMC != nil {
				extra += fmt.Sprintf(" topamc=\"%s %.0f\"", cc.TopAMC.Label, cc.TopAMC.Pct)
			}
			line("conc: hhi=%.0f %s holdings=%d overlap=%d%s",
				cc.SectorHHI, cc.SectorHHILabel, cc.HoldingsCount, len(cc.Overlaps), extra)
		}
		for _, ch := range an.Cohort {
			line("cohort: %q v=%.1f pctile=%d median=%.1f n=%d hib=%t",
				ch.Metric, ch.Value, ch.Percentile, ch.PeerMedian, ch.SampleSize, ch.HigherIsBetter)
		}
		if wc := an.WhatChanged; wc != nil && (wc.LevelBefore != "" || wc.EquityPctDelta != 0 || wc.WealthPctDelta != 0) {
			notes := ""
			for _, cg := range wc.Changes {
				notes += "; " + cg.Text
			}
			line("drift: since=%dd before=%s after=%s deq=%.1f dwealth=%.1f%s",
				wc.SinceDays, wc.LevelBefore, wc.LevelAfter, wc.EquityPctDelta, wc.WealthPctDelta, notes)
		}
		if tx := an.Tax; tx != nil && len(tx.Lots) > 0 {
			line("tax: ltcg=%.0f stcg=%.0f esttax=%.0f nearing=%.0f elsslocked=%.0f lots=%d",
				tx.LTCGGain, tx.STCGGain, tx.EstLTCGTax+tx.EstSTCGTax, tx.NearingLTCGValue, tx.ELSSLockedValue, len(tx.Lots))
		}
	}

	if adv != nil {
		ctx := ""
		if adv.IdleCash != nil && adv.IdleCash.IdleAmount > 0 {
			ctx += fmt.Sprintf(" idlecash=%.0f monthsheld=%.1f", adv.IdleCash.IdleAmount, adv.IdleCash.MonthsOfSpendHeld)
		}
		if adv.XIRR != nil && adv.XIRR.Computed {
			ctx += fmt.Sprintf(" xirr=%.1f", adv.XIRR.OverallXIRRPct)
		}
		for _, gp := range adv.GoalProjections {
			if gp.ProjectedShortfall > 0 {
				ctx += fmt.Sprintf(" goalgap=\"%s short %.0f +%.0f/mo\"", gp.Name, gp.ProjectedShortfall, gp.AdditionalMonthlyRequired)
			}
		}
		if ctx != "" {
			line("ctx:%s", ctx)
		}
	}

	return b.String()
}

func dayStr(d int) string {
	if d < 0 {
		return "ongoing"
	}
	return fmt.Sprintf("%dd", d)
}

func norm100(v float64) float64 {
	if v <= 1.5 {
		return v * 100
	}
	return v
}
