// Package rmseed holds the idempotent RM/Admin starter-desk seeding logic,
// shared by the standalone `cmd/rmseed` command and the optional
// RM_SEED_ON_BOOT path in cmd/api. Re-running it never duplicates staff and
// only assigns users that still have no RM.
package rmseed

import (
	"context"
	"fmt"
	"os"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Config struct {
	AdminPhone string
	RM1Phone   string
	RM2Phone   string
}

// ConfigFromEnv builds a Config from RM_SEED_ADMIN_PHONE / RM_SEED_RM1_PHONE
// / RM_SEED_RM2_PHONE, falling back to placeholder numbers.
func ConfigFromEnv() Config {
	return Config{
		AdminPhone: envOr("RM_SEED_ADMIN_PHONE", "+919000000001"),
		RM1Phone:   envOr("RM_SEED_RM1_PHONE", "+919000000002"),
		RM2Phone:   envOr("RM_SEED_RM2_PHONE", "+919000000003"),
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

type Result struct {
	StaffIDs      map[string]uuid.UUID // keyed by email
	UsersAssigned int
}

type staffRow struct {
	EmployeeCode string
	Email        string
	Name         string
	Role         string
	Phone        string
}

// Run seeds one admin + two RMs and backfills unassigned users round-robin
// across the two RMs. Safe to call on every boot.
func Run(ctx context.Context, pool *pgxpool.Pool, cfg Config) (Result, error) {
	res := Result{StaffIDs: make(map[string]uuid.UUID)}

	staff := []staffRow{
		{EmployeeCode: "EMP001", Email: "admin@astra.in", Name: "Astra Admin", Role: "admin", Phone: cfg.AdminPhone},
		{EmployeeCode: "EMP002", Email: "rm1@astra.in", Name: "Priya Nair", Role: "rm", Phone: cfg.RM1Phone},
		{EmployeeCode: "EMP003", Email: "rm2@astra.in", Name: "Arjun Mehta", Role: "rm", Phone: cfg.RM2Phone},
	}

	for _, s := range staff {
		var id uuid.UUID
		err := pool.QueryRow(ctx, `
			INSERT INTO rm_users (employee_code, email, name, role, phone_number)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (email) DO UPDATE SET
				employee_code = EXCLUDED.employee_code,
				phone_number  = EXCLUDED.phone_number,
				name          = EXCLUDED.name,
				role          = EXCLUDED.role
			RETURNING id
		`, s.EmployeeCode, s.Email, s.Name, s.Role, s.Phone).Scan(&id)
		if err != nil {
			return res, fmt.Errorf("seed staff %s: %w", s.Email, err)
		}
		res.StaffIDs[s.Email] = id
	}

	ring := []uuid.UUID{res.StaffIDs["rm1@astra.in"], res.StaffIDs["rm2@astra.in"]}

	rows, err := pool.Query(ctx, `SELECT id FROM users WHERE assigned_rm_id IS NULL ORDER BY created_at`)
	if err != nil {
		return res, fmt.Errorf("list unassigned users: %w", err)
	}
	var unassigned []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return res, fmt.Errorf("scan user id: %w", err)
		}
		unassigned = append(unassigned, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return res, err
	}

	last := uuid.Nil
	for i, userID := range unassigned {
		rmID := ring[i%len(ring)]
		err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `UPDATE users SET assigned_rm_id = $1 WHERE id = $2`, rmID, userID); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `
				INSERT INTO rm_assignment_history (user_id, from_rm_id, to_rm_id, action, reason, actor_rm_id)
				VALUES ($1, NULL, $2, 'assign', 'rmseed backfill', NULL)
			`, userID, rmID)
			return err
		})
		if err != nil {
			return res, fmt.Errorf("assign user %s: %w", userID, err)
		}
		last = rmID
		res.UsersAssigned++
	}

	if last != uuid.Nil {
		if _, err := pool.Exec(ctx, `
			UPDATE rm_queue_state SET last_assigned_rm_id = $1, updated_at = now() WHERE id = true
		`, last); err != nil {
			return res, fmt.Errorf("advance queue state: %w", err)
		}
	}

	return res, nil
}
