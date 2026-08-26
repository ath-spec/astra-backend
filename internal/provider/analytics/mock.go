package analytics

import (
	"context"
	"fmt"
	"hash/fnv"
	"math/rand"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	analyticsdomain "github.com/yourusername/astra-backend/internal/domain/analytics"
)

const seedWindowDays = 180

type merchantProfile struct {
	category   string
	merchant   string
	minAmount  float64
	maxAmount  float64
	perWeek    float64 // expected occurrences per week
	eveningish bool    // biases transaction hour towards evening/night
}

// frequentProfiles are rolled independently each day using a per-day
// probability derived from perWeek/7.
var frequentProfiles = []merchantProfile{
	{"Groceries", "BigBasket", 300, 1800, 2.0, false},
	{"Groceries", "DMart", 250, 1500, 1.0, false},
	{"Food & Dining", "Swiggy", 150, 700, 2.5, true},
	{"Food & Dining", "Zomato", 150, 750, 2.0, true},
	{"Food & Dining", "Starbucks", 200, 600, 1.0, true},
	{"Shopping", "Amazon", 400, 4000, 1.2, false},
	{"Shopping", "Myntra", 500, 3500, 0.7, false},
	{"Transport", "Uber", 100, 600, 2.5, true},
	{"Transport", "Ola", 100, 550, 1.0, true},
	{"Transport", "Indian Oil Petrol Pump", 500, 2000, 0.5, false},
	{"Entertainment", "BookMyShow", 300, 1500, 0.4, true},
	{"Health", "Apollo Pharmacy", 200, 1200, 0.5, false},
}

// monthlyProfiles are inserted once per month, on a per-user fixed day, with
// a consistent (low-variance) amount — this is what makes the recurring
// detector find them.
var monthlyProfiles = []merchantProfile{
	{"Bills & Utilities", "Tata Power", 900, 2200, 0, false},
	{"Bills & Utilities", "Airtel", 500, 900, 0, false},
	{"Bills & Utilities", "Jio Fiber", 800, 1200, 0, false},
	{"Subscriptions", "Netflix", 199, 649, 0, false},
	{"Subscriptions", "Spotify", 119, 179, 0, false},
	{"Subscriptions", "Amazon Prime", 299, 299, 0, false},
}

const (
	rentAmountMin   = 15000
	rentAmountMax   = 28000
	salaryAmountMin = 55000
	salaryAmountMax = 95000
	rentMerchant    = "Landlord - Rent"
	salaryMerchant  = "Employer Payroll"
)

// MockSource lazily generates ~180 days of realistic, categorized
// transaction history per user and persists it, so repeated analytics calls
// see consistent, evolving data rather than being regenerated each time.
type MockSource struct {
	pool *pgxpool.Pool
}

func NewMockSource(pool *pgxpool.Pool) *MockSource {
	return &MockSource{pool: pool}
}

func userSeed(userID uuid.UUID) int64 {
	h := fnv.New64a()
	_, _ = h.Write(userID[:])
	return int64(h.Sum64())
}

func (s *MockSource) GetTransactions(ctx context.Context, userID uuid.UUID, from, to time.Time) ([]analyticsdomain.Transaction, error) {
	if err := s.ensureSeeded(ctx, userID); err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, amount, type, category, merchant, occurred_at
		FROM spend_transactions
		WHERE user_id = $1 AND occurred_at >= $2 AND occurred_at <= $3
		ORDER BY occurred_at
	`, userID, from, to)
	if err != nil {
		return nil, fmt.Errorf("query spend transactions: %w", err)
	}
	defer rows.Close()

	txns := make([]analyticsdomain.Transaction, 0)
	for rows.Next() {
		var t analyticsdomain.Transaction
		var typ string
		if err := rows.Scan(&t.ID, &t.Amount, &typ, &t.Category, &t.Merchant, &t.OccurredAt); err != nil {
			return nil, fmt.Errorf("scan spend transaction: %w", err)
		}
		t.Type = analyticsdomain.TxnType(typ)
		txns = append(txns, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate spend transactions: %w", err)
	}
	return txns, nil
}

// ensureSeeded generates and inserts seed data exactly once per user. A
// Postgres advisory transaction lock (keyed by the user) serializes
// concurrent first-requests so two simultaneous callers can't double-seed —
// the lock needs no existing row to hold, unlike a SELECT ... FOR UPDATE.
func (s *MockSource) ensureSeeded(ctx context.Context, userID uuid.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin seed tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	lockKey := userSeed(userID)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1::text))`, lockKey); err != nil {
		return fmt.Errorf("acquire seed lock: %w", err)
	}

	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM spend_transactions WHERE user_id = $1)`, userID).Scan(&exists); err != nil {
		return fmt.Errorf("check existing spend transactions: %w", err)
	}
	if exists {
		return nil
	}

	generated := generateTransactions(userID)

	batch := &pgx.Batch{}
	for _, t := range generated {
		batch.Queue(`
			INSERT INTO spend_transactions (user_id, amount, type, category, merchant, occurred_at)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, userID, t.Amount, string(t.Type), t.Category, t.Merchant, t.OccurredAt)
	}
	br := tx.SendBatch(ctx, batch)
	for range generated {
		if _, err := br.Exec(); err != nil {
			_ = br.Close()
			return fmt.Errorf("insert seed transaction: %w", err)
		}
	}
	if err := br.Close(); err != nil {
		return fmt.Errorf("close seed batch: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit seed tx: %w", err)
	}
	return nil
}

func generateTransactions(userID uuid.UUID) []analyticsdomain.Transaction {
	r := rand.New(rand.NewSource(userSeed(userID))) //nolint:gosec // deterministic mock data generation, not security-sensitive

	now := time.Now().UTC()
	start := now.AddDate(0, 0, -seedWindowDays)
	billingDay := 1 + r.Intn(5) // fixed day-of-month for this user's rent/bills/salary

	var txns []analyticsdomain.Transaction

	for d := 0; d <= seedWindowDays; d++ {
		day := start.AddDate(0, 0, d)

		if day.Day() == billingDay {
			txns = append(txns, analyticsdomain.Transaction{
				Amount: round2(rentAmountMin + r.Float64()*(rentAmountMax-rentAmountMin)),
				Type:   analyticsdomain.TxnDebit, Category: "Rent", Merchant: rentMerchant,
				OccurredAt: atHour(day, 9, r),
			})
			txns = append(txns, analyticsdomain.Transaction{
				Amount: round2(salaryAmountMin + r.Float64()*(salaryAmountMax-salaryAmountMin)),
				Type:   analyticsdomain.TxnCredit, Category: "Salary", Merchant: salaryMerchant,
				OccurredAt: atHour(day, 10, r),
			})
			for _, mp := range monthlyProfiles {
				// Small jitter around a base amount per user, so the amount
				// is consistent across months (feeds recurring detection)
				// but not identical to the profile's raw range.
				base := mp.minAmount + (mp.maxAmount-mp.minAmount)*0.5
				jitter := base * 0.03 * (r.Float64()*2 - 1)
				txns = append(txns, analyticsdomain.Transaction{
					Amount: round2(base + jitter), Type: analyticsdomain.TxnDebit,
					Category: mp.category, Merchant: mp.merchant,
					OccurredAt: atHour(day, 8+r.Intn(10), r),
				})
			}
		}

		for _, mp := range frequentProfiles {
			prob := mp.perWeek / 7
			// Weekends see modestly more food/entertainment/shopping activity.
			if isWeekend(day) && (mp.category == "Food & Dining" || mp.category == "Entertainment" || mp.category == "Shopping") {
				prob *= 1.4
			}
			if r.Float64() >= prob {
				continue
			}
			hour := 9 + r.Intn(11)
			if mp.eveningish && r.Float64() < 0.35 {
				hour = 19 + r.Intn(7) // 19:00-01:00-ish evening/night skew
				if hour >= 24 {
					hour -= 24
				}
			}
			txns = append(txns, analyticsdomain.Transaction{
				Amount: round2(mp.minAmount + r.Float64()*(mp.maxAmount-mp.minAmount)),
				Type:   analyticsdomain.TxnDebit, Category: mp.category, Merchant: mp.merchant,
				OccurredAt: atHour(day, hour, r),
			})
		}
	}

	for i := range txns {
		txns[i].ID = uuid.New().String()
	}
	return txns
}

func atHour(day time.Time, hour int, r *rand.Rand) time.Time {
	minute := r.Intn(60)
	return time.Date(day.Year(), day.Month(), day.Day(), hour%24, minute, 0, 0, time.UTC)
}

func isWeekend(t time.Time) bool {
	wd := t.Weekday()
	return wd == time.Saturday || wd == time.Sunday
}

func round2(v float64) float64 {
	return float64(int64(v*100+0.5)) / 100
}
