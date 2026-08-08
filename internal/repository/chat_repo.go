package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/yourusername/astra-backend/internal/database"
)

type ChatSession struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Messages  []map[string]interface{}
	CreatedAt time.Time
	UpdatedAt time.Time
}

type ChatRepository interface {
	GetSessionForUser(ctx context.Context, userID uuid.UUID) (*ChatSession, error)
	SaveSession(ctx context.Context, session *ChatSession) error
}

type PostgresChatRepository struct {
	db *database.Database
}

func NewPostgresChatRepository(db *database.Database) *PostgresChatRepository {
	return &PostgresChatRepository{db: db}
}

// GetSessionForUser returns the most recent chat session for a user, or creates a new one in memory if none exist
func (r *PostgresChatRepository) GetSessionForUser(ctx context.Context, userID uuid.UUID) (*ChatSession, error) {
	query := `
		SELECT id, user_id, messages, created_at, updated_at 
		FROM chat_sessions 
		WHERE user_id = $1 
		ORDER BY updated_at DESC 
		LIMIT 1
	`
	
	row := r.db.Pool.QueryRow(ctx, query, userID)
	
	var session ChatSession
	var messagesBytes []byte
	
	err := row.Scan(&session.ID, &session.UserID, &messagesBytes, &session.CreatedAt, &session.UpdatedAt)
	
	if err == pgx.ErrNoRows {
		// Return a fresh session if none exists
		return &ChatSession{
			ID:       uuid.New(),
			UserID:   userID,
			Messages: []map[string]interface{}{},
		}, nil
	}
	
	if err != nil {
		return nil, fmt.Errorf("error fetching chat session: %w", err)
	}

	if err := json.Unmarshal(messagesBytes, &session.Messages); err != nil {
		return nil, fmt.Errorf("error parsing messages json: %w", err)
	}

	return &session, nil
}

// SaveSession inserts or updates the chat session
func (r *PostgresChatRepository) SaveSession(ctx context.Context, session *ChatSession) error {
	messagesBytes, err := json.Marshal(session.Messages)
	if err != nil {
		return fmt.Errorf("error marshalling messages: %w", err)
	}

	query := `
		INSERT INTO chat_sessions (id, user_id, messages, updated_at)
		VALUES ($1, $2, $3, CURRENT_TIMESTAMP)
		ON CONFLICT (id) DO UPDATE 
		SET messages = EXCLUDED.messages, updated_at = CURRENT_TIMESTAMP
	`
	
	_, err = r.db.Pool.Exec(ctx, query, session.ID, session.UserID, messagesBytes)
	if err != nil {
		return fmt.Errorf("error saving chat session: %w", err)
	}

	return nil
}
