package repository

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/yourusername/astra-backend/internal/database"
)

type User struct {
	ID          uuid.UUID
	AstraUserID string
	Name        *string
	PhoneNumber string
	PanNumber   *string
	// WantsRM is the user's advisory opt-in, captured on the signup form.
	// A Relationship Manager is auto-assigned only when this is true.
	WantsRM   bool
	CreatedAt time.Time
}

type BankAccount struct {
	ID          uuid.UUID
	BankName    string
	AccountType string
	Balance     float64
}

type RefreshToken struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	ExpiresAt time.Time
	RevokedAt *time.Time
}

type UserRepository interface {
	// wantsRM is a pointer so a caller that omits the field (e.g. a re-auth
	// call that isn't the signup form) leaves the user's existing opt-in
	// untouched instead of silently resetting it to false and unassigning
	// their RM. nil means "don't change it"; a new user with nil defaults
	// to false (opted out) since there is nothing to preserve yet.
	FindOrCreateUser(ctx context.Context, astraUserID, phoneNumber, name string, wantsRM *bool, uiBanks interface{}) (user *User, isNew bool, err error)
	UpdateUserName(ctx context.Context, userID uuid.UUID, name string) error
	GetByID(ctx context.Context, userID uuid.UUID) (*User, error)
	GetLatestVerifiedPAN(ctx context.Context, userID uuid.UUID) (string, error)
	GetBankAccounts(ctx context.Context, userID uuid.UUID) ([]BankAccount, error)
	GetPrimaryBankAccount(ctx context.Context, userID uuid.UUID) (*BankAccount, error)

	CreateRefreshToken(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error
	GetRefreshToken(ctx context.Context, tokenHash string) (*RefreshToken, error)
	RevokeRefreshToken(ctx context.Context, tokenHash string) error
	// ConsumeRefreshToken atomically validates and revokes a refresh token in
	// one statement, returning the owning user ID only if this call is the
	// one that flipped it from unused to revoked. Two concurrent replays of
	// the same token (e.g. a leaked token used from two places at once, or a
	// client-side race) can otherwise both pass a separate "is it revoked?"
	// read before either write lands, and both mint a valid new token pair
	// from what should be a single-use token. Callers MUST use this instead
	// of GetRefreshToken+RevokeRefreshToken for the rotate-on-use flow.
	ConsumeRefreshToken(ctx context.Context, tokenHash string) (userID uuid.UUID, ok bool, err error)
	// RotateRefreshToken is ConsumeRefreshToken plus issuing the replacement,
	// both in one transaction. Use this for the refresh-endpoint flow.
	RotateRefreshToken(ctx context.Context, oldHash, newHash string, newExpiresAt time.Time) (userID uuid.UUID, ok bool, err error)
	DeleteUserByPhone(ctx context.Context, phoneNumber string) error
}

type PostgresUserRepository struct {
	db *database.Database

	// assigner, when set, auto-assigns every newly created user to a
	// Relationship Manager via the round-robin "active queue" (see
	// AssignmentRepository). Optional: if nil, or if it reports no active
	// RMs, the user is still created and simply lands in the admin
	// console's unassigned pool.
	assigner AssignmentRepository
}

func NewPostgresUserRepository(db *database.Database) *PostgresUserRepository {
	return &PostgresUserRepository{db: db}
}

// SetAssigner wires the RM auto-assignment engine into the signup path.
// Called once at startup after the assignment repository is constructed.
func (r *PostgresUserRepository) SetAssigner(a AssignmentRepository) {
	r.assigner = a
}

func (r *PostgresUserRepository) FindOrCreateUser(ctx context.Context, astraUserID, phoneNumber, name string, wantsRM *bool, uiBanks interface{}) (*User, bool, error) {
	if existing, err := r.findByPhone(ctx, phoneNumber); err != nil {
		return nil, false, err
	} else if existing != nil {
		// Fill in the name ONLY while none is set yet. The account row is
		// created on the first OTP verify (often with a placeholder name),
		// and the user corrects it during onboarding via a second verify
		// call before any real name exists — that's the case this backfills.
		// The mobile app sends *some* name on every verify call, including
		// plain returning-user logins (it defaults to a placeholder like
		// "Investor" when the login screen never asked for one), so once a
		// real name is stored it must never be overwritten here again — a
		// deliberate name change belongs to the dedicated profile-update
		// endpoint, not the login path.
		if trimmed := strings.TrimSpace(name); trimmed != "" && (existing.Name == nil || *existing.Name == "") {
			if _, err := r.db.Pool.Exec(ctx,
				`UPDATE users SET name = $1 WHERE id = $2`, trimmed, existing.ID); err != nil {
				return nil, false, fmt.Errorf("update user name: %w", err)
			}
			existing.Name = &trimmed
		}
		// Honour a change to the advisory opt-in: turning it on assigns an RM
		// if the user has none; turning it off releases their current RM.
		// wantsRM == nil means the caller (e.g. a background re-auth call
		// that isn't the signup form) has no opinion — leave it as-is rather
		// than treating "field omitted" as "opt out".
		if wantsRM != nil && *wantsRM != existing.WantsRM {
			if _, err := r.db.Pool.Exec(ctx,
				`UPDATE users SET wants_rm = $1 WHERE id = $2`, *wantsRM, existing.ID); err != nil {
				return nil, false, fmt.Errorf("update user wants_rm: %w", err)
			}
			existing.WantsRM = *wantsRM
			r.syncRMAssignment(ctx, existing.ID, *wantsRM)
		}
		return existing, false, nil
	}

	newUserWantsRM := wantsRM != nil && *wantsRM

	var user User
	var isNew bool
	err := r.db.Pool.QueryRow(ctx, `
		INSERT INTO users (id, astra_user_id, phone_number, name, wants_rm)
		VALUES (gen_random_uuid(), $1, $2, $3, $4)
		ON CONFLICT (phone_number) DO UPDATE SET phone_number = EXCLUDED.phone_number
		RETURNING id, astra_user_id, phone_number, name, wants_rm, created_at, (xmax = 0) AS is_new
	`, astraUserID, phoneNumber, name, newUserWantsRM).Scan(
		&user.ID,
		&user.AstraUserID,
		&user.PhoneNumber,
		&user.Name,
		&user.WantsRM,
		&user.CreatedAt,
		&isNew,
	)
	if err != nil {
		return nil, false, fmt.Errorf("create user: %w", err)
	}

	if !isNew {
		return &user, false, nil
	}

	if err := r.seedInitialUserData(ctx, user.ID, phoneNumber); err != nil {
		return nil, false, err
	}

	// Route the new user to a Relationship Manager via the round-robin
	// active queue — only when they opted in on the signup form. Best-effort:
	// a failure here (no active RMs, transient DB error) must never block
	// signup — the user is created either way and, if they wanted an RM,
	// shows up in the admin console's unassigned pool.
	if newUserWantsRM && r.assigner != nil {
		if _, err := r.assigner.AssignNextRM(ctx, user.ID); err != nil && !errors.Is(err, ErrNoActiveRM) {
			fmt.Printf("user_repo: auto-assign RM for user %s failed: %v\n", user.ID, err)
		}
	}

	return &user, true, nil
}

// syncRMAssignment reconciles a user's RM assignment with their advisory
// opt-in after it flips. Best-effort: assignment is a convenience, never a
// blocker for the auth flow, so failures are logged and swallowed.
func (r *PostgresUserRepository) syncRMAssignment(ctx context.Context, userID uuid.UUID, wantsRM bool) {
	if r.assigner == nil {
		return
	}
	if wantsRM {
		if _, err := r.assigner.AssignNextRM(ctx, userID); err != nil && !errors.Is(err, ErrNoActiveRM) {
			fmt.Printf("user_repo: opt-in RM assign for user %s failed: %v\n", userID, err)
		}
		return
	}
	// Opted out: release the current RM if they have one.
	if err := r.assigner.Unassign(ctx, userID, uuid.Nil, "user opted out of relationship manager"); err != nil {
		fmt.Printf("user_repo: opt-out RM unassign for user %s failed: %v\n", userID, err)
	}
}

// UpdateUserName sets the user's display name. Used by the app's onboarding
// name step, which runs after the account row already exists (it is created
// on the first OTP verify, before the name is collected).
func (r *PostgresUserRepository) UpdateUserName(ctx context.Context, userID uuid.UUID, name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return fmt.Errorf("name is required")
	}
	ct, err := r.db.Pool.Exec(ctx, `UPDATE users SET name = $1 WHERE id = $2`, trimmed, userID)
	if err != nil {
		return fmt.Errorf("update user name: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("user %s not found: %w", userID, pgx.ErrNoRows)
	}
	return nil
}

func (r *PostgresUserRepository) GetByID(ctx context.Context, userID uuid.UUID) (*User, error) {
	var user User
	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, astra_user_id, phone_number, name, wants_rm, created_at
		FROM users WHERE id = $1
	`, userID).Scan(&user.ID, &user.AstraUserID, &user.PhoneNumber, &user.Name, &user.WantsRM, &user.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("user %s not found: %w", userID, pgx.ErrNoRows)
		}
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return &user, nil
}

// GetLatestVerifiedPAN returns the PAN from this user's most recent CKYC
// verification (idbi_ckyc, written by POST /api/v1/kyc/pan/verify), or ""
// if they've never completed that step. This is the only place a user's PAN
// is actually persisted server-side — the account-details screen previously
// relied solely on a client-side value collected during onboarding that was
// never sent anywhere and never survived an app restart.
func (r *PostgresUserRepository) GetLatestVerifiedPAN(ctx context.Context, userID uuid.UUID) (string, error) {
	var pan string
	err := r.db.Pool.QueryRow(ctx, `
		SELECT pan FROM idbi_ckyc WHERE user_id = $1 ORDER BY verified_at DESC LIMIT 1
	`, userID).Scan(&pan)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("get latest verified pan: %w", err)
	}
	return pan, nil
}

func (r *PostgresUserRepository) findByPhone(ctx context.Context, phoneNumber string) (*User, error) {
	var user User
	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, astra_user_id, phone_number, name, wants_rm, created_at
		FROM users WHERE phone_number = $1
	`, phoneNumber).Scan(&user.ID, &user.AstraUserID, &user.PhoneNumber, &user.Name, &user.WantsRM, &user.CreatedAt)

	switch {
	case err == nil:
		return &user, nil
	case errors.Is(err, pgx.ErrNoRows):
		return nil, nil
	default:
		return nil, fmt.Errorf("lookup user by phone: %w", err)
	}
}

func (r *PostgresUserRepository) DeleteUserByPhone(ctx context.Context, phoneNumber string) error {
	_, err := r.db.Pool.Exec(ctx, `DELETE FROM users WHERE phone_number = $1`, phoneNumber)
	return err
}

// SeedBankDependentData creates this user's demo FD + mandate rows against a
// bank account they actually linked themselves (discover -> APPROVE AND
// CONNECT, or the manual "connect more accounts" picker) — never a
// hardcoded bank_accounts row inserted at signup before any consent. Called
// by AAHandler.AddAccount the first time a user's bank_accounts goes from
// zero to one; a no-op (every insert here is ON CONFLICT DO NOTHING, keyed
// off fixed demo IDs) if called again for a later account. Every user gets
// the same single Good Investor FD/mandate flavor (see
// seedGoodInvestorBankData), matching whatever holdings/goals
// seedInitialUserData already seeded them with.
func (r *PostgresUserRepository) SeedBankDependentData(ctx context.Context, userID, bankAccountID uuid.UUID) error {
	return r.seedGoodInvestorBankData(ctx, userID, bankAccountID)
}

// seedInitialUserData used to hash phone+userID into one of 4 wildly
// different investor archetypes (aggressive thematic growth, balanced,
// globally diversified, conservative) — every demo screen (funds, stocks,
// FD, discipline, allocation, performance, net worth) told a different,
// disconnected story depending on which bucket a phone number happened to
// land in. Now every user gets the same single "Good Investor" persona —
// a realistic, moderately diversified, consistently disciplined portfolio
// — so the whole app tells one coherent story.
func (r *PostgresUserRepository) seedInitialUserData(ctx context.Context, userID uuid.UUID, phoneNumber string) error {
	if err := r.seedGoodInvestorArchetype(ctx, userID); err != nil {
		return err
	}

	// spendProfiles[1] ("Balanced Bluechip Wealth Builder") is the steady,
	// ~25%-savings-rate profile that actually matches a disciplined good
	// investor — kept as an indexed array (rather than inlined here) only
	// because seedSpendHistory's signature takes a spendArchetypeProfile.
	return r.seedSpendHistory(ctx, userID, spendProfiles[1])
}

// seedGoodInvestorArchetype seeds a single, realistic "disciplined
// moderately-aggressive investor" persona: two solid, diversified equity
// bets (large cap + flexi cap) instead of a concentrated sector bet,
// consistent SIP discipline with no gaps, and a steady upward net worth
// trend — every number here is meant to read as plausible, not
// aspirational. This replaces what used to be 4 separate, disconnected
// archetypes (aggressive thematic growth, balanced, globally diversified,
// conservative) hashed off phone+userID; one consistent story now, always.
func (r *PostgresUserRepository) seedGoodInvestorArchetype(ctx context.Context, userID uuid.UUID) error {
	_, _ = r.db.Pool.Exec(ctx, `
		INSERT INTO demat_holdings (user_id, isin, trading_symbol, exchange, product, quantity, average_price, last_price, close_price, authorized_date)
		VALUES
		($1, 'INE255X01014', 'MSTCLTD', 'NSE', 'CNC', 75, 670.00, 745.00, 730.00, CURRENT_DATE - 60),
		($1, 'INE704P01017', 'COCHINSHIP', 'NSE', 'CNC', 18, 1440.00, 1510.00, 1480.00, CURRENT_DATE - 30)
		ON CONFLICT DO NOTHING
	`, userID)

	_, _ = r.db.Pool.Exec(ctx, `
		INSERT INTO stock_orders (order_id, user_id, exchange, trading_symbol, isin, transaction_type, quantity, product, order_type, price, status, filled_quantity, average_price, order_timestamp)
		VALUES
		('ORD-GOOD-01', $1, 'NSE', 'MSTCLTD', 'INE255X01014', 'BUY', 75, 'CNC', 'LIMIT', 670.00, 'COMPLETE', 75, 670.00, NOW() - INTERVAL '60 days'),
		('ORD-GOOD-02', $1, 'NSE', 'COCHINSHIP', 'INE704P01017', 'BUY', 18, 'CNC', 'LIMIT', 1440.00, 'COMPLETE', 18, 1440.00, NOW() - INTERVAL '30 days')
		ON CONFLICT DO NOTHING
	`, userID)

	// Three equity funds — a flexi cap and a large cap for active,
	// benchmark-beating exposure, plus a Nifty 50 index fund for passive
	// exposure (migration 000040) so the Allocation screen's "Equity
	// Exposure" index-fund percentage — and the "Investors Like You" peer
	// benchmark in peerIndexFundPct, which is asset-weighted across every
	// seeded user's folios — has real, non-zero data instead of reading 0%
	// because no catalog fund's name/category ever matched index/nifty/
	// sensex. All three scheme_codes match fund_catalog exactly so viewing
	// any fund's profile finds a real catalog row.
	var f1, f2, f3 uuid.UUID
	_ = r.db.Pool.QueryRow(ctx, `
		INSERT INTO mf_folios (user_id, folio_number, amc_name, scheme_code, scheme_name, isin, units_held, nav, nav_date, cost_value, category, plan_type)
		VALUES ($1, 'FOL-GOOD-01', 'PPFAS Mutual Fund', 'PARAG-FLX-G', 'Parag Parikh Flexi Cap Fund', 'INF879O01027', 350.000, 74.9012, CURRENT_DATE, 22000.00, 'Equity - Flexi Cap', 'DIRECT')
		RETURNING id
	`, userID).Scan(&f1)

	_ = r.db.Pool.QueryRow(ctx, `
		INSERT INTO mf_folios (user_id, folio_number, amc_name, scheme_code, scheme_name, isin, units_held, nav, nav_date, cost_value, category, plan_type)
		VALUES ($1, 'FOL-GOOD-02', 'SBI Mutual Fund', 'SBI-BLC-G', 'SBI Bluechip Fund', 'INF200K01158', 210.000, 78.4521, CURRENT_DATE, 14000.00, 'Equity - Large Cap', 'DIRECT')
		RETURNING id
	`, userID).Scan(&f2)

	_ = r.db.Pool.QueryRow(ctx, `
		INSERT INTO mf_folios (user_id, folio_number, amc_name, scheme_code, scheme_name, isin, units_held, nav, nav_date, cost_value, category, plan_type)
		VALUES ($1, 'FOL-GOOD-03', 'UTI Mutual Fund', 'UTI-N50-G', 'UTI Nifty 50 Index Fund', 'INF789F01XA1', 120.000, 285.4210, CURRENT_DATE, 30000.00, 'Equity - Large Cap', 'DIRECT')
		RETURNING id
	`, userID).Scan(&f3)

	// 12 months of unbroken monthly SIPs across all three funds — no missed
	// months across the entire 12-month window Discipline() and the RM
	// inflow/composition queries scan, which is exactly what should read as
	// "good discipline" on the discipline gauge instead of a middling score.
	// transaction_type must be exactly 'SIP' (not 'SIP_PURCHASE') — every
	// reader (portfolio_analysis.go Discipline/yearlyInvestmentHistory,
	// rm_composition.go, rm_advisory.go, rm_analytics.go) filters on
	// transaction_type IN ('PURCHASE','SIP'), so a mismatched literal here
	// silently drops every seeded transaction from every one of those calcs.
	for i := 0; i < 12; i++ {
		tDate := time.Now().AddDate(0, -i, -5)
		if f1 != uuid.Nil {
			_, _ = r.db.Pool.Exec(ctx, `INSERT INTO mf_transactions (folio_id, transaction_type, transaction_date, amount, units, price) VALUES ($1, 'SIP', $2, 5000.00, 66.75, 74.90)`, f1, tDate)
		}
		if f2 != uuid.Nil {
			_, _ = r.db.Pool.Exec(ctx, `INSERT INTO mf_transactions (folio_id, transaction_type, transaction_date, amount, units, price) VALUES ($1, 'SIP', $2, 3000.00, 38.24, 78.45)`, f2, tDate)
		}
		if f3 != uuid.Nil {
			_, _ = r.db.Pool.Exec(ctx, `INSERT INTO mf_transactions (folio_id, transaction_type, transaction_date, amount, units, price) VALUES ($1, 'SIP', $2, 2000.00, 7.01, 285.42)`, f3, tDate)
		}
	}

	_, _ = r.db.Pool.Exec(ctx, `
		INSERT INTO goals (user_id, title, category, target_amount, current_amount, target_date, status)
		VALUES
		($1, 'Home Down Payment', 'HOME', 3500000.00, 1250000.00, CURRENT_DATE + 1095, 'IN_PROGRESS'),
		($1, 'Emergency Fund', 'EMERGENCY', 500000.00, 320000.00, CURRENT_DATE + 365, 'IN_PROGRESS')
		ON CONFLICT DO NOTHING
	`, userID)

	// Seed 180 days of a steady, realistic upward net worth trend.
	return r.seedPortfolioSnapshots(ctx, userID, 180, 1650000.0, 2480000.0)
}

// seedGoodInvestorBankData creates the demo FD + mandates tied to a real,
// user-approved bank account (see SeedBankDependentData) instead of a
// hardcoded bank_accounts row inserted at signup before the user ever
// consented to linking anything. Every good-investor user always gets an
// FD here — it's the one consistent debt/safety-net sleeve behind the
// equity funds above, not something only some personas had.
func (r *PostgresUserRepository) seedGoodInvestorBankData(ctx context.Context, userID, bankAccountID uuid.UUID) error {
	_, _ = r.db.Pool.Exec(ctx, `
		INSERT INTO fd_accounts (fd_account_number, user_id, bank_account_id, principal_amount, interest_rate, tenure_months, interest_payout, auto_renewal, nominee_name, booking_date, maturity_date, maturity_amount, status)
		VALUES ('FD-GOOD-201', $1, $2, 50000.00, 7.10, 12, 'ON_MATURITY', true, 'Self', CURRENT_DATE - 60, CURRENT_DATE + 305, 53645.00, 'ACTIVE')
		ON CONFLICT DO NOTHING
	`, userID, bankAccountID)

	// category = 'SIP' (not the 'OTHER' default) so the "SIPs & Mandates"
	// screen — which shows only category='SIP' mandates, see
	// seedDemoSubscriptions in provider/payments/recurring.go for the
	// separate category='SUBSCRIPTION' seed that screen deliberately
	// excludes — actually finds these two rows.
	_, err := r.db.Pool.Exec(ctx, `
		INSERT INTO mandates (mandate_id, user_id, bank_account_id, mandate_type, upi_id, payee_name, payee_vpa_or_id, category, max_amount, frequency, mandate_start_date, next_debit_date, status)
		VALUES
		('MND-GOOD-01', $1, $2, 'UPI_AUTOPAY', 'user@okhdfc', 'Parag Parikh Flexi Cap SIP', 'ppfas@upi', 'SIP', 5000.00, 'MONTHLY', CURRENT_DATE - 240, CURRENT_DATE + 10, 'ACTIVE'),
		('MND-GOOD-02', $1, $2, 'UPI_AUTOPAY', 'user@okhdfc', 'SBI Bluechip SIP', 'sbi@upi', 'SIP', 3000.00, 'MONTHLY', CURRENT_DATE - 120, CURRENT_DATE + 15, 'ACTIVE')
		ON CONFLICT DO NOTHING
	`, userID, bankAccountID)
	return err
}


// seedPortfolioSnapshots backfills `days` daily rows in portfolio_snapshots,
// linearly growing from `startValue` to `endValue` with slight daily noise.
// This gives every freshly-seeded user a realistic growth chart on first login.
func (r *PostgresUserRepository) seedPortfolioSnapshots(
	ctx context.Context,
	userID uuid.UUID,
	days int,
	startValue, endValue float64,
) error {
	today := time.Now().UTC().Truncate(24 * time.Hour)
	for i := days - 1; i >= 0; i-- {
		day := today.AddDate(0, 0, -i)
		// Linear interpolation between startValue and endValue.
		t := float64(days-1-i) / math.Max(float64(days-1), 1)
		value := math.Round((startValue+t*(endValue-startValue))*100) / 100
		_, err := r.db.Pool.Exec(ctx, `
			INSERT INTO portfolio_snapshots
				(user_id, snapshot_date, total_wealth, mutual_funds_value, stocks_value, fixed_deposits_value, bank_balance_value)
			VALUES ($1, $2, $3, $3 * 0.65, $3 * 0.20, $3 * 0.10, $3 * 0.05)
			ON CONFLICT (user_id, snapshot_date) DO NOTHING
		`, userID, day, value)
		if err != nil {
			return fmt.Errorf("seed portfolio snapshot day %d: %w", i, err)
		}
	}
	return nil
}

func (r *PostgresUserRepository) GetBankAccounts(ctx context.Context, userID uuid.UUID) ([]BankAccount, error) {
	rows, err := r.db.Pool.Query(ctx, `SELECT id, bank_name, account_type, balance FROM bank_accounts WHERE user_id = $1 AND unlinked_at IS NULL ORDER BY created_at`, userID)
	if err != nil {
		return nil, fmt.Errorf("query bank accounts: %w", err)
	}
	defer rows.Close()

	var accounts []BankAccount
	for rows.Next() {
		var acc BankAccount
		if err := rows.Scan(&acc.ID, &acc.BankName, &acc.AccountType, &acc.Balance); err != nil {
			return nil, fmt.Errorf("scan bank account: %w", err)
		}
		accounts = append(accounts, acc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate bank accounts: %w", err)
	}
	return accounts, nil
}

func (r *PostgresUserRepository) CreateRefreshToken(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error {
	if _, err := r.db.Pool.Exec(ctx, `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
	`, userID, tokenHash, expiresAt); err != nil {
		return fmt.Errorf("create refresh token: %w", err)
	}
	return nil
}

func (r *PostgresUserRepository) GetRefreshToken(ctx context.Context, tokenHash string) (*RefreshToken, error) {
	var rt RefreshToken
	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, user_id, expires_at, revoked_at FROM refresh_tokens WHERE token_hash = $1
	`, tokenHash).Scan(&rt.ID, &rt.UserID, &rt.ExpiresAt, &rt.RevokedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get refresh token: %w", err)
	}
	return &rt, nil
}

func (r *PostgresUserRepository) RevokeRefreshToken(ctx context.Context, tokenHash string) error {
	if _, err := r.db.Pool.Exec(ctx, `
		UPDATE refresh_tokens SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL
	`, tokenHash); err != nil {
		return fmt.Errorf("revoke refresh token: %w", err)
	}
	return nil
}

// RotateRefreshToken atomically revokes oldHash and inserts newHash as its
// replacement in a single DB transaction. Without the transaction, a caller
// doing "ConsumeRefreshToken, then separately CreateRefreshToken" could have
// the revoke succeed and the create fail (a dropped connection, a full disk,
// any transient DB error between the two calls) — leaving the user's old
// token burned and no new one issued, stranding them with no way back in
// except a full re-login. Wrapping both in one transaction means either the
// whole rotation lands or none of it does; the old token stays valid to
// retry against if the create step fails.
func (r *PostgresUserRepository) RotateRefreshToken(ctx context.Context, oldHash, newHash string, newExpiresAt time.Time) (uuid.UUID, bool, error) {
	tx, err := r.db.Pool.Begin(ctx)
	if err != nil {
		return uuid.UUID{}, false, fmt.Errorf("begin refresh rotation: %w", err)
	}
	defer tx.Rollback(ctx)

	var userID uuid.UUID
	err = tx.QueryRow(ctx, `
		UPDATE refresh_tokens
		SET revoked_at = now()
		WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now()
		RETURNING user_id
	`, oldHash).Scan(&userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.UUID{}, false, nil
		}
		return uuid.UUID{}, false, fmt.Errorf("consume refresh token: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
	`, userID, newHash, newExpiresAt); err != nil {
		return uuid.UUID{}, false, fmt.Errorf("create refresh token: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.UUID{}, false, fmt.Errorf("commit refresh rotation: %w", err)
	}
	return userID, true, nil
}

// ConsumeRefreshToken validates and revokes in a single atomic statement: the
// WHERE clause re-checks not-revoked/not-expired at the same instant the row
// is claimed, so of any number of concurrent callers passing the same
// tokenHash, exactly one gets ok=true (and the user ID), and the rest get
// ok=false immediately — no separate read-then-write window for two callers
// to both see "still valid" before either commits. Prefer RotateRefreshToken
// above for the rotate-on-refresh flow; this is kept for logout-style
// call sites that only need to revoke without issuing a replacement.
func (r *PostgresUserRepository) ConsumeRefreshToken(ctx context.Context, tokenHash string) (uuid.UUID, bool, error) {
	var userID uuid.UUID
	err := r.db.Pool.QueryRow(ctx, `
		UPDATE refresh_tokens
		SET revoked_at = now()
		WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now()
		RETURNING user_id
	`, tokenHash).Scan(&userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.UUID{}, false, nil
		}
		return uuid.UUID{}, false, fmt.Errorf("consume refresh token: %w", err)
	}
	return userID, true, nil
}

func (r *PostgresUserRepository) GetPrimaryBankAccount(ctx context.Context, userID uuid.UUID) (*BankAccount, error) {
	var acc BankAccount
	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, bank_name, account_type, balance
		FROM bank_accounts WHERE user_id = $1 AND unlinked_at IS NULL
		ORDER BY created_at LIMIT 1
	`, userID).Scan(&acc.ID, &acc.BankName, &acc.AccountType, &acc.Balance)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("user has no bank accounts: %w", err)
		}
		return nil, fmt.Errorf("query primary bank account: %w", err)
	}
	return &acc, nil
}
