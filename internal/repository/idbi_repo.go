package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourusername/astra-backend/internal/idbimap"
)

// IDBIRepository owns the IDBI mirror tables (migration 000027):
// idbi_customer_link and idbi_accounts. It stores only what the IDBI APIs
// return, mapped through internal/idbimap — no business logic here.
type IDBIRepository struct {
	pool *pgxpool.Pool
}

func NewIDBIRepository(pool *pgxpool.Pool) *IDBIRepository {
	return &IDBIRepository{pool: pool}
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
	var l CustomerLink
	err := r.pool.QueryRow(ctx, `
		SELECT user_id, cif_id, cust_id, source, linked_at
		FROM idbi_customer_link
		WHERE user_id = $1
	`, userID).Scan(&l.UserID, &l.CifID, &l.CustID, &l.Source, &l.LinkedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return CustomerLink{}, ErrNoCustomerLink
	}
	if err != nil {
		return CustomerLink{}, fmt.Errorf("get idbi customer link: %w", err)
	}
	return l, nil
}

// UpsertCustomerLink creates or updates the user's IDBI identity link.
func (r *IDBIRepository) UpsertCustomerLink(ctx context.Context, userID uuid.UUID, cifID, custID, source string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO idbi_customer_link (user_id, cif_id, cust_id, source, linked_at, updated_at)
		VALUES ($1, $2, $3, $4, now(), now())
		ON CONFLICT (user_id) DO UPDATE SET
			cif_id     = EXCLUDED.cif_id,
			cust_id    = CASE WHEN EXCLUDED.cust_id <> '' THEN EXCLUDED.cust_id ELSE idbi_customer_link.cust_id END,
			source     = EXCLUDED.source,
			updated_at = now()
	`, userID, cifID, custID, source)
	if err != nil {
		return fmt.Errorf("upsert idbi customer link: %w", err)
	}
	return nil
}

// ReplaceAccounts replaces the whole set of mirrored accounts for a user in one
// transaction: existing rows for accounts not in accs are deleted, the rest are
// upserted. Balances of 0 in accs are written as-is (the mapper already picked
// the best available figure).
func (r *IDBIRepository) ReplaceAccounts(ctx context.Context, userID uuid.UUID, accs []idbimap.Account) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("replace idbi accounts: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	keep := make([]string, 0, len(accs))
	for _, a := range accs {
		keep = append(keep, a.AccountNumber)

		var openedAt any
		if t, perr := idbimap.ParseTime(a.OpenedAt); perr == nil && !t.IsZero() {
			openedAt = t
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO idbi_accounts (
				user_id, account_number, account_type, currency, cif_id, cust_id,
				holder_name, status, branch_id, branch_name, opened_at,
				ledger_balance, available_balance, effective_balance, lien_amount, synced_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15, now())
			ON CONFLICT (user_id, account_number) DO UPDATE SET
				account_type      = EXCLUDED.account_type,
				currency          = EXCLUDED.currency,
				cif_id            = CASE WHEN EXCLUDED.cif_id <> '' THEN EXCLUDED.cif_id ELSE idbi_accounts.cif_id END,
				cust_id           = CASE WHEN EXCLUDED.cust_id <> '' THEN EXCLUDED.cust_id ELSE idbi_accounts.cust_id END,
				holder_name       = CASE WHEN EXCLUDED.holder_name <> '' THEN EXCLUDED.holder_name ELSE idbi_accounts.holder_name END,
				status            = EXCLUDED.status,
				branch_id         = CASE WHEN EXCLUDED.branch_id <> '' THEN EXCLUDED.branch_id ELSE idbi_accounts.branch_id END,
				branch_name       = CASE WHEN EXCLUDED.branch_name <> '' THEN EXCLUDED.branch_name ELSE idbi_accounts.branch_name END,
				opened_at         = COALESCE(EXCLUDED.opened_at, idbi_accounts.opened_at),
				ledger_balance    = EXCLUDED.ledger_balance,
				available_balance = EXCLUDED.available_balance,
				effective_balance = EXCLUDED.effective_balance,
				lien_amount       = EXCLUDED.lien_amount,
				synced_at         = now()
		`,
			userID, a.AccountNumber, a.AccountType, defStr(a.Currency, "INR"), a.CifID, a.CustID,
			a.HolderName, a.Status, a.BranchID, a.BranchName, openedAt,
			a.LedgerBalance, a.AvailableBalance, a.EffectiveBalance, a.LienAmount,
		)
		if err != nil {
			return fmt.Errorf("replace idbi accounts: upsert %s: %w", a.AccountNumber, err)
		}
	}

	if len(keep) > 0 {
		_, err = tx.Exec(ctx, `
			DELETE FROM idbi_accounts
			WHERE user_id = $1 AND account_number <> ALL($2)
		`, userID, keep)
	} else {
		_, err = tx.Exec(ctx, `DELETE FROM idbi_accounts WHERE user_id = $1`, userID)
	}
	if err != nil {
		return fmt.Errorf("replace idbi accounts: prune: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("replace idbi accounts: commit: %w", err)
	}
	return nil
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

// ListAccounts returns the user's mirrored IDBI accounts, newest sync first.
func (r *IDBIRepository) ListAccounts(ctx context.Context, userID uuid.UUID) ([]MirroredAccount, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT account_number, account_type, currency, holder_name, status, branch_id, branch_name,
		       ledger_balance, available_balance, effective_balance, lien_amount, synced_at
		FROM idbi_accounts
		WHERE user_id = $1
		ORDER BY account_type, account_number
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list idbi accounts: %w", err)
	}
	defer rows.Close()

	var out []MirroredAccount
	for rows.Next() {
		var a MirroredAccount
		if err := rows.Scan(
			&a.AccountNumber, &a.AccountType, &a.Currency, &a.HolderName, &a.Status, &a.BranchID, &a.BranchName,
			&a.LedgerBalance, &a.AvailableBalance, &a.EffectiveBalance, &a.LienAmount, &a.SyncedAt,
		); err != nil {
			return nil, fmt.Errorf("scan idbi account: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func defStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// UpsertCreditExposure writes the RM credit-risk snapshot for a client
// (feature 5, one row per user).
func (r *IDBIRepository) UpsertCreditExposure(ctx context.Context, userID uuid.UUID, e idbimap.CreditExposure) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO idbi_credit_exposure (
			user_id, cif_id, customer_name, account_manager, cust_rating,
			total_limit, funded_limit, non_funded_limit, total_outstanding, utilisation_pct,
			loan_count, total_overdue, max_dpd, worst_npa_status, risk_band, synced_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15, now())
		ON CONFLICT (user_id) DO UPDATE SET
			cif_id = EXCLUDED.cif_id, customer_name = EXCLUDED.customer_name,
			account_manager = EXCLUDED.account_manager, cust_rating = EXCLUDED.cust_rating,
			total_limit = EXCLUDED.total_limit, funded_limit = EXCLUDED.funded_limit,
			non_funded_limit = EXCLUDED.non_funded_limit, total_outstanding = EXCLUDED.total_outstanding,
			utilisation_pct = EXCLUDED.utilisation_pct, loan_count = EXCLUDED.loan_count,
			total_overdue = EXCLUDED.total_overdue, max_dpd = EXCLUDED.max_dpd,
			worst_npa_status = EXCLUDED.worst_npa_status, risk_band = EXCLUDED.risk_band,
			synced_at = now()
	`, userID, e.CifID, e.CustomerName, e.AccountManager, e.CustRating,
		e.TotalLimit, e.FundedLimit, e.NonFundedLimit, e.TotalOutstanding, e.UtilisationPct,
		e.LoanCount, e.TotalOverdue, e.MaxDPD, e.WorstNpaStatus, e.RiskBand)
	if err != nil {
		return fmt.Errorf("upsert credit exposure: %w", err)
	}
	return nil
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

// GetCreditExposure returns the client's snapshot, ok=false if none yet.
func (r *IDBIRepository) GetCreditExposure(ctx context.Context, userID uuid.UUID) (CreditExposureRow, bool, error) {
	var e CreditExposureRow
	err := r.pool.QueryRow(ctx, `
		SELECT cif_id, customer_name, account_manager, cust_rating,
		       total_limit, funded_limit, non_funded_limit, total_outstanding, utilisation_pct,
		       loan_count, total_overdue, max_dpd, worst_npa_status, risk_band, synced_at
		FROM idbi_credit_exposure WHERE user_id = $1
	`, userID).Scan(
		&e.CifID, &e.CustomerName, &e.AccountManager, &e.CustRating,
		&e.TotalLimit, &e.FundedLimit, &e.NonFundedLimit, &e.TotalOutstanding, &e.UtilisationPct,
		&e.LoanCount, &e.TotalOverdue, &e.MaxDPD, &e.WorstNpaStatus, &e.RiskBand, &e.SyncedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return CreditExposureRow{}, false, nil
	}
	if err != nil {
		return CreditExposureRow{}, false, fmt.Errorf("get credit exposure: %w", err)
	}
	return e, true, nil
}

// UpsertSpendTransactions writes IDBI/AA transactions into spend_transactions
// (migration 000028 added source/external_id/account_ref). Idempotent on
// (user_id, source, external_id): a re-sync updates in place instead of
// duplicating. Rows with an empty ExternalID are skipped (nothing to dedupe
// on). source is 'idbi' (393) or 'aa' (595/739).
func (r *IDBIRepository) UpsertSpendTransactions(ctx context.Context, userID uuid.UUID, source string, rows []idbimap.SpendRow) (int, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("upsert spend txns: begin: %w", err)
	}
	defer tx.Rollback(ctx)

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
		_, err = tx.Exec(ctx, `
			INSERT INTO spend_transactions
				(user_id, amount, type, category, merchant, occurred_at, source, external_id, account_ref)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
			ON CONFLICT (user_id, source, external_id) WHERE external_id IS NOT NULL
			DO UPDATE SET
				amount      = EXCLUDED.amount,
				type        = EXCLUDED.type,
				category    = EXCLUDED.category,
				merchant    = EXCLUDED.merchant,
				occurred_at = EXCLUDED.occurred_at,
				account_ref = EXCLUDED.account_ref
		`, userID, row.Amount, typ, cat, merchant, row.OccurredAt, source, row.ExternalID, row.AccountRef)
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

// ReplaceLoans replaces the user's mirrored loan accounts (feature 3).
func (r *IDBIRepository) ReplaceLoans(ctx context.Context, userID uuid.UUID, loans []idbimap.Loan) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("replace idbi loans: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	keep := make([]string, 0, len(loans))
	for _, l := range loans {
		keep = append(keep, l.LoanAccountID)

		var openedAt, npaDate any
		if t, e := idbimap.ParseTime(l.OpenedAt); e == nil && !t.IsZero() {
			openedAt = t
		}
		if t, e := idbimap.ParseTime(l.NpaDate); e == nil && !t.IsZero() {
			npaDate = t
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO idbi_loans (
				user_id, loan_account_id, cust_id, holder_name,
				sanctioned_amount, disbursed_amount, available_amount, interest_rate,
				tenure_months, repayment_method, interest_method, opened_at,
				outstanding_balance, overdue_amount, dpd, npa_status, npa_date, synced_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17, now())
			ON CONFLICT (user_id, loan_account_id) DO UPDATE SET
				cust_id            = CASE WHEN EXCLUDED.cust_id <> '' THEN EXCLUDED.cust_id ELSE idbi_loans.cust_id END,
				holder_name        = CASE WHEN EXCLUDED.holder_name <> '' THEN EXCLUDED.holder_name ELSE idbi_loans.holder_name END,
				sanctioned_amount  = EXCLUDED.sanctioned_amount,
				disbursed_amount   = EXCLUDED.disbursed_amount,
				available_amount   = EXCLUDED.available_amount,
				interest_rate      = EXCLUDED.interest_rate,
				tenure_months      = EXCLUDED.tenure_months,
				repayment_method   = EXCLUDED.repayment_method,
				interest_method    = EXCLUDED.interest_method,
				opened_at          = COALESCE(EXCLUDED.opened_at, idbi_loans.opened_at),
				outstanding_balance = EXCLUDED.outstanding_balance,
				overdue_amount     = EXCLUDED.overdue_amount,
				dpd                = EXCLUDED.dpd,
				npa_status         = EXCLUDED.npa_status,
				npa_date           = EXCLUDED.npa_date,
				synced_at          = now()
		`,
			userID, l.LoanAccountID, l.CustID, l.HolderName,
			l.SanctionedAmount, l.DisbursedAmount, l.AvailableAmount, l.InterestRate,
			l.TenureMonths, l.RepaymentMethod, l.InterestMethod, openedAt,
			l.OutstandingBalance, l.OverdueAmount, l.DPD, l.NpaStatus, npaDate,
		)
		if err != nil {
			return fmt.Errorf("replace idbi loans: upsert %s: %w", l.LoanAccountID, err)
		}
	}

	if len(keep) > 0 {
		_, err = tx.Exec(ctx, `DELETE FROM idbi_loans WHERE user_id = $1 AND loan_account_id <> ALL($2)`, userID, keep)
	} else {
		_, err = tx.Exec(ctx, `DELETE FROM idbi_loans WHERE user_id = $1`, userID)
	}
	if err != nil {
		return fmt.Errorf("replace idbi loans: prune: %w", err)
	}
	return tx.Commit(ctx)
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

// ListLoans returns the user's mirrored loan accounts.
func (r *IDBIRepository) ListLoans(ctx context.Context, userID uuid.UUID) ([]MirroredLoan, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT loan_account_id, holder_name, sanctioned_amount, disbursed_amount, interest_rate,
		       tenure_months, repayment_method, outstanding_balance, overdue_amount, dpd, npa_status, synced_at
		FROM idbi_loans WHERE user_id = $1
		ORDER BY loan_account_id
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list idbi loans: %w", err)
	}
	defer rows.Close()
	var out []MirroredLoan
	for rows.Next() {
		var l MirroredLoan
		if err := rows.Scan(
			&l.LoanAccountID, &l.HolderName, &l.SanctionedAmount, &l.DisbursedAmount, &l.InterestRate,
			&l.TenureMonths, &l.RepaymentMethod, &l.OutstandingBalance, &l.OverdueAmount, &l.DPD, &l.NpaStatus, &l.SyncedAt,
		); err != nil {
			return nil, fmt.Errorf("scan idbi loan: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// LatestSpendSync returns the newest occurred_at for a user's rows from the
// given source, or zero time if none. Used to window the next sync.
func (r *IDBIRepository) LatestSpendSync(ctx context.Context, userID uuid.UUID, source string) (time.Time, error) {
	var t *time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT max(occurred_at) FROM spend_transactions
		WHERE user_id = $1 AND source = $2
	`, userID, source).Scan(&t)
	if err != nil {
		return time.Time{}, fmt.Errorf("latest spend sync: %w", err)
	}
	if t == nil {
		return time.Time{}, nil
	}
	return *t, nil
}
