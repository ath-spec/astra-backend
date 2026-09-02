package repository

import (
	"context"
	"encoding/binary"
	"hash/fnv"
	"math/rand"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// This file extends the investor-archetype seeding (see seedInitialUserData
// in user_repo.go) with ~6 months of categorized spend_transactions, so the
// budget feature's diagnosis / suggestion flow has real per-user history the
// moment a user signs up. Each archetype gets a distinct income level,
// category mix and savings rate that lines up with its portfolio persona.
//
// Category strings use the seeded-analytics vocabulary ("Groceries",
// "Rent", ...); the budget service normalizes them onto the canonical
// budget_global_categories names.

const spendHistoryMonths = 6

type spendCategorySeed struct {
	name         string
	merchant     string
	perMonthMin  int
	perMonthMax  int
	amountMin    float64
	amountMax    float64
}

type spendArchetypeProfile struct {
	monthlyIncome  float64
	incomeMerchant string
	categories     []spendCategorySeed
}

// spendProfiles is indexed by the same archetype id seedInitialUserData
// derives (0..3).
var spendProfiles = [4]spendArchetypeProfile{
	// 0 — Tech & Semiconductor Growth: high earner, owns home (light
	// maintenance only), spends freely on dining / shopping / gadgets, still
	// saves a healthy chunk.
	{
		monthlyIncome:  225000,
		incomeMerchant: "Tech Corp Payroll",
		categories: []spendCategorySeed{
			{"Groceries", "BigBasket", 5, 8, 900, 2600},
			{"Food & Dining", "Swiggy", 9, 16, 350, 1600},
			{"Shopping", "Amazon", 4, 8, 1500, 12000},
			{"Entertainment", "BookMyShow", 2, 5, 400, 2200},
			{"Subscriptions", "Notion + GitHub + Netflix", 3, 4, 200, 1800},
			{"Transport", "Uber", 6, 12, 150, 700},
			{"Bills & Utilities", "Tata Power", 2, 3, 1400, 3800},
			{"Health", "Cult.fit", 1, 3, 800, 3000},
			{"Bills & Utilities", "Apartment Maintenance", 1, 1, 4000, 6500},
		},
	},
	// 1 — Balanced Bluechip Wealth Builder: solid salary, rents, spreads
	// spend evenly, ~25% savings.
	{
		monthlyIncome:  152000,
		incomeMerchant: "Employer Payroll",
		categories: []spendCategorySeed{
			{"Rent", "Landlord - Rent", 1, 1, 30000, 34000},
			{"Groceries", "DMart", 4, 7, 700, 2200},
			{"Food & Dining", "Zomato", 6, 10, 250, 1200},
			{"Shopping", "Myntra", 2, 5, 800, 5000},
			{"Entertainment", "Netflix + PVR", 2, 4, 300, 1500},
			{"Transport", "Ola", 6, 10, 120, 600},
			{"Bills & Utilities", "Airtel + Electricity", 3, 4, 500, 2600},
			{"Health", "Apollo Pharmacy", 1, 3, 300, 1600},
		},
	},
	// 2 — Global Markets / Gold / REITs Diversifier: top earner, premium
	// rental, heavy travel and dining, moderate savings.
	{
		monthlyIncome:  268000,
		incomeMerchant: "Global Advisory Payroll",
		categories: []spendCategorySeed{
			{"Rent", "Landlord - Rent", 1, 1, 44000, 48000},
			{"Travel", "MakeMyTrip", 1, 3, 6000, 42000},
			{"Food & Dining", "Fine Dining", 8, 14, 600, 3500},
			{"Groceries", "Nature's Basket", 4, 6, 1200, 3200},
			{"Shopping", "Nykaa + Apple", 3, 6, 2000, 15000},
			{"Transport", "Uber", 8, 14, 200, 900},
			{"Bills & Utilities", "Broadband + Power", 3, 4, 900, 3200},
			{"Entertainment", "Concerts + OTT", 2, 4, 500, 3000},
			{"Health", "Max Healthcare", 1, 2, 900, 4000},
		},
	},
	// 3 — Conservative Hybrid / Capital Preservation: steady modest salary,
	// low rent, frugal discretionary spend, ~35% savings.
	{
		monthlyIncome:  118000,
		incomeMerchant: "Employer Payroll",
		categories: []spendCategorySeed{
			{"Rent", "Landlord - Rent", 1, 1, 16000, 19000},
			{"Groceries", "DMart", 4, 6, 600, 1800},
			{"Food & Dining", "Local Restaurants", 3, 6, 200, 800},
			{"Transport", "Metro + Bus", 8, 14, 30, 220},
			{"Bills & Utilities", "Electricity + Mobile", 3, 4, 400, 2000},
			{"Health", "Jan Aushadhi", 1, 3, 200, 1200},
			{"Entertainment", "Netflix", 1, 2, 199, 649},
			{"Shopping", "Reliance Trends", 1, 3, 500, 2500},
		},
	},
}

func spendSeedRand(userID uuid.UUID) *rand.Rand {
	h := fnv.New64a()
	_, _ = h.Write(userID[:])
	_, _ = h.Write([]byte("spend-history"))
	return rand.New(rand.NewSource(int64(binary.BigEndian.Uint64(h.Sum(nil))))) //nolint:gosec // deterministic mock data
}

// seedSpendHistory inserts ~6 months of DEBIT category spend + one monthly
// CREDIT salary for the user, unless spend_transactions already has rows for
// them (idempotent; also avoids clashing with the analytics MockSource).
func (r *PostgresUserRepository) seedSpendHistory(ctx context.Context, userID uuid.UUID, prof spendArchetypeProfile) error {
	var exists bool
	if err := r.db.Pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM spend_transactions WHERE user_id = $1)`, userID).Scan(&exists); err != nil {
		return nil // non-fatal: spend seeding must never block signup
	}
	if exists {
		return nil
	}

	rng := spendSeedRand(userID)
	now := time.Now().UTC()
	firstOfThisMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	batch := &pgx.Batch{}
	queue := func(amount float64, typ, category, merchant string, at time.Time) {
		batch.Queue(`
			INSERT INTO spend_transactions (user_id, amount, type, category, merchant, occurred_at)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, userID, round2Money(amount), typ, category, merchant, at)
	}

	for m := 1; m <= spendHistoryMonths; m++ {
		monthStart := firstOfThisMonth.AddDate(0, -m, 0)
		daysInMonth := monthStart.AddDate(0, 1, -1).Day()

		// Monthly salary credit, first few days of the month.
		salary := prof.monthlyIncome * (0.97 + rng.Float64()*0.06)
		queue(salary, "CREDIT", "Salary", prof.incomeMerchant,
			atRandomHour(monthStart.AddDate(0, 0, rng.Intn(3)), rng))

		for _, c := range prof.categories {
			n := c.perMonthMin
			if c.perMonthMax > c.perMonthMin {
				n += rng.Intn(c.perMonthMax - c.perMonthMin + 1)
			}
			for i := 0; i < n; i++ {
				day := monthStart.AddDate(0, 0, rng.Intn(daysInMonth))
				amt := c.amountMin + rng.Float64()*(c.amountMax-c.amountMin)
				queue(amt, "DEBIT", c.name, c.merchant, atRandomHour(day, rng))
			}
		}
	}

	br := r.db.Pool.SendBatch(ctx, batch)
	defer br.Close()
	for i := 0; i < batch.Len(); i++ {
		if _, err := br.Exec(); err != nil {
			return nil // best-effort; a partial history is still usable
		}
	}
	return nil
}

func atRandomHour(day time.Time, rng *rand.Rand) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), 8+rng.Intn(13), rng.Intn(60), 0, 0, time.UTC)
}

func round2Money(v float64) float64 { return float64(int64(v*100+0.5)) / 100 }
