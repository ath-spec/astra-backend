// Command rmseed seeds the RM/Admin console with a starter desk: one admin
// and two Relationship Managers, then backfills every currently-unassigned
// user across the two RMs round-robin so the admin console is populated
// from day one.
//
// Idempotent: re-running it never duplicates staff (ON CONFLICT (email)) and
// only assigns users that still have no RM.
//
// Staff log in with their employee code (or email) + an OTP sent to their
// phone, so set real numbers via env for a deployment:
//
//	RM_SEED_ADMIN_PHONE=+9199... RM_SEED_RM1_PHONE=+9199... RM_SEED_RM2_PHONE=+9199... \
//	  go run ./cmd/rmseed
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourusername/astra-backend/internal/config"
	"github.com/yourusername/astra-backend/internal/database"
)

type seedStaff struct {
	EmployeeCode string
	Email        string
	Name         string
	Role         string
	Phone        string
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	ctx := context.Background()
	cfg := config.Load()

	if err := database.RunMigrations(cfg.DatabaseURL); err != nil {
		log.Fatalf("migrations: %v", err)
	}
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	staff := []seedStaff{
		{EmployeeCode: "EMP001", Email: "admin@astra.in", Name: "Astra Admin", Role: "admin", Phone: envOr("RM_SEED_ADMIN_PHONE", "+919000000001")},
		{EmployeeCode: "EMP002", Email: "rm1@astra.in", Name: "Priya Nair", Role: "rm", Phone: envOr("RM_SEED_RM1_PHONE", "+919000000002")},
		{EmployeeCode: "EMP003", Email: "rm2@astra.in", Name: "Arjun Mehta", Role: "rm", Phone: envOr("RM_SEED_RM2_PHONE", "+919000000003")},
	}

	ids := make(map[string]uuid.UUID)
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
			log.Fatalf("seed %s: %v", s.Email, err)
		}
		ids[s.Email] = id
		fmt.Printf("  %-8s %-16s %-6s %-14s %s\n", s.EmployeeCode, s.Email, s.Role, s.Phone, id)
	}

	rmRing := []uuid.UUID{ids["rm1@astra.in"], ids["rm2@astra.in"]}

	// Backfill unassigned users round-robin across the two RMs.
	rows, err := pool.Query(ctx, `SELECT id FROM users WHERE assigned_rm_id IS NULL ORDER BY created_at`)
	if err != nil {
		log.Fatalf("list unassigned users: %v", err)
	}
	var unassigned []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			log.Fatalf("scan user: %v", err)
		}
		unassigned = append(unassigned, id)
	}
	rows.Close()

	assigned := 0
	last := uuid.Nil
	for i, userID := range unassigned {
		rmID := rmRing[i%len(rmRing)]
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
			log.Fatalf("assign user %s: %v", userID, err)
		}
		last = rmID
		assigned++
	}

	if last != uuid.Nil {
		if _, err := pool.Exec(ctx, `
			UPDATE rm_queue_state SET last_assigned_rm_id = $1, updated_at = now() WHERE id = true
		`, last); err != nil {
			log.Fatalf("advance queue state: %v", err)
		}
	}

	fmt.Printf("\nBackfilled %d user(s).\n", assigned)
	fmt.Println("Login: POST /api/rm/auth/otp/send {\"identifier\":\"EMP001\"}  then  /api/rm/auth/otp/verify {\"identifier\":\"EMP001\",\"otp\":\"<code from server log>\"}")
	fmt.Println("Set RM_OTP_DEV_CODE on the server for a fixed testable code.")
}
