package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// IDBIAARepository owns the Account Aggregator mirror tables (migration
// 000032): idbi_aa_consents and idbi_aa_linked_accounts. Statement
// transactions pulled for a consent are written into spend_transactions
// via IDBIRepository.UpsertSpendTransactions with source='aa'.
type IDBIAARepository struct {
	pool *pgxpool.Pool
}

func NewIDBIAARepository(pool *pgxpool.Pool) *IDBIAARepository {
	return &IDBIAARepository{pool: pool}
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

// CreateConsent inserts a freshly requested consent (590). The handle is the
// primary key; a repeat request for the same handle refreshes the mutable
// fields.
func (r *IDBIAARepository) CreateConsent(ctx context.Context, row AAConsentRow) error {
	if row.PartyIDType == "" {
		row.PartyIDType = "MOBILE"
	}
	if row.Status == "" {
		row.Status = "PENDING"
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO idbi_aa_consents
			(consent_handle, user_id, consent_id, status, party_id_type, party_id_value,
			 vua, product_id, account_id, redirect_url, web_redirect_url, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11, now(), now())
		ON CONFLICT (consent_handle) DO UPDATE SET
			consent_id       = COALESCE(NULLIF(EXCLUDED.consent_id, ''), idbi_aa_consents.consent_id),
			-- A repeat 590 (the sandbox reuses one fixed handle) must never
			-- drag an already-advanced consent back to PENDING; the lifecycle
			-- only moves forward, driven by 591 / the 497 webhook.
			status           = CASE WHEN idbi_aa_consents.status = 'PENDING'
			                        THEN EXCLUDED.status ELSE idbi_aa_consents.status END,
			redirect_url     = COALESCE(NULLIF(EXCLUDED.redirect_url, ''), idbi_aa_consents.redirect_url),
			web_redirect_url = COALESCE(NULLIF(EXCLUDED.web_redirect_url, ''), idbi_aa_consents.web_redirect_url),
			updated_at       = now()
	`, row.ConsentHandle, row.UserID, row.ConsentID, row.Status, row.PartyIDType, row.PartyIDValue,
		row.VUA, row.ProductID, row.AccountID, row.RedirectURL, row.WebRedirectURL)
	if err != nil {
		return fmt.Errorf("aa create consent: %w", err)
	}
	return nil
}

// UpdateConsentStatus updates status (and consent_id / approved_at when
// supplied) for a handle.
func (r *IDBIAARepository) UpdateConsentStatus(ctx context.Context, handle, status, consentID string, approvedAt *time.Time) error {
	ct, err := r.pool.Exec(ctx, `
		UPDATE idbi_aa_consents SET
			status      = $2,
			consent_id  = COALESCE(NULLIF($3, ''), consent_id),
			approved_at = COALESCE($4, approved_at),
			updated_at  = now()
		WHERE consent_handle = $1
	`, handle, status, consentID, approvedAt)
	if err != nil {
		return fmt.Errorf("aa update consent status: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrNoConsent
	}
	return nil
}

// MarkFetched stamps last_fetched_at = now() for a handle.
func (r *IDBIAARepository) MarkFetched(ctx context.Context, handle string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE idbi_aa_consents SET last_fetched_at = now(), updated_at = now() WHERE consent_handle = $1`, handle)
	if err != nil {
		return fmt.Errorf("aa mark fetched: %w", err)
	}
	return nil
}

func scanConsent(row pgx.Row) (AAConsentRow, error) {
	var c AAConsentRow
	err := row.Scan(
		&c.ConsentHandle, &c.UserID, &c.ConsentID, &c.Status, &c.PartyIDType, &c.PartyIDValue,
		&c.VUA, &c.ProductID, &c.AccountID, &c.RedirectURL, &c.WebRedirectURL,
		&c.ApprovedAt, &c.ExpiresAt, &c.LastFetchedAt, &c.CreatedAt, &c.UpdatedAt,
	)
	return c, err
}

const aaConsentCols = `consent_handle, user_id, consent_id, status, party_id_type, party_id_value,
	vua, product_id, account_id, redirect_url, web_redirect_url,
	approved_at, expires_at, last_fetched_at, created_at, updated_at`

// GetConsent returns one consent by handle. ok=false means no such row.
func (r *IDBIAARepository) GetConsent(ctx context.Context, handle string) (AAConsentRow, bool, error) {
	c, err := scanConsent(r.pool.QueryRow(ctx,
		`SELECT `+aaConsentCols+` FROM idbi_aa_consents WHERE consent_handle = $1`, handle))
	if errors.Is(err, pgx.ErrNoRows) {
		return AAConsentRow{}, false, nil
	}
	if err != nil {
		return AAConsentRow{}, false, fmt.Errorf("aa get consent: %w", err)
	}
	return c, true, nil
}

// ListConsents returns every consent for a user, newest first.
func (r *IDBIAARepository) ListConsents(ctx context.Context, userID uuid.UUID) ([]AAConsentRow, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+aaConsentCols+` FROM idbi_aa_consents WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("aa list consents: %w", err)
	}
	defer rows.Close()

	var out []AAConsentRow
	for rows.Next() {
		c, err := scanConsent(rows)
		if err != nil {
			return nil, fmt.Errorf("aa list consents scan: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ReplaceLinkedAccounts sets the linked-account set for a handle to exactly
// accs (upsert + prune), in one transaction.
func (r *IDBIAARepository) ReplaceLinkedAccounts(ctx context.Context, handle string, accs []AALinkedAccountRow) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("aa replace linked: begin: %w", err)
	}
	defer tx.Rollback(ctx)

	keep := make([]string, 0, len(accs))
	for _, a := range accs {
		if a.LinkRefNumber == "" {
			continue
		}
		keep = append(keep, a.LinkRefNumber)
		_, err = tx.Exec(ctx, `
			INSERT INTO idbi_aa_linked_accounts
				(consent_handle, link_ref_number, fip_id, fip_name, account_type, fi_type, masked_account_number, linked_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7, now())
			ON CONFLICT (consent_handle, link_ref_number) DO UPDATE SET
				fip_id                = EXCLUDED.fip_id,
				fip_name              = EXCLUDED.fip_name,
				account_type          = EXCLUDED.account_type,
				fi_type               = EXCLUDED.fi_type,
				masked_account_number = EXCLUDED.masked_account_number
		`, handle, a.LinkRefNumber, a.FipID, a.FipName, a.AccountType, a.FiType, a.MaskedAccountNumber)
		if err != nil {
			return fmt.Errorf("aa replace linked: upsert %s: %w", a.LinkRefNumber, err)
		}
	}

	if len(keep) == 0 {
		_, err = tx.Exec(ctx, `DELETE FROM idbi_aa_linked_accounts WHERE consent_handle = $1`, handle)
	} else {
		_, err = tx.Exec(ctx,
			`DELETE FROM idbi_aa_linked_accounts WHERE consent_handle = $1 AND link_ref_number <> ALL($2)`, handle, keep)
	}
	if err != nil {
		return fmt.Errorf("aa replace linked: prune: %w", err)
	}
	return tx.Commit(ctx)
}

// ListLinkedAccounts returns the linked accounts for a handle.
func (r *IDBIAARepository) ListLinkedAccounts(ctx context.Context, handle string) ([]AALinkedAccountRow, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT consent_handle, link_ref_number, fip_id, fip_name, account_type, fi_type, masked_account_number, linked_at
		FROM idbi_aa_linked_accounts WHERE consent_handle = $1 ORDER BY link_ref_number
	`, handle)
	if err != nil {
		return nil, fmt.Errorf("aa list linked: %w", err)
	}
	defer rows.Close()

	var out []AALinkedAccountRow
	for rows.Next() {
		var a AALinkedAccountRow
		if err := rows.Scan(&a.ConsentHandle, &a.LinkRefNumber, &a.FipID, &a.FipName,
			&a.AccountType, &a.FiType, &a.MaskedAccountNumber, &a.LinkedAt); err != nil {
			return nil, fmt.Errorf("aa list linked scan: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
