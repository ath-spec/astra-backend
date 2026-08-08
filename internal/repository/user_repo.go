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
	FindOrCreateUser(ctx context.Context, astraUserID string, phoneNumber string) (*User, error)
}

type PostgresUserRepository struct {
	db *database.Database
}

func NewPostgresUserRepository(db *database.Database) *PostgresUserRepository {
	return &PostgresUserRepository{db: db}
}

func (r *PostgresUserRepository) FindOrCreateUser(ctx context.Context, astraUserID string, phoneNumber string) (*User, error) {
	// First, try to find the user
	query := `SELECT id, astra_user_id, name, phone_number, pan_number, created_at FROM users WHERE astra_user_id = $1 OR phone_number = $2 LIMIT 1`
	
	row := r.db.Pool.QueryRow(ctx, query, astraUserID, phoneNumber)
	
	var user User
	err := row.Scan(&user.ID, &user.AstraUserID, &user.Name, &user.PhoneNumber, &user.PanNumber, &user.CreatedAt)
	
	if err == nil {
		return &user, nil // Found existing user
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
