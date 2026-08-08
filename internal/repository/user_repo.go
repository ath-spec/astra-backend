package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/yourusername/astra-backend/internal/database"
)

type User struct {
	ID           uuid.UUID
	AstraUserID  string
	Name         *string
	PhoneNumber  string
	PanNumber    *string
	CreatedAt    time.Time
}

type BankAccount struct {
	BankName    string
	AccountType string
	Balance     float64
}

type UserRepository interface {
	FindOrCreateUser(ctx context.Context, astraUserID, phoneNumber, name string, uiBanks interface{}) (*User, error)
	GetBankAccounts(ctx context.Context, userID uuid.UUID) ([]BankAccount, error)
}

type PostgresUserRepository struct {
	db *database.Database
}

func NewPostgresUserRepository(db *database.Database) *PostgresUserRepository {
	return &PostgresUserRepository{db: db}
}

func (r *PostgresUserRepository) FindOrCreateUser(ctx context.Context, astraUserID, phoneNumber, name string, uiBanks interface{}) (*User, error) {
	// 1. Hackathon "Fresh Start": Delete any existing user with this phone number
	// This will cascade and delete all their old chats and bank accounts.
	_, err := r.db.Pool.Exec(ctx, `DELETE FROM users WHERE phone_number = $1`, phoneNumber)
	if err != nil {
		return nil, err
	}

	var user User

	query := `
		INSERT INTO users (id, astra_user_id, phone_number, name)
		VALUES (gen_random_uuid(), $1, $2, $3)
		RETURNING id, astra_user_id, phone_number, name, created_at
	`
	
	err = r.db.Pool.QueryRow(ctx, query, astraUserID, phoneNumber, name).Scan(
		&user.ID,
		&user.AstraUserID,
		&user.PhoneNumber,
		&user.Name,
		&user.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("error creating user: %w", err)
	}

	// 3. Inject Dummy Chat History for the Demo!
	dummyMessages := `[
		{"role": "user", "content": "Can you analyze my current investment portfolio based on my linked accounts?"},
		{"role": "assistant", "content": "I have analyzed your connected accounts. You currently hold ₹2,50,000 in Mutual Funds (primarily large-cap equity) and ₹1,20,000 in direct stocks. Your portfolio is somewhat conservative for your age. Given the current market rally, I recommend rebalancing 15% of your debt exposure into mid-cap index funds to optimize your long-term returns. Would you like a detailed breakdown of which funds to consider?"}
	]`
	
	_, _ = r.db.Pool.Exec(ctx, `
		INSERT INTO chat_sessions (id, user_id, messages) 
		VALUES (gen_random_uuid(), $1, $2::jsonb)
	`, user.ID, dummyMessages)

	// 4. Inject Dynamic Bank Accounts from UI!
	// Type assert the slice of structs passed from handler
	// In Go, since we pass from another package with a different struct, we can marshal/unmarshal or use a common struct.
	// Actually, wait, auth.go defines BankAccount, and user_repo defines BankAccount.
	// To avoid circular deps, we can just pass []BankAccount from user_repo directly into the handler.
	// Let's assume uiBanks is []BankAccount (the one from user_repo).
	banks, ok := uiBanks.([]BankAccount)
	if ok && len(banks) > 0 {
		for _, b := range banks {
			_, _ = r.db.Pool.Exec(ctx, `
				INSERT INTO bank_accounts (user_id, bank_name, account_type, balance)
				VALUES ($1, $2, $3, $4)
			`, user.ID, b.BankName, b.AccountType, b.Balance)
		}
	} else {
		// Fallback if none provided
		_, _ = r.db.Pool.Exec(ctx, `
			INSERT INTO bank_accounts (user_id, bank_name, account_type, balance)
			VALUES 
			($1, 'HDFC Mutual Fund', 'Investment', 250000.00),
			($1, 'Zerodha Demat', 'Stocks', 120000.00)
		`, user.ID)
	}

	return &user, nil
}

func (r *PostgresUserRepository) GetBankAccounts(ctx context.Context, userID uuid.UUID) ([]BankAccount, error) {
	query := `SELECT bank_name, account_type, balance FROM bank_accounts WHERE user_id = $1`
	rows, err := r.db.Pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var accounts []BankAccount
	for rows.Next() {
		var acc BankAccount
		if err := rows.Scan(&acc.BankName, &acc.AccountType, &acc.Balance); err != nil {
			return nil, err
		}
		accounts = append(accounts, acc)
	}
	return accounts, nil
}
