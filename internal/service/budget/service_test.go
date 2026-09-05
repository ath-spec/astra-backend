package budget

import (
	"strings"
	"testing"
	"time"

	analyticsdomain "github.com/yourusername/astra-backend/internal/domain/analytics"
)

// testCanon stands in for the DB-backed category index in unit tests.
func testCanon(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "groceries", "food & dining":
		return "food_dining"
	case "rent", "bills & utilities":
		return "utilities"
	case "transport":
		return "transportation"
	case "subscriptions":
		return "entertainment"
	case "health":
		return "healthcare"
	default:
		return "other"
	}
}

func TestComputeFallbackCategories_NoHistory(t *testing.T) {
	got := computeFallbackCategories(10000, nil)
	if len(got) == 0 {
		t.Fatal("expected default distribution, got none")
	}
	var sum float64
	for _, c := range got {
		sum += c.SuggestedAmount
	}
	if sum < 9000 || sum > 11000 {
		t.Errorf("default distribution sums to %.2f, want ~10000", sum)
	}
}

func TestComputeFallbackCategories_ProportionalToHistory(t *testing.T) {
	hist := map[string][]map[string]interface{}{
		"food_dining":    {{"spent": 6000.0, "month": "2026-01"}},
		"transportation": {{"spent": 2000.0, "month": "2026-01"}},
		"shopping":       {{"spent": 2000.0, "month": "2026-01"}},
	}
	got := computeFallbackCategories(20000, hist)
	byID := map[string]float64{}
	for _, c := range got {
		byID[c.CategoryID] = c.SuggestedAmount
	}
	if byID["food_dining"] < 11000 || byID["food_dining"] > 13000 {
		t.Errorf("food_dining = %.2f, want ~12000 (60%% of 20000)", byID["food_dining"])
	}
}

func TestBuildHistory_SplitsIncomeExpenseAndCategories(t *testing.T) {
	now := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	mk := func(y, m int, typ analyticsdomain.TxnType, cat string, amt float64) analyticsdomain.Transaction {
		return analyticsdomain.Transaction{
			Type: typ, Category: cat, Amount: amt,
			OccurredAt: time.Date(y, time.Month(m), 10, 12, 0, 0, 0, time.UTC),
		}
	}
	txns := []analyticsdomain.Transaction{
		mk(2026, 4, analyticsdomain.TxnCredit, "Salary", 100000),
		mk(2026, 4, analyticsdomain.TxnDebit, "Groceries", 8000),
		mk(2026, 4, analyticsdomain.TxnDebit, "Rent", 30000),
		mk(2026, 5, analyticsdomain.TxnCredit, "Salary", 100000),
		mk(2026, 5, analyticsdomain.TxnDebit, "Groceries", 9000),
		// June is the target month — must be excluded from history.
		mk(2026, 6, analyticsdomain.TxnDebit, "Groceries", 5000),
	}

	h := buildHistory(txns, now, testCanon)

	if len(h.historical) != 2 {
		t.Fatalf("expected 2 historical months (Apr, May), got %d", len(h.historical))
	}
	if h.avgIncome != 100000 {
		t.Errorf("avgIncome = %.2f, want 100000", h.avgIncome)
	}
	if _, ok := h.categoryHistory["food_dining"]; !ok {
		t.Error("expected food_dining in category history")
	}
	if _, ok := h.categoryHistory["utilities"]; !ok {
		t.Error("expected utilities (Rent folds in) in category history")
	}
	for _, entries := range h.categoryHistory {
		for _, e := range entries {
			if e["month"] == "2026-06" {
				t.Error("target month leaked into category history")
			}
		}
	}
}

func TestCategoryStatus(t *testing.T) {
	if categoryStatus(50) != "positive" || categoryStatus(85) != "warning" || categoryStatus(120) != "critical" {
		t.Error("categoryStatus thresholds wrong")
	}
}
