//go:build ignore

// seed_idbi_customers.go parses docs/IDBI APIs - Data.csv and upserts every
// row into idbi_seed_customers via the SQLC-generated repository.
//
// Usage:
//
//	go run scripts/seed_idbi_customers.go
//
// It reads DATABASE_URL from the environment (or .env if present).
package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/yourusername/astra-backend/internal/repository"
)

func main() {
	// Load .env if present (non-fatal).
	_ = godotenv.Load()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}
	defer pool.Close()

	repo := repository.NewIDBISeedRepository(pool)

	csvPath := "docs/IDBI APIs - Data.csv"
	f, err := os.Open(csvPath)
	if err != nil {
		log.Fatalf("open csv: %v", err)
	}
	defer f.Close()

	var (
		accountType string
		accountCode string
		inserted    int
		skipped     int
	)

	// Account-type header patterns and their codes.
	// Keys must match the section header prefix in the CSV exactly.
	codeMap := map[string]string{
		"SAV_REGULAR": "SB001",
		"SAV_ZEROBAL": "SB002",
		"SAV_SALARY":  "SB003",
		"SAV_SENIOR":  "SB004",
		"SAV_MINOR":   "SB005",
		"SAV_WOMEN":   "SB006",
		"SAV_STUDENT": "SB007",
		"SAV_PREMIUM": "SB008",
		"CUR_REGULAR": "CA001",
		"CUR_PREMIUM": "CA002",
		"CUR_STARTUP": "CA003",
		"CUR_TRADER":  "CA004",
		"CUR_CORP":    "CA005",
		"CUR_FOREIGN": "CA006",
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r\n")

		// Skip blank / comma-only lines.
		if strings.Trim(line, ", ") == "" {
			continue
		}

		// Detect section headers like "SAV_REGULAR (SB001 / RSAV),,"
		for key, code := range codeMap {
			if strings.HasPrefix(line, key) {
				accountType = key
				accountCode = code
				break
			}
		}

		// Skip non-data lines (headers, preamble, section titles).
		parts := strings.Split(line, ",")
		if len(parts) < 3 {
			continue
		}
		custID := strings.TrimSpace(parts[0])
		pan := strings.TrimSpace(parts[1])
		name := strings.TrimSpace(parts[2])

		// Must be a numeric cust_id row.
		if !isNumeric(custID) || pan == "PAN Number" || pan == "" {
			continue
		}
		if accountType == "" {
			skipped++
			continue
		}

		if err := repo.UpsertSeedCustomer(ctx, repository.SeedCustomer{
			CustID:       custID,
			PANNumber:    pan,
			CustomerName: name,
			AccountType:  accountType,
			AccountCode:  accountCode,
		}); err != nil {
			log.Printf("WARN: skip %s: %v", custID, err)
			skipped++
			continue
		}
		inserted++
	}
	if err := scanner.Err(); err != nil {
		log.Fatalf("scan csv: %v", err)
	}

	total, _ := repo.Count(ctx)
	fmt.Printf("Done. inserted/updated=%d  skipped=%d  total_in_db=%d\n", inserted, skipped, total)
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
