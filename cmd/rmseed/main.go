// Command rmseed seeds the RM/Admin console with a starter desk (one admin
// + two RMs) and backfills unassigned users round-robin. Idempotent — safe
// to re-run. The same logic runs automatically on boot when
// RM_SEED_ON_BOOT=true (see cmd/api).
//
//	go run ./cmd/rmseed
//	RM_SEED_ADMIN_PHONE=+9199... RM_SEED_RM1_PHONE=+9199... RM_SEED_RM2_PHONE=+9199... go run ./cmd/rmseed
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourusername/astra-backend/internal/config"
	"github.com/yourusername/astra-backend/internal/database"
	"github.com/yourusername/astra-backend/internal/rmseed"
)

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

	res, err := rmseed.Run(ctx, pool, rmseed.ConfigFromEnv())
	if err != nil {
		log.Fatalf("seed: %v", err)
	}

	for email, id := range res.StaffIDs {
		fmt.Printf("  %-16s %s\n", email, id)
	}
	fmt.Printf("\nBackfilled %d user(s).\n", res.UsersAssigned)
	fmt.Println("Login: POST /api/rm/auth/otp/send {\"identifier\":\"EMP001\"}  then  /api/rm/auth/otp/verify {\"identifier\":\"EMP001\",\"otp\":\"<code from server log>\"}")
	fmt.Println("Set RM_OTP_DEV_CODE on the server for a fixed testable code.")
}
