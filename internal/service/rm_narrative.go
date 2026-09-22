package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/yourusername/astra-backend/internal/ai/agents"
	rmdomain "github.com/yourusername/astra-backend/internal/domain/rm"
	"github.com/yourusername/astra-backend/internal/provider/llm"
)

// narrativeTopics is the fixed set of report sections the copilot may write.
var narrativeTopics = []string{
	"allocation", "genome", "discipline", "performance",
	"risk", "cost", "concentration", "cohort", "drift", "tax",
	"spend_overview", "spend_categories", "spend_habits", "spend_income", "budget_health",
}

// Narrative groups split the one-shot 15-topic prompt into the two sets the
// RM console actually asks for (portfolio tabs vs the spend-intelligence
// tab). Asking the model for all 15 paragraphs in a single JSON completion
// meant one truncated/malformed response (more likely the bigger the ask)
// blanked every Pro Tip on every tab at once, and there was no way to retry
// just the half that failed. Splitting the ask in two shrinks each prompt,
// isolates failures to the tab that actually needs those topics, and lets a
// client with no budget/spend data skip the spend call entirely.
const (
	NarrativeGroupPortfolio = "portfolio"
	NarrativeGroupSpend     = "spend"
)

var (
	portfolioNarrativeTopics = []string{"allocation", "genome", "discipline", "performance", "risk", "cost", "concentration", "cohort", "drift", "tax"}
	spendNarrativeTopics     = []string{"spend_overview", "spend_categories", "spend_habits", "spend_income", "budget_health"}
)

// topicsForGroup resolves which topics a request is asking for. An empty or
// unrecognized group keeps the old "everything in one call" behaviour so any
// caller that doesn't pass one yet still works.
func topicsForGroup(group string) []string {
	switch group {
	case NarrativeGroupPortfolio:
		return portfolioNarrativeTopics
	case NarrativeGroupSpend:
		return spendNarrativeTopics
	default:
		return narrativeTopics
	}
}

func filterTopics(m map[string]string, topics []string) map[string]string {
	out := make(map[string]string, len(topics))
	for _, t := range topics {
		if v, ok := m[t]; ok {
			out[t] = v
		}
	}
	return out
}

func hasAnyTopic(m map[string]string, topics []string) bool {
	for _, t := range topics {
		if _, ok := m[t]; ok {
			return true
		}
	}
	return false
}

// narrativeMaxAge forces a refresh of an otherwise-unchanged narrative after
// this long, so nothing goes stale indefinitely.
const narrativeMaxAge = 30 * 24 * time.Hour

// Token-lean system prompt. Every constraint here earns its place.
const narrativeSystemPrompt = `You write the analysis paragraphs of an internal portfolio review, for the client's own relationship manager. Input: that client's computed figures, one line per topic tag.

For each topic present in the input, write one flowing paragraph, 2-3 sentences, at most ~55 words: what the pattern is, what it means for this client, the implication for the next review. Do not simply recite the numbers.

Rules: use only the given figures, invent nothing; no fund, stock, ETF or product names; plain prose, no markdown, no lists; skip any topic not in the input.

Reply ONLY in valid JSON format with a compact JSON object mapping each topic to its paragraph string. Example: {"risk": "your paragraph", "cost": "your paragraph"}`

// ClientNarrative returns AI-written, figure-grounded report paragraphs per
// analytic topic. A cheap fingerprint query gates everything: while the
// fingerprint and cache age are unchanged, the cached JSON is returned with no
// analytics recompute and no model call.
func (s *RMService) ClientNarrative(ctx context.Context, callerRMID uuid.UUID, isAdmin bool, userID uuid.UUID, group string, force bool) (map[string]string, error) {
	if err := s.authorizeClient(ctx, callerRMID, isAdmin, userID); err != nil {
		return nil, err
	}

	topics := topicsForGroup(group)
	needPortfolio := group == "" || group == NarrativeGroupPortfolio
	needSpend := group == "" || group == NarrativeGroupSpend

	// The fingerprint query is just as capable of hitting a transient DB
	// hiccup (pool exhaustion under a Client360-page burst of concurrent
	// requests, a slow connection acquire) as any of the analytics calls
	// below — but unlike those, this one used to abort the whole request
	// with a hard error before ever reaching the deterministic fallback.
	// That's exactly what made a transient failure here look like "no
	// fallback ran at all": it never got the chance to. Degrade the same
	// way the rest of this function does — skip the cache (we have no valid
	// fingerprint to compare against or persist) and fall through to a live
	// recompute + deterministic narrative instead of erroring out.
	fp, fpErr := s.narrativeFingerprint(ctx, userID)
	if fpErr != nil {
		slog.Warn("rm narrative: fingerprint query failed, skipping cache", "user", userID, "error", fpErr)
		force = true
	}

	var (
		cachedFP  string
		cachedRaw []byte
		updatedAt time.Time
	)
	if fpErr == nil {
		_ = s.pool.QueryRow(ctx,
			`SELECT fingerprint, topics, updated_at FROM rm_client_narratives WHERE user_id = $1`, userID,
		).Scan(&cachedFP, &cachedRaw, &updatedAt)
	}

	var cachedAll map[string]string
	if len(cachedRaw) > 0 {
		_ = json.Unmarshal(cachedRaw, &cachedAll)
	}

	fresh := cachedFP == fp && time.Since(updatedAt) < narrativeMaxAge
	if !force && fresh && hasAnyTopic(cachedAll, topics) {
		return filterTopics(cachedAll, topics), nil
	}

	// Fingerprint moved (or forced / aged / never generated for this group)
	// — do the recompute + one call, scoped to just what this group needs.
	var (
		pa  *rmdomain.ClientPortfolioAnalysis
		an  *rmdomain.ClientAnalytics
		adv *rmdomain.ClientAdvisory
		si  *rmdomain.ClientSpendIntelligence
	)
	// Every source below degrades independently: a failure in any one of
	// them (sparse data, a missing snapshot, an analyzer edge case) must
	// never blank out the whole report. Each keeps its slice nil on error and
	// the deterministic builder below (and the model prompt) simply omits
	// whatever wasn't available — it does not abort the request. This used
	// to only apply to SpendIntelligence; PortfolioAnalysis/ClientAnalytics/
	// ClientAdvisory erroring here used to fail g.Wait() and take down every
	// Pro Tip on every tab for the client, with no deterministic fallback at
	// all.
	g, gctx := errgroup.WithContext(ctx)
	if needPortfolio {
		g.Go(func() error {
			v, e := s.PortfolioAnalysis(gctx, callerRMID, isAdmin, userID)
			if e != nil {
				slog.Warn("rm narrative: portfolio analysis failed", "user", userID, "error", e)
				return nil
			}
			pa = v
			return nil
		})
		g.Go(func() error {
			v, e := s.ClientAnalytics(gctx, callerRMID, isAdmin, userID)
			if e != nil {
				slog.Warn("rm narrative: client analytics failed", "user", userID, "error", e)
				return nil
			}
			an = v
			return nil
		})
		g.Go(func() error {
			v, e := s.ClientAdvisory(gctx, callerRMID, isAdmin, userID)
			if e != nil {
				slog.Warn("rm narrative: client advisory failed", "user", userID, "error", e)
				return nil
			}
			adv = v
			return nil
		})
	}
	if needSpend {
		g.Go(func() error {
			v, e := s.SpendIntelligence(gctx, callerRMID, isAdmin, userID)
			if e != nil {
				slog.Warn("rm narrative: spend intelligence failed", "user", userID, "error", e)
				return nil
			}
			si = v
			return nil
		})
	}
	_ = g.Wait()

	// authorizeClient runs first inside each of the calls above, so if
	// the caller genuinely isn't allowed to see this client, surface that
	// instead of silently returning an empty narrative.
	if err := s.authorizeClient(ctx, callerRMID, isAdmin, userID); err != nil {
		return nil, err
	}

	blob := buildFiguresBlob(pa, an, adv, si)

	// 1. Try AI-generated narrative through the LLM seam (Groq today), asking
	// only for this group's topics — a smaller prompt/response is both
	// cheaper and far less likely to be truncated or malformed than the old
	// single 15-topic call. Skip the call entirely when there's nothing to
	// summarize (blob empty), but still fall through to the deterministic
	// builder below.
	var m map[string]string
	if s.llm != nil && strings.TrimSpace(blob) != "" {
		m, _ = s.generateNarrative(ctx, blob, topics)
	}

	// If the model successfully generated this group's narrative, merge it
	// into whatever's already cached for the OTHER group and persist — a
	// plain overwrite here would wipe out the other group's cached topics
	// every time either one regenerates.
	if len(m) > 0 {
		merged := cachedAll
		if merged == nil {
			merged = map[string]string{}
		}
		for k, v := range m {
			merged[k] = v
		}
		// Only persist against a real fingerprint — writing one keyed on the
		// empty string from a failed fingerprint query would make a later,
		// healthy request with a real (non-empty) fingerprint spuriously
		// look stale-but-different forever, and vice versa.
		if fpErr == nil {
			raw, _ := json.Marshal(merged)
			_, _ = s.pool.Exec(ctx, `
				INSERT INTO rm_client_narratives (user_id, fingerprint, topics, updated_at)
				VALUES ($1, $2, $3, now())
				ON CONFLICT (user_id) DO UPDATE
				SET fingerprint = EXCLUDED.fingerprint, topics = EXCLUDED.topics, updated_at = now()
			`, userID, fp, raw)
		}
		return filterTopics(merged, topics), nil
	}

	// 2. If Groq failed or is unconfigured:
	// If an earlier Groq narrative exists in cache for this group, serve it
	// rather than a blank/deterministic-only result.
	if hasAnyTopic(cachedAll, topics) {
		return filterTopics(cachedAll, topics), nil
	}

	// 3. Compute dynamic deterministic fallback on the fly (NOT cached, always
	// live) so the RM always sees *something* rather than an empty Pro Tip.
	return filterTopics(buildDeterministicNarratives(pa, an, adv, si), topics), nil
}

// narrativeFingerprint is a single round-trip digest of the client's material
// state. It deliberately ignores tiny NAV noise (values rounded to ₹1k) so the
// cache does not thrash, but moves on any new snapshot day, transaction, goal,
// risk-level change, or meaningful value shift.
func (s *RMService) narrativeFingerprint(ctx context.Context, userID uuid.UUID) (string, error) {
	var (
		snapDate     *time.Time
		dnaLevel     string
		mfTxns       int
		stockOrders  int
		mfValue      float64
		bankBal      float64
		activeGoals  int
		spendTxns    int
		lastSpendAt  *time.Time
		activeBudget int
	)
	err := s.pool.QueryRow(ctx, `
		SELECT
			(SELECT max(snapshot_date) FROM portfolio_snapshots WHERE user_id = $1),
			COALESCE((SELECT level FROM portfolio_dna_snapshots WHERE user_id = $1 ORDER BY snapshot_date DESC LIMIT 1), ''),
			(SELECT count(*) FROM mf_transactions t JOIN mf_folios f ON f.id = t.folio_id WHERE f.user_id = $1),
			(SELECT count(*) FROM stock_orders WHERE user_id = $1),
			COALESCE((SELECT sum(units_held * COALESCE(nav, 0)) FROM mf_folios WHERE user_id = $1), 0),
			COALESCE((SELECT sum(balance) FROM bank_accounts WHERE user_id = $1 AND unlinked_at IS NULL), 0),
			(SELECT count(*) FROM goals WHERE user_id = $1 AND status = 'ACTIVE'),
			COALESCE((SELECT count(*) FROM spend_transactions WHERE user_id = $1), 0),
			(SELECT max(occurred_at) FROM spend_transactions WHERE user_id = $1),
			(SELECT count(*) FROM budgets WHERE user_id = $1 AND is_active)
	`, userID).Scan(&snapDate, &dnaLevel, &mfTxns, &stockOrders, &mfValue, &bankBal, &activeGoals,
		&spendTxns, &lastSpendAt, &activeBudget)
	if err != nil {
		return "", fmt.Errorf("narrative fingerprint: %w", err)
	}
	day := ""
	if snapDate != nil {
		day = snapDate.Format("20060102")
	}
	lastSpendDay := ""
	if lastSpendAt != nil {
		lastSpendDay = lastSpendAt.Format("20060102")
	}
	return fmt.Sprintf("%s|%s|%d|%d|%.0f|%.0f|%d|%d|%s|%d",
		day, dnaLevel, mfTxns, stockOrders,
		math.Round(mfValue/1000)*1000, math.Round(bankBal/1000)*1000, activeGoals,
		spendTxns, lastSpendDay, activeBudget), nil
}

func (s *RMService) generateNarrative(ctx context.Context, figures string, topics []string) (map[string]string, error) {
	if s.llm == nil {
		return nil, fmt.Errorf("llm provider not set")
	}

	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	system := narrativeSystemPrompt + "\n\nOnly write these topics: " + strings.Join(topics, ", ") + "."
	req := s.agents.Get(agents.KeyRMNarrator).Request(
		system,
		[]llm.Message{{Role: llm.RoleUser, Content: figures}},
	)
	resp, err := s.llm.Complete(callCtx, req)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("narrative model returned nil response")
	}

	content := strings.TrimSpace(resp.Text)
	if start := strings.IndexByte(content, '{'); start >= 0 {
		if end := strings.LastIndexByte(content, '}'); end > start {
			content = content[start : end+1]
		}
	}

	var parsedMap map[string]string
	if err := json.Unmarshal([]byte(content), &parsedMap); err != nil {
		return nil, fmt.Errorf("narrative JSON invalid: %w", err)
	}

	out := map[string]string{}
	for _, t := range topics {
		if v := strings.TrimSpace(parsedMap[t]); v != "" {
			out[t] = v
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("narrative model produced no usable topics")
	}
	return out, nil
}

// buildDeterministicNarratives provides figure-grounded fallback narratives when the LLM is unreachable.
func buildDeterministicNarratives(pa *rmdomain.ClientPortfolioAnalysis, an *rmdomain.ClientAnalytics, adv *rmdomain.ClientAdvisory, si *rmdomain.ClientSpendIntelligence) map[string]string {
	out := make(map[string]string)

	if pa != nil && pa.Allocation != nil {
		a := pa.Allocation
		out["allocation"] = fmt.Sprintf(
			"The portfolio maintains a %s allocation with %.1f%% equity, %.1f%% debt, and %.1f%% in other assets (total %s). Asset allocation aligns with current risk capacity while preserving compounding momentum.",
			titleCaseWords(a.Level), a.EquityPct, a.DebtPct, a.OtherPct, inr(a.TotalValue),
		)

		gm := a.Genome
		out["genome"] = fmt.Sprintf(
			"Risk DNA registers Growth at %.0f%% and Capital Preservation at %.0f%%, balanced by Liquidity at %.0f%% and Inflation Defense at %.0f%%. This factor profile provides stable portfolio defense across market cycles.",
			norm100(gm.Growth), norm100(gm.CapitalPreservation), norm100(gm.Liquidity), norm100(gm.InflationDefense),
		)
	}

	if pa != nil && pa.Discipline != nil {
		d := pa.Discipline
		out["discipline"] = fmt.Sprintf(
			"Client discipline is categorized as %s with an active %d-month investment streak and %.0f%% SIP consistency. Automated mandate execution is at %.0f%%.",
			titleCaseWords(d.Level), d.CurrentStreakMonths, d.SIPConsistencyPct, d.SIPAutomationPct,
		)
	}

	if pa != nil && pa.Performance != nil {
		p := pa.Performance
		out["performance"] = fmt.Sprintf(
			"Portfolio return is %.1f%% (%.1f%% annualized) with cumulative gains of %s. Alpha metrics vs benchmark indices remain %s.",
			p.TotalReturnPct, p.AnnualizedReturnPct, inr(p.TotalGainAmount), strings.ToLower(titleCaseWords(p.Level)),
		)
	}

	if an != nil {
		if r := an.Risk; r != nil && r.Points >= 3 {
			out["risk"] = fmt.Sprintf(
				"Historical max drawdown is %.1f%% with annualized volatility at %.1f%%. The portfolio has demonstrated solid drawdown recovery across observed holding periods.",
				r.MaxDrawdownPct, r.VolatilityPct,
			)
		}
		if c := an.Cost; c != nil && c.MFValue > 0 {
			out["cost"] = fmt.Sprintf(
				"Weighted expense ratio is %.2f%% with an annual fee impact of %s. Moving out of high-cost regular schemes could save approximately %s annually.",
				c.WeightedExpenseRatio, inr(c.AnnualFee), inr(c.AvoidableAnnualFee),
			)
		}
		if cc := an.Concentration; cc != nil && cc.HoldingsCount > 0 {
			out["concentration"] = fmt.Sprintf(
				"Sector concentration index is %s (HHI %.0f) across %d active holdings, maintaining suitable multi-sector diversification.",
				strings.ToLower(cc.SectorHHILabel), cc.SectorHHI, cc.HoldingsCount,
			)
		}
		if wc := an.WhatChanged; wc != nil && wc.SinceDays > 0 {
			out["drift"] = fmt.Sprintf(
				"Over the past %d days, equity allocation shifted by %+.1f%% and overall portfolio wealth changed by %+.1f%%.",
				wc.SinceDays, wc.EquityPctDelta, wc.WealthPctDelta,
			)
		}
		if tx := an.Tax; tx != nil && (tx.LTCGGain > 0 || tx.STCGGain > 0) {
			out["tax"] = fmt.Sprintf(
				"Estimated tax exposure is %s (LTCG) and %s (STCG). Consider opportunistic tax-loss harvesting to optimize net realized yields.",
				inr(tx.EstLTCGTax), inr(tx.EstSTCGTax),
			)
		}
	}

	if si != nil {
		if wd, tr := si.WeekdayWeekend, si.Trend; wd != nil || tr != nil {
			var parts []string
			if wd != nil {
				parts = append(parts, fmt.Sprintf("weekend spend runs %.0f%% of weekday spend (peak day %s)", wd.WeekendVsWeekdayRatio*100, titleCaseWords(wd.PeakDay)))
			}
			if tr != nil {
				parts = append(parts, fmt.Sprintf("the %s spend trend is %s, projecting %s over the next 30 days", tr.Period, strings.ToLower(titleCaseWords(tr.Direction)), inr(tr.Projected30Day)))
			}
			out["spend_overview"] = "Day-to-day spending: " + strings.Join(parts, "; ") + "."
		}

		if ct := si.CategoryTrend; ct != nil && len(ct.Categories) > 0 {
			top := ct.Categories[0]
			risingNote := ""
			if si.CategoryMomentum != nil && len(si.CategoryMomentum.Rising) > 0 {
				risingNote = fmt.Sprintf(" %s is accelerating fastest.", si.CategoryMomentum.Rising[0].Category)
			}
			out["spend_categories"] = fmt.Sprintf(
				"%s leads spend at %.0f%% of the total (%s), %s vs the prior period.%s",
				top.Category, top.SharePct, inr(top.CurrentTotal), strings.ToLower(titleCaseWords(top.Direction)), risingNote,
			)
		}

		if rec, ni, bnpl := si.Recurring, si.NightImpulse, si.BNPLExposure; rec != nil || ni != nil || bnpl != nil {
			var parts []string
			if rec != nil && len(rec.Recurring) > 0 {
				parts = append(parts, fmt.Sprintf("%d recurring charges totalling ~%s/month", len(rec.Recurring), inr(rec.TotalMonthlyEstimate)))
			}
			if ni != nil && ni.ImpulseScore > 0 {
				parts = append(parts, fmt.Sprintf("an impulse score of %.0f/100", ni.ImpulseScore))
			}
			if bnpl != nil && bnpl.Last30DayTotal > 0 {
				riskNote := ""
				if bnpl.IsDangerZone {
					riskNote = " (elevated relative to income)"
				}
				parts = append(parts, fmt.Sprintf("BNPL usage of %s in the last 30 days%s", inr(bnpl.Last30DayTotal), riskNote))
			}
			if len(parts) > 0 {
				out["spend_habits"] = "Spending habits: " + strings.Join(parts, "; ") + "."
			}
		}

		if inc := si.Income; inc != nil && inc.CreditCount > 0 {
			paydayNote := "no clear next payday detected"
			if inc.NextPredictedPayday != nil {
				paydayNote = "next payday predicted at " + inc.NextPredictedPayday.Time().Format("Jan 2")
			}
			out["spend_income"] = fmt.Sprintf(
				"Income is %s and %s, averaging %s per credit; %s.",
				strings.ToLower(titleCaseWords(inc.StabilityLabel)), strings.ToLower(titleCaseWords(inc.FrequencyLabel)), inr(inc.AvgCreditAmount), paydayNote,
			)
		}
	}

	if si != nil && si.Budget != nil {
		b := si.Budget
		if b.HasBudget {
			out["budget_health"] = fmt.Sprintf(
				"The %s budget is %s at %s of %s spent (%.0f%% used), projected to land at %s with %d days remaining.",
				b.ActiveMonth, strings.ToLower(titleCaseWords(b.Status)), inr(b.TotalSpent), inr(b.TotalBudget),
				safePct(b.TotalSpent, b.TotalBudget), inr(b.ProjectedSpend), b.DaysRemaining,
			)
		} else {
			out["budget_health"] = "This client has not set up a monthly budget yet — spend is tracked but not benchmarked against a plan."
		}
	}

	return out
}

func safePct(part, whole float64) float64 {
	if whole == 0 {
		return 0
	}
	return part / whole * 100
}

// buildFiguresBlob renders the client's computed analytics as a compact,
// deterministic, one-line-per-topic block — the sole input the model may use.
// Topics with no meaningful data are omitted entirely.
func buildFiguresBlob(pa *rmdomain.ClientPortfolioAnalysis, an *rmdomain.ClientAnalytics, adv *rmdomain.ClientAdvisory, si *rmdomain.ClientSpendIntelligence) string {
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

	if si != nil {
		if wd := si.WeekdayWeekend; wd != nil {
			line("wkdwknd: weekday=%.0f weekend=%.0f ratio=%.2f peak=%s",
				wd.WeekdayTotal, wd.WeekendTotal, wd.WeekendVsWeekdayRatio, wd.PeakDay)
		}
		if tr := si.Trend; tr != nil {
			line("trend: period=%s dir=%s vol=%s proj30d=%.0f changepct=%.1f",
				tr.Period, tr.Direction, tr.VolatilityLevel, tr.Projected30Day, tr.OverallTrendChangePct)
		}
		if ct := si.CategoryTrend; ct != nil && len(ct.Categories) > 0 {
			cats := ""
			for i, c := range ct.Categories {
				if i >= 5 {
					break
				}
				if i > 0 {
					cats += ", "
				}
				cats += fmt.Sprintf("%s %.0f(%s)", c.Category, c.SharePct, c.Direction)
			}
			line("spendcats: top=%s topshare=%.0f cats=[%s]", ct.TopCategory, ct.TopCategorySharePct, cats)
		}
		if rec := si.Recurring; rec != nil && len(rec.Recurring) > 0 {
			line("recurring: count=%d monthlyest=%.0f", len(rec.Recurring), rec.TotalMonthlyEstimate)
		}
		if ni := si.NightImpulse; ni != nil {
			line("impulse: score=%.0f nightsharepct=%.0f nighttotal=%.0f",
				ni.ImpulseScore, ni.NightSpendSharePct, ni.NightSpendTotal)
		}
		if bnpl := si.BNPLExposure; bnpl != nil && bnpl.Last30DayTotal > 0 {
			line("bnpl: last30d=%.0f incomeratio=%.1f danger=%t", bnpl.Last30DayTotal, bnpl.IncomeRatioPct, bnpl.IsDangerZone)
		}
		if inc := si.Income; inc != nil {
			line("income: avgcredit=%.0f stability=%s frequency=%s", inc.AvgCreditAmount, inc.StabilityLabel, inc.FrequencyLabel)
		}
		if b := si.Budget; b != nil && b.HasBudget {
			line("budget: month=%s total=%.0f spent=%.0f status=%s health=%.0f daysleft=%d",
				b.ActiveMonth, b.TotalBudget, b.TotalSpent, b.Status, b.HealthScore, b.DaysRemaining)
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
