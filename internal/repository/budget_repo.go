package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	budgetdomain "github.com/yourusername/astra-backend/internal/domain/budget"
)

// BudgetRepository owns the budget/session/settings tables (migration
// 000025). Actual spend is never stored here — the service aggregates it
// from spend_transactions via the analytics TransactionSource.
type BudgetRepository struct {
	pool *pgxpool.Pool
}

func NewBudgetRepository(pool *pgxpool.Pool) *BudgetRepository {
	return &BudgetRepository{pool: pool}
}

var ErrBudgetSessionNotFound = errors.New("budget session not found")

// ─── Categories ─────────────────────────────────────────────────────────

func (r *BudgetRepository) ListGlobalCategories(ctx context.Context) ([]budgetdomain.GlobalCategory, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, display_name, COALESCE(description,''), COALESCE(icon,''), COALESCE(color,''), is_active, sort_order
		FROM budget_global_categories
		WHERE is_active = true
		ORDER BY sort_order, name
	`)
	if err != nil {
		return nil, fmt.Errorf("list global categories: %w", err)
	}
	defer rows.Close()

	var out []budgetdomain.GlobalCategory
	for rows.Next() {
		var c budgetdomain.GlobalCategory
		if err := rows.Scan(&c.ID, &c.Name, &c.DisplayName, &c.Description, &c.Icon, &c.Color, &c.IsActive, &c.SortOrder); err != nil {
			return nil, fmt.Errorf("scan global category: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CategoryMeta is the display metadata joined onto budget rows.
type CategoryMeta struct {
	ID          uuid.UUID
	Name        string
	DisplayName string
	Icon        string
	Color       string
}

// CategoryIndex maps every lower-cased lookup key (canonical name, display
// name, and each configured alias) to its category. Built from
// budget_global_categories so the taxonomy lives in data, not code.
func (r *BudgetRepository) CategoryIndex(ctx context.Context) (map[string]CategoryMeta, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, name, display_name, COALESCE(icon,''), COALESCE(color,''), aliases
		FROM budget_global_categories
	`)
	if err != nil {
		return nil, fmt.Errorf("category index: %w", err)
	}
	defer rows.Close()

	idx := map[string]CategoryMeta{}
	for rows.Next() {
		var m CategoryMeta
		var aliases []string
		if err := rows.Scan(&m.ID, &m.Name, &m.DisplayName, &m.Icon, &m.Color, &aliases); err != nil {
			return nil, fmt.Errorf("scan category index: %w", err)
		}
		idx[strings.ToLower(m.Name)] = m
		idx[strings.ToLower(m.DisplayName)] = m
		for _, a := range aliases {
			idx[strings.ToLower(strings.TrimSpace(a))] = m
		}
	}
	return idx, rows.Err()
}

// CategoryByName resolves a category by canonical name, display name or alias
// (case-insensitive). Unknown names fall back to "other" so finalize never
// drops an allocation.
func (r *BudgetRepository) CategoryByName(ctx context.Context, name string) (CategoryMeta, error) {
	idx, err := r.CategoryIndex(ctx)
	if err != nil {
		return CategoryMeta{}, err
	}
	if m, ok := idx[strings.ToLower(strings.TrimSpace(name))]; ok {
		return m, nil
	}
	if m, ok := idx["other"]; ok {
		return m, nil
	}
	return CategoryMeta{}, fmt.Errorf("no categories configured")
}

// ─── Setup sessions ─────────────────────────────────────────────────────

// SessionRow is the persisted wizard state.
type SessionRow struct {
	ID          uuid.UUID
	Month       string
	TotalBudget *float64
	Allocations []budgetdomain.CategoryAllocation
	Status      string
	ExpiresAt   time.Time
}

func scanSession(row pgx.Row) (SessionRow, error) {
	var s SessionRow
	var allocRaw []byte
	if err := row.Scan(&s.ID, &s.Month, &s.TotalBudget, &allocRaw, &s.Status, &s.ExpiresAt); err != nil {
		return SessionRow{}, err
	}
	if len(allocRaw) > 0 {
		_ = json.Unmarshal(allocRaw, &s.Allocations)
	}
	if s.Allocations == nil {
		s.Allocations = []budgetdomain.CategoryAllocation{}
	}
	return s, nil
}

// ExpirePendingSessions abandons any still-pending session for the month so
// each wizard run starts clean.
func (r *BudgetRepository) ExpirePendingSessions(ctx context.Context, userID uuid.UUID, month string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE budget_setup_sessions
		SET status = 'abandoned', updated_at = now()
		WHERE user_id = $1 AND month = $2 AND status = 'pending'
	`, userID, month)
	return err
}

func (r *BudgetRepository) CreateSession(ctx context.Context, userID uuid.UUID, month string, expiresAt time.Time) (SessionRow, error) {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO budget_setup_sessions (user_id, month, expires_at)
		VALUES ($1, $2, $3)
		RETURNING id, month, total_budget, category_allocations, status, expires_at
	`, userID, month, expiresAt)
	return scanSession(row)
}

func (r *BudgetRepository) GetSession(ctx context.Context, userID, sessionID uuid.UUID) (SessionRow, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, month, total_budget, category_allocations, status, expires_at
		FROM budget_setup_sessions
		WHERE id = $1 AND user_id = $2
	`, sessionID, userID)
	s, err := scanSession(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return SessionRow{}, ErrBudgetSessionNotFound
	}
	return s, err
}

func (r *BudgetRepository) UpdateSession(ctx context.Context, userID, sessionID uuid.UUID, totalBudget *float64, allocations []budgetdomain.CategoryAllocation) (SessionRow, error) {
	allocRaw, _ := json.Marshal(allocations)
	row := r.pool.QueryRow(ctx, `
		UPDATE budget_setup_sessions
		SET total_budget = COALESCE($3, total_budget),
		    category_allocations = COALESCE($4, category_allocations),
		    updated_at = now()
		WHERE id = $1 AND user_id = $2
		RETURNING id, month, total_budget, category_allocations, status, expires_at
	`, sessionID, userID, totalBudget, nullableJSON(allocations, allocRaw))
	s, err := scanSession(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return SessionRow{}, ErrBudgetSessionNotFound
	}
	return s, err
}

// CreateDraftSession creates a pre-filled session in a non-'pending' state
// (e.g. 'rollover_draft') so the user can review + finalize it.
func (r *BudgetRepository) CreateDraftSession(ctx context.Context, userID uuid.UUID, month, status string,
	total float64, allocs []budgetdomain.CategoryAllocation, expiresAt time.Time) (SessionRow, error) {
	allocRaw, _ := json.Marshal(allocs)
	row := r.pool.QueryRow(ctx, `
		INSERT INTO budget_setup_sessions (user_id, month, total_budget, category_allocations, status, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, month, total_budget, category_allocations, status, expires_at
	`, userID, month, total, allocRaw, status, expiresAt)
	return scanSession(row)
}

// LatestSessionByStatus returns the most recent session for the user in the
// given status, if any.
func (r *BudgetRepository) LatestSessionByStatus(ctx context.Context, userID uuid.UUID, status string) (SessionRow, bool, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, month, total_budget, category_allocations, status, expires_at
		FROM budget_setup_sessions
		WHERE user_id = $1 AND status = $2
		ORDER BY created_at DESC LIMIT 1
	`, userID, status)
	s, err := scanSession(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return SessionRow{}, false, nil
	}
	if err != nil {
		return SessionRow{}, false, err
	}
	return s, true, nil
}

// SessionExistsForMonth reports whether the user has a session for month in
// any of the given statuses.
func (r *BudgetRepository) SessionExistsForMonth(ctx context.Context, userID uuid.UUID, month string, statuses []string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM budget_setup_sessions
			WHERE user_id = $1 AND month = $2 AND status = ANY($3)
		)
	`, userID, month, statuses).Scan(&exists)
	return exists, err
}

// UsersWithActiveBudgets lists every user that currently has an active budget.
func (r *BudgetRepository) UsersWithActiveBudgets(ctx context.Context) ([]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx, `SELECT DISTINCT user_id FROM budgets WHERE is_active = true`)
	if err != nil {
		return nil, fmt.Errorf("users with active budgets: %w", err)
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (r *BudgetRepository) MarkSessionFinalized(ctx context.Context, userID, sessionID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE budget_setup_sessions SET status = 'finalized', updated_at = now()
		WHERE id = $1 AND user_id = $2
	`, sessionID, userID)
	return err
}

func (r *BudgetRepository) DeleteSession(ctx context.Context, userID, sessionID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE budget_setup_sessions SET status = 'abandoned', updated_at = now()
		WHERE id = $1 AND user_id = $2
	`, sessionID, userID)
	return err
}

func nullableJSON(allocs []budgetdomain.CategoryAllocation, raw []byte) interface{} {
	if allocs == nil {
		return nil
	}
	return raw
}

// ─── Budgets ───────────────────────────────────────────────────────────

// BudgetRow is one persisted category budget joined to its category meta.
type BudgetRow struct {
	ID             uuid.UUID
	CategoryID     uuid.UUID
	CategoryType   string
	CategoryName   string
	CategoryIcon   string
	CategoryColor  string
	Year           int
	Month          int
	BudgetedAmount float64
	IsActive       bool
	IsHidden       bool
	Notes          string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// FinalizedAllocation is a tracked allocation ready to be written as a budget row.
type FinalizedAllocation struct {
	CategoryID uuid.UUID
	Amount     float64
	IsHidden   bool
}

// ReplaceMonth atomically deactivates the month's existing budgets and
// inserts the given allocations fresh (atomic month replace).
func (r *BudgetRepository) ReplaceMonth(ctx context.Context, userID uuid.UUID, year, month int, allocs []FinalizedAllocation) error {
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			UPDATE budgets SET is_active = false, updated_at = now()
			WHERE user_id = $1 AND year = $2 AND month = $3 AND is_active = true
		`, userID, year, month); err != nil {
			return fmt.Errorf("deactivate month: %w", err)
		}
		for _, a := range allocs {
			if _, err := tx.Exec(ctx, `
				INSERT INTO budgets (user_id, category_id, category_type, year, month, budgeted_amount, is_active, is_hidden)
				VALUES ($1, $2, 'global', $3, $4, $5, true, $6)
				ON CONFLICT (user_id, category_id, category_type, year, month)
				DO UPDATE SET budgeted_amount = EXCLUDED.budgeted_amount,
				              is_active = true, is_hidden = EXCLUDED.is_hidden, updated_at = now()
			`, userID, a.CategoryID, year, month, a.Amount, a.IsHidden); err != nil {
				return fmt.Errorf("insert budget: %w", err)
			}
		}
		return nil
	})
}

// LatestBudgets returns active budget rows for the user's most recent
// (year, month), newest first, with category meta joined.
func (r *BudgetRepository) LatestBudgets(ctx context.Context, userID uuid.UUID, limit int) ([]BudgetRow, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	rows, err := r.pool.Query(ctx, `
		WITH latest AS (
			SELECT year, month FROM budgets
			WHERE user_id = $1 AND is_active = true
			ORDER BY year DESC, month DESC
			LIMIT 1
		)
		SELECT b.id, b.category_id, b.category_type,
		       COALESCE(gc.display_name, ''), COALESCE(gc.icon, ''), COALESCE(gc.color, ''),
		       b.year, b.month, b.budgeted_amount, b.is_active, b.is_hidden, COALESCE(b.notes, ''),
		       b.created_at, b.updated_at
		FROM budgets b
		JOIN latest l ON b.year = l.year AND b.month = l.month
		LEFT JOIN budget_global_categories gc ON gc.id = b.category_id
		WHERE b.user_id = $1 AND b.is_active = true
		ORDER BY b.updated_at DESC
		LIMIT $2
	`, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("latest budgets: %w", err)
	}
	defer rows.Close()

	var out []BudgetRow
	for rows.Next() {
		var b BudgetRow
		if err := rows.Scan(&b.ID, &b.CategoryID, &b.CategoryType, &b.CategoryName, &b.CategoryIcon, &b.CategoryColor,
			&b.Year, &b.Month, &b.BudgetedAmount, &b.IsActive, &b.IsHidden, &b.Notes, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan budget row: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *BudgetRepository) PatchCategoryAmount(ctx context.Context, userID, categoryID uuid.UUID, amount float64) (bool, error) {
	tag, err := r.pool.Exec(ctx, `
		WITH latest AS (
			SELECT year, month FROM budgets
			WHERE user_id = $1 AND is_active = true
			ORDER BY year DESC, month DESC LIMIT 1
		)
		UPDATE budgets b SET budgeted_amount = $3, updated_at = now()
		FROM latest l
		WHERE b.user_id = $1 AND b.category_id = $2 AND b.year = l.year AND b.month = l.month AND b.is_active = true
	`, userID, categoryID, amount)
	if err != nil {
		return false, fmt.Errorf("patch category amount: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

func (r *BudgetRepository) DeactivateMonth(ctx context.Context, userID uuid.UUID, year, month int) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE budgets SET is_active = false, updated_at = now()
		WHERE user_id = $1 AND year = $2 AND month = $3 AND is_active = true
	`, userID, year, month)
	return err
}

// ─── Settings ──────────────────────────────────────────────────────────

type SettingsRow struct {
	LinkedIncome        *float64
	SpendingLimit       *float64
	EssentialCategories []string
	Bills               []string
	LastResetAt         *time.Time
	Found               bool
}

func (r *BudgetRepository) GetSettings(ctx context.Context, userID uuid.UUID) (SettingsRow, error) {
	var s SettingsRow
	var essRaw, billRaw []byte
	err := r.pool.QueryRow(ctx, `
		SELECT linked_income, spending_limit, essential_categories, bills, last_reset_at
		FROM budget_settings WHERE user_id = $1
	`, userID).Scan(&s.LinkedIncome, &s.SpendingLimit, &essRaw, &billRaw, &s.LastResetAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return SettingsRow{Found: false}, nil
	}
	if err != nil {
		return SettingsRow{}, fmt.Errorf("get settings: %w", err)
	}
	s.Found = true
	_ = json.Unmarshal(essRaw, &s.EssentialCategories)
	_ = json.Unmarshal(billRaw, &s.Bills)
	return s, nil
}

// UpsertSettings applies a partial patch. Nil pointers/slices are left
// untouched; non-nil values overwrite.
func (r *BudgetRepository) UpsertSettings(ctx context.Context, userID uuid.UUID, p budgetdomain.UpdateSettingsRequest) error {
	var essRaw, billRaw interface{}
	if p.EssentialCategories != nil {
		b, _ := json.Marshal(*p.EssentialCategories)
		essRaw = b
	}
	if p.Bills != nil {
		b, _ := json.Marshal(*p.Bills)
		billRaw = b
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO budget_settings (user_id, linked_income, spending_limit, essential_categories, bills)
		VALUES ($1, $2, $3, COALESCE($4, '[]'::jsonb), COALESCE($5, '[]'::jsonb))
		ON CONFLICT (user_id) DO UPDATE SET
			linked_income        = COALESCE($2, budget_settings.linked_income),
			spending_limit       = COALESCE($3, budget_settings.spending_limit),
			essential_categories = COALESCE($4, budget_settings.essential_categories),
			bills                = COALESCE($5, budget_settings.bills),
			updated_at = now()
	`, userID, p.LinkedIncome, p.SpendingLimit, essRaw, billRaw)
	return err
}

// SetSpendingLimit + stamp last_reset_at = now() (finalize).
func (r *BudgetRepository) SetSpendingLimitAndActivate(ctx context.Context, userID uuid.UUID, limit float64) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO budget_settings (user_id, spending_limit, last_reset_at)
		VALUES ($1, $2, now())
		ON CONFLICT (user_id) DO UPDATE SET
			spending_limit = $2, last_reset_at = now(), updated_at = now()
	`, userID, limit)
	return err
}

// ClearActive nulls last_reset_at (reset) — marks "no active budget".
func (r *BudgetRepository) ClearActive(ctx context.Context, userID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE budget_settings SET last_reset_at = NULL, updated_at = now() WHERE user_id = $1
	`, userID)
	return err
}
