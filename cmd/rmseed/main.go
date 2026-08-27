// Command rmseed seeds the RM/Admin console with a starter desk: one admin
// and two Relationship Managers, then backfills every currently-unassigned
// user across the two RMs round-robin so the admin console is populated
// from day one.
//
// Idempotent: re-running it never duplicates staff (ON CONFLICT (email)) and
// only assigns users that still have no RM.
//
//	go run ./cmd/rmseed                 # uses defaults / env
//	RM_SEED_PASSWORD=... go run ./cmd/rmseed
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
	"github.com/yourusername/astra-backend/internal/service"
)

type seedStaff struct {
	Email string
	Name  string
	Role  string
}

func main() {
	ctx := context.Background()
	cfg := config.Load()

	password := os.Getenv("RM_SEED_PASSWORD")
	if password == "" {
		password = "Astra@123"
		log.Println("RM_SEED_PASSWORD not set — using dev default 'Astra@123'")
	}

	if err := database.RunMigrations(cfg.DatabaseURL); err != nil {
		log.Fatalf("migrations: %v", err)
	}
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	hash, err := service.HashPassword(password)
	if err != nil {
		log.Fatalf("hash password: %v", err)
	}

	staff := []seedStaff{
		{Email: "admin@astra.in", Name: "Astra Admin", Role: "admin"},
		{Email: "rm1@astra.in", Name: "Priya Nair", Role: "rm"},
		{Email: "rm2@astra.in", Name: "Arjun Mehta", Role: "rm"},
	}

	ids := make(map[string]uuid.UUID)
	for _, s := range staff {
		var id uuid.UUID
		err := pool.QueryRow(ctx, `
			INSERT INTO rm_users (email, password_hash, name, role)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (email) DO UPDATE SET email = EXCLUDED.email
			RETURNING id
		`, s.Email, hash, s.Name, s.Role).Scan(&id)
		if err != nil {
			log.Fatalf("seed %s: %v", s.Email, err)
		}
		ids[s.Email] = id
		fmt.Printf("  %-16s %-12s %s\n", s.Email, s.Role, id)
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

	fmt.Printf("\nBackfilled %d user(s). Login password for all seeded staff: %s\n", assigned, password)
}
