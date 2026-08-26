// Package watchlist defines the wire types for a user's saved/bookmarked
// funds — backs the bookmark button on the fund profile screen and the
// Watchlist screen's list.
package watchlist

import "github.com/yourusername/astra-backend/internal/apitime"

// Item is one watchlisted fund, joined with enough catalog display fields
// for the Watchlist screen's list to render without a second round trip per
// item.
type Item struct {
	SchemeCode string       `json:"scheme_code"`
	SchemeName string       `json:"scheme_name"`
	AMCName    string       `json:"amc_name"`
	Category   string       `json:"category"`
	RiskLevel  string       `json:"risk_level"`
	NAV        float64      `json:"nav"`
	Returns1Y  *float64     `json:"returns_1y,omitempty"`
	AddedAt    apitime.Time `json:"added_at"`
}
