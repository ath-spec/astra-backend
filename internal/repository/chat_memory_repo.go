package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ChatMemory is one durable, cross-session fact the assistant knows about a user.
// The kind "summary" is special: a single rolling digest row per user, keyed by
// summaryKeyNorm, never returned by TopFacts.
type ChatMemory struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	Kind       string
	Content    string
	KeyNorm    string
	Salience   int
	Source     string // "auto" | "user"
	CapturedAt time.Time
	LastSeenAt time.Time
	ExpiresAt  *time.Time
}

// SummaryKeyNorm is the reserved key_norm for the per-user rolling summary row.
const SummaryKeyNorm = "__conversation_summary__"

type ChatMemoryRepository interface {
	// TopFacts returns the most salient, non-expired memories for a user,
	// excluding the rolling summary row.
	TopFacts(ctx context.Context, userID uuid.UUID, limit int) ([]ChatMemory, error)
	// ListAll returns every memory for a user (facts first, then summary),
	// newest first — for the inspect/forget API.
	ListAll(ctx context.Context, userID uuid.UUID) ([]ChatMemory, error)
	// Upsert inserts or, on (user_id, key_norm) conflict, refreshes a memory:
	// content/kind replaced, salience kept at the max, captured_at bumped so the
	// "as of" date always reflects the latest mention. A user-sourced row is
	// never downgraded to auto.
	Upsert(ctx context.Context, m ChatMemory) error
	// DeleteByID removes one memory the user owns; reports whether a row went.
	DeleteByID(ctx context.Context, userID, id uuid.UUID) (bool, error)
	// GetSummary returns the rolling summary text and when it was last written.
	GetSummary(ctx context.Context, userID uuid.UUID) (string, time.Time, error)
	// SetSummary replaces the rolling summary row.
	SetSummary(ctx context.Context, userID uuid.UUID, summary string) error
}

type PostgresChatMemoryRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresChatMemoryRepository(pool *pgxpool.Pool) *PostgresChatMemoryRepository {
	return &PostgresChatMemoryRepository{pool: pool}
}

const chatMemoryCols = `id, user_id, kind, content, key_norm, salience, source, captured_at, last_seen_at, expires_at`

func scanChatMemory(rows pgx.Rows) (ChatMemory, error) {
	var m ChatMemory
	err := rows.Scan(&m.ID, &m.UserID, &m.Kind, &m.Content, &m.KeyNorm,
		&m.Salience, &m.Source, &m.CapturedAt, &m.LastSeenAt, &m.ExpiresAt)
	return m, err
}

func (r *PostgresChatMemoryRepository) TopFacts(ctx context.Context, userID uuid.UUID, limit int) ([]ChatMemory, error) {
	if limit <= 0 {
		limit = 8
	}
	rows, err := r.pool.Query(ctx, `
		SELECT `+chatMemoryCols+`
		FROM chat_memories
		WHERE user_id = $1
		  AND kind <> 'summary'
		  AND (expires_at IS NULL OR expires_at > now())
		ORDER BY salience DESC, captured_at DESC
		LIMIT $2`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("chat memory top: %w", err)
	}
	defer rows.Close()

	out := make([]ChatMemory, 0, limit)
	for rows.Next() {
		m, err := scanChatMemory(rows)
		if err != nil {
			return nil, fmt.Errorf("chat memory scan: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *PostgresChatMemoryRepository) ListAll(ctx context.Context, userID uuid.UUID) ([]ChatMemory, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT `+chatMemoryCols+`
		FROM chat_memories
		WHERE user_id = $1
		ORDER BY (kind = 'summary'), captured_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("chat memory list: %w", err)
	}
	defer rows.Close()

	var out []ChatMemory
	for rows.Next() {
		m, err := scanChatMemory(rows)
		if err != nil {
			return nil, fmt.Errorf("chat memory scan: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *PostgresChatMemoryRepository) Upsert(ctx context.Context, m ChatMemory) error {
	if m.Salience <= 0 {
		m.Salience = 3
	}
	if m.Source == "" {
		m.Source = "auto"
	}
	if m.Kind == "" {
		m.Kind = "fact"
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO chat_memories (user_id, kind, content, key_norm, salience, source, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (user_id, key_norm) DO UPDATE SET
			content      = EXCLUDED.content,
			kind         = EXCLUDED.kind,
			salience     = GREATEST(chat_memories.salience, EXCLUDED.salience),
			source       = CASE WHEN chat_memories.source = 'user' THEN 'user' ELSE EXCLUDED.source END,
			expires_at   = EXCLUDED.expires_at,
			captured_at  = now(),
			last_seen_at = now()`,
		m.UserID, m.Kind, m.Content, m.KeyNorm, m.Salience, m.Source, m.ExpiresAt)
	if err != nil {
		return fmt.Errorf("chat memory upsert: %w", err)
	}
	return nil
}

func (r *PostgresChatMemoryRepository) DeleteByID(ctx context.Context, userID, id uuid.UUID) (bool, error) {
	tag, err := r.pool.Exec(ctx,
		`DELETE FROM chat_memories WHERE user_id = $1 AND id = $2`, userID, id)
	if err != nil {
		return false, fmt.Errorf("chat memory delete: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (r *PostgresChatMemoryRepository) GetSummary(ctx context.Context, userID uuid.UUID) (string, time.Time, error) {
	var content string
	var at time.Time
	err := r.pool.QueryRow(ctx, `
		SELECT content, captured_at FROM chat_memories
		WHERE user_id = $1 AND key_norm = $2`, userID, SummaryKeyNorm).Scan(&content, &at)
	if err == pgx.ErrNoRows {
		return "", time.Time{}, nil
	}
	if err != nil {
		return "", time.Time{}, fmt.Errorf("chat memory get summary: %w", err)
	}
	return content, at, nil
}

func (r *PostgresChatMemoryRepository) SetSummary(ctx context.Context, userID uuid.UUID, summary string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO chat_memories (user_id, kind, content, key_norm, salience, source)
		VALUES ($1, 'summary', $2, $3, 0, 'auto')
		ON CONFLICT (user_id, key_norm) DO UPDATE SET
			content = EXCLUDED.content,
			captured_at = now(),
			last_seen_at = now()`,
		userID, summary, SummaryKeyNorm)
	if err != nil {
		return fmt.Errorf("chat memory set summary: %w", err)
	}
	return nil
}
