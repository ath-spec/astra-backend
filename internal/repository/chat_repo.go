package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/yourusername/astra-backend/internal/database"
)

type ChatSession struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Title     string
	Messages  []map[string]interface{}
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ChatSessionSummary is the lightweight row used by the History list — no
// message bodies, just enough to render and pick a thread to resume.
type ChatSessionSummary struct {
	ID           uuid.UUID
	Title        string
	MessageCount int
	UpdatedAt    time.Time
}

type ChatRepository interface {
	// GetSessionForUser returns the most recent chat session for a user —
	// the pre-multi-thread behaviour, kept for callers that don't care which
	// specific thread they're in (e.g. a client that hasn't sent a
	// session_id yet).
	GetSessionForUser(ctx context.Context, userID uuid.UUID) (*ChatSession, error)
	// GetSessionByID loads one specific session, scoped to userID so a user
	// can never load another user's thread. Returns a fresh, empty session
	// with this exact ID (not an error) if it doesn't exist yet — the same
	// "doesn't exist yet" behaviour GetSessionForUser has always had, so a
	// client-generated session_id for a brand-new thread just works.
	GetSessionByID(ctx context.Context, userID, sessionID uuid.UUID) (*ChatSession, error)
	// ListSessionsForUser returns every thread for a user, newest first —
	// backs the History screen.
	ListSessionsForUser(ctx context.Context, userID uuid.UUID) ([]ChatSessionSummary, error)
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
		SELECT id, user_id, title, messages, created_at, updated_at
		FROM chat_sessions
		WHERE user_id = $1
		ORDER BY updated_at DESC
		LIMIT 1
	`

	row := r.db.Pool.QueryRow(ctx, query, userID)
	session, err := scanChatSession(row)
	if err == pgx.ErrNoRows {
		return &ChatSession{ID: uuid.New(), UserID: userID, Messages: []map[string]interface{}{}}, nil
	}
	return session, err
}

// GetSessionByID loads exactly the session the caller asked for, scoped to
// userID. A missing row isn't an error — it means "this is a new thread the
// client hasn't saved yet," so an empty session with that same ID is
// returned instead, matching GetSessionForUser's existing not-found shape.
func (r *PostgresChatRepository) GetSessionByID(ctx context.Context, userID, sessionID uuid.UUID) (*ChatSession, error) {
	query := `
		SELECT id, user_id, title, messages, created_at, updated_at
		FROM chat_sessions
		WHERE id = $1 AND user_id = $2
	`
	row := r.db.Pool.QueryRow(ctx, query, sessionID, userID)
	session, err := scanChatSession(row)
	if err == pgx.ErrNoRows {
		return &ChatSession{ID: sessionID, UserID: userID, Messages: []map[string]interface{}{}}, nil
	}
	return session, err
}

func scanChatSession(row pgx.Row) (*ChatSession, error) {
	var session ChatSession
	var messagesBytes []byte
	if err := row.Scan(&session.ID, &session.UserID, &session.Title, &messagesBytes, &session.CreatedAt, &session.UpdatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("error fetching chat session: %w", err)
	}
	if err := json.Unmarshal(messagesBytes, &session.Messages); err != nil {
		return nil, fmt.Errorf("error parsing messages json: %w", err)
	}
	return &session, nil
}

// ListSessionsForUser backs the History screen — every thread, newest
// first, with a message count instead of the full message bodies (kept
// light; a thread's full content only loads when the user actually opens
// it, via GetSessionByID).
func (r *PostgresChatRepository) ListSessionsForUser(ctx context.Context, userID uuid.UUID) ([]ChatSessionSummary, error) {
	rows, err := r.db.Pool.Query(ctx, `
		SELECT id, title, jsonb_array_length(messages), updated_at
		FROM chat_sessions
		WHERE user_id = $1
		ORDER BY updated_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("error listing chat sessions: %w", err)
	}
	defer rows.Close()

	sessions := make([]ChatSessionSummary, 0)
	for rows.Next() {
		var s ChatSessionSummary
		if err := rows.Scan(&s.ID, &s.Title, &s.MessageCount, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("error scanning chat session summary: %w", err)
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

// SaveSession inserts or updates the chat session. Title is auto-derived
// from the first user message the first time a session actually has one
// (same truncation the frontend used to do purely client-side in
// chat_session_provider.dart) — callers never have to set it themselves.
func (r *PostgresChatRepository) SaveSession(ctx context.Context, session *ChatSession) error {
	if session.Title == "" || session.Title == "New Chat" {
		if derived := deriveSessionTitle(session.Messages); derived != "" {
			session.Title = derived
		}
	}

	messagesBytes, err := json.Marshal(session.Messages)
	if err != nil {
		return fmt.Errorf("error marshalling messages: %w", err)
	}

	query := `
		INSERT INTO chat_sessions (id, user_id, title, messages, updated_at)
		VALUES ($1, $2, $3, $4, CURRENT_TIMESTAMP)
		ON CONFLICT (id) DO UPDATE
		SET messages = EXCLUDED.messages, title = EXCLUDED.title, updated_at = CURRENT_TIMESTAMP
	`

	_, err = r.db.Pool.Exec(ctx, query, session.ID, session.UserID, session.Title, messagesBytes)
	if err != nil {
		return fmt.Errorf("error saving chat session: %w", err)
	}

	return nil
}

func deriveSessionTitle(messages []map[string]interface{}) string {
	for _, m := range messages {
		role, _ := m["role"].(string)
		content, _ := m["content"].(string)
		content = strings.TrimSpace(content)
		if role == "user" && content != "" {
			runes := []rune(content)
			if len(runes) > 40 {
				return string(runes[:40]) + "..."
			}
			return content
		}
	}
	return ""
}
