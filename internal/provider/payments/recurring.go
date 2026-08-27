package payments

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/yourusername/astra-backend/internal/apiresponse"
	"github.com/yourusername/astra-backend/internal/apitime"
	paymentsdomain "github.com/yourusername/astra-backend/internal/domain/payments"
)

// nextDebitDate advances a mandate's debit date by one billing cycle. Pure
// and deterministic so it's directly unit-testable without a database.
func nextDebitDate(from time.Time, frequency string) time.Time {
	switch frequency {
	case paymentsdomain.FrequencyQuarterly:
		return from.AddDate(0, 3, 0)
	case paymentsdomain.FrequencyYearly:
		return from.AddDate(1, 0, 0)
	default: // MONTHLY, and any unrecognized value falls back to monthly
		return from.AddDate(0, 1, 0)
	}
}

// demoSubscription describes one starter subscription mandate seeded for a
// new user, mirroring the Stocks/MF domains' lazy-seed-on-first-read
// pattern so the Recurring screen ("Track your bills") isn't empty on first
// login. daysAgo backdates the mandate's start so processDueMandates (run
// right after seeding) immediately produces real execution history for some
// of them, while the most recent one stays upcoming rather than overdue.
type demoSubscription struct {
	payeeName, payeeVPA string
	amount              float64
	daysAgo             int
}

var demoSubscriptions = []demoSubscription{
	{"Netflix", "netflix@upi", 649, 75},
	{"YouTube Premium", "youtube@upi", 129, 45},
	{"Spotify", "spotify@upi", 119, 20},
	{"Amazon Prime", "prime@upi", 299, 60},
	{"Disney+ Hotstar", "hotstar@upi", 299, 15},
	{"Apple One", "apple@upi", 195, 10},
	{"Google One Storage", "googleone@upi", 130, 5},
	{"Airtel Fiber Broadband", "airtelfiber@upi", 999, 25},
	{"Cult.fit Pass", "cultfit@upi", 1250, 85},
}

// seedDemoSubscriptions lazily seeds a new user's starter subscription
// mandates the first time their mandates are listed. Idempotent: only runs
// when the user has zero subscription mandates, so it never re-seeds after
// the user creates/cancels their own.
func (p *MockProvider) seedDemoSubscriptions(ctx context.Context, userID uuid.UUID) error {
	var hasAny bool
	if err := p.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM mandates WHERE user_id = $1 AND category = 'SUBSCRIPTION')`, userID).Scan(&hasAny); err != nil {
		return fmt.Errorf("check existing mandates: %w", err)
	}
	if hasAny {
		return nil
	}

	bankAccount, err := p.userRepo.GetPrimaryBankAccount(ctx, userID)
	if err != nil {
		return fmt.Errorf("resolve bank account for demo subscriptions: %w", err)
	}

	now := time.Now().UTC()
	for _, ds := range demoSubscriptions {
		startDate := now.AddDate(0, 0, -ds.daysAgo)
		mandateID := newID("MNDT")
		if _, err := p.pool.Exec(ctx, `
			INSERT INTO mandates (
				mandate_id, user_id, bank_account_id, mandate_type, payee_name, payee_vpa_or_id, category,
				max_amount, frequency, mandate_start_date, next_debit_date, status, approved_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$10,$11,$12)
		`, mandateID, userID, bankAccount.ID, paymentsdomain.MandateTypeUPIAutopay, ds.payeeName, ds.payeeVPA, "SUBSCRIPTION",
			ds.amount, paymentsdomain.FrequencyMonthly, startDate, paymentsdomain.MandateStatusActive, startDate); err != nil {
			return fmt.Errorf("seed demo subscription %s: %w", ds.payeeName, err)
		}
	}
	return nil
}

// maxCatchUpCycles is the absolute ceiling on how many missed billing cycles
// processDueMandates will back-fill in one call — used on write paths
// (MandateAction) where the user just mutated the mandate and we want it
// fully up-to-date before returning.
const maxCatchUpCycles = 36

// maxCatchUpPerRead is the per-mandate cycle budget for read paths
// (ListMandates, MandateHistory, RecurringSummary). Limiting to 3 cycles
// (~3 months of catch-up) per request keeps read latency bounded: a mandate
// that is years overdue catches up gradually across successive reads rather
// than blocking a single request for seconds.
const maxCatchUpPerRead = 3

// processDueMandates catches up every ACTIVE mandate belonging to userID
// whose next_debit_date has already passed. maxCyclesPerMandate controls
// how many missed cycles are settled per mandate in this single call:
//   - pass maxCatchUpPerRead (3)  on read paths  — bounded latency
//   - pass maxCatchUpCycles  (36) on write paths — fully up-to-date
func (p *MockProvider) processDueMandates(ctx context.Context, userID uuid.UUID, maxCyclesPerMandate int) error {
	rows, err := p.pool.Query(ctx, `
		SELECT mandate_id FROM mandates
		WHERE user_id = $1 AND status = $2 AND next_debit_date <= CURRENT_DATE
	`, userID, paymentsdomain.MandateStatusActive)
	if err != nil {
		return fmt.Errorf("find due mandates: %w", err)
	}
	dueIDs := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return fmt.Errorf("scan due mandate id: %w", err)
		}
		dueIDs = append(dueIDs, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate due mandates: %w", err)
	}
	rows.Close()

	for _, mandateID := range dueIDs {
		if err := p.catchUpMandate(ctx, userID, mandateID, maxCyclesPerMandate); err != nil {
			return err
		}
	}
	return nil
}

// catchUpMandate processes one mandate's missed cycles up to maxCycles, one
// DB transaction per cycle so each execution is durably recorded even if a
// later cycle in the same run fails. It also respects context cancellation —
// if the HTTP request times out mid-loop, the next iteration exits cleanly
// rather than continuing to issue transactions against a cancelled context.
func (p *MockProvider) catchUpMandate(ctx context.Context, userID uuid.UUID, mandateID string, maxCycles int) error {
	for i := 0; i < maxCycles; i++ {
		// Honour context cancellation (e.g. 60 s HTTP timeout) between cycles.
		if ctx.Err() != nil {
			return nil
		}
		processedOne, err := p.runOneMandateCycle(ctx, userID, mandateID)
		if err != nil {
			return err
		}
		if !processedOne {
			return nil
		}
	}
	return nil
}

// runOneMandateCycle processes at most one due billing cycle for a mandate
// inside a single transaction, reports whether a cycle was actually
// processed (false means it's no longer due, e.g. paused/cancelled by a
// concurrent request, or caught up by a concurrent call already).
func (p *MockProvider) runOneMandateCycle(ctx context.Context, userID uuid.UUID, mandateID string) (bool, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin mandate cycle tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var (
		bankAccountID uuid.UUID
		status        string
		maxAmount     float64
		frequency     string
		nextDebit     time.Time
		mandateEnd    *time.Time
	)
	err = tx.QueryRow(ctx, `
		SELECT bank_account_id, status, max_amount, frequency, next_debit_date, mandate_end_date
		FROM mandates WHERE mandate_id = $1 AND user_id = $2 FOR UPDATE
	`, mandateID, userID).Scan(&bankAccountID, &status, &maxAmount, &frequency, &nextDebit, &mandateEnd)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("lock mandate for cycle: %w", err)
	}

	if status != paymentsdomain.MandateStatusActive || nextDebit.After(time.Now().UTC()) {
		return false, nil
	}

	var balance float64
	if err := tx.QueryRow(ctx, `SELECT balance FROM bank_accounts WHERE id = $1 AND user_id = $2 FOR UPDATE`, bankAccountID, userID).Scan(&balance); err != nil {
		return false, fmt.Errorf("lock bank account for mandate cycle: %w", err)
	}

	execStatus := paymentsdomain.ExecutionSuccess
	var failureReason *string
	if balance >= maxAmount {
		if _, err := tx.Exec(ctx, `UPDATE bank_accounts SET balance = balance - $1 WHERE id = $2 AND user_id = $3`, maxAmount, bankAccountID, userID); err != nil {
			return false, fmt.Errorf("debit mandate account: %w", err)
		}
	} else {
		execStatus = paymentsdomain.ExecutionFailed
		reason := "INSUFFICIENT_FUNDS"
		failureReason = &reason
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO mandate_executions (mandate_id, user_id, scheduled_date, amount, status, failure_reason)
		VALUES ($1,$2,$3,$4,$5,$6)
	`, mandateID, userID, nextDebit, maxAmount, execStatus, failureReason); err != nil {
		return false, fmt.Errorf("insert mandate execution: %w", err)
	}

	newNextDebit := nextDebitDate(nextDebit, frequency)
	newStatus := status
	if mandateEnd != nil && !newNextDebit.Before(*mandateEnd) {
		newStatus = paymentsdomain.MandateStatusExpired
	}

	if _, err := tx.Exec(ctx, `
		UPDATE mandates SET next_debit_date = $1, status = $2, updated_at = now()
		WHERE mandate_id = $3 AND user_id = $4
	`, newNextDebit, newStatus, mandateID, userID); err != nil {
		return false, fmt.Errorf("advance mandate next_debit_date: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit mandate cycle: %w", err)
	}
	return true, nil
}

func (p *MockProvider) MandateHistory(ctx context.Context, userID uuid.UUID, mandateID string, limit, offset int) (paymentsdomain.ExecutionPage, error) {
	if err := p.processDueMandates(ctx, userID, maxCatchUpPerRead); err != nil {
		return paymentsdomain.ExecutionPage{}, err
	}

	const defaultLimit = 25
	const maxLimit = 100
	if limit <= 0 {
		limit = defaultLimit
	} else if limit > maxLimit {
		limit = maxLimit
	}
	if offset < 0 {
		offset = 0
	}

	var exists bool
	if err := p.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM mandates WHERE mandate_id = $1 AND user_id = $2)`, mandateID, userID).Scan(&exists); err != nil {
		return paymentsdomain.ExecutionPage{}, fmt.Errorf("verify mandate ownership: %w", err)
	}
	if !exists {
		return paymentsdomain.ExecutionPage{}, apiresponse.NotFound("mandate %s not found", mandateID)
	}

	var total int
	if err := p.pool.QueryRow(ctx, `SELECT COUNT(*) FROM mandate_executions WHERE mandate_id = $1 AND user_id = $2`, mandateID, userID).Scan(&total); err != nil {
		return paymentsdomain.ExecutionPage{}, fmt.Errorf("count mandate executions: %w", err)
	}

	rows, err := p.pool.Query(ctx, `
		SELECT scheduled_date, amount, status, failure_reason, executed_at
		FROM mandate_executions
		WHERE mandate_id = $1 AND user_id = $2
		ORDER BY scheduled_date DESC
		LIMIT $3 OFFSET $4
	`, mandateID, userID, limit, offset)
	if err != nil {
		return paymentsdomain.ExecutionPage{}, fmt.Errorf("list mandate executions: %w", err)
	}
	defer rows.Close()

	items := make([]paymentsdomain.MandateExecution, 0, limit)
	for rows.Next() {
		var scheduled, executedAt time.Time
		var e paymentsdomain.MandateExecution
		if err := rows.Scan(&scheduled, &e.Amount, &e.Status, &e.FailureReason, &executedAt); err != nil {
			return paymentsdomain.ExecutionPage{}, fmt.Errorf("scan mandate execution: %w", err)
		}
		e.ScheduledDate = apitime.New(scheduled)
		e.ExecutedAt = apitime.New(executedAt)
		items = append(items, e)
	}
	if err := rows.Err(); err != nil {
		return paymentsdomain.ExecutionPage{}, fmt.Errorf("iterate mandate executions: %w", err)
	}
	return paymentsdomain.ExecutionPage{Items: items, Total: total, Limit: limit, Offset: offset}, nil
}


// RecurringSummary backs the Recurring screen's stat tiles: mandates due in
// the next 7 days (upcoming), still ACTIVE but somehow past due — meaning a
// concurrent read hasn't caught them up yet, since processDueMandates just
// ran above and should have already cleared these (overdue), and executions
// settled so far this calendar month (paid).
func (p *MockProvider) RecurringSummary(ctx context.Context, userID uuid.UUID) (paymentsdomain.RecurringSummary, error) {
	if err := p.processDueMandates(ctx, userID, maxCatchUpPerRead); err != nil {
		return paymentsdomain.RecurringSummary{}, err
	}

	var s paymentsdomain.RecurringSummary
	err := p.pool.QueryRow(ctx, `
		SELECT
			COALESCE(COUNT(*) FILTER (WHERE status = $2 AND next_debit_date BETWEEN CURRENT_DATE AND CURRENT_DATE + INTERVAL '7 days'), 0),
			COALESCE(SUM(max_amount) FILTER (WHERE status = $2 AND next_debit_date BETWEEN CURRENT_DATE AND CURRENT_DATE + INTERVAL '7 days'), 0),
			COALESCE(COUNT(*) FILTER (WHERE status = $2 AND next_debit_date < CURRENT_DATE), 0),
			COALESCE(SUM(max_amount) FILTER (WHERE status = $2 AND next_debit_date < CURRENT_DATE), 0)
		FROM mandates WHERE user_id = $1
	`, userID, paymentsdomain.MandateStatusActive).Scan(&s.UpcomingCount, &s.UpcomingTotal, &s.OverdueCount, &s.OverdueTotal)
	if err != nil {
		return paymentsdomain.RecurringSummary{}, fmt.Errorf("summarize upcoming/overdue mandates: %w", err)
	}

	err = p.pool.QueryRow(ctx, `
		SELECT COALESCE(COUNT(*), 0), COALESCE(SUM(amount), 0)
		FROM mandate_executions
		WHERE user_id = $1 AND status = $2 AND date_trunc('month', executed_at) = date_trunc('month', CURRENT_DATE)
	`, userID, paymentsdomain.ExecutionSuccess).Scan(&s.PaidThisMonthCount, &s.PaidThisMonthTotal)
	if err != nil {
		return paymentsdomain.RecurringSummary{}, fmt.Errorf("summarize paid mandates: %w", err)
	}

	return s, nil
}
