package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

type UserRepository interface {
	FindOrCreateUser(ctx context.Context, astraUserID, phoneNumber, name string) (*User, error)
}

type PostgresUserRepository struct {
	db *database.Database
}

func NewPostgresUserRepository(db *database.Database) *PostgresUserRepository {
	return &PostgresUserRepository{db: db}
}

func (r *PostgresUserRepository) FindOrCreateUser(ctx context.Context, astraUserID, phoneNumber, name string) (*User, error) {
	// 1. Hackathon "Fresh Start": Delete any existing user with this phone number
	// This will cascade and delete all their old chats and bank accounts.
	_, err := r.db.Pool.Exec(ctx, `DELETE FROM users WHERE phone_number = $1`, phoneNumber)
	if err != nil {
		return nil, err
	}

	var user User
	}
	
	if err != pgx.ErrNoRows {
		return nil, fmt.Errorf("error querying user: %w", err)
	}

	// User not found, create them
	user.ID = uuid.New()
	user.AstraUserID = astraUserID
	user.PhoneNumber = phoneNumber


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

	return &user, nil
}
