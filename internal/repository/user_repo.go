package repository

import (
	"context"
	"errors"
	"fmt"
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

type UserRepository interface {
	FindOrCreateUser(ctx context.Context, astraUserID, phoneNumber, name string, uiBanks interface{}) (*User, error)
	GetBankAccounts(ctx context.Context, userID uuid.UUID) ([]BankAccount, error)
	GetPrimaryBankAccount(ctx context.Context, userID uuid.UUID) (*BankAccount, error)
}

type PostgresUserRepository struct {
	db *database.Database
}

func NewPostgresUserRepository(db *database.Database) *PostgresUserRepository {
	return &PostgresUserRepository{db: db}
}

// FindOrCreateUser looks a user up by phone number and returns them if found;
// otherwise it creates a new user (and seeds their initial bank accounts, if
// any were supplied). It never deletes existing data — every login before
// this fix wiped the entire users table via an unconditional DELETE, which
// destroyed all other users' data on every OTP verify.
func (r *PostgresUserRepository) FindOrCreateUser(ctx context.Context, astraUserID, phoneNumber, name string, uiBanks interface{}) (*User, error) {
	if existing, err := r.findByPhone(ctx, phoneNumber); err != nil {
		return nil, err
	} else if existing != nil {
		return existing, nil
	}

	var user User
	// ON CONFLICT guards against two concurrent first-logins for the same
	// phone number racing into duplicate rows; the unique constraint on
	// phone_number (see migration 000001) is what makes this atomic.
	err := r.db.Pool.QueryRow(ctx, `
		INSERT INTO users (id, astra_user_id, phone_number, name)
		VALUES (gen_random_uuid(), $1, $2, $3)
		ON CONFLICT (phone_number) DO UPDATE SET phone_number = EXCLUDED.phone_number
		RETURNING id, astra_user_id, phone_number, name, created_at
	`, astraUserID, phoneNumber, name).Scan(
		&user.ID,
		&user.AstraUserID,
		&user.PhoneNumber,
		&user.Name,
		&user.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	if err := r.seedBankAccounts(ctx, user.ID, uiBanks); err != nil {
		return nil, err
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

// seedBankAccounts only ever runs once, immediately after a brand-new user
// row is created, so it can never duplicate accounts on repeat logins.
func (r *PostgresUserRepository) seedBankAccounts(ctx context.Context, userID uuid.UUID, uiBanks interface{}) error {
	banks, ok := uiBanks.([]BankAccount)
	if ok && len(banks) > 0 {
		for _, b := range banks {
			if _, err := r.db.Pool.Exec(ctx, `
				INSERT INTO bank_accounts (user_id, bank_name, account_type, balance)
				VALUES ($1, $2, $3, $4)
			`, userID, b.BankName, b.AccountType, b.Balance); err != nil {
				return fmt.Errorf("seed bank account: %w", err)
			}
		}
		return nil
	}

	if _, err := r.db.Pool.Exec(ctx, `
		INSERT INTO bank_accounts (user_id, bank_name, account_type, balance)
		VALUES
		($1, 'HDFC Mutual Fund', 'Investment', 250000.00),
		($1, 'Zerodha Demat', 'Stocks', 120000.00)
	`, userID); err != nil {
		return fmt.Errorf("seed default bank accounts: %w", err)
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

// GetPrimaryBankAccount returns the user's first-created bank account. New
// users always have at least one, since seedBankAccounts runs at creation
// time; domains that need a bank_account_id to attach a payment/FD/mandate
// to (and have none specified by the caller) fall back to this one.
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
