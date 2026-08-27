package repository

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
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
	CreatedAt   time.Time
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
	FindOrCreateUser(ctx context.Context, astraUserID, phoneNumber, name string, uiBanks interface{}) (user *User, isNew bool, err error)
	GetByID(ctx context.Context, userID uuid.UUID) (*User, error)
	GetBankAccounts(ctx context.Context, userID uuid.UUID) ([]BankAccount, error)
	GetPrimaryBankAccount(ctx context.Context, userID uuid.UUID) (*BankAccount, error)

	CreateRefreshToken(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error
	GetRefreshToken(ctx context.Context, tokenHash string) (*RefreshToken, error)
	RevokeRefreshToken(ctx context.Context, tokenHash string) error
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

func (r *PostgresUserRepository) FindOrCreateUser(ctx context.Context, astraUserID, phoneNumber, name string, uiBanks interface{}) (*User, bool, error) {
	if existing, err := r.findByPhone(ctx, phoneNumber); err != nil {
		return nil, false, err
	} else if existing != nil {
		return existing, false, nil
	}

	var user User
	var isNew bool
	err := r.db.Pool.QueryRow(ctx, `
		INSERT INTO users (id, astra_user_id, phone_number, name)
		VALUES (gen_random_uuid(), $1, $2, $3)
		ON CONFLICT (phone_number) DO UPDATE SET phone_number = EXCLUDED.phone_number
		RETURNING id, astra_user_id, phone_number, name, created_at, (xmax = 0) AS is_new
	`, astraUserID, phoneNumber, name).Scan(
		&user.ID,
		&user.AstraUserID,
		&user.PhoneNumber,
		&user.Name,
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
	// active queue. Best-effort: a failure here (no active RMs, transient
	// DB error) must never block signup — the user is created either way
	// and shows up in the admin console's unassigned pool.
	if r.assigner != nil {
		if _, err := r.assigner.AssignNextRM(ctx, user.ID); err != nil && !errors.Is(err, ErrNoActiveRM) {
			fmt.Printf("user_repo: auto-assign RM for user %s failed: %v\n", user.ID, err)
		}
	}

	return &user, true, nil
}

func (r *PostgresUserRepository) GetByID(ctx context.Context, userID uuid.UUID) (*User, error) {
	var user User
	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, astra_user_id, phone_number, name, created_at
		FROM users WHERE id = $1
	`, userID).Scan(&user.ID, &user.AstraUserID, &user.PhoneNumber, &user.Name, &user.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("user %s not found: %w", userID, pgx.ErrNoRows)
		}
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return &user, nil
}

func (r *PostgresUserRepository) findByPhone(ctx context.Context, phoneNumber string) (*User, error) {
	var user User
	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, astra_user_id, phone_number, name, created_at
		FROM users WHERE phone_number = $1
	`, phoneNumber).Scan(&user.ID, &user.AstraUserID, &user.PhoneNumber, &user.Name, &user.CreatedAt)

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

// seedInitialUserData selects from 4 distinct, rich investor archetypes based on phone number hash,
// ensuring different users experience varied portfolio distributions, funds, balances, and goals.
func (r *PostgresUserRepository) seedInitialUserData(ctx context.Context, userID uuid.UUID, phoneNumber string) error {
	sum := sha256.Sum256([]byte(phoneNumber + userID.String()))
	archetype := int(binary.BigEndian.Uint32(sum[:4]) % 4)

	switch archetype {
	case 0:
		return r.seedTechGrowthArchetype(ctx, userID)
	case 1:
		return r.seedBalancedWealthArchetype(ctx, userID)
	case 2:
		return r.seedGlobalMultiAssetArchetype(ctx, userID)
	default:
		return r.seedConservativeIncomeArchetype(ctx, userID)
	}
}

// Archetype 0: Tech & Semiconductor Growth Investor
func (r *PostgresUserRepository) seedTechGrowthArchetype(ctx context.Context, userID uuid.UUID) error {
	var primaryBankID uuid.UUID
	err := r.db.Pool.QueryRow(ctx, `
		INSERT INTO bank_accounts (user_id, bank_name, account_type, balance)
		VALUES ($1, 'ICICI Bank - Wealth', 'SAVINGS', 345000.00)
		RETURNING id
	`, userID).Scan(&primaryBankID)
	if err != nil {
		return err
	}

	_, _ = r.db.Pool.Exec(ctx, `
		INSERT INTO bank_accounts (user_id, bank_name, account_type, balance)
		VALUES ($1, 'HDFC Bank - Savings', 'SAVINGS', 185000.00)
		ON CONFLICT DO NOTHING
	`, userID)

	_, _ = r.db.Pool.Exec(ctx, `
		INSERT INTO demat_holdings (user_id, isin, trading_symbol, exchange, product, quantity, average_price, last_price, close_price, authorized_date)
		VALUES
		($1, 'INE249Z01012', 'MAZDOCK', 'NSE', 'CNC', 25, 2100.00, 2340.50, 2305.00, CURRENT_DATE - 75),
		($1, 'INE704P01017', 'COCHINSHIP', 'NSE', 'CNC', 40, 1380.00, 1510.00, 1480.00, CURRENT_DATE - 45)
		ON CONFLICT DO NOTHING
	`, userID)

	_, _ = r.db.Pool.Exec(ctx, `
		INSERT INTO stock_orders (order_id, user_id, exchange, trading_symbol, isin, transaction_type, quantity, product, order_type, price, status, filled_quantity, average_price, order_timestamp)
		VALUES
		('ORD-TECH-01', $1, 'NSE', 'MAZDOCK', 'INE249Z01012', 'BUY', 25, 'CNC', 'LIMIT', 2100.00, 'COMPLETE', 25, 2100.00, NOW() - INTERVAL '75 days'),
		('ORD-TECH-02', $1, 'NSE', 'COCHINSHIP', 'INE704P01017', 'BUY', 40, 'CNC', 'LIMIT', 1380.00, 'COMPLETE', 40, 1380.00, NOW() - INTERVAL '45 days')
		ON CONFLICT DO NOTHING
	`, userID)

	var f1, f2, f3 uuid.UUID
	_ = r.db.Pool.QueryRow(ctx, `
		INSERT INTO mf_folios (user_id, folio_number, amc_name, scheme_code, scheme_name, isin, units_held, nav, nav_date, cost_value, category, plan_type)
		VALUES ($1, 'FOL-TECH-01', 'Mirae Asset Mutual Fund', 'MIRAE-SEMICON-G', 'Mirae Asset Semiconductor & AI ETF Fund of Fund - Growth', 'INF769K01CX6', 520.000, 18.9034, CURRENT_DATE, 8000.00, 'Equity - Thematic', 'DIRECT')
		RETURNING id
	`, userID).Scan(&f1)

	_ = r.db.Pool.QueryRow(ctx, `
		INSERT INTO mf_folios (user_id, folio_number, amc_name, scheme_code, scheme_name, isin, units_held, nav, nav_date, cost_value, category, plan_type)
		VALUES ($1, 'FOL-TECH-02', 'ICICI Prudential Mutual Fund', 'ICICI-TECH-G', 'ICICI Prudential Technology Fund - Growth', 'INF109K01VG1', 80.000, 198.4521, CURRENT_DATE, 13500.00, 'Equity - Thematic', 'DIRECT')
		RETURNING id
	`, userID).Scan(&f2)

	_ = r.db.Pool.QueryRow(ctx, `
		INSERT INTO mf_folios (user_id, folio_number, amc_name, scheme_code, scheme_name, isin, units_held, nav, nav_date, cost_value, category, plan_type)
		VALUES ($1, 'FOL-TECH-03', 'Axis Mutual Fund', 'AXIS-SC-G', 'Axis Small Cap Fund - Growth', 'INF846K01EY7', 160.000, 95.1032, CURRENT_DATE, 12000.00, 'Equity - Small Cap', 'DIRECT')
		RETURNING id
	`, userID).Scan(&f3)

	for i := 0; i < 9; i++ {
		tDate := time.Now().AddDate(0, -i, -7)
		if f1 != uuid.Nil {
			_, _ = r.db.Pool.Exec(ctx, `INSERT INTO mf_transactions (folio_id, transaction_type, transaction_date, amount, units, price) VALUES ($1, 'SIP_PURCHASE', $2, 6000.00, 317.40, 18.90)`, f1, tDate)
		}
		if f2 != uuid.Nil {
			_, _ = r.db.Pool.Exec(ctx, `INSERT INTO mf_transactions (folio_id, transaction_type, transaction_date, amount, units, price) VALUES ($1, 'SIP_PURCHASE', $2, 4000.00, 20.15, 198.45)`, f2, tDate)
		}
	}

	_, _ = r.db.Pool.Exec(ctx, `
		INSERT INTO fd_accounts (fd_account_number, user_id, bank_account_id, principal_amount, interest_rate, tenure_months, interest_payout, auto_renewal, nominee_name, booking_date, maturity_date, maturity_amount, status)
		VALUES ('FD-TECH-901', $1, $2, 75000.00, 7.25, 18, 'ON_MATURITY', true, 'Self', CURRENT_DATE - 40, CURRENT_DATE + 505, 83450.00, 'ACTIVE')
		ON CONFLICT DO NOTHING
	`, userID, primaryBankID)

	_, _ = r.db.Pool.Exec(ctx, `
		INSERT INTO mandates (mandate_id, user_id, bank_account_id, mandate_type, upi_id, payee_name, payee_vpa_or_id, max_amount, frequency, mandate_start_date, next_debit_date, status)
		VALUES
		('MND-TECH-01', $1, $2, 'UPI_AUTOPAY', 'user@okicici', 'Mirae AI & Tech SIP', 'mirae@upi', 6000.00, 'MONTHLY', CURRENT_DATE - 270, CURRENT_DATE + 5, 'ACTIVE'),
		('MND-TECH-02', $1, $2, 'UPI_AUTOPAY', 'user@okicici', 'ICICI Tech Fund SIP', 'icici@upi', 4000.00, 'MONTHLY', CURRENT_DATE - 180, CURRENT_DATE + 12, 'ACTIVE')
		ON CONFLICT DO NOTHING
	`, userID, primaryBankID)

	_, _ = r.db.Pool.Exec(ctx, `
		INSERT INTO goals (user_id, title, category, target_amount, current_amount, target_date, status)
		VALUES
		($1, 'AI Venture Angel Fund', 'INVESTMENT', 5000000.00, 1850000.00, CURRENT_DATE + 1460, 'IN_PROGRESS'),
		($1, 'EV Car Upgrade', 'PURCHASE', 2500000.00, 850000.00, CURRENT_DATE + 730, 'IN_PROGRESS')
		ON CONFLICT DO NOTHING
	`, userID)

	// Seed 365 days of portfolio history so the growth chart has data from day 1.
	return r.seedPortfolioSnapshots(ctx, userID, 365, 2850000.0, 3950000.0)
}

// Archetype 1: Balanced Bluechip & Flexicap Wealth Builder
func (r *PostgresUserRepository) seedBalancedWealthArchetype(ctx context.Context, userID uuid.UUID) error {
	var primaryBankID uuid.UUID
	err := r.db.Pool.QueryRow(ctx, `
		INSERT INTO bank_accounts (user_id, bank_name, account_type, balance)
		VALUES ($1, 'HDFC Bank - Salary', 'SAVINGS', 265000.00)
		RETURNING id
	`, userID).Scan(&primaryBankID)
	if err != nil {
		return err
	}

	_, _ = r.db.Pool.Exec(ctx, `
		INSERT INTO bank_accounts (user_id, bank_name, account_type, balance)
		VALUES ($1, 'State Bank of India - Savings', 'SAVINGS', 95000.00)
		ON CONFLICT DO NOTHING
	`, userID)

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
		('ORD-BAL-01', $1, 'NSE', 'MSTCLTD', 'INE255X01014', 'BUY', 75, 'CNC', 'LIMIT', 670.00, 'COMPLETE', 75, 670.00, NOW() - INTERVAL '60 days'),
		('ORD-BAL-02', $1, 'NSE', 'COCHINSHIP', 'INE704P01017', 'BUY', 18, 'CNC', 'LIMIT', 1440.00, 'COMPLETE', 18, 1440.00, NOW() - INTERVAL '30 days')
		ON CONFLICT DO NOTHING
	`, userID)

	var f1, f2, f3 uuid.UUID
	_ = r.db.Pool.QueryRow(ctx, `
		INSERT INTO mf_folios (user_id, folio_number, amc_name, scheme_code, scheme_name, isin, units_held, nav, nav_date, cost_value, category, plan_type)
		VALUES ($1, 'FOL-BAL-01', 'PPFAS Mutual Fund', 'PARAG-FLX-G', 'Parag Parikh Flexi Cap Fund - Growth', 'INF879O01027', 350.000, 74.9012, CURRENT_DATE, 22000.00, 'Equity - Flexi Cap', 'DIRECT')
		RETURNING id
	`, userID).Scan(&f1)

	_ = r.db.Pool.QueryRow(ctx, `
		INSERT INTO mf_folios (user_id, folio_number, amc_name, scheme_code, scheme_name, isin, units_held, nav, nav_date, cost_value, category, plan_type)
		VALUES ($1, 'FOL-BAL-02', 'SBI Mutual Fund', 'SBI-BLC-G', 'SBI Bluechip Fund - Growth', 'INF200K01158', 210.000, 78.4521, CURRENT_DATE, 14000.00, 'Equity - Large Cap', 'DIRECT')
		RETURNING id
	`, userID).Scan(&f2)

	_ = r.db.Pool.QueryRow(ctx, `
		INSERT INTO mf_folios (user_id, folio_number, amc_name, scheme_code, scheme_name, isin, units_held, nav, nav_date, cost_value, category, plan_type)
		VALUES ($1, 'FOL-BAL-03', 'ICICI Prudential Mutual Fund', 'ICICI-BAF-G', 'ICICI Prudential Balanced Advantage Fund - Growth', 'INF109K01AA1', 280.000, 61.2290, CURRENT_DATE, 15000.00, 'Hybrid - Balanced Advantage', 'DIRECT')
		RETURNING id
	`, userID).Scan(&f3)

	for i := 0; i < 8; i++ {
		tDate := time.Now().AddDate(0, -i, -5)
		if f1 != uuid.Nil {
			_, _ = r.db.Pool.Exec(ctx, `INSERT INTO mf_transactions (folio_id, transaction_type, transaction_date, amount, units, price) VALUES ($1, 'SIP_PURCHASE', $2, 5000.00, 66.75, 74.90)`, f1, tDate)
		}
		if f2 != uuid.Nil {
			_, _ = r.db.Pool.Exec(ctx, `INSERT INTO mf_transactions (folio_id, transaction_type, transaction_date, amount, units, price) VALUES ($1, 'SIP_PURCHASE', $2, 3000.00, 38.24, 78.45)`, f2, tDate)
		}
	}

	_, _ = r.db.Pool.Exec(ctx, `
		INSERT INTO fd_accounts (fd_account_number, user_id, bank_account_id, principal_amount, interest_rate, tenure_months, interest_payout, auto_renewal, nominee_name, booking_date, maturity_date, maturity_amount, status)
		VALUES ('FD-BAL-201', $1, $2, 50000.00, 7.10, 12, 'ON_MATURITY', true, 'Self', CURRENT_DATE - 60, CURRENT_DATE + 305, 53645.00, 'ACTIVE')
		ON CONFLICT DO NOTHING
	`, userID, primaryBankID)

	_, _ = r.db.Pool.Exec(ctx, `
		INSERT INTO mandates (mandate_id, user_id, bank_account_id, mandate_type, upi_id, payee_name, payee_vpa_or_id, max_amount, frequency, mandate_start_date, next_debit_date, status)
		VALUES
		('MND-BAL-01', $1, $2, 'UPI_AUTOPAY', 'user@okhdfc', 'Parag Parikh Flexi Cap SIP', 'ppfas@upi', 5000.00, 'MONTHLY', CURRENT_DATE - 240, CURRENT_DATE + 10, 'ACTIVE'),
		('MND-BAL-02', $1, $2, 'UPI_AUTOPAY', 'user@okhdfc', 'SBI Bluechip SIP', 'sbi@upi', 3000.00, 'MONTHLY', CURRENT_DATE - 120, CURRENT_DATE + 15, 'ACTIVE')
		ON CONFLICT DO NOTHING
	`, userID, primaryBankID)

	_, _ = r.db.Pool.Exec(ctx, `
		INSERT INTO goals (user_id, title, category, target_amount, current_amount, target_date, status)
		VALUES
		($1, 'Home Down Payment', 'HOME', 3500000.00, 1250000.00, CURRENT_DATE + 1095, 'IN_PROGRESS'),
		($1, 'Emergency Fund', 'EMERGENCY', 500000.00, 320000.00, CURRENT_DATE + 365, 'IN_PROGRESS')
		ON CONFLICT DO NOTHING
	`, userID)

	// Seed 180 days of portfolio history.
	return r.seedPortfolioSnapshots(ctx, userID, 180, 1650000.0, 2480000.0)
}

// Archetype 2: Global Markets, Gold & REITs Diversifier
func (r *PostgresUserRepository) seedGlobalMultiAssetArchetype(ctx context.Context, userID uuid.UUID) error {
	var primaryBankID uuid.UUID
	err := r.db.Pool.QueryRow(ctx, `
		INSERT INTO bank_accounts (user_id, bank_name, account_type, balance)
		VALUES ($1, 'Axis Bank - Priority', 'SAVINGS', 390000.00)
		RETURNING id
	`, userID).Scan(&primaryBankID)
	if err != nil {
		return err
	}

	_, _ = r.db.Pool.Exec(ctx, `
		INSERT INTO bank_accounts (user_id, bank_name, account_type, balance)
		VALUES ($1, 'Bank of Baroda - Savings', 'SAVINGS', 140000.00)
		ON CONFLICT DO NOTHING
	`, userID)

	_, _ = r.db.Pool.Exec(ctx, `
		INSERT INTO demat_holdings (user_id, isin, trading_symbol, exchange, product, quantity, average_price, last_price, close_price, authorized_date)
		VALUES
		($1, 'INE249Z01012', 'MAZDOCK', 'NSE', 'CNC', 12, 2180.00, 2340.50, 2305.00, CURRENT_DATE - 100),
		($1, 'INE255X01014', 'MSTCLTD', 'NSE', 'CNC', 90, 690.00, 745.00, 730.00, CURRENT_DATE - 40)
		ON CONFLICT DO NOTHING
	`, userID)

	_, _ = r.db.Pool.Exec(ctx, `
		INSERT INTO stock_orders (order_id, user_id, exchange, trading_symbol, isin, transaction_type, quantity, product, order_type, price, status, filled_quantity, average_price, order_timestamp)
		VALUES
		('ORD-GLOB-01', $1, 'NSE', 'MAZDOCK', 'INE249Z01012', 'BUY', 12, 'CNC', 'LIMIT', 2180.00, 'COMPLETE', 12, 2180.00, NOW() - INTERVAL '100 days'),
		('ORD-GLOB-02', $1, 'NSE', 'MSTCLTD', 'INE255X01014', 'BUY', 90, 'CNC', 'LIMIT', 690.00, 'COMPLETE', 90, 690.00, NOW() - INTERVAL '40 days')
		ON CONFLICT DO NOTHING
	`, userID)

	var f1, f2, f3, f4 uuid.UUID
	_ = r.db.Pool.QueryRow(ctx, `
		INSERT INTO mf_folios (user_id, folio_number, amc_name, scheme_code, scheme_name, isin, units_held, nav, nav_date, cost_value, category, plan_type)
		VALUES ($1, 'FOL-GLOB-01', 'Motilal Oswal Mutual Fund', 'MOTILAL-NASDAQ100-G', 'Motilal Oswal Nasdaq 100 FOF - Growth', 'INF247L01AQ2', 480.000, 28.4521, CURRENT_DATE, 11500.00, 'Equity - Global', 'DIRECT')
		RETURNING id
	`, userID).Scan(&f1)

	_ = r.db.Pool.QueryRow(ctx, `
		INSERT INTO mf_folios (user_id, folio_number, amc_name, scheme_code, scheme_name, isin, units_held, nav, nav_date, cost_value, category, plan_type)
		VALUES ($1, 'FOL-GLOB-02', 'Kotak Mutual Fund', 'KOTAK-GOLD-G', 'Kotak Gold Fund - Growth', 'INF174K01LS3', 420.000, 28.3345, CURRENT_DATE, 10000.00, 'Other - Gold', 'DIRECT')
		RETURNING id
	`, userID).Scan(&f2)

	_ = r.db.Pool.QueryRow(ctx, `
		INSERT INTO mf_folios (user_id, folio_number, amc_name, scheme_code, scheme_name, isin, units_held, nav, nav_date, cost_value, category, plan_type)
		VALUES ($1, 'FOL-GLOB-03', 'Kotak Mutual Fund', 'KOTAK-REIT-G', 'Kotak International REIT Fund of Fund - Growth', 'INF174KA1RT3', 650.000, 12.3456, CURRENT_DATE, 7500.00, 'Other - REIT', 'DIRECT')
		RETURNING id
	`, userID).Scan(&f3)

	_ = r.db.Pool.QueryRow(ctx, `
		INSERT INTO mf_folios (user_id, folio_number, amc_name, scheme_code, scheme_name, isin, units_held, nav, nav_date, cost_value, category, plan_type)
		VALUES ($1, 'FOL-GLOB-04', 'Edelweiss Mutual Fund', 'EDEL-EUROPE-G', 'Edelweiss Europe Dynamic Equity Offshore Fund - Growth', 'INF754K01DZ9', 250.000, 22.7789, CURRENT_DATE, 5200.00, 'Equity - Global', 'DIRECT')
		RETURNING id
	`, userID).Scan(&f4)

	for i := 0; i < 7; i++ {
		tDate := time.Now().AddDate(0, -i, -3)
		if f1 != uuid.Nil {
			_, _ = r.db.Pool.Exec(ctx, `INSERT INTO mf_transactions (folio_id, transaction_type, transaction_date, amount, units, price) VALUES ($1, 'SIP_PURCHASE', $2, 7500.00, 263.60, 28.45)`, f1, tDate)
		}
		if f2 != uuid.Nil {
			_, _ = r.db.Pool.Exec(ctx, `INSERT INTO mf_transactions (folio_id, transaction_type, transaction_date, amount, units, price) VALUES ($1, 'SIP_PURCHASE', $2, 2500.00, 88.23, 28.33)`, f2, tDate)
		}
	}

	_, _ = r.db.Pool.Exec(ctx, `
		INSERT INTO fd_accounts (fd_account_number, user_id, bank_account_id, principal_amount, interest_rate, tenure_months, interest_payout, auto_renewal, nominee_name, booking_date, maturity_date, maturity_amount, status)
		VALUES ('FD-GLOB-301', $1, $2, 60000.00, 7.40, 24, 'ON_MATURITY', true, 'Self', CURRENT_DATE - 120, CURRENT_DATE + 610, 69450.00, 'ACTIVE')
		ON CONFLICT DO NOTHING
	`, userID, primaryBankID)

	_, _ = r.db.Pool.Exec(ctx, `
		INSERT INTO mandates (mandate_id, user_id, bank_account_id, mandate_type, upi_id, payee_name, payee_vpa_or_id, max_amount, frequency, mandate_start_date, next_debit_date, status)
		VALUES
		('MND-GLOB-01', $1, $2, 'UPI_AUTOPAY', 'user@okaxis', 'Nasdaq 100 Index SIP', 'motilal@upi', 7500.00, 'MONTHLY', CURRENT_DATE - 210, CURRENT_DATE + 3, 'ACTIVE'),
		('MND-GLOB-02', $1, $2, 'UPI_AUTOPAY', 'user@okaxis', 'Kotak Gold SIP', 'kotak@upi', 2500.00, 'MONTHLY', CURRENT_DATE - 150, CURRENT_DATE + 18, 'ACTIVE')
		ON CONFLICT DO NOTHING
	`, userID, primaryBankID)

	_, _ = r.db.Pool.Exec(ctx, `
		INSERT INTO goals (user_id, title, category, target_amount, current_amount, target_date, status)
		VALUES
		($1, 'European Sabbatical', 'TRAVEL', 800000.00, 450000.00, CURRENT_DATE + 540, 'IN_PROGRESS'),
		($1, 'Global Portfolio Corpus', 'WEALTH', 8000000.00, 2200000.00, CURRENT_DATE + 2190, 'IN_PROGRESS')
		ON CONFLICT DO NOTHING
	`, userID)

	// Seed 90 days of portfolio history.
	return r.seedPortfolioSnapshots(ctx, userID, 90, 2100000.0, 2650000.0)
}

// Archetype 3: Conservative Hybrid & Capital Preservation Planner
func (r *PostgresUserRepository) seedConservativeIncomeArchetype(ctx context.Context, userID uuid.UUID) error {
	var primaryBankID uuid.UUID
	err := r.db.Pool.QueryRow(ctx, `
		INSERT INTO bank_accounts (user_id, bank_name, account_type, balance)
		VALUES ($1, 'State Bank of India - Savings', 'SAVINGS', 480000.00)
		RETURNING id
	`, userID).Scan(&primaryBankID)
	if err != nil {
		return err
	}

	_, _ = r.db.Pool.Exec(ctx, `
		INSERT INTO demat_holdings (user_id, isin, trading_symbol, exchange, product, quantity, average_price, last_price, close_price, authorized_date)
		VALUES
		($1, 'INE249Z01012', 'MAZDOCK', 'NSE', 'CNC', 8, 2140.00, 2340.50, 2305.00, CURRENT_DATE - 120)
		ON CONFLICT DO NOTHING
	`, userID)

	_, _ = r.db.Pool.Exec(ctx, `
		INSERT INTO stock_orders (order_id, user_id, exchange, trading_symbol, isin, transaction_type, quantity, product, order_type, price, status, filled_quantity, average_price, order_timestamp)
		VALUES
		('ORD-CONS-01', $1, 'NSE', 'MAZDOCK', 'INE249Z01012', 'BUY', 8, 'CNC', 'LIMIT', 2140.00, 'COMPLETE', 8, 2140.00, NOW() - INTERVAL '120 days')
		ON CONFLICT DO NOTHING
	`, userID)

	var f1, f2, f3 uuid.UUID
	_ = r.db.Pool.QueryRow(ctx, `
		INSERT INTO mf_folios (user_id, folio_number, amc_name, scheme_code, scheme_name, isin, units_held, nav, nav_date, cost_value, category, plan_type)
		VALUES ($1, 'FOL-CONS-01', 'HDFC Mutual Fund', 'HDFC-CORPBOND-G', 'HDFC Corporate Bond Fund - Growth', 'INF179K01BM3', 550.000, 32.5601, CURRENT_DATE, 16500.00, 'Debt - Corporate Bond', 'DIRECT')
		RETURNING id
	`, userID).Scan(&f1)

	_ = r.db.Pool.QueryRow(ctx, `
		INSERT INTO mf_folios (user_id, folio_number, amc_name, scheme_code, scheme_name, isin, units_held, nav, nav_date, cost_value, category, plan_type)
		VALUES ($1, 'FOL-CONS-02', 'SBI Mutual Fund', 'SBI-CONSHYBRID-G', 'SBI Conservative Hybrid Fund - Growth', 'INF200K01582', 290.000, 68.3345, CURRENT_DATE, 18000.00, 'Hybrid - Conservative', 'DIRECT')
		RETURNING id
	`, userID).Scan(&f2)

	_ = r.db.Pool.QueryRow(ctx, `
		INSERT INTO mf_folios (user_id, folio_number, amc_name, scheme_code, scheme_name, isin, units_held, nav, nav_date, cost_value, category, plan_type)
		VALUES ($1, 'FOL-CONS-03', 'HDFC Mutual Fund', 'HDFC-LIQ-G', 'HDFC Liquid Fund - Growth', 'INF179K01158', 8.000, 4521.6634, CURRENT_DATE, 32000.00, 'Debt - Liquid', 'DIRECT')
		RETURNING id
	`, userID).Scan(&f3)

	for i := 0; i < 6; i++ {
		tDate := time.Now().AddDate(0, -i, -10)
		if f1 != uuid.Nil {
			_, _ = r.db.Pool.Exec(ctx, `INSERT INTO mf_transactions (folio_id, transaction_type, transaction_date, amount, units, price) VALUES ($1, 'SIP_PURCHASE', $2, 5000.00, 153.56, 32.56)`, f1, tDate)
		}
	}

	_, _ = r.db.Pool.Exec(ctx, `
		INSERT INTO fd_accounts (fd_account_number, user_id, bank_account_id, principal_amount, interest_rate, tenure_months, interest_payout, auto_renewal, nominee_name, booking_date, maturity_date, maturity_amount, status)
		VALUES
		('FD-CONS-401', $1, $2, 120000.00, 7.50, 36, 'ON_MATURITY', true, 'Self', CURRENT_DATE - 90, CURRENT_DATE + 1005, 149850.00, 'ACTIVE'),
		('FD-CONS-402', $1, $2, 80000.00, 7.10, 12, 'ON_MATURITY', true, 'Self', CURRENT_DATE - 30, CURRENT_DATE + 335, 85830.00, 'ACTIVE')
		ON CONFLICT DO NOTHING
	`, userID, primaryBankID)

	_, _ = r.db.Pool.Exec(ctx, `
		INSERT INTO mandates (mandate_id, user_id, bank_account_id, mandate_type, upi_id, payee_name, payee_vpa_or_id, max_amount, frequency, mandate_start_date, next_debit_date, status)
		VALUES
		('MND-CONS-01', $1, $2, 'UPI_AUTOPAY', 'user@oksbi', 'HDFC Corporate Bond SIP', 'hdfc@upi', 5000.00, 'MONTHLY', CURRENT_DATE - 180, CURRENT_DATE + 20, 'ACTIVE')
		ON CONFLICT DO NOTHING
	`, userID, primaryBankID)

	_, _ = r.db.Pool.Exec(ctx, `
		INSERT INTO goals (user_id, title, category, target_amount, current_amount, target_date, status)
		VALUES
		($1, 'Child Higher Education', 'EDUCATION', 4000000.00, 2100000.00, CURRENT_DATE + 1825, 'IN_PROGRESS'),
		($1, 'Retirement Safety Net', 'RETIREMENT', 12000000.00, 4800000.00, CURRENT_DATE + 5475, 'IN_PROGRESS')
		ON CONFLICT DO NOTHING
	`, userID)

	// Seed 30 days of portfolio history.
	return r.seedPortfolioSnapshots(ctx, userID, 30, 3400000.0, 3720000.0)
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
	rows, err := r.db.Pool.Query(ctx, `SELECT id, bank_name, account_type, balance FROM bank_accounts WHERE user_id = $1 ORDER BY created_at`, userID)
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

func (r *PostgresUserRepository) GetPrimaryBankAccount(ctx context.Context, userID uuid.UUID) (*BankAccount, error) {
	var acc BankAccount
	err := r.db.Pool.QueryRow(ctx, `
		SELECT id, bank_name, account_type, balance
		FROM bank_accounts WHERE user_id = $1
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
