package mf

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"math/rand"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourusername/astra-backend/internal/apiresponse"
	"github.com/yourusername/astra-backend/internal/apitime"
	mfdomain "github.com/yourusername/astra-backend/internal/domain/mf"
)

// MockProvider is the stateful mock implementation of Provider: it persists
// realistic per-user folios/transactions to Postgres against the real
// fund_catalog reference data (so NAVs, categories, AMC names etc. are
// consistent with what the Catalog/Explore endpoints already show), and
// simulates day-to-day NAV movement the same way the Stocks mock simulates
// live equity prices.
type MockProvider struct {
	pool *pgxpool.Pool
}

func NewMockProvider(pool *pgxpool.Pool) *MockProvider {
	return &MockProvider{pool: pool}
}

// navOnDate derives a small, deterministic day-to-day NAV move around a
// fund's catalog NAV. Real mutual fund NAVs are struck once per business
// day (unlike a live-ticking equity price), so the jitter bucket here is a
// calendar day, not a 30-second window like the Stocks mock's livePrice.
func navOnDate(schemeCode string, baseNAV float64, date time.Time) float64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(schemeCode))
	bucket := date.UTC().Truncate(24*time.Hour).Unix() / 86400
	r := rand.New(rand.NewSource(int64(h.Sum64()) + bucket)) //nolint:gosec // mock market data, not security-sensitive
	pctMove := (r.Float64() - 0.5) * 0.02                    // +/- 1% day-to-day NAV drift
	return round4(baseNAV * (1 + pctMove))
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
func round4(v float64) float64 { return math.Round(v*10000) / 10000 }

func newTxnDate() time.Time { return time.Now().UTC() }

// seedProfile describes one fund this mock seeds a new user into, spanning
// a mix of categories and holding durations so returns/XIRR differ
// meaningfully across holdings instead of all looking identical.
type seedProfile struct {
	schemeCode string
	costValue  float64
	daysHeld   int
	isSIP      bool
}

var seedProfiles = []seedProfile{
	{"HDFC-MC-G", 118000, 520, true},
	{"SBI-BLC-G", 82000, 340, false},
	{"PARAG-FLX-G", 96000, 260, true},
	{"KOTAK-GOLD-G", 31000, 180, false},
}

// seedFolios lazily seeds a new user's starting mutual-fund portfolio the
// first time their holdings are read, mirroring the Stocks domain's
// seedHoldings pattern. Idempotent via the (user_id, scheme_code) unique
// constraint (migration 000011).
func (p *MockProvider) seedFolios(ctx context.Context, userID uuid.UUID) error {
	for _, sp := range seedProfiles {
		var baseNAV float64
		var amcName, schemeName, isin, category string
		err := p.pool.QueryRow(ctx, `SELECT amc_name, scheme_name, isin, category, nav FROM fund_catalog WHERE scheme_code = $1`, sp.schemeCode).
			Scan(&amcName, &schemeName, &isin, &category, &baseNAV)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				continue // catalog entry missing; skip rather than fail the whole seed
			}
			return fmt.Errorf("lookup seed fund %s: %w", sp.schemeCode, err)
		}

		purchaseDate := time.Now().UTC().AddDate(0, 0, -sp.daysHeld)
		purchaseNAV := navOnDate(sp.schemeCode, baseNAV, purchaseDate)
		units := round4(sp.costValue / purchaseNAV)
		folioNumber := fmt.Sprintf("FOLIO%d", time.Now().UTC().UnixNano()%1_000_000_000)

		var folioID uuid.UUID
		err = p.pool.QueryRow(ctx, `
			INSERT INTO mf_folios (user_id, folio_number, amc_name, scheme_code, scheme_name, isin, units_held, nav, nav_date, cost_value, category, plan_type, is_sip, created_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'GROWTH',$12,$13)
			ON CONFLICT (user_id, scheme_code) DO NOTHING
			RETURNING id
		`, userID, folioNumber, amcName, sp.schemeCode, schemeName, isin, units, purchaseNAV, purchaseDate, sp.costValue, category, sp.isSIP, purchaseDate).Scan(&folioID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				continue // ON CONFLICT DO NOTHING: already seeded for this user
			}
			return fmt.Errorf("seed folio %s: %w", sp.schemeCode, err)
		}

		if _, err := p.pool.Exec(ctx, `
			INSERT INTO mf_transactions (folio_id, transaction_type, transaction_date, amount, units, price)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, folioID, mfdomain.TxnPurchase, purchaseDate, sp.costValue, units, purchaseNAV); err != nil {
			return fmt.Errorf("seed purchase txn for %s: %w", sp.schemeCode, err)
		}
	}
	return nil
}

type folioRow struct {
	folioNumber, amcName, schemeCode, schemeName, isin, category, planType string
	isSIP                                                                  bool
	unitsHeld, costValue, baseNAV                                          float64
	createdAt                                                              time.Time
}

func (p *MockProvider) GetHoldings(ctx context.Context, userID uuid.UUID) (*mfdomain.HoldingsResult, error) {
	if err := p.seedFolios(ctx, userID); err != nil {
		return nil, err
	}

	// Joins fund_catalog directly rather than looking up each folio's NAV in
	// a separate query afterward — avoids an N+1 query pattern here.
	rows, err := p.pool.Query(ctx, `
		SELECT f.folio_number, f.amc_name, f.scheme_code, f.scheme_name, f.isin, f.category, f.plan_type, f.is_sip, f.units_held, f.cost_value, f.created_at, c.nav
		FROM mf_folios f JOIN fund_catalog c ON c.scheme_code = f.scheme_code
		WHERE f.user_id = $1 AND f.units_held > 0
		ORDER BY f.scheme_name
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("query mf holdings: %w", err)
	}

	type folioWithNAV struct {
		row     folioRow
		baseNAV float64
	}
	var rowsData []folioWithNAV
	for rows.Next() {
		var fr folioWithNAV
		if err := rows.Scan(&fr.row.folioNumber, &fr.row.amcName, &fr.row.schemeCode, &fr.row.schemeName, &fr.row.isin, &fr.row.category, &fr.row.planType, &fr.row.isSIP, &fr.row.unitsHeld, &fr.row.costValue, &fr.row.createdAt, &fr.baseNAV); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan mf holding: %w", err)
		}
		rowsData = append(rowsData, fr)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate mf holdings: %w", err)
	}
	rows.Close()

	result := &mfdomain.HoldingsResult{Folios: make([]mfdomain.Folio, 0, len(rowsData))}
	now := time.Now().UTC()

	for _, fr := range rowsData {
		f := computeFolio(fr.row, fr.baseNAV, now)
		result.Folios = append(result.Folios, f)
		result.Summary.InvestedValue += f.InvestedValue
		result.Summary.CurrentValue += f.CurrentValue
		result.Summary.OneDayChangeAmount += f.OneDayChangeAmount
	}

	result.Summary.InvestedValue = round2(result.Summary.InvestedValue)
	result.Summary.CurrentValue = round2(result.Summary.CurrentValue)
	result.Summary.OneDayChangeAmount = round2(result.Summary.OneDayChangeAmount)
	result.Summary.ReturnsAmount = round2(result.Summary.CurrentValue - result.Summary.InvestedValue)
	result.Summary.FolioCount = len(result.Folios)
	if result.Summary.InvestedValue > 0 {
		result.Summary.ReturnsPct = round2(result.Summary.ReturnsAmount / result.Summary.InvestedValue * 100)
	}
	prevValue := result.Summary.CurrentValue - result.Summary.OneDayChangeAmount
	if prevValue > 0 {
		result.Summary.OneDayChangePct = round2(result.Summary.OneDayChangeAmount / prevValue * 100)
	}
	if len(result.Folios) > 0 {
		var weightedXIRR float64
		for _, f := range result.Folios {
			if result.Summary.CurrentValue > 0 {
				weightedXIRR += f.XIRRPct * (f.CurrentValue / result.Summary.CurrentValue)
			}
		}
		result.Summary.XIRRPct = round2(weightedXIRR)
	}

	return result, nil
}

// computeFolio derives a Folio's live-view fields (current value, returns,
// one-day change, approximate XIRR) from its persisted state. Shared by
// GetHoldings (list) and GetHolding (single-scheme lookup) so the two never
// drift into computing "current value" differently.
func computeFolio(fr folioRow, baseNAV float64, now time.Time) mfdomain.Folio {
	yesterday := now.AddDate(0, 0, -1)
	todayNAV := navOnDate(fr.schemeCode, baseNAV, now)
	yesterdayNAV := navOnDate(fr.schemeCode, baseNAV, yesterday)
	currentValue := round2(fr.unitsHeld * todayNAV)
	yesterdayValue := round2(fr.unitsHeld * yesterdayNAV)

	f := mfdomain.Folio{
		FolioNumber:        fr.folioNumber,
		AMCName:            fr.amcName,
		SchemeCode:         fr.schemeCode,
		SchemeName:         fr.schemeName,
		ISIN:               fr.isin,
		Category:           fr.category,
		PlanType:           fr.planType,
		IsSIP:              fr.isSIP,
		UnitsHeld:          fr.unitsHeld,
		NAV:                todayNAV,
		NAVDate:            apitime.New(now),
		InvestedValue:      round2(fr.costValue),
		CurrentValue:       currentValue,
		OneDayChangeAmount: round2(currentValue - yesterdayValue),
		FirstPurchaseDate:  apitime.New(fr.createdAt),
	}
	f.ReturnsAmount = round2(f.CurrentValue - f.InvestedValue)
	if f.InvestedValue > 0 {
		f.ReturnsPct = round2(f.ReturnsAmount / f.InvestedValue * 100)
	}
	if yesterdayValue > 0 {
		f.OneDayChangePct = round2(f.OneDayChangeAmount / yesterdayValue * 100)
	}
	f.XIRRPct = approxAnnualizedReturnPct(f.InvestedValue, f.CurrentValue, fr.createdAt, now)
	return f
}

func (p *MockProvider) GetHolding(ctx context.Context, userID uuid.UUID, schemeCode string) (*mfdomain.Folio, error) {
	var fr folioRow
	var baseNAV float64
	err := p.pool.QueryRow(ctx, `
		SELECT f.folio_number, f.amc_name, f.scheme_code, f.scheme_name, f.isin, f.category, f.plan_type, f.is_sip, f.units_held, f.cost_value, f.created_at, c.nav
		FROM mf_folios f JOIN fund_catalog c ON c.scheme_code = f.scheme_code
		WHERE f.user_id = $1 AND f.scheme_code = $2 AND f.units_held > 0
	`, userID, schemeCode).Scan(&fr.folioNumber, &fr.amcName, &fr.schemeCode, &fr.schemeName, &fr.isin, &fr.category, &fr.planType, &fr.isSIP, &fr.unitsHeld, &fr.costValue, &fr.createdAt, &baseNAV)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("lookup mf holding for %s: %w", schemeCode, err)
	}
	f := computeFolio(fr, baseNAV, time.Now().UTC())
	return &f, nil
}

// approxAnnualizedReturnPct estimates a money-weighted annualized return for
// a single lumpsum-equivalent holding: ((current/invested)^(365/daysHeld) - 1) * 100.
// This is a simplification, not a true XIRR solve (which needs the full
// cashflow schedule — every individual SIP installment's date and amount);
// it's a reasonable stand-in for a single-purchase mock holding and is
// clearly documented as such rather than presented as a precise XIRR.
func approxAnnualizedReturnPct(invested, current float64, since, now time.Time) float64 {
	daysHeld := now.Sub(since).Hours() / 24
	if invested <= 0 || current <= 0 || daysHeld < 1 {
		return 0
	}
	ratio := current / invested
	annualized := math.Pow(ratio, 365.0/daysHeld) - 1
	return round2(annualized * 100)
}

func (p *MockProvider) Purchase(ctx context.Context, userID uuid.UUID, req mfdomain.PurchaseRequest) (*mfdomain.Transaction, error) {
	if req.SchemeCode == "" {
		return nil, apiresponse.Validation("scheme_code is required")
	}
	if req.Amount <= 0 {
		return nil, apiresponse.Validation("amount must be positive")
	}

	var amcName, schemeName, isin, category string
	var baseNAV, minInvestment, minSIP float64
	err := p.pool.QueryRow(ctx, `SELECT amc_name, scheme_name, isin, category, nav, min_investment, min_sip_amount FROM fund_catalog WHERE scheme_code = $1`, req.SchemeCode).
		Scan(&amcName, &schemeName, &isin, &category, &baseNAV, &minInvestment, &minSIP)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apiresponse.NotFound("fund %s not found", req.SchemeCode)
		}
		return nil, fmt.Errorf("lookup fund: %w", err)
	}
	minRequired := minInvestment
	if req.IsSIP {
		minRequired = minSIP
	}
	if req.Amount < minRequired {
		return nil, apiresponse.Validation("amount must be at least %.2f for this fund", minRequired)
	}

	now := newTxnDate()
	price := navOnDate(req.SchemeCode, baseNAV, now)
	units := round4(req.Amount / price)

	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin purchase tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var folioID uuid.UUID
	err = tx.QueryRow(ctx, `SELECT id FROM mf_folios WHERE user_id = $1 AND scheme_code = $2 FOR UPDATE`, userID, req.SchemeCode).Scan(&folioID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		folioNumber := fmt.Sprintf("FOLIO%d", time.Now().UTC().UnixNano()%1_000_000_000)
		if err := tx.QueryRow(ctx, `
			INSERT INTO mf_folios (user_id, folio_number, amc_name, scheme_code, scheme_name, isin, units_held, nav, nav_date, cost_value, category, plan_type, is_sip, created_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'GROWTH',$12,$13)
			RETURNING id
		`, userID, folioNumber, amcName, req.SchemeCode, schemeName, isin, units, price, now, req.Amount, category, req.IsSIP, now).Scan(&folioID); err != nil {
			return nil, fmt.Errorf("create folio: %w", err)
		}
	case err != nil:
		return nil, fmt.Errorf("lock folio: %w", err)
	default:
		isSIP := req.IsSIP
		if _, err := tx.Exec(ctx, `
			UPDATE mf_folios SET
				units_held = units_held + $1, cost_value = cost_value + $2,
				nav = $3, nav_date = $4, is_sip = is_sip OR $5, updated_at = now()
			WHERE id = $6
		`, units, req.Amount, price, now, isSIP, folioID); err != nil {
			return nil, fmt.Errorf("update folio on purchase: %w", err)
		}
	}

	txnType := mfdomain.TxnPurchase
	if req.IsSIP {
		txnType = mfdomain.TxnSIP
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO mf_transactions (folio_id, transaction_type, transaction_date, amount, units, price)
		VALUES ($1,$2,$3,$4,$5,$6)
	`, folioID, txnType, now, req.Amount, units, price); err != nil {
		return nil, fmt.Errorf("insert purchase txn: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit purchase: %w", err)
	}

	return &mfdomain.Transaction{
		SchemeCode: req.SchemeCode, SchemeName: schemeName, TransactionType: txnType,
		TransactionDate: apitime.New(now), Amount: round2(req.Amount), Units: units, Price: price,
	}, nil
}

func (p *MockProvider) Redeem(ctx context.Context, userID uuid.UUID, req mfdomain.RedeemRequest) (*mfdomain.RedeemResult, error) {
	if req.SchemeCode == "" {
		return nil, apiresponse.Validation("scheme_code is required")
	}

	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin redeem tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var folioID uuid.UUID
	var unitsHeld, costValue, baseNAV float64
	err = tx.QueryRow(ctx, `
		SELECT f.id, f.units_held, f.cost_value, c.nav
		FROM mf_folios f JOIN fund_catalog c ON c.scheme_code = f.scheme_code
		WHERE f.user_id = $1 AND f.scheme_code = $2 FOR UPDATE
	`, userID, req.SchemeCode).Scan(&folioID, &unitsHeld, &costValue, &baseNAV)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apiresponse.NotFound("no holding found for %s", req.SchemeCode)
		}
		return nil, fmt.Errorf("lock folio for redeem: %w", err)
	}

	redeemUnits := unitsHeld
	if req.Units != nil {
		redeemUnits = *req.Units
	}
	if redeemUnits <= 0 || redeemUnits > unitsHeld {
		return nil, apiresponse.Validation("units to redeem must be between 0 and %.4f", unitsHeld)
	}

	now := newTxnDate()
	price := navOnDate(req.SchemeCode, baseNAV, now)
	amount := round2(redeemUnits * price)

	// Reduce cost_value proportionally so the remaining holding's invested
	// value (and therefore returns/XIRR) still reflects only what's left.
	remainingUnits := round4(unitsHeld - redeemUnits)
	remainingCostValue := costValue
	if unitsHeld > 0 {
		remainingCostValue = round2(costValue * (remainingUnits / unitsHeld))
	}

	if _, err := tx.Exec(ctx, `
		UPDATE mf_folios SET units_held = $1, cost_value = $2, nav = $3, nav_date = $4, updated_at = now()
		WHERE id = $5
	`, remainingUnits, remainingCostValue, price, now, folioID); err != nil {
		return nil, fmt.Errorf("update folio on redeem: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO mf_transactions (folio_id, transaction_type, transaction_date, amount, units, price)
		VALUES ($1,$2,$3,$4,$5,$6)
	`, folioID, mfdomain.TxnRedeem, now, amount, redeemUnits, price); err != nil {
		return nil, fmt.Errorf("insert redeem txn: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit redeem: %w", err)
	}

	return &mfdomain.RedeemResult{
		SchemeCode: req.SchemeCode, UnitsRedeemed: redeemUnits, Amount: amount,
		RemainingUnits: remainingUnits, Status: mfdomain.RedeemStatusSuccess,
	}, nil
}

func (p *MockProvider) GetTransactions(ctx context.Context, userID uuid.UUID, schemeCode string) ([]mfdomain.Transaction, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT f.scheme_code, f.scheme_name, t.transaction_type, t.transaction_date, t.amount, t.units, t.price
		FROM mf_transactions t JOIN mf_folios f ON f.id = t.folio_id
		WHERE f.user_id = $1 AND ($2 = '' OR f.scheme_code = $2)
		ORDER BY t.transaction_date DESC
	`, userID, schemeCode)
	if err != nil {
		return nil, fmt.Errorf("list mf transactions: %w", err)
	}
	defer rows.Close()

	txns := make([]mfdomain.Transaction, 0)
	for rows.Next() {
		var t mfdomain.Transaction
		var date time.Time
		if err := rows.Scan(&t.SchemeCode, &t.SchemeName, &t.TransactionType, &date, &t.Amount, &t.Units, &t.Price); err != nil {
			return nil, fmt.Errorf("scan mf transaction: %w", err)
		}
		t.TransactionDate = apitime.New(date)
		txns = append(txns, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate mf transactions: %w", err)
	}
	return txns, nil
}
