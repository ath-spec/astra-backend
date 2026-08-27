package repository

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourusername/astra-backend/internal/apitime"
	rmdomain "github.com/yourusername/astra-backend/internal/domain/rm"
)

// ErrNoActiveRM is returned by AssignNextRM when there are no active RMs to
// route a new user to. Callers (the signup path) log and continue — the
// user still gets created and lands in the admin's unassigned pool.
var ErrNoActiveRM = errors.New("no active RM available for assignment")

// --- Staff (RM/Admin) user repository ---

type RMUserRepository interface {
	Create(ctx context.Context, employeeCode, email, name, phone, role string, maxPortfolios int) (*rmdomain.StaffUser, error)
	GetByEmail(ctx context.Context, email string) (*rmdomain.StaffUser, error)
	// GetByIdentifier resolves a staff member by employee code (case-
	// insensitive) or email. Returns nil if neither matches.
	GetByIdentifier(ctx context.Context, identifier string) (*rmdomain.StaffUser, error)
	GetByID(ctx context.Context, id uuid.UUID) (*rmdomain.StaffUser, error)
	List(ctx context.Context) ([]rmdomain.StaffUser, error)
	Update(ctx context.Context, id uuid.UUID, req rmdomain.UpdateRMRequest) (*rmdomain.StaffUser, error)

	CreateRefreshToken(ctx context.Context, rmID uuid.UUID, tokenHash string, expiresAt time.Time) error
	GetRefreshToken(ctx context.Context, tokenHash string) (*RMRefreshToken, error)
	RevokeRefreshToken(ctx context.Context, tokenHash string) error

	// OTP login codes.
	CreateOTP(ctx context.Context, rmID uuid.UUID, codeHash string, expiresAt time.Time) error
	LatestActiveOTP(ctx context.Context, rmID uuid.UUID) (*RMOTPCode, error)
	IncrementOTPAttempts(ctx context.Context, id uuid.UUID) error
	ConsumeOTP(ctx context.Context, id uuid.UUID) error
	InvalidateOTPs(ctx context.Context, rmID uuid.UUID) error
}

type RMRefreshToken struct {
	ID        uuid.UUID
	RMID      uuid.UUID
	ExpiresAt time.Time
	RevokedAt *time.Time
}

type RMOTPCode struct {
	ID        uuid.UUID
	RMID      uuid.UUID
	CodeHash  string
	ExpiresAt time.Time
	Attempts  int
}

type PostgresRMUserRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRMUserRepository(pool *pgxpool.Pool) *PostgresRMUserRepository {
	return &PostgresRMUserRepository{pool: pool}
}

const rmUserColumns = `id, employee_code, email, name, phone_number, role, status, max_portfolios, created_at, updated_at`

func scanRMUser(row pgx.Row) (*rmdomain.StaffUser, error) {
	var s rmdomain.StaffUser
	var phone *string
	var createdAt, updatedAt time.Time
	if err := row.Scan(&s.ID, &s.EmployeeCode, &s.Email, &s.Name, &phone, &s.Role, &s.Status, &s.MaxPortfolios, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	s.PhoneNumber = phone
	s.CreatedAt = apitime.New(createdAt)
	s.UpdatedAt = apitime.New(updatedAt)
	return &s, nil
}

func (r *PostgresRMUserRepository) Create(ctx context.Context, employeeCode, email, name, phone, role string, maxPortfolios int) (*rmdomain.StaffUser, error) {
	var phonePtr *string
	if phone != "" {
		phonePtr = &phone
	}
	if maxPortfolios <= 0 {
		maxPortfolios = 150
	}
	row := r.pool.QueryRow(ctx, `
		INSERT INTO rm_users (employee_code, email, name, phone_number, role, max_portfolios)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+rmUserColumns,
		strings.ToUpper(strings.TrimSpace(employeeCode)),
		strings.ToLower(strings.TrimSpace(email)), name, phonePtr, role, maxPortfolios)
	s, err := scanRMUser(row)
	if err != nil {
		return nil, fmt.Errorf("create rm user: %w", err)
	}
	return s, nil
}

func (r *PostgresRMUserRepository) GetByEmail(ctx context.Context, email string) (*rmdomain.StaffUser, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+rmUserColumns+` FROM rm_users WHERE email = $1`, strings.ToLower(strings.TrimSpace(email)))
	s, err := scanRMUser(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get rm user by email: %w", err)
	}
	return s, nil
}

func (r *PostgresRMUserRepository) GetByIdentifier(ctx context.Context, identifier string) (*rmdomain.StaffUser, error) {
	id := strings.TrimSpace(identifier)
	row := r.pool.QueryRow(ctx, `
		SELECT `+rmUserColumns+` FROM rm_users
		WHERE email = lower($1) OR employee_code = upper($1)
		LIMIT 1
	`, id)
	s, err := scanRMUser(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get rm user by identifier: %w", err)
	}
	return s, nil
}

func (r *PostgresRMUserRepository) CreateOTP(ctx context.Context, rmID uuid.UUID, codeHash string, expiresAt time.Time) error {
	if _, err := r.pool.Exec(ctx, `
		INSERT INTO rm_otp_codes (rm_id, code_hash, expires_at) VALUES ($1, $2, $3)
	`, rmID, codeHash, expiresAt); err != nil {
		return fmt.Errorf("create rm otp: %w", err)
	}
	return nil
}

func (r *PostgresRMUserRepository) LatestActiveOTP(ctx context.Context, rmID uuid.UUID) (*RMOTPCode, error) {
	var c RMOTPCode
	err := r.pool.QueryRow(ctx, `
		SELECT id, rm_id, code_hash, expires_at, attempts
		FROM rm_otp_codes
		WHERE rm_id = $1 AND consumed_at IS NULL AND expires_at > now()
		ORDER BY created_at DESC
		LIMIT 1
	`, rmID).Scan(&c.ID, &c.RMID, &c.CodeHash, &c.ExpiresAt, &c.Attempts)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("latest active rm otp: %w", err)
	}
	return &c, nil
}

func (r *PostgresRMUserRepository) IncrementOTPAttempts(ctx context.Context, id uuid.UUID) error {
	if _, err := r.pool.Exec(ctx, `UPDATE rm_otp_codes SET attempts = attempts + 1 WHERE id = $1`, id); err != nil {
		return fmt.Errorf("increment rm otp attempts: %w", err)
	}
	return nil
}

func (r *PostgresRMUserRepository) ConsumeOTP(ctx context.Context, id uuid.UUID) error {
	if _, err := r.pool.Exec(ctx, `UPDATE rm_otp_codes SET consumed_at = now() WHERE id = $1 AND consumed_at IS NULL`, id); err != nil {
		return fmt.Errorf("consume rm otp: %w", err)
	}
	return nil
}

func (r *PostgresRMUserRepository) InvalidateOTPs(ctx context.Context, rmID uuid.UUID) error {
	if _, err := r.pool.Exec(ctx, `UPDATE rm_otp_codes SET consumed_at = now() WHERE rm_id = $1 AND consumed_at IS NULL`, rmID); err != nil {
		return fmt.Errorf("invalidate rm otps: %w", err)
	}
	return nil
}

func (r *PostgresRMUserRepository) GetByID(ctx context.Context, id uuid.UUID) (*rmdomain.StaffUser, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+rmUserColumns+` FROM rm_users WHERE id = $1`, id)
	s, err := scanRMUser(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get rm user by id: %w", err)
	}
	return s, nil
}

func (r *PostgresRMUserRepository) List(ctx context.Context) ([]rmdomain.StaffUser, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+rmUserColumns+` FROM rm_users ORDER BY role DESC, created_at`)
	if err != nil {
		return nil, fmt.Errorf("list rm users: %w", err)
	}
	defer rows.Close()

	out := make([]rmdomain.StaffUser, 0)
	for rows.Next() {
		s, err := scanRMUser(rows)
		if err != nil {
			return nil, fmt.Errorf("scan rm user: %w", err)
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

func (r *PostgresRMUserRepository) Update(ctx context.Context, id uuid.UUID, req rmdomain.UpdateRMRequest) (*rmdomain.StaffUser, error) {
	sets := make([]string, 0, 4)
	args := make([]any, 0, 5)
	i := 1
	if req.Name != nil {
		sets = append(sets, fmt.Sprintf("name = $%d", i))
		args = append(args, *req.Name)
		i++
	}
	if req.Email != nil {
		sets = append(sets, fmt.Sprintf("email = $%d", i))
		args = append(args, strings.ToLower(strings.TrimSpace(*req.Email)))
		i++
	}
	if req.Phone != nil {
		sets = append(sets, fmt.Sprintf("phone_number = $%d", i))
		args = append(args, *req.Phone)
		i++
	}
	if req.Status != nil {
		sets = append(sets, fmt.Sprintf("status = $%d", i))
		args = append(args, *req.Status)
		i++
	}
	if req.MaxPortfolios != nil {
		sets = append(sets, fmt.Sprintf("max_portfolios = $%d", i))
		args = append(args, *req.MaxPortfolios)
		i++
	}
	if len(sets) == 0 {
		return r.GetByID(ctx, id)
	}
	sets = append(sets, "updated_at = now()")
	args = append(args, id)
	q := fmt.Sprintf(`UPDATE rm_users SET %s WHERE id = $%d RETURNING %s`, strings.Join(sets, ", "), i, rmUserColumns)
	s, err := scanRMUser(r.pool.QueryRow(ctx, q, args...))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("update rm user: %w", err)
	}
	return s, nil
}

func (r *PostgresRMUserRepository) CreateRefreshToken(ctx context.Context, rmID uuid.UUID, tokenHash string, expiresAt time.Time) error {
	if _, err := r.pool.Exec(ctx, `
		INSERT INTO rm_refresh_tokens (rm_id, token_hash, expires_at) VALUES ($1, $2, $3)
	`, rmID, tokenHash, expiresAt); err != nil {
		return fmt.Errorf("create rm refresh token: %w", err)
	}
	return nil
}

func (r *PostgresRMUserRepository) GetRefreshToken(ctx context.Context, tokenHash string) (*RMRefreshToken, error) {
	var rt RMRefreshToken
	err := r.pool.QueryRow(ctx, `
		SELECT id, rm_id, expires_at, revoked_at FROM rm_refresh_tokens WHERE token_hash = $1
	`, tokenHash).Scan(&rt.ID, &rt.RMID, &rt.ExpiresAt, &rt.RevokedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get rm refresh token: %w", err)
	}
	return &rt, nil
}

func (r *PostgresRMUserRepository) RevokeRefreshToken(ctx context.Context, tokenHash string) error {
	if _, err := r.pool.Exec(ctx, `
		UPDATE rm_refresh_tokens SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL
	`, tokenHash); err != nil {
		return fmt.Errorf("revoke rm refresh token: %w", err)
	}
	return nil
}

// --- Assignment repository ---

type AssignmentRepository interface {
	// AssignNextRM routes a freshly-created user to an RM via capacity-aware
	// round-robin. Returns ErrNoActiveRM if there is no active desk.
	AssignNextRM(ctx context.Context, userID uuid.UUID) (uuid.UUID, error)

	// ManualAssign sets a user's RM explicitly (admin "assign" from the
	// unassigned pool, or "transfer" between RMs). action is one of
	// rmdomain.ActionAssign / ActionTransfer.
	ManualAssign(ctx context.Context, userID, toRMID, actorRMID uuid.UUID, action, reason string) error

	// Unassign clears a user's RM (admin "remove"), dropping them into the
	// unassigned pool.
	Unassign(ctx context.Context, userID, actorRMID uuid.UUID, reason string) error

	// OffboardRM redistributes every client currently held by rmID across
	// the rest of the active desk (round-robin) and returns how many moved.
	OffboardRM(ctx context.Context, rmID, actorRMID uuid.UUID, reason string) (int, error)

	CountsByRM(ctx context.Context) (map[uuid.UUID]int, error)
	CountUnassigned(ctx context.Context) (int, error)
	CountUsers(ctx context.Context) (int, error)

	// AUMByRM sums the latest recorded portfolio value of every client,
	// grouped by their assigned RM. TotalAUM is the same sum across all
	// users (assigned or not).
	AUMByRM(ctx context.Context) (map[uuid.UUID]float64, error)
	TotalAUM(ctx context.Context) (float64, error)

	// OwnerOf returns the RM currently assigned to userID (nil if
	// unassigned). found is false if the user does not exist.
	OwnerOf(ctx context.Context, userID uuid.UUID) (owner *uuid.UUID, found bool, err error)
	GetClientProfile(ctx context.Context, userID uuid.UUID) (*rmdomain.ClientProfile, error)

	ListClients(ctx context.Context, rmID *uuid.UUID, assigned *bool, f rmdomain.ListFilters) ([]rmdomain.ClientListItem, int, error)
	History(ctx context.Context, userID, rmID *uuid.UUID, limit, offset int) (rmdomain.AssignmentHistory, error)
}

type PostgresAssignmentRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresAssignmentRepository(pool *pgxpool.Pool) *PostgresAssignmentRepository {
	return &PostgresAssignmentRepository{pool: pool}
}

// activeRingTx reads the active RMs as an ordered assignment ring, inside a
// transaction so the counts are consistent with the locked queue-state row.
func activeRingTx(ctx context.Context, tx pgx.Tx) ([]rmdomain.Slot, error) {
	rows, err := tx.Query(ctx, `
		SELECT r.id, r.max_portfolios, COUNT(u.id)
		FROM rm_users r
		LEFT JOIN users u ON u.assigned_rm_id = r.id
		WHERE r.status = 'active' AND r.role = 'rm'
		GROUP BY r.id, r.max_portfolios, r.created_at
		ORDER BY r.created_at
	`)
	if err != nil {
		return nil, fmt.Errorf("load active rm ring: %w", err)
	}
	defer rows.Close()

	var ring []rmdomain.Slot
	for rows.Next() {
		var s rmdomain.Slot
		if err := rows.Scan(&s.RMID, &s.MaxPortfolios, &s.ClientCount); err != nil {
			return nil, fmt.Errorf("scan rm slot: %w", err)
		}
		ring = append(ring, s)
	}
	return ring, rows.Err()
}

func currentRMTx(ctx context.Context, tx pgx.Tx, userID uuid.UUID) (*uuid.UUID, error) {
	var rmID *uuid.UUID
	err := tx.QueryRow(ctx, `SELECT assigned_rm_id FROM users WHERE id = $1`, userID).Scan(&rmID)
	if err != nil {
		return nil, fmt.Errorf("read current rm: %w", err)
	}
	return rmID, nil
}

func applyAssignmentTx(ctx context.Context, tx pgx.Tx, userID uuid.UUID, from *uuid.UUID, to *uuid.UUID, action, reason string, actor *uuid.UUID) error {
	if _, err := tx.Exec(ctx, `UPDATE users SET assigned_rm_id = $1 WHERE id = $2`, to, userID); err != nil {
		return fmt.Errorf("update user assigned_rm_id: %w", err)
	}
	var reasonPtr *string
	if strings.TrimSpace(reason) != "" {
		reasonPtr = &reason
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO rm_assignment_history (user_id, from_rm_id, to_rm_id, action, reason, actor_rm_id)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, userID, from, to, action, reasonPtr, actor); err != nil {
		return fmt.Errorf("insert assignment history: %w", err)
	}
	return nil
}

func (r *PostgresAssignmentRepository) AssignNextRM(ctx context.Context, userID uuid.UUID) (uuid.UUID, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("begin assign txn: %w", err)
	}
	defer tx.Rollback(ctx)

	var lastAssigned *uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT last_assigned_rm_id FROM rm_queue_state WHERE id = true FOR UPDATE`).Scan(&lastAssigned); err != nil {
		return uuid.Nil, fmt.Errorf("lock queue state: %w", err)
	}

	ring, err := activeRingTx(ctx, tx)
	if err != nil {
		return uuid.Nil, err
	}
	after := uuid.Nil
	if lastAssigned != nil {
		after = *lastAssigned
	}
	chosen, _, ok := rmdomain.PickNextRM(ring, after)
	if !ok {
		return uuid.Nil, ErrNoActiveRM
	}

	from, err := currentRMTx(ctx, tx, userID)
	if err != nil {
		return uuid.Nil, err
	}
	if err := applyAssignmentTx(ctx, tx, userID, from, &chosen, rmdomain.ActionAutoAssign, "", nil); err != nil {
		return uuid.Nil, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE rm_queue_state SET last_assigned_rm_id = $1, rotation_seq = rotation_seq + 1, updated_at = now() WHERE id = true
	`, chosen); err != nil {
		return uuid.Nil, fmt.Errorf("advance queue state: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, fmt.Errorf("commit assign txn: %w", err)
	}
	return chosen, nil
}

func (r *PostgresAssignmentRepository) ManualAssign(ctx context.Context, userID, toRMID, actorRMID uuid.UUID, action, reason string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin manual assign txn: %w", err)
	}
	defer tx.Rollback(ctx)

	from, err := currentRMTx(ctx, tx, userID)
	if err != nil {
		return err
	}
	var actor *uuid.UUID
	if actorRMID != uuid.Nil {
		actor = &actorRMID
	}
	if err := applyAssignmentTx(ctx, tx, userID, from, &toRMID, action, reason, actor); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresAssignmentRepository) Unassign(ctx context.Context, userID, actorRMID uuid.UUID, reason string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin unassign txn: %w", err)
	}
	defer tx.Rollback(ctx)

	from, err := currentRMTx(ctx, tx, userID)
	if err != nil {
		return err
	}
	var actor *uuid.UUID
	if actorRMID != uuid.Nil {
		actor = &actorRMID
	}
	if err := applyAssignmentTx(ctx, tx, userID, from, nil, rmdomain.ActionRemove, reason, actor); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *PostgresAssignmentRepository) OffboardRM(ctx context.Context, rmID, actorRMID uuid.UUID, reason string) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin offboard txn: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT 1 FROM rm_queue_state WHERE id = true FOR UPDATE`); err != nil {
		return 0, fmt.Errorf("lock queue state: %w", err)
	}

	rows, err := tx.Query(ctx, `SELECT id FROM users WHERE assigned_rm_id = $1 ORDER BY created_at`, rmID)
	if err != nil {
		return 0, fmt.Errorf("list rm clients for offboard: %w", err)
	}
	var clients []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan client id: %w", err)
		}
		clients = append(clients, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(clients) == 0 {
		return 0, tx.Commit(ctx)
	}

	var actor *uuid.UUID
	if actorRMID != uuid.Nil {
		actor = &actorRMID
	}
	moved := 0
	after := uuid.Nil
	for _, userID := range clients {
		ring, err := activeRingTx(ctx, tx)
		if err != nil {
			return 0, err
		}
		// The RM being offboarded may still be 'active' (admin can rebalance
		// before deactivating) — exclude them from the target ring.
		filtered := ring[:0:0]
		for _, s := range ring {
			if s.RMID != rmID {
				filtered = append(filtered, s)
			}
		}
		chosen, _, ok := rmdomain.PickNextRM(filtered, after)
		if !ok {
			return 0, fmt.Errorf("offboard: no other active RM to receive clients: %w", ErrNoActiveRM)
		}
		from := rmID
		if err := applyAssignmentTx(ctx, tx, userID, &from, &chosen, rmdomain.ActionTransfer, reason, actor); err != nil {
			return 0, err
		}
		after = chosen
		moved++
	}
	if _, err := tx.Exec(ctx, `UPDATE rm_queue_state SET last_assigned_rm_id = $1, updated_at = now() WHERE id = true`, after); err != nil {
		return 0, fmt.Errorf("advance queue state after offboard: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit offboard txn: %w", err)
	}
	return moved, nil
}

func (r *PostgresAssignmentRepository) CountsByRM(ctx context.Context) (map[uuid.UUID]int, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT r.id, COUNT(u.id)
		FROM rm_users r
		LEFT JOIN users u ON u.assigned_rm_id = r.id
		GROUP BY r.id
	`)
	if err != nil {
		return nil, fmt.Errorf("counts by rm: %w", err)
	}
	defer rows.Close()

	out := make(map[uuid.UUID]int)
	for rows.Next() {
		var id uuid.UUID
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, fmt.Errorf("scan count: %w", err)
		}
		out[id] = n
	}
	return out, rows.Err()
}

func (r *PostgresAssignmentRepository) CountUnassigned(ctx context.Context) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE assigned_rm_id IS NULL`).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count unassigned: %w", err)
	}
	return n, nil
}

func (r *PostgresAssignmentRepository) AUMByRM(ctx context.Context) (map[uuid.UUID]float64, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT u.assigned_rm_id, COALESCE(SUM(s.total_wealth), 0)
		FROM users u
		LEFT JOIN LATERAL (
			SELECT total_wealth FROM portfolio_snapshots ps
			WHERE ps.user_id = u.id ORDER BY snapshot_date DESC LIMIT 1
		) s ON true
		WHERE u.assigned_rm_id IS NOT NULL
		GROUP BY u.assigned_rm_id
	`)
	if err != nil {
		return nil, fmt.Errorf("aum by rm: %w", err)
	}
	defer rows.Close()

	out := make(map[uuid.UUID]float64)
	for rows.Next() {
		var id uuid.UUID
		var v float64
		if err := rows.Scan(&id, &v); err != nil {
			return nil, fmt.Errorf("scan aum: %w", err)
		}
		out[id] = round2(v)
	}
	return out, rows.Err()
}

func (r *PostgresAssignmentRepository) TotalAUM(ctx context.Context) (float64, error) {
	var v float64
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(SUM(s.total_wealth), 0)
		FROM users u
		LEFT JOIN LATERAL (
			SELECT total_wealth FROM portfolio_snapshots ps
			WHERE ps.user_id = u.id ORDER BY snapshot_date DESC LIMIT 1
		) s ON true
	`).Scan(&v)
	if err != nil {
		return 0, fmt.Errorf("total aum: %w", err)
	}
	return round2(v), nil
}

func (r *PostgresAssignmentRepository) CountUsers(ctx context.Context) (int, error) {
	var n int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return n, nil
}

func (r *PostgresAssignmentRepository) OwnerOf(ctx context.Context, userID uuid.UUID) (*uuid.UUID, bool, error) {
	var owner *uuid.UUID
	err := r.pool.QueryRow(ctx, `SELECT assigned_rm_id FROM users WHERE id = $1`, userID).Scan(&owner)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("owner of user: %w", err)
	}
	return owner, true, nil
}

func (r *PostgresAssignmentRepository) GetClientProfile(ctx context.Context, userID uuid.UUID) (*rmdomain.ClientProfile, error) {
	var (
		p         rmdomain.ClientProfile
		name      *string
		phone     *string
		pan       *string
		joined    time.Time
		rmID      *uuid.UUID
		rmName    *string
		assignedT *time.Time
	)
	err := r.pool.QueryRow(ctx, `
		SELECT u.id, u.name, u.phone_number, u.created_at, u.assigned_rm_id, r.name,
		       k.pan_number, hist.created_at
		FROM users u
		LEFT JOIN rm_users r ON r.id = u.assigned_rm_id
		LEFT JOIN LATERAL (
			SELECT pan_number FROM kyc_verifications kv WHERE kv.user_id = u.id ORDER BY created_at DESC LIMIT 1
		) k ON true
		LEFT JOIN LATERAL (
			SELECT created_at FROM rm_assignment_history ah
			WHERE ah.user_id = u.id AND ah.to_rm_id = u.assigned_rm_id
			ORDER BY created_at DESC LIMIT 1
		) hist ON true
		WHERE u.id = $1
	`, userID).Scan(&p.UserID, &name, &phone, &joined, &rmID, &rmName, &pan, &assignedT)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get client profile: %w", err)
	}
	if name != nil {
		p.Name = *name
	}
	if phone != nil {
		p.Phone = *phone
	}
	p.PAN = pan
	p.JoinedAt = apitime.New(joined)
	p.RMID = rmID
	p.RMName = rmName
	if assignedT != nil {
		at := apitime.New(*assignedT)
		p.AssignedAt = &at
	}
	return &p, nil
}

func (r *PostgresAssignmentRepository) ListClients(ctx context.Context, rmID *uuid.UUID, assigned *bool, f rmdomain.ListFilters) ([]rmdomain.ClientListItem, int, error) {
	where := make([]string, 0, 3)
	args := make([]any, 0, 4)
	i := 1
	if rmID != nil {
		where = append(where, fmt.Sprintf("u.assigned_rm_id = $%d", i))
		args = append(args, *rmID)
		i++
	}
	if assigned != nil {
		if *assigned {
			where = append(where, "u.assigned_rm_id IS NOT NULL")
		} else {
			where = append(where, "u.assigned_rm_id IS NULL")
		}
	}
	if s := strings.TrimSpace(f.Search); s != "" {
		where = append(where, fmt.Sprintf("(u.name ILIKE $%d OR u.phone_number ILIKE $%d)", i, i))
		args = append(args, "%"+s+"%")
		i++
	}
	whereSQL := ""
	if len(where) > 0 {
		whereSQL = "WHERE " + strings.Join(where, " AND ")
	}

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users u `+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count clients: %w", err)
	}

	orderCol := "u.created_at"
	switch f.Sort {
	case "wealth":
		orderCol = "s.total_wealth"
	case "name":
		orderCol = "u.name"
	case "joined":
		orderCol = "u.created_at"
	}
	dir := "DESC"
	if strings.EqualFold(f.Order, "asc") {
		dir = "ASC"
	}
	limit := f.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}
	args = append(args, limit, offset)
	limitSQL := fmt.Sprintf("LIMIT $%d OFFSET $%d", i, i+1)

	q := `
		SELECT u.id, u.name, u.phone_number, u.created_at,
		       u.assigned_rm_id, r.name AS rm_name,
		       k.pan_number,
		       hist.created_at AS assigned_at,
		       (SELECT COUNT(*) FROM goals g WHERE g.user_id = u.id) AS goals_count,
		       s.total_wealth, s.mutual_funds_value, s.stocks_value, s.fixed_deposits_value, s.bank_balance_value,
		       prev.total_wealth AS prev_wealth
		FROM users u
		LEFT JOIN rm_users r ON r.id = u.assigned_rm_id
		LEFT JOIN LATERAL (
			SELECT pan_number FROM kyc_verifications kv WHERE kv.user_id = u.id ORDER BY created_at DESC LIMIT 1
		) k ON true
		LEFT JOIN LATERAL (
			SELECT total_wealth, mutual_funds_value, stocks_value, fixed_deposits_value, bank_balance_value, snapshot_date
			FROM portfolio_snapshots ps WHERE ps.user_id = u.id ORDER BY snapshot_date DESC LIMIT 1
		) s ON true
		LEFT JOIN LATERAL (
			SELECT total_wealth FROM portfolio_snapshots ps
			WHERE ps.user_id = u.id AND ps.snapshot_date < s.snapshot_date
			ORDER BY snapshot_date DESC LIMIT 1
		) prev ON true
		LEFT JOIN LATERAL (
			SELECT created_at FROM rm_assignment_history ah
			WHERE ah.user_id = u.id AND ah.to_rm_id = u.assigned_rm_id
			ORDER BY created_at DESC LIMIT 1
		) hist ON true
		` + whereSQL + `
		ORDER BY ` + orderCol + ` ` + dir + ` NULLS LAST, u.id
		` + limitSQL

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list clients: %w", err)
	}
	defer rows.Close()

	items := make([]rmdomain.ClientListItem, 0)
	for rows.Next() {
		var (
			it                 rmdomain.ClientListItem
			name               *string
			phone              *string
			joined             time.Time
			rmUUID             *uuid.UUID
			rmName             *string
			pan                *string
			assignedT          *time.Time
			total              *float64
			mfv, stv, fdv, bkv *float64
			prev               *float64
		)
		if err := rows.Scan(&it.UserID, &name, &phone, &joined, &rmUUID, &rmName, &pan, &assignedT,
			&it.GoalsCount, &total, &mfv, &stv, &fdv, &bkv, &prev); err != nil {
			return nil, 0, fmt.Errorf("scan client row: %w", err)
		}
		if name != nil {
			it.Name = *name
		}
		if phone != nil {
			it.Phone = *phone
		}
		it.JoinedAt = apitime.New(joined)
		it.PAN = pan
		it.AssignedRMID = rmUUID
		it.AssignedRMName = rmName
		if assignedT != nil {
			at := apitime.New(*assignedT)
			it.AssignedAt = &at
		}
		if total != nil {
			it.TotalWealth = round2(*total)
			it.AssetMix = rmdomain.AssetMix{
				MutualFunds:   round2(deref(mfv)),
				Stocks:        round2(deref(stv)),
				FixedDeposits: round2(deref(fdv)),
				Bank:          round2(deref(bkv)),
			}
			if prev != nil && *prev > 0 {
				it.OneDayChangeAmount = round2(*total - *prev)
				it.OneDayChangePct = round2((*total - *prev) / *prev * 100)
			}
		}
		items = append(items, it)
	}
	return items, total, rows.Err()
}

func (r *PostgresAssignmentRepository) History(ctx context.Context, userID, rmID *uuid.UUID, limit, offset int) (rmdomain.AssignmentHistory, error) {
	where := make([]string, 0, 2)
	args := make([]any, 0, 4)
	i := 1
	if userID != nil {
		where = append(where, fmt.Sprintf("h.user_id = $%d", i))
		args = append(args, *userID)
		i++
	}
	if rmID != nil {
		where = append(where, fmt.Sprintf("(h.from_rm_id = $%d OR h.to_rm_id = $%d)", i, i))
		args = append(args, *rmID)
		i++
	}
	whereSQL := ""
	if len(where) > 0 {
		whereSQL = "WHERE " + strings.Join(where, " AND ")
	}

	var out rmdomain.AssignmentHistory
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM rm_assignment_history h `+whereSQL, args...).Scan(&out.Total); err != nil {
		return out, fmt.Errorf("count history: %w", err)
	}

	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	args = append(args, limit, offset)

	q := `
		SELECT h.id, h.user_id, COALESCE(u.name, ''),
		       h.from_rm_id, fr.name, h.to_rm_id, tr.name,
		       h.action, h.reason, h.actor_rm_id, ar.name, h.created_at
		FROM rm_assignment_history h
		LEFT JOIN users u ON u.id = h.user_id
		LEFT JOIN rm_users fr ON fr.id = h.from_rm_id
		LEFT JOIN rm_users tr ON tr.id = h.to_rm_id
		LEFT JOIN rm_users ar ON ar.id = h.actor_rm_id
		` + whereSQL + `
		ORDER BY h.created_at DESC
		LIMIT $` + fmt.Sprint(i) + ` OFFSET $` + fmt.Sprint(i+1)

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return out, fmt.Errorf("query history: %w", err)
	}
	defer rows.Close()

	out.Items = make([]rmdomain.AssignmentHistoryItem, 0)
	for rows.Next() {
		var (
			it        rmdomain.AssignmentHistoryItem
			createdAt time.Time
		)
		if err := rows.Scan(&it.ID, &it.UserID, &it.UserName,
			&it.FromRMID, &it.FromRMName, &it.ToRMID, &it.ToRMName,
			&it.Action, &it.Reason, &it.ActorRMID, &it.ActorRMName, &createdAt); err != nil {
			return out, fmt.Errorf("scan history row: %w", err)
		}
		it.CreatedAt = apitime.New(createdAt)
		out.Items = append(out.Items, it)
	}
	return out, rows.Err()
}

func deref(f *float64) float64 {
	if f == nil {
		return 0
	}
	return *f
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
