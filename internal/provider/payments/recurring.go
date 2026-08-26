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

// maxCatchUpCycles bounds how many missed billing cycles processDueMandates
// will back-fill in one call, so a mandate nobody has touched in years can't
// turn a single request into an unbounded loop.
const maxCatchUpCycles = 36

// processDueMandates catches up every ACTIVE mandate belonging to userID
// whose next_debit_date has already passed: each due cycle is recorded as a
// mandate_executions row (SUCCESS if the linked bank account has sufficient
// balance, FAILED otherwise) and next_debit_date is advanced accordingly.
// This is the same lazy, idempotent-on-read pattern used elsewhere in this
// backend (holdings, spend analytics) rather than a background scheduler —
// there is no cron/worker infra in this deployment, so "time has passed"
// facts are settled the moment something reads them.
func (p *MockProvider) processDueMandates(ctx context.Context, userID uuid.UUID) error {
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
		if err := p.catchUpMandate(ctx, userID, mandateID); err != nil {
			return err
		}
	}
	return nil
}

// catchUpMandate processes one mandate's missed cycles, one transaction per
// cycle so each execution is durably recorded even if a later cycle in the
// same catch-up run fails.
func (p *MockProvider) catchUpMandate(ctx context.Context, userID uuid.UUID, mandateID string) error {
	for i := 0; i < maxCatchUpCycles; i++ {
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

func (p *MockProvider) MandateHistory(ctx context.Context, userID uuid.UUID, mandateID string) ([]paymentsdomain.MandateExecution, error) {
	if err := p.processDueMandates(ctx, userID); err != nil {
		return nil, err
	}

	var exists bool
	if err := p.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM mandates WHERE mandate_id = $1 AND user_id = $2)`, mandateID, userID).Scan(&exists); err != nil {
		return nil, fmt.Errorf("verify mandate ownership: %w", err)
	}
	if !exists {
		return nil, apiresponse.NotFound("mandate %s not found", mandateID)
	}

	rows, err := p.pool.Query(ctx, `
		SELECT scheduled_date, amount, status, failure_reason, executed_at
		FROM mandate_executions
		WHERE mandate_id = $1 AND user_id = $2
		ORDER BY scheduled_date DESC
	`, mandateID, userID)
	if err != nil {
		return nil, fmt.Errorf("list mandate executions: %w", err)
	}
	defer rows.Close()

	executions := make([]paymentsdomain.MandateExecution, 0)
	for rows.Next() {
		var scheduled, executedAt time.Time
		var e paymentsdomain.MandateExecution
		if err := rows.Scan(&scheduled, &e.Amount, &e.Status, &e.FailureReason, &executedAt); err != nil {
			return nil, fmt.Errorf("scan mandate execution: %w", err)
		}
		e.ScheduledDate = apitime.New(scheduled)
		e.ExecutedAt = apitime.New(executedAt)
		executions = append(executions, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate mandate executions: %w", err)
	}
	return executions, nil
}

// RecurringSummary backs the Recurring screen's stat tiles: mandates due in
// the next 7 days (upcoming), still ACTIVE but somehow past due — meaning a
// concurrent read hasn't caught them up yet, since processDueMandates just
// ran above and should have already cleared these (overdue), and executions
// settled so far this calendar month (paid).
func (p *MockProvider) RecurringSummary(ctx context.Context, userID uuid.UUID) (paymentsdomain.RecurringSummary, error) {
	if err := p.processDueMandates(ctx, userID); err != nil {
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
