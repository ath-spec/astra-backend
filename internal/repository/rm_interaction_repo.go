package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Interaction is one entry in a client's RM interaction log.
type Interaction struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	RMID       uuid.UUID
	RMName     string
	Kind       string
	Body       string
	FollowUpAt *time.Time
	DoneAt     *time.Time
	CreatedAt  time.Time
}

// PendingFollowUp is a not-yet-completed follow-up task with client context.
type PendingFollowUp struct {
	Interaction
	ClientName string
}

type RMInteractionRepository interface {
	List(ctx context.Context, userID uuid.UUID) ([]Interaction, error)
	Add(ctx context.Context, in Interaction) (Interaction, error)
	Complete(ctx context.Context, id, rmID uuid.UUID) error
	PendingFollowUps(ctx context.Context, rmID uuid.UUID) ([]PendingFollowUp, error)
}

type PostgresRMInteractionRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRMInteractionRepository(pool *pgxpool.Pool) *PostgresRMInteractionRepository {
	return &PostgresRMInteractionRepository{pool: pool}
}

func (r *PostgresRMInteractionRepository) List(ctx context.Context, userID uuid.UUID) ([]Interaction, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT i.id, i.user_id, i.rm_id, COALESCE(s.name, ''), i.kind, i.body,
		       i.follow_up_at, i.done_at, i.created_at
		FROM rm_client_interactions i
		LEFT JOIN rm_users s ON s.id = i.rm_id
		WHERE i.user_id = $1
		ORDER BY i.created_at DESC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list interactions: %w", err)
	}
	defer rows.Close()

	out := make([]Interaction, 0)
	for rows.Next() {
		var it Interaction
		if err := rows.Scan(&it.ID, &it.UserID, &it.RMID, &it.RMName, &it.Kind, &it.Body,
			&it.FollowUpAt, &it.DoneAt, &it.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan interaction: %w", err)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func (r *PostgresRMInteractionRepository) Add(ctx context.Context, in Interaction) (Interaction, error) {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO rm_client_interactions (user_id, rm_id, kind, body, follow_up_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at
	`, in.UserID, in.RMID, in.Kind, in.Body, in.FollowUpAt)
	if err := row.Scan(&in.ID, &in.CreatedAt); err != nil {
		return Interaction{}, fmt.Errorf("add interaction: %w", err)
	}
	return in, nil
}

func (r *PostgresRMInteractionRepository) Complete(ctx context.Context, id, rmID uuid.UUID) error {
	ct, err := r.pool.Exec(ctx, `
		UPDATE rm_client_interactions
		SET done_at = now()
		WHERE id = $1 AND rm_id = $2 AND done_at IS NULL
	`, id, rmID)
	if err != nil {
		return fmt.Errorf("complete interaction: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *PostgresRMInteractionRepository) PendingFollowUps(ctx context.Context, rmID uuid.UUID) ([]PendingFollowUp, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT i.id, i.user_id, i.rm_id, '', i.kind, i.body, i.follow_up_at, i.done_at, i.created_at,
		       COALESCE(u.name, '')
		FROM rm_client_interactions i
		JOIN users u ON u.id = i.user_id
		WHERE i.rm_id = $1 AND i.follow_up_at IS NOT NULL AND i.done_at IS NULL
		ORDER BY i.follow_up_at ASC
	`, rmID)
	if err != nil {
		return nil, fmt.Errorf("pending follow-ups: %w", err)
	}
	defer rows.Close()

	out := make([]PendingFollowUp, 0)
	for rows.Next() {
		var p PendingFollowUp
		if err := rows.Scan(&p.ID, &p.UserID, &p.RMID, &p.RMName, &p.Kind, &p.Body,
			&p.FollowUpAt, &p.DoneAt, &p.CreatedAt, &p.ClientName); err != nil {
			return nil, fmt.Errorf("scan follow-up: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
