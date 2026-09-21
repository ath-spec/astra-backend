package advisortips

import (
	"fmt"
	"strings"

	paDomain "github.com/yourusername/astra-backend/internal/domain/portfolioanalysis"
)

// deterministicTip computes a rule-based tip straight from the analysis
// payload, with no LLM call at all. It backs Generate whenever the LLM call
// fails, times out, or comes back empty — the "Pro Tip" widget should never
// go blank just because Groq had a bad moment; it should degrade to a
// template-based tip computed from the same numbers the agent would have
// seen. Returns ok=false only for topics/payload shapes it doesn't know how
// to summarize (currently: fund profiles, which are too payload-shaped/
// arbitrary to template safely).
func deterministicTip(topic Topic, payload any) (string, bool) {
	switch topic {
	case TopicAllocation:
		r, ok := payload.(*paDomain.AllocationResult)
		if !ok || r == nil {
			return "", false
		}
		return deterministicAllocationTip(r), true
	case TopicDiscipline:
		r, ok := payload.(*paDomain.DisciplineResult)
		if !ok || r == nil {
			return "", false
		}
		return deterministicDisciplineTip(r), true
	case TopicPerformance:
		r, ok := payload.(*paDomain.PerformanceResult)
		if !ok || r == nil {
			return "", false
		}
		return deterministicPerformanceTip(r), true
	default:
		return "", false
	}
}

func deterministicAllocationTip(r *paDomain.AllocationResult) string {
	level := strings.ToLower(strings.ReplaceAll(r.Level, "_", " "))
	switch {
	case r.TotalValue <= 0:
		return "Once you add holdings, this tab will show how your money is split across equity, debt and other assets."
	case r.EquityPct >= 80:
		return fmt.Sprintf("Your portfolio is %.0f%% equity, giving it a %s risk profile. That can mean sharper swings in the short term — a debt or gold allocation would smooth out the ride.", r.EquityPct, level)
	case r.EquityPct <= 20 && r.TotalValue > 0:
		return fmt.Sprintf("With only %.0f%% in equity, your portfolio leans heavily %s. That protects capital but may not outpace inflation over the long run — consider building some equity exposure if your goals are years away.", r.EquityPct, level)
	default:
		return fmt.Sprintf("Your portfolio is %.0f%% equity and %.0f%% debt, landing it in the %s band. That's a reasonable balance — review it again as your goals or timeline shift.", r.EquityPct, r.DebtPct, level)
	}
}

func deterministicDisciplineTip(r *paDomain.DisciplineResult) string {
	switch {
	case r.ActiveSegments == 0:
		return "Start a SIP to build the habit of regular investing — consistency matters more than the amount you begin with."
	case r.CurrentStreakMonths >= 6:
		return fmt.Sprintf("You've kept up a %d-month investing streak with %.0f%% SIP consistency — that discipline compounds over time. Keep the streak going.", r.CurrentStreakMonths, r.SIPConsistencyPct)
	case r.MissedMonths > 0:
		return fmt.Sprintf("You've missed %d month(s) of planned investing. Automating your SIPs (currently %.0f%% automated) is the easiest way to stay consistent without relying on willpower.", r.MissedMonths, r.SIPAutomationPct)
	default:
		return fmt.Sprintf("Your SIP consistency is at %.0f%% — steady investing, even in small amounts, tends to beat trying to time the market.", r.SIPConsistencyPct)
	}
}

func deterministicPerformanceTip(r *paDomain.PerformanceResult) string {
	switch {
	case r.TotalInvested <= 0:
		return "Once you have invested holdings, this tab will compare your returns against market benchmarks."
	case len(r.ExpensiveFunds) > 0:
		return fmt.Sprintf("Your portfolio has returned %.1f%%, but %d of your funds carry above-average expense ratios that are quietly eating into returns — worth a look.", r.TotalReturnPct, len(r.ExpensiveFunds))
	case r.TotalReturnPct >= 0:
		return fmt.Sprintf("Your portfolio has gained %.1f%% overall (%.1f%% annualized). Check the benchmark comparison below to see how that stacks up against the market.", r.TotalReturnPct, r.AnnualizedReturnPct)
	default:
		return fmt.Sprintf("Your portfolio is down %.1f%% overall. Short-term dips are normal for equity-heavy portfolios — avoid reacting to volatility alone.", -r.TotalReturnPct)
	}
}
