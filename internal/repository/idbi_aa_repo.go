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
)

// IDBIAARepository owns the Account Aggregator mirror tables using
// SQLC-generated queries (internal/db). Tables:
//   - idbi_aa_consents        (migration 000032)
//   - idbi_aa_linked_accounts (migration 000032)
//
// AA statement transactions are written into spend_transactions via
// IDBIRepository.UpsertSpendTransactions with source='aa'.
type IDBIAARepository struct {
	pool    *pgxpool.Pool
	queries *db.Queries
}

func NewIDBIAARepository(pool *pgxpool.Pool) *IDBIAARepository {
	return &IDBIAARepository{pool: pool, queries: db.New(pool)}
}

// ErrNoConsent means no AA consent row exists for the given handle.
var ErrNoConsent = errors.New("no idbi aa consent for handle")

// AAConsentRow mirrors one idbi_aa_consents row.
type AAConsentRow struct {
	ConsentHandle  string
	UserID         uuid.UUID
	ConsentID      string
	Status         string
	PartyIDType    string
	PartyIDValue   string
	VUA            string
	ProductID      string
	AccountID      string
	RedirectURL    string
	WebRedirectURL string
	ApprovedAt     *time.Time
	ExpiresAt      *time.Time
	LastFetchedAt  *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// AALinkedAccountRow mirrors one idbi_aa_linked_accounts row.
type AALinkedAccountRow struct {
	ConsentHandle       string
	LinkRefNumber       string
	FipID               string
	FipName             string
	AccountType         string
	FiType              string
	MaskedAccountNumber string
	LinkedAt            time.Time
}

func rowToConsent(row db.IdbiAaConsent) AAConsentRow {
	c := AAConsentRow{
		ConsentHandle:  row.ConsentHandle,
		UserID:         row.UserID,
		ConsentID:      row.ConsentID,
		Status:         row.Status,
		PartyIDType:    row.PartyIDType,
		PartyIDValue:   row.PartyIDValue,
		VUA:            row.Vua,
		ProductID:      row.ProductID,
		AccountID:      row.AccountID,
		RedirectURL:    row.RedirectUrl,
		WebRedirectURL: row.WebRedirectUrl,
		CreatedAt:      row.CreatedAt.Time,
		UpdatedAt:      row.UpdatedAt.Time,
	}
	if row.ApprovedAt.Valid {
		t := row.ApprovedAt.Time
		c.ApprovedAt = &t
	}
	if row.ExpiresAt.Valid {
		t := row.ExpiresAt.Time
		c.ExpiresAt = &t
	}
	if row.LastFetchedAt.Valid {
		t := row.LastFetchedAt.Time
		c.LastFetchedAt = &t
	}
	return c
}

func listRowToConsent(row db.IdbiAaConsent) AAConsentRow {
	c := AAConsentRow{
		ConsentHandle:  row.ConsentHandle,
		UserID:         row.UserID,
		ConsentID:      row.ConsentID,
		Status:         row.Status,
		PartyIDType:    row.PartyIDType,
		PartyIDValue:   row.PartyIDValue,
		VUA:            row.Vua,
		ProductID:      row.ProductID,
		AccountID:      row.AccountID,
		RedirectURL:    row.RedirectUrl,
		WebRedirectURL: row.WebRedirectUrl,
		CreatedAt:      row.CreatedAt.Time,
		UpdatedAt:      row.UpdatedAt.Time,
	}
	if row.ApprovedAt.Valid {
		t := row.ApprovedAt.Time
		c.ApprovedAt = &t
	}
	if row.ExpiresAt.Valid {
		t := row.ExpiresAt.Time
		c.ExpiresAt = &t
	}
	if row.LastFetchedAt.Valid {
		t := row.LastFetchedAt.Time
		c.LastFetchedAt = &t
	}
	return c
}

// CreateConsent inserts a freshly requested consent (590). Idempotent on handle.
func (r *IDBIAARepository) CreateConsent(ctx context.Context, row AAConsentRow) error {
	if row.PartyIDType == "" {
		row.PartyIDType = "MOBILE"
	}
	if row.Status == "" {
		row.Status = "PENDING"
	}
	err := r.queries.CreateConsent(ctx, db.CreateConsentParams{
		ConsentHandle:  row.ConsentHandle,
		UserID:         row.UserID,
		ConsentID:      row.ConsentID,
		Status:         row.Status,
		PartyIDType:    row.PartyIDType,
		PartyIDValue:   row.PartyIDValue,
		Vua:            row.VUA,
		ProductID:      row.ProductID,
		AccountID:      row.AccountID,
		RedirectUrl:    row.RedirectURL,
		WebRedirectUrl: row.WebRedirectURL,
	})
	if err != nil {
		return fmt.Errorf("aa create consent: %w", err)
	}
	return nil
}

// UpdateConsentStatus updates status (and consent_id / approved_at when supplied).
func (r *IDBIAARepository) UpdateConsentStatus(ctx context.Context, handle, status, consentID string, approvedAt *time.Time) error {
	var pgApprovedAt pgtype.Timestamptz
	if approvedAt != nil {
		pgApprovedAt = pgtype.Timestamptz{Time: *approvedAt, Valid: true}
	}
	n, err := r.queries.UpdateConsentStatus(ctx, db.UpdateConsentStatusParams{
		ConsentHandle: handle,
		Status:        status,
		Column3:       consentID,
		ApprovedAt:    pgApprovedAt,
	})
	if err != nil {
		return fmt.Errorf("aa update consent status: %w", err)
	}
	if n == 0 {
		return ErrNoConsent
	}
	return nil
}

// MarkFetched stamps last_fetched_at = now() for a handle.
func (r *IDBIAARepository) MarkFetched(ctx context.Context, handle string) error {
	if err := r.queries.MarkConsentFetched(ctx, handle); err != nil {
		return fmt.Errorf("aa mark fetched: %w", err)
	}
	return nil
}

// GetConsent returns one consent by handle. ok=false means no such row.
func (r *IDBIAARepository) GetConsent(ctx context.Context, handle string) (AAConsentRow, bool, error) {
	row, err := r.queries.GetConsent(ctx, handle)
	if errors.Is(err, pgx.ErrNoRows) {
		return AAConsentRow{}, false, nil
	}
	if err != nil {
		return AAConsentRow{}, false, fmt.Errorf("aa get consent: %w", err)
	}
	return rowToConsent(row), true, nil
}

// ListConsents returns every consent for a user, newest first.
func (r *IDBIAARepository) ListConsents(ctx context.Context, userID uuid.UUID) ([]AAConsentRow, error) {
	rows, err := r.queries.ListConsentsByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("aa list consents: %w", err)
	}
	out := make([]AAConsentRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, listRowToConsent(row))
	}
	return out, nil
}

// ReplaceLinkedAccounts sets the linked-account set for a handle to exactly
// accs (upsert + prune), in one transaction.
func (r *IDBIAARepository) ReplaceLinkedAccounts(ctx context.Context, handle string, accs []AALinkedAccountRow) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("aa replace linked: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	qtx := db.New(tx)
	keep := make([]string, 0, len(accs))

	for _, a := range accs {
		if a.LinkRefNumber == "" {
			continue
		}
		keep = append(keep, a.LinkRefNumber)
		err = qtx.UpsertLinkedAccount(ctx, db.UpsertLinkedAccountParams{
			ConsentHandle:       handle,
			LinkRefNumber:       a.LinkRefNumber,
			FipID:               a.FipID,
			FipName:             a.FipName,
			AccountType:         a.AccountType,
			FiType:              a.FiType,
			MaskedAccountNumber: a.MaskedAccountNumber,
		})
		if err != nil {
			return fmt.Errorf("aa replace linked: upsert %s: %w", a.LinkRefNumber, err)
		}
	}

	if len(keep) == 0 {
		err = qtx.DeleteAllLinkedAccounts(ctx, handle)
	} else {
		err = qtx.DeleteLinkedAccountsExcept(ctx, db.DeleteLinkedAccountsExceptParams{
			ConsentHandle: handle,
			Column2:       keep,
		})
	}
	if err != nil {
		return fmt.Errorf("aa replace linked: prune: %w", err)
	}
	return tx.Commit(ctx)
}

// ListLinkedAccounts returns the linked accounts for a handle.
func (r *IDBIAARepository) ListLinkedAccounts(ctx context.Context, handle string) ([]AALinkedAccountRow, error) {
	rows, err := r.queries.ListLinkedAccounts(ctx, handle)
	if err != nil {
		return nil, fmt.Errorf("aa list linked: %w", err)
	}
	out := make([]AALinkedAccountRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, AALinkedAccountRow{
			ConsentHandle:       row.ConsentHandle,
			LinkRefNumber:       row.LinkRefNumber,
			FipID:               row.FipID,
			FipName:             row.FipName,
			AccountType:         row.AccountType,
			FiType:              row.FiType,
			MaskedAccountNumber: row.MaskedAccountNumber,
			LinkedAt:            row.LinkedAt.Time,
		})
	}
	return out, nil
}
