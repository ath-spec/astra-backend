// Package watchlist implements the Watchlist domain directly against
// Postgres — there is no external vendor concept for "which funds has this
// user bookmarked" (unlike Stocks/MF/FD, there's nothing to later swap in a
// real provider for), so this is a plain real implementation, not a mock.
package watchlist

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourusername/astra-backend/internal/apiresponse"
	watchlistdomain "github.com/yourusername/astra-backend/internal/domain/watchlist"
)

type PostgresProvider struct {
	pool *pgxpool.Pool
}

func NewPostgresProvider(pool *pgxpool.Pool) *PostgresProvider {
	return &PostgresProvider{pool: pool}
}

// Add is idempotent: bookmarking an already-watched fund is a no-op, not an
// error, so the bookmark button never has to distinguish "already added"
// from "just added" client-side.
func (p *PostgresProvider) Add(ctx context.Context, userID uuid.UUID, schemeCode string) error {
	var exists bool
	if err := p.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fund_catalog WHERE scheme_code = $1)`, schemeCode).Scan(&exists); err != nil {
		return fmt.Errorf("verify fund exists: %w", err)
	}
	if !exists {
		return apiresponse.NotFound("fund %s not found", schemeCode)
	}

	if _, err := p.pool.Exec(ctx, `
		INSERT INTO watchlist_items (user_id, scheme_code) VALUES ($1, $2)
		ON CONFLICT (user_id, scheme_code) DO NOTHING
	`, userID, schemeCode); err != nil {
		return fmt.Errorf("add watchlist item: %w", err)
	}
	return nil
}

// Remove is idempotent: removing a fund that isn't watched is a no-op.
func (p *PostgresProvider) Remove(ctx context.Context, userID uuid.UUID, schemeCode string) error {
	if _, err := p.pool.Exec(ctx, `DELETE FROM watchlist_items WHERE user_id = $1 AND scheme_code = $2`, userID, schemeCode); err != nil {
		return fmt.Errorf("remove watchlist item: %w", err)
	}
	return nil
}

func (p *PostgresProvider) IsWatched(ctx context.Context, userID uuid.UUID, schemeCode string) (bool, error) {
	var watched bool
	if err := p.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM watchlist_items WHERE user_id = $1 AND scheme_code = $2)`, userID, schemeCode).Scan(&watched); err != nil {
		return false, fmt.Errorf("check watchlist item: %w", err)
	}
	return watched, nil
}

func (p *PostgresProvider) List(ctx context.Context, userID uuid.UUID) ([]watchlistdomain.Item, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT c.scheme_code, c.scheme_name, c.amc_name, c.category, c.risk_level, c.nav, c.returns_1y, w.created_at
		FROM watchlist_items w JOIN fund_catalog c ON c.scheme_code = w.scheme_code
		WHERE w.user_id = $1
		ORDER BY w.created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list watchlist items: %w", err)
	}
	defer rows.Close()

	items := make([]watchlistdomain.Item, 0)
	for rows.Next() {
		var it watchlistdomain.Item
		if err := rows.Scan(&it.SchemeCode, &it.SchemeName, &it.AMCName, &it.Category, &it.RiskLevel, &it.NAV, &it.Returns1Y, &it.AddedAt); err != nil {
			return nil, fmt.Errorf("scan watchlist item: %w", err)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate watchlist items: %w", err)
	}
	return items, nil
}
