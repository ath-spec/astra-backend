package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/yourusername/astra-backend/internal/apitime"
	watchlistdomain "github.com/yourusername/astra-backend/internal/domain/watchlist"
)

// ClientWatchlist returns the funds a client has bookmarked — same data the
// app's Watchlist screen shows the user, now visible to their assigned RM
// (or an admin) so a review conversation can reference what the client is
// actually eyeing, not just what they already hold.
func (s *RMService) ClientWatchlist(ctx context.Context, callerRMID uuid.UUID, isAdmin bool, userID uuid.UUID) ([]watchlistdomain.Item, error) {
	if err := s.authorizeClient(ctx, callerRMID, isAdmin, userID); err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT c.scheme_code, c.scheme_name, c.amc_name, c.category, c.risk_level, c.nav, c.returns_1y, w.created_at
		FROM watchlist_items w JOIN fund_catalog c ON c.scheme_code = w.scheme_code
		WHERE w.user_id = $1
		ORDER BY w.created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list client watchlist: %w", err)
	}
	defer rows.Close()

	items := make([]watchlistdomain.Item, 0)
	for rows.Next() {
		var it watchlistdomain.Item
		var addedAt apitime.Time
		if err := rows.Scan(&it.SchemeCode, &it.SchemeName, &it.AMCName, &it.Category, &it.RiskLevel, &it.NAV, &it.Returns1Y, &addedAt); err != nil {
			return nil, fmt.Errorf("scan client watchlist item: %w", err)
		}
		it.AddedAt = addedAt
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate client watchlist: %w", err)
	}
	return items, nil
}
