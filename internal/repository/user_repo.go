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

	insertQuery := `
		INSERT INTO users (id, astra_user_id, phone_number)
		VALUES ($1, $2, $3)
		RETURNING created_at
	`
	
	err = r.db.Pool.QueryRow(ctx, insertQuery, user.ID, user.AstraUserID, user.PhoneNumber).Scan(&user.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("error creating user: %w", err)
	}

	return &user, nil
}
