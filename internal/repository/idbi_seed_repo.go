package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourusername/astra-backend/internal/db"
)

// IDBISeedRepository reads and writes the idbi_seed_customers table
// (migration 000033), which stores the sandbox PAN/CustID/Name fixture data
// from docs/IDBI APIs - Data.csv. This table is used to:
//   - Seed demo users' idbi_customer_link rows for local development.
//   - Back the dedupe-check (456) and CKYC (415) sandbox flows.
type IDBISeedRepository struct {
	queries *db.Queries
}

func NewIDBISeedRepository(pool *pgxpool.Pool) *IDBISeedRepository {
	return &IDBISeedRepository{queries: db.New(pool)}
}

// SeedCustomer is the public representation of an idbi_seed_customers row.
type SeedCustomer struct {
	CustID       string
	PANNumber    string
	CustomerName string
	AccountType  string // SAV_REGULAR, CUR_CORP, ...
	AccountCode  string // SB001, CA005, ...
}

// UpsertSeedCustomer inserts or updates one seed customer row.
func (r *IDBISeedRepository) UpsertSeedCustomer(ctx context.Context, c SeedCustomer) error {
	err := r.queries.UpsertSeedCustomer(ctx, db.UpsertSeedCustomerParams{
		CustID:       c.CustID,
		PanNumber:    c.PANNumber,
		CustomerName: c.CustomerName,
		AccountType:  c.AccountType,
		AccountCode:  c.AccountCode,
	})
	if err != nil {
		return fmt.Errorf("upsert seed customer %s: %w", c.CustID, err)
	}
	return nil
}

// GetSeedCustomer returns a seed customer by cust_id, ok=false if not found.
func (r *IDBISeedRepository) GetSeedCustomer(ctx context.Context, custID string) (SeedCustomer, bool, error) {
	row, err := r.queries.GetSeedCustomer(ctx, custID)
	if errors.Is(err, pgx.ErrNoRows) {
		return SeedCustomer{}, false, nil
	}
	if err != nil {
		return SeedCustomer{}, false, fmt.Errorf("get seed customer: %w", err)
	}
	return toSeedCustomer(row), true, nil
}

// GetSeedCustomerByPAN returns the first seed customer matching a PAN number.
func (r *IDBISeedRepository) GetSeedCustomerByPAN(ctx context.Context, pan string) (SeedCustomer, bool, error) {
	row, err := r.queries.GetSeedCustomerByPAN(ctx, pan)
	if errors.Is(err, pgx.ErrNoRows) {
		return SeedCustomer{}, false, nil
	}
	if err != nil {
		return SeedCustomer{}, false, fmt.Errorf("get seed customer by pan: %w", err)
	}
	return toSeedCustomer(row), true, nil
}

// ListByAccountType returns all seed customers for a given account type
// (e.g. "SAV_REGULAR", "CUR_CORP").
func (r *IDBISeedRepository) ListByAccountType(ctx context.Context, accountType string) ([]SeedCustomer, error) {
	rows, err := r.queries.ListSeedCustomersByAccountType(ctx, accountType)
	if err != nil {
		return nil, fmt.Errorf("list seed customers by type: %w", err)
	}
	out := make([]SeedCustomer, 0, len(rows))
	for _, row := range rows {
		out = append(out, toSeedCustomer(row))
	}
	return out, nil
}

// ListAll returns every seed customer row, ordered by account_type then cust_id.
func (r *IDBISeedRepository) ListAll(ctx context.Context) ([]SeedCustomer, error) {
	rows, err := r.queries.ListAllSeedCustomers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list all seed customers: %w", err)
	}
	out := make([]SeedCustomer, 0, len(rows))
	for _, row := range rows {
		out = append(out, toSeedCustomer(row))
	}
	return out, nil
}

// Count returns the number of rows in idbi_seed_customers.
func (r *IDBISeedRepository) Count(ctx context.Context) (int64, error) {
	n, err := r.queries.CountSeedCustomers(ctx)
	if err != nil {
		return 0, fmt.Errorf("count seed customers: %w", err)
	}
	return n, nil
}

func toSeedCustomer(row db.IdbiSeedCustomer) SeedCustomer {
	return SeedCustomer{
		CustID:       row.CustID,
		PANNumber:    row.PanNumber,
		CustomerName: row.CustomerName,
		AccountType:  row.AccountType,
		AccountCode:  row.AccountCode,
	}
}
