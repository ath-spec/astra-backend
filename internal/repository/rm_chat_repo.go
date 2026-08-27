package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RMChatSession is one staff copilot conversation, isolated per (staff, scope).
type RMChatSession struct {
	ID       uuid.UUID
	RMID     uuid.UUID
	Scope    string
	Messages []map[string]interface{}
}

type RMChatRepository interface {
	GetSession(ctx context.Context, rmID uuid.UUID, scope string) (*RMChatSession, error)
	SaveSession(ctx context.Context, session *RMChatSession) error
	ClearSession(ctx context.Context, rmID uuid.UUID, scope string) error
}

type PostgresRMChatRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRMChatRepository(pool *pgxpool.Pool) *PostgresRMChatRepository {
	return &PostgresRMChatRepository{pool: pool}
}

func (r *PostgresRMChatRepository) GetSession(ctx context.Context, rmID uuid.UUID, scope string) (*RMChatSession, error) {
	var s RMChatSession
	var raw []byte
	err := r.pool.QueryRow(ctx, `
		SELECT id, rm_id, scope, messages FROM rm_chat_sessions
		WHERE rm_id = $1 AND scope = $2
	`, rmID, scope).Scan(&s.ID, &s.RMID, &s.Scope, &raw)
	if err == pgx.ErrNoRows {
		return &RMChatSession{ID: uuid.New(), RMID: rmID, Scope: scope, Messages: []map[string]interface{}{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get rm chat session: %w", err)
	}
	if err := json.Unmarshal(raw, &s.Messages); err != nil {
		return nil, fmt.Errorf("parse rm chat messages: %w", err)
	}
	return &s, nil
}

func (r *PostgresRMChatRepository) SaveSession(ctx context.Context, s *RMChatSession) error {
	raw, err := json.Marshal(s.Messages)
	if err != nil {
		return fmt.Errorf("marshal rm chat messages: %w", err)
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO rm_chat_sessions (id, rm_id, scope, messages, updated_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (rm_id, scope) DO UPDATE
		SET messages = EXCLUDED.messages, updated_at = now()
	`, s.ID, s.RMID, s.Scope, raw)
	if err != nil {
		return fmt.Errorf("save rm chat session: %w", err)
	}
	return nil
}

func (r *PostgresRMChatRepository) ClearSession(ctx context.Context, rmID uuid.UUID, scope string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM rm_chat_sessions WHERE rm_id = $1 AND scope = $2`, rmID, scope)
	return err
}
