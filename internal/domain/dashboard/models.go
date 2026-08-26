// Package dashboard defines the wire types for the cross-domain portfolio
// summary that backs the Home screen: total wealth and a per-asset-class
// breakdown composed from the Stocks, MF, Fixed Deposits and bank-account
// domains that already exist. This package has no provider of its own — it
// only aggregates numbers already computed by those domains' own providers,
// so there's nothing here to swap for a "real" version later.
package dashboard

import "github.com/yourusername/astra-backend/internal/apitime"

type AssetBucket struct {
	Value              float64 `json:"value"`
	InvestedValue      float64 `json:"invested_value,omitempty"`
	ReturnsAmount      float64 `json:"returns_amount,omitempty"`
	ReturnsPct         float64 `json:"returns_pct,omitempty"`
	OneDayChangeAmount float64 `json:"one_day_change_amount"`
	OneDayChangePct    float64 `json:"one_day_change_pct"`
	SharePct           float64 `json:"share_pct"`
}

// Summary is the Home screen's top-level payload. TotalWealth is Mutual
// Funds + Stocks + Fixed Deposits (bank balance is reported separately,
// as most net-worth views treat cash differently from investments — the
// client can add BankBalance.Value into its own total if it wants a
// different definition).
type Summary struct {
	TotalWealth        float64     `json:"total_wealth"`
	OneDayChangeAmount float64     `json:"one_day_change_amount"`
	OneDayChangePct    float64     `json:"one_day_change_pct"`
	MutualFunds        AssetBucket `json:"mutual_funds"`
	Stocks             AssetBucket `json:"stocks"`
	FixedDeposits      AssetBucket `json:"fixed_deposits"`
	BankBalance        AssetBucket `json:"bank_balance"`
}

// SnapshotPoint is one day's recorded portfolio value, backing the Home
// screen's growth chart. History only exists from whenever a user's first
// dashboard read happened onward — see migration 000013.
type SnapshotPoint struct {
	Date        apitime.Time `json:"date"`
	TotalWealth float64      `json:"total_wealth"`
}
