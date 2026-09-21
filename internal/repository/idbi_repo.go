package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourusername/astra-backend/internal/db"
	"github.com/yourusername/astra-backend/internal/idbimap"
)

// IDBIRepository owns the IDBI mirror tables using SQLC-generated queries
// (internal/db). It is the sole write path for:
//   - idbi_customer_link  (migration 000027)
//   - idbi_accounts       (migration 000027)
//   - idbi_loans          (migration 000029)
//   - idbi_credit_exposure (migration 000031)
//   - spend_transactions  IDBI/AA rows (migration 000028 cols)
type IDBIRepository struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

func NewIDBIRepository(pool *pgxpool.Pool) *IDBIRepository {
	return &IDBIRepository{pool: pool, queries: db.New(pool)}
}

// ErrNoCustomerLink means the user has not been linked to an IDBI customer
// identity yet — the caller cannot fetch banking data for them.
var ErrNoCustomerLink = errors.New("no idbi customer link for user")

// CustomerLink is the user <-> IDBI identity mapping.
type CustomerLink struct {
	UserID   uuid.UUID
	CifID    string
	CustID   string
	Source   string
	LinkedAt time.Time
}

// GetCustomerLink returns the user's IDBI identity, or ErrNoCustomerLink.
func (r *IDBIRepository) GetCustomerLink(ctx context.Context, userID uuid.UUID) (CustomerLink, error) {
	row, err := r.queries.GetCustomerLink(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return CustomerLink{}, ErrNoCustomerLink
	}
	if err != nil {
		return CustomerLink{}, fmt.Errorf("get idbi customer link: %w", err)
	}
	return CustomerLink{
		UserID:   row.UserID,
		CifID:    row.CifID,
		CustID:   row.CustID,
		Source:   row.Source,
		LinkedAt: row.LinkedAt.Time,
	}, nil
}

// UpsertCustomerLink creates or updates the user's IDBI identity link.
func (r *IDBIRepository) UpsertCustomerLink(ctx context.Context, userID uuid.UUID, cifID, custID, source string) error {
	err := r.queries.UpsertCustomerLink(ctx, db.UpsertCustomerLinkParams{
		UserID: userID,
		CifID:  cifID,
		CustID: custID,
		Source: source,
	})
	if err != nil {
		return fmt.Errorf("upsert idbi customer link: %w", err)
	}
	return nil
}

// ListLinkedUserIDs returns every user with an idbi_customer_link row — the
// full set the nightly spend-sync scheduler needs to iterate. Plain pool
// query since sqlc has no generated query for this yet.
func (r *IDBIRepository) ListLinkedUserIDs(ctx context.Context) ([]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx, `SELECT user_id FROM idbi_customer_link`)
	if err != nil {
		return nil, fmt.Errorf("list idbi linked users: %w", err)
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan idbi linked user: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// MirroredAccount is a row of idbi_accounts.
type MirroredAccount struct {
	AccountNumber    string
	AccountType      string
	Currency         string
	HolderName       string
	Status           string
	BranchID         string
	BranchName       string
	LedgerBalance    float64
	AvailableBalance float64
	EffectiveBalance float64
	LienAmount       float64
	SyncedAt         time.Time
}

// ReplaceAccounts replaces the whole set of mirrored accounts for a user in one
// transaction: stale accounts are deleted, the rest are upserted.
func (r *IDBIRepository) ReplaceAccounts(ctx context.Context, userID uuid.UUID, accs []idbimap.Account) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("replace idbi accounts: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	qtx := db.New(tx)
	keep := make([]string, 0, len(accs))

	for _, a := range accs {
		keep = append(keep, a.AccountNumber)

		var openedAt pgtype.Timestamptz
		if t, perr := idbimap.ParseTime(a.OpenedAt); perr == nil && !t.IsZero() {
			openedAt = pgtype.Timestamptz{Time: t, Valid: true}
		}

		err = qtx.UpsertAccount(ctx, db.UpsertAccountParams{
			UserID:           userID,
			AccountNumber:    a.AccountNumber,
			AccountType:      a.AccountType,
			Currency:         defStr(a.Currency, "INR"),
			CifID:            a.CifID,
			CustID:           a.CustID,
			HolderName:       a.HolderName,
			Status:           a.Status,
			BranchID:         a.BranchID,
			BranchName:       a.BranchName,
			OpenedAt:         openedAt,
			LedgerBalance:    numericFromFloat(a.LedgerBalance),
			AvailableBalance: numericFromFloat(a.AvailableBalance),
			EffectiveBalance: numericFromFloat(a.EffectiveBalance),
			LienAmount:       numericFromFloat(a.LienAmount),
		})
		if err != nil {
			return fmt.Errorf("replace idbi accounts: upsert %s: %w", a.AccountNumber, err)
		}
	}

	if len(keep) > 0 {
		err = qtx.DeleteAccountsExcept(ctx, db.DeleteAccountsExceptParams{
			UserID:  userID,
			Column2: keep,
		})
	} else {
		err = qtx.DeleteAllAccounts(ctx, userID)
	}
	if err != nil {
		return fmt.Errorf("replace idbi accounts: prune: %w", err)
	}

	return tx.Commit(ctx)
}

// ListAccounts returns the user's mirrored IDBI accounts.
func (r *IDBIRepository) ListAccounts(ctx context.Context, userID uuid.UUID) ([]MirroredAccount, error) {
	rows, err := r.queries.ListAccounts(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list idbi accounts: %w", err)
	}
	out := make([]MirroredAccount, 0, len(rows))
	for _, row := range rows {
		out = append(out, MirroredAccount{
			AccountNumber:    row.AccountNumber,
			AccountType:      row.AccountType,
			Currency:         row.Currency,
			HolderName:       row.HolderName,
			Status:           row.Status,
			BranchID:         row.BranchID,
			BranchName:       row.BranchName,
			LedgerBalance:    floatFromNumeric(row.LedgerBalance),
			AvailableBalance: floatFromNumeric(row.AvailableBalance),
			EffectiveBalance: floatFromNumeric(row.EffectiveBalance),
			LienAmount:       floatFromNumeric(row.LienAmount),
			SyncedAt:         row.SyncedAt.Time,
		})
	}
	return out, nil
}

// MirroredLoan is a row of idbi_loans.
type MirroredLoan struct {
	LoanAccountID      string
	HolderName         string
	SanctionedAmount   float64
	DisbursedAmount    float64
	InterestRate       float64
	TenureMonths       int
	RepaymentMethod    string
	OutstandingBalance float64
	OverdueAmount      float64
	DPD                int
	NpaStatus          string
	SyncedAt           time.Time
}

// ReplaceLoans replaces the user's mirrored loan accounts (feature 3).
func (r *IDBIRepository) ReplaceLoans(ctx context.Context, userID uuid.UUID, loans []idbimap.Loan) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("replace idbi loans: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	qtx := db.New(tx)
	keep := make([]string, 0, len(loans))

	for _, l := range loans {
		keep = append(keep, l.LoanAccountID)

		var openedAt, npaDate pgtype.Timestamptz
		if t, e := idbimap.ParseTime(l.OpenedAt); e == nil && !t.IsZero() {
			openedAt = pgtype.Timestamptz{Time: t, Valid: true}
		}
		if t, e := idbimap.ParseTime(l.NpaDate); e == nil && !t.IsZero() {
			npaDate = pgtype.Timestamptz{Time: t, Valid: true}
		}

		err = qtx.UpsertLoan(ctx, db.UpsertLoanParams{
			UserID:             userID,
			LoanAccountID:      l.LoanAccountID,
			CustID:             l.CustID,
			HolderName:         l.HolderName,
			SanctionedAmount:   numericFromFloat(l.SanctionedAmount),
			DisbursedAmount:    numericFromFloat(l.DisbursedAmount),
			AvailableAmount:    numericFromFloat(l.AvailableAmount),
			InterestRate:       numericFromFloat(l.InterestRate),
			TenureMonths:       int32(l.TenureMonths),
			RepaymentMethod:    l.RepaymentMethod,
			InterestMethod:     l.InterestMethod,
			OpenedAt:           openedAt,
			OutstandingBalance: numericFromFloat(l.OutstandingBalance),
			OverdueAmount:      numericFromFloat(l.OverdueAmount),
			Dpd:                int32(l.DPD),
			NpaStatus:          l.NpaStatus,
			NpaDate:            npaDate,
		})
		if err != nil {
			return fmt.Errorf("replace idbi loans: upsert %s: %w", l.LoanAccountID, err)
		}
	}

	if len(keep) > 0 {
		err = qtx.DeleteLoansExcept(ctx, db.DeleteLoansExceptParams{
			UserID:  userID,
			Column2: keep,
		})
	} else {
		err = qtx.DeleteAllLoans(ctx, userID)
	}
	if err != nil {
		return fmt.Errorf("replace idbi loans: prune: %w", err)
	}
	return tx.Commit(ctx)
}

// ListLoans returns the user's mirrored loan accounts.
func (r *IDBIRepository) ListLoans(ctx context.Context, userID uuid.UUID) ([]MirroredLoan, error) {
	rows, err := r.queries.ListLoans(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list idbi loans: %w", err)
	}
	out := make([]MirroredLoan, 0, len(rows))
	for _, row := range rows {
		out = append(out, MirroredLoan{
			LoanAccountID:      row.LoanAccountID,
			HolderName:         row.HolderName,
			SanctionedAmount:   floatFromNumeric(row.SanctionedAmount),
			DisbursedAmount:    floatFromNumeric(row.DisbursedAmount),
			InterestRate:       floatFromNumeric(row.InterestRate),
			TenureMonths:       int(row.TenureMonths),
			RepaymentMethod:    row.RepaymentMethod,
			OutstandingBalance: floatFromNumeric(row.OutstandingBalance),
			OverdueAmount:      floatFromNumeric(row.OverdueAmount),
			DPD:                int(row.Dpd),
			NpaStatus:          row.NpaStatus,
			SyncedAt:           row.SyncedAt.Time,
		})
	}
	return out, nil
}

// CreditExposureRow is a row of idbi_credit_exposure.
type CreditExposureRow struct {
	CifID            string
	CustomerName     string
	AccountManager   string
	CustRating       string
	TotalLimit       float64
	FundedLimit      float64
	NonFundedLimit   float64
	TotalOutstanding float64
	UtilisationPct   float64
	LoanCount        int
	TotalOverdue     float64
	MaxDPD           int
	WorstNpaStatus   string
	RiskBand         string
	SyncedAt         time.Time
}

// UpsertCreditExposure writes the RM credit-risk snapshot for a client.
func (r *IDBIRepository) UpsertCreditExposure(ctx context.Context, userID uuid.UUID, e idbimap.CreditExposure) error {
	err := r.queries.UpsertCreditExposure(ctx, db.UpsertCreditExposureParams{
		UserID:           userID,
		CifID:            e.CifID,
		CustomerName:     e.CustomerName,
		AccountManager:   e.AccountManager,
		CustRating:       e.CustRating,
		TotalLimit:       numericFromFloat(e.TotalLimit),
		FundedLimit:      numericFromFloat(e.FundedLimit),
		NonFundedLimit:   numericFromFloat(e.NonFundedLimit),
		TotalOutstanding: numericFromFloat(e.TotalOutstanding),
		UtilisationPct:   numericFromFloat(e.UtilisationPct),
		LoanCount:        int32(e.LoanCount),
		TotalOverdue:     numericFromFloat(e.TotalOverdue),
		MaxDpd:           int32(e.MaxDPD),
		WorstNpaStatus:   e.WorstNpaStatus,
		RiskBand:         e.RiskBand,
	})
	if err != nil {
		return fmt.Errorf("upsert credit exposure: %w", err)
	}
	return nil
}

// GetCreditExposure returns the client's snapshot; ok=false if none yet.
func (r *IDBIRepository) GetCreditExposure(ctx context.Context, userID uuid.UUID) (CreditExposureRow, bool, error) {
	row, err := r.queries.GetCreditExposure(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return CreditExposureRow{}, false, nil
	}
	if err != nil {
		return CreditExposureRow{}, false, fmt.Errorf("get credit exposure: %w", err)
	}
	return CreditExposureRow{
		CifID:            row.CifID,
		CustomerName:     row.CustomerName,
		AccountManager:   row.AccountManager,
		CustRating:       row.CustRating,
		TotalLimit:       floatFromNumeric(row.TotalLimit),
		FundedLimit:      floatFromNumeric(row.FundedLimit),
		NonFundedLimit:   floatFromNumeric(row.NonFundedLimit),
		TotalOutstanding: floatFromNumeric(row.TotalOutstanding),
		UtilisationPct:   floatFromNumeric(row.UtilisationPct),
		LoanCount:        int(row.LoanCount),
		TotalOverdue:     floatFromNumeric(row.TotalOverdue),
		MaxDPD:           int(row.MaxDpd),
		WorstNpaStatus:   row.WorstNpaStatus,
		RiskBand:         row.RiskBand,
		SyncedAt:         row.SyncedAt.Time,
	}, true, nil
}

// UpsertSpendTransactions writes IDBI/AA transactions into spend_transactions.
// source is 'idbi' (393) or 'aa' (595/739). Idempotent on external_id.
func (r *IDBIRepository) UpsertSpendTransactions(ctx context.Context, userID uuid.UUID, source string, rows []idbimap.SpendRow) (int, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("upsert spend txns: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	qtx := db.New(tx)
	n := 0
	for _, row := range rows {
		if row.ExternalID == "" || row.OccurredAt.IsZero() {
			continue
		}
		typ := string(row.Type)
		if typ == "" {
			typ = "DEBIT"
		}
		cat := row.Category
		if cat == "" {
			cat = "Other"
		}
		merchant := row.Merchant
		if len(merchant) > 100 {
			merchant = merchant[:100]
		}
		err = qtx.UpsertSpendTransaction(ctx, db.UpsertSpendTransactionParams{
			UserID:     userID,
			Amount:     numericFromFloat(row.Amount),
			Type:       typ,
			Category:   cat,
			Merchant:   merchant,
			OccurredAt: pgtype.Timestamptz{Time: row.OccurredAt, Valid: true},
			Source:     source,
			ExternalID: pgtype.Text{String: row.ExternalID, Valid: true},
			AccountRef: pgtype.Text{String: row.AccountRef, Valid: row.AccountRef != ""},
		})
		if err != nil {
			return 0, fmt.Errorf("upsert spend txn %s: %w", row.ExternalID, err)
		}
		n++
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("upsert spend txns: commit: %w", err)
	}
	return n, nil
}

// LatestSpendSync returns the newest occurred_at for IDBI/AA rows for a user.
func (r *IDBIRepository) LatestSpendSync(ctx context.Context, userID uuid.UUID, source string) (time.Time, error) {
	t, err := r.queries.LatestSpendSync(ctx, db.LatestSpendSyncParams{
		UserID: userID,
		Source: source,
	})
	if err != nil {
		return time.Time{}, fmt.Errorf("latest spend sync: %w", err)
	}
	return t.Time, nil
}

// --- helpers ---------------------------------------------------------------

func defStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// numericFromFloat converts a float64 to pgtype.Numeric for SQLC params.
func numericFromFloat(f float64) pgtype.Numeric {
	var n pgtype.Numeric
	_ = n.Scan(fmt.Sprintf("%f", f))
	return n
}

// floatFromNumeric converts a pgtype.Numeric row value to float64.
func floatFromNumeric(n pgtype.Numeric) float64 {
	if !n.Valid {
		return 0
	}
	f, _ := n.Float64Value()
	return f.Float64
}
