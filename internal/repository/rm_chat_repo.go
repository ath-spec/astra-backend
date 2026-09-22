package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RMChatSession is one staff copilot conversation.
type RMChatSession struct {
	ID        uuid.UUID                `json:"id"`
	RMID      uuid.UUID                `json:"rm_id"`
	Scope     string                   `json:"scope"`
	Title     string                   `json:"title"`
	Messages  []map[string]interface{} `json:"messages"`
	CreatedAt time.Time                `json:"created_at"`
	UpdatedAt time.Time                `json:"updated_at"`
}

// RMChatSessionListItem is a summary representation for sidebar/dropdown listing.
type RMChatSessionListItem struct {
	ID           uuid.UUID `json:"id"`
	Title        string    `json:"title"`
	Scope        string    `json:"scope"`
	MessageCount int       `json:"message_count"`
	LastMessage  string    `json:"last_message,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type RMChatRepository interface {
	ListSessions(ctx context.Context, rmID uuid.UUID, scope string) ([]RMChatSessionListItem, error)
	GetSessionByID(ctx context.Context, id uuid.UUID, rmID uuid.UUID) (*RMChatSession, error)
	GetLatestSession(ctx context.Context, rmID uuid.UUID, scope string) (*RMChatSession, error)
	CreateSession(ctx context.Context, rmID uuid.UUID, scope string, title string) (*RMChatSession, error)
	SaveSession(ctx context.Context, session *RMChatSession) error
	DeleteSession(ctx context.Context, id uuid.UUID, rmID uuid.UUID) error
	// Backward compatibility methods
	GetSession(ctx context.Context, rmID uuid.UUID, scope string) (*RMChatSession, error)
	ClearSession(ctx context.Context, rmID uuid.UUID, scope string) error
}

type PostgresRMChatRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRMChatRepository(pool *pgxpool.Pool) *PostgresRMChatRepository {
	return &PostgresRMChatRepository{pool: pool}
}

func (r *PostgresRMChatRepository) ListSessions(ctx context.Context, rmID uuid.UUID, scope string) ([]RMChatSessionListItem, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, title, scope, jsonb_array_length(messages), messages, created_at, updated_at
		FROM rm_chat_sessions
		WHERE rm_id = $1 AND scope = $2
		ORDER BY updated_at DESC
	`, rmID, scope)
	if err != nil {
		return nil, fmt.Errorf("list rm chat sessions: %w", err)
	}
	defer rows.Close()

	var list []RMChatSessionListItem
	for rows.Next() {
		var (
			item    RMChatSessionListItem
			msgsRaw []byte
			count   int
		)
		if err := rows.Scan(&item.ID, &item.Title, &item.Scope, &count, &msgsRaw, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan rm chat session item: %w", err)
		}
		item.MessageCount = count
		// Extract last message content for quick preview
		var msgs []map[string]interface{}
		if err := json.Unmarshal(msgsRaw, &msgs); err == nil && len(msgs) > 0 {
			last := msgs[len(msgs)-1]
			if content, ok := last["content"].(string); ok {
				item.LastMessage = content
			}
		}
		list = append(list, item)
	}
	if list == nil {
		list = []RMChatSessionListItem{}
	}
	return list, nil
}

func (r *PostgresRMChatRepository) GetSessionByID(ctx context.Context, id uuid.UUID, rmID uuid.UUID) (*RMChatSession, error) {
	var s RMChatSession
	var raw []byte
	err := r.pool.QueryRow(ctx, `
		SELECT id, rm_id, scope, title, messages, created_at, updated_at
		FROM rm_chat_sessions
		WHERE id = $1 AND rm_id = $2
	`, id, rmID).Scan(&s.ID, &s.RMID, &s.Scope, &s.Title, &raw, &s.CreatedAt, &s.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get rm chat session by id: %w", err)
	}
	if err := json.Unmarshal(raw, &s.Messages); err != nil {
		return nil, fmt.Errorf("parse rm chat messages: %w", err)
	}
	if s.Messages == nil {
		s.Messages = []map[string]interface{}{}
	}
	return &s, nil
}

func (r *PostgresRMChatRepository) GetLatestSession(ctx context.Context, rmID uuid.UUID, scope string) (*RMChatSession, error) {
	var s RMChatSession
	var raw []byte
	err := r.pool.QueryRow(ctx, `
		SELECT id, rm_id, scope, title, messages, created_at, updated_at
		FROM rm_chat_sessions
		WHERE rm_id = $1 AND scope = $2
		ORDER BY updated_at DESC LIMIT 1
	`, rmID, scope).Scan(&s.ID, &s.RMID, &s.Scope, &s.Title, &raw, &s.CreatedAt, &s.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get latest rm chat session: %w", err)
	}
	if err := json.Unmarshal(raw, &s.Messages); err != nil {
		return nil, fmt.Errorf("parse rm chat messages: %w", err)
	}
	if s.Messages == nil {
		s.Messages = []map[string]interface{}{}
	}
	return &s, nil
}

func (r *PostgresRMChatRepository) CreateSession(ctx context.Context, rmID uuid.UUID, scope string, title string) (*RMChatSession, error) {
	if title == "" {
		title = "New Conversation"
	}
	sessionID := uuid.New()
	now := time.Now().UTC()
	var s RMChatSession
	var raw []byte
	err := r.pool.QueryRow(ctx, `
		INSERT INTO rm_chat_sessions (id, rm_id, scope, title, messages, created_at, updated_at)
		VALUES ($1, $2, $3, $4, '[]'::jsonb, $5, $5)
		RETURNING id, rm_id, scope, title, messages, created_at, updated_at
	`, sessionID, rmID, scope, title, now).Scan(&s.ID, &s.RMID, &s.Scope, &s.Title, &raw, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create rm chat session: %w", err)
	}
	s.Messages = []map[string]interface{}{}
	return &s, nil
}

func (r *PostgresRMChatRepository) SaveSession(ctx context.Context, s *RMChatSession) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	if s.Title == "" {
		s.Title = "New Conversation"
	}
	raw, err := json.Marshal(s.Messages)
	if err != nil {
		return fmt.Errorf("marshal rm chat messages: %w", err)
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO rm_chat_sessions (id, rm_id, scope, title, messages, updated_at)
		VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (id) DO UPDATE
		SET title = EXCLUDED.title, messages = EXCLUDED.messages, updated_at = now()
	`, s.ID, s.RMID, s.Scope, s.Title, raw)
	if err != nil {
		return fmt.Errorf("save rm chat session: %w", err)
	}
	return nil
}

func (r *PostgresRMChatRepository) DeleteSession(ctx context.Context, id uuid.UUID, rmID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM rm_chat_sessions WHERE id = $1 AND rm_id = $2`, id, rmID)
	return err
}

func (r *PostgresRMChatRepository) GetSession(ctx context.Context, rmID uuid.UUID, scope string) (*RMChatSession, error) {
	latest, err := r.GetLatestSession(ctx, rmID, scope)
	if err != nil {
		return nil, err
	}
	if latest != nil {
		return latest, nil
	}
	return r.CreateSession(ctx, rmID, scope, "New Conversation")
}

func (r *PostgresRMChatRepository) ClearSession(ctx context.Context, rmID uuid.UUID, scope string) error {
	_, err := r.CreateSession(ctx, rmID, scope, "New Conversation")
	return err
}
