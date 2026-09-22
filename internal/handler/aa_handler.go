package handler

import (
	"context"
	"encoding/json"
	"hash/fnv"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourusername/astra-backend/internal/apiresponse"
	"github.com/yourusername/astra-backend/internal/events"
	authmw "github.com/yourusername/astra-backend/internal/middleware"
	"github.com/yourusername/astra-backend/internal/provider/idbi"
	"github.com/yourusername/astra-backend/internal/service/idbiaa"
	"github.com/yourusername/astra-backend/internal/service/idbiaccounts"
)

// idbiAcctNamespace derives a stable UUID for an IDBI account number so it can
// occupy the "id" field the accounts screen already expects.
var idbiAcctNamespace = uuid.MustParse("1b671a64-40d5-491e-99b0-da01ff1f3341")

// discoveryAcctNamespace namespaces synthetic IDs for DiscoverAccounts —
// separate from idbiAcctNamespace so a discovered-but-unlinked candidate's
// ID can never collide with a real IDBI-synced account's ID.
var discoveryAcctNamespace = uuid.MustParse("7d3b6e2a-9c41-4b8f-8e2d-5a1f9c6b0d47")

// discoveryBankPool is the small set of banks DiscoverAccounts can offer as
// "found via Account Aggregator" candidates. There is no real cross-bank AA
// discovery integration (IDBI's sandbox can only simulate IDBI's own
// accounts, not other banks'), so this stands in for it the same way the
// stocks/MF/FD MockProviders stand in for their real vendors: a real
// backend endpoint with deterministic, per-user output, not hardcoded
// client-side data.
var discoveryBankPool = []string{"ICICI Bank", "HDFC Bank", "Axis Bank", "State Bank of India"}

// DiscoverAccounts simulates an AA discovery step: it returns up to two
// bank accounts the user hasn't already linked, with a deterministic
// (stable per user+bank, not random per call) balance derived from their
// user ID — so re-opening the linking screen shows the same candidates
// instead of a new set each time. Discovered accounts are never written to
// bank_accounts; the user still has to select them and hit APPROVE AND
// CONNECT (which calls AddAccount) for that.
func (h *AAHandler) DiscoverAccounts(w http.ResponseWriter, r *http.Request) {
	userID, ok := authmw.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}

	rows, err := h.pool.Query(r.Context(), `
		SELECT bank_name FROM bank_accounts WHERE user_id = $1 AND unlinked_at IS NULL
	`, userID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	linked := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			apiresponse.Error(w, err)
			return
		}
		linked[name] = true
	}
	rows.Close()

	accounts := make([]BankAccountResponse, 0, 2)
	for _, bankName := range discoveryBankPool {
		if linked[bankName] || len(accounts) >= 2 {
			continue
		}
		hasher := fnv.New64a()
		_, _ = hasher.Write([]byte(userID.String() + bankName))
		sum := hasher.Sum64()
		balance := float64(80000+int(sum%220000)) / 1.0

		accounts = append(accounts, BankAccountResponse{
			ID:          uuid.NewSHA1(discoveryAcctNamespace, []byte(userID.String()+bankName)),
			BankName:    bankName,
			AccountType: "SAVINGS",
			Balance:     balance,
			CreatedAt:   time.Now(),
			IsLinked:    false,
		})
	}

	apiresponse.OK(w, map[string]any{
		"accounts": accounts,
		"count":    len(accounts),
	})
}

// aaWebhookProcessTimeout bounds the detached processing of an inbound AA
// notification (which itself makes an outbound IDBI call). The webhook is
// acked immediately; this only limits the background work.
const aaWebhookProcessTimeout = 30 * time.Second

// bankDependentSeeder creates demo data (FDs, mandates) tied to a bank
// account the user actually linked themselves, instead of that data being
// hardcoded against a fake bank account inserted at signup before any
// consent. Implemented by *repository.PostgresUserRepository.
type bankDependentSeeder interface {
	SeedBankDependentData(ctx context.Context, userID, bankAccountID uuid.UUID) error
}

type AAHandler struct {
	pool         *pgxpool.Pool
	aa           *idbiaa.Service       // nil unless IDBI_AA_ENABLED — then the consent flow is real
	idbiAccounts *idbiaccounts.Service // nil unless IDBI_ACCOUNTS_ENABLED — then GET /accounts serves real IDBI accounts
	events       *events.Publisher
	seeder       bankDependentSeeder // nil until WithSeeder is called
}

func NewAAHandler(pool *pgxpool.Pool) *AAHandler {
	return &AAHandler{pool: pool}
}

// WithEvents attaches the live-update publisher so adding or unlinking a
// bank account pushes an invalidation to the RM portal.
func (h *AAHandler) WithEvents(pub *events.Publisher) *AAHandler {
	h.events = pub
	return h
}

// WithIDBI attaches the real AA consent service (feature 4). When it is not
// attached, CreateConsent / GetAccountTransactions keep their original stub
// behaviour, so the app is unchanged with the flag off.
func (h *AAHandler) WithIDBI(svc *idbiaa.Service) *AAHandler {
	h.aa = svc
	return h
}

// WithIDBIAccounts folds the customer's real IDBI deposit accounts (feature 1)
// into GET /api/v1/aa/accounts, so the existing accounts screens show them with
// no frontend change. When the user has synced IDBI accounts they replace the
// seeded bank_accounts rows in the response; otherwise the bank_accounts rows
// are returned exactly as before.
func (h *AAHandler) WithIDBIAccounts(svc *idbiaccounts.Service) *AAHandler {
	h.idbiAccounts = svc
	return h
}

// IDBIEnabled reports whether the real AA flow is wired.
func (h *AAHandler) IDBIEnabled() bool { return h.aa != nil }

// WithSeeder attaches the demo-data seeder so a user's first successful
// AddAccount call (from discover -> approve, or the manual "connect more
// accounts" flow) can backfill their FD/mandate demo data against that real
// bank account. Without it, AddAccount behaves exactly as before (no demo
// FD/mandate seeding at all).
func (h *AAHandler) WithSeeder(s bankDependentSeeder) *AAHandler {
	h.seeder = s
	return h
}

type AddBankAccountRequest struct {
	BankName    string  `json:"bank_name"`
	AccountType string  `json:"account_type"`
	Balance     float64 `json:"balance"`
}

type BankAccountResponse struct {
	ID          uuid.UUID `json:"id"`
	BankName    string    `json:"bank_name"`
	AccountType string    `json:"account_type"`
	Balance     float64   `json:"balance"`
	CreatedAt   time.Time `json:"created_at"`
	// IsLinked distinguishes a real, already-persisted account (GetAccounts)
	// from a simulated AA-discovery candidate (DiscoverAccounts) that the
	// user still has to approve before it's ever written to bank_accounts.
	IsLinked bool `json:"is_linked"`
}

func (h *AAHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/accounts", h.GetAccounts)
	r.Get("/accounts/discover", h.DiscoverAccounts)
	r.Post("/accounts", h.AddAccount)
	r.Post("/accounts/link", h.AddAccount)
	r.Delete("/accounts/{accountID}", h.UnlinkAccount)
	r.Get("/accounts/{accountID}/transactions", h.GetAccountTransactions)
	r.Post("/consent", h.CreateConsent)

	// Real AA consent flow — only routed when IDBI_AA_ENABLED. Existing
	// routes above are untouched.
	if h.aa != nil {
		r.Get("/consents", h.ListConsents)
		r.Post("/consents/callback", h.ConsentCallback) // "live" mode redirect return (593)
		r.Get("/consents/{consentHandle}", h.GetConsent)
		r.Post("/consents/{consentHandle}/refresh", h.RefreshConsent)
		r.Post("/consents/{consentHandle}/fetch", h.FetchConsentData)
	}
	return r
}

// WebhookRoutes returns the inbound AA notification endpoints (497/498).
// These are called by IDBI's gateway, not the app, so they carry no user
// JWT and must be mounted in an unprotected group. Returns nil when the
// AA flow is not wired.
func (h *AAHandler) WebhookRoutes() chi.Router {
	if h.aa == nil {
		return nil
	}
	r := chi.NewRouter()
	r.Post("/consent-notification", h.consentNotification) // 497
	r.Post("/data-notification", h.dataNotification)       // 498
	return r
}

func (h *AAHandler) GetAccounts(w http.ResponseWriter, r *http.Request) {
	userID, ok := authmw.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}

	// Feature 1: when the user has synced IDBI accounts, fold those in
	// alongside bank_accounts (below) rather than replacing them — a bank
	// added manually via the "connect more accounts" search (AddAccount,
	// which only ever writes to bank_accounts) must stay visible even after
	// the user also has real IDBI accounts synced, otherwise it silently
	// vanishes from this list the moment IDBI accounts exist.
	var accounts []BankAccountResponse
	if h.idbiAccounts != nil {
		if idbiAccs, ierr := h.idbiAccounts.List(r.Context(), userID); ierr == nil && len(idbiAccs) > 0 {
			for _, a := range idbiAccs {
				bal := a.LedgerBalance
				if bal == 0 {
					bal = a.AvailableBalance
				}
				name := "IDBI Bank"
				if a.BranchName != "" {
					name = "IDBI Bank — " + a.BranchName
				}
				accounts = append(accounts, BankAccountResponse{
					ID:          uuid.NewSHA1(idbiAcctNamespace, []byte(a.AccountNumber)),
					BankName:    name,
					AccountType: a.AccountType,
					Balance:     bal,
					CreatedAt:   a.SyncedAt,
					IsLinked:    true,
				})
			}
		}
	}

	rows, err := h.pool.Query(r.Context(), `
		SELECT id, bank_name, account_type, balance, created_at
		FROM bank_accounts
		WHERE user_id = $1 AND unlinked_at IS NULL
		ORDER BY created_at ASC
	`, userID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var acc BankAccountResponse
		if err := rows.Scan(&acc.ID, &acc.BankName, &acc.AccountType, &acc.Balance, &acc.CreatedAt); err != nil {
			apiresponse.Error(w, err)
			return
		}
		acc.IsLinked = true
		accounts = append(accounts, acc)
	}

	if accounts == nil {
		accounts = []BankAccountResponse{}
	}

	apiresponse.OK(w, map[string]any{
		"accounts": accounts,
		"count":    len(accounts),
	})
}

func (h *AAHandler) AddAccount(w http.ResponseWriter, r *http.Request) {
	userID, ok := authmw.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}

	var req AddBankAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiresponse.Error(w, apiresponse.Validation("invalid request body: %v", err))
		return
	}

	if req.BankName == "" {
		req.BankName = "Bank Account"
	}
	if req.AccountType == "" {
		req.AccountType = "SAVINGS"
	}
	if req.Balance <= 0 {
		req.Balance = 25000.00
	}

	// Checked before the insert below so this always reflects "had zero
	// bank accounts before this call," not "has one now" (which would be
	// true after every single AddAccount, including the second, third, ...).
	var hadNoAccounts bool
	if h.seeder != nil {
		var existing int
		if err := h.pool.QueryRow(r.Context(),
			`SELECT count(*) FROM bank_accounts WHERE user_id = $1 AND unlinked_at IS NULL`, userID,
		).Scan(&existing); err == nil {
			hadNoAccounts = existing == 0
		}
	}

	var acc BankAccountResponse
	err := h.pool.QueryRow(r.Context(), `
		INSERT INTO bank_accounts (user_id, bank_name, account_type, balance)
		VALUES ($1, $2, $3, $4)
		RETURNING id, bank_name, account_type, balance, created_at
	`, userID, req.BankName, req.AccountType, req.Balance).Scan(
		&acc.ID,
		&acc.BankName,
		&acc.AccountType,
		&acc.Balance,
		&acc.CreatedAt,
	)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	if h.events != nil {
		go h.events.UserChanged(context.Background(), userID, events.TypeBankAccountChanged)
	}
	// Best-effort, same pattern as the RM auto-assign on signup: this is
	// demo/mock enrichment (FDs, mandates), never something that should
	// fail or slow down the user's actual bank-linking request.
	if h.seeder != nil && hadNoAccounts {
		go func() {
			if err := h.seeder.SeedBankDependentData(context.Background(), userID, acc.ID); err != nil {
				slog.Error("seed bank dependent data failed", "user_id", userID, "error", err)
			}
		}()
	}

	apiresponse.Created(w, acc)
}

func (h *AAHandler) UnlinkAccount(w http.ResponseWriter, r *http.Request) {
	userID, ok := authmw.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}

	accountIDStr := chi.URLParam(r, "accountID")
	accountID, err := uuid.Parse(accountIDStr)
	if err != nil {
		apiresponse.Error(w, apiresponse.Validation("invalid account ID: %v", err))
		return
	}

	// Soft-delete: bank_account_id is a NOT NULL, ON-DELETE-RESTRICT FK from
	// payments, mandates, and fd_accounts, so a hard DELETE here fails the
	// moment the account has any transaction history. Marking it unlinked
	// removes it from every "your accounts" / balance view while preserving
	// that history intact — the same behaviour a real bank's "close account"
	// flow has.
	tag, err := h.pool.Exec(r.Context(), `
		UPDATE bank_accounts
		SET unlinked_at = now()
		WHERE id = $1 AND user_id = $2 AND unlinked_at IS NULL
	`, accountID, userID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		// Not a bank_accounts row — check whether it's one of the IDs
		// GetAccounts synthesizes for the user's IDBI-synced accounts
		// (uuid.NewSHA1(idbiAcctNamespace, accountNumber)). Those don't
		// exist in bank_accounts at all, so without this the "remove"
		// action on an IDBI account just 404'd and silently did nothing.
		// IDBI mirrors a whole customer's account list under one link —
		// there's no per-account revoke, so removing any one of them
		// revokes the whole IDBI sync for this user.
		if h.idbiAccounts != nil {
			idbiAccs, ierr := h.idbiAccounts.List(r.Context(), userID)
			if ierr == nil {
				for _, a := range idbiAccs {
					if uuid.NewSHA1(idbiAcctNamespace, []byte(a.AccountNumber)) == accountID {
						if rerr := h.idbiAccounts.Revoke(r.Context(), userID); rerr != nil {
							apiresponse.Error(w, rerr)
							return
						}
						if h.events != nil {
							go h.events.UserChanged(context.Background(), userID, events.TypeBankAccountChanged)
						}
						apiresponse.OK(w, map[string]string{
							"message": "IDBI bank connection revoked successfully",
						})
						return
					}
				}
			}
		}
		apiresponse.Error(w, apiresponse.NotFound("bank account not found"))
		return
	}
	if h.events != nil {
		go h.events.UserChanged(context.Background(), userID, events.TypeBankAccountChanged)
	}

	apiresponse.OK(w, map[string]string{
		"message": "account unlinked successfully",
	})
}

func (h *AAHandler) GetAccountTransactions(w http.ResponseWriter, r *http.Request) {
	apiresponse.OK(w, map[string]any{
		"transactions": []any{},
		"message":      "no transactions for this account",
	})
}

// consentCreateRequest is accepted by CreateConsent when the real AA flow is
// wired. All fields optional — mobile falls back to the user's registered
// phone.
type consentCreateRequest struct {
	Mobile    string `json:"mobile"`
	AccountID string `json:"account_id"`
}

func (h *AAHandler) CreateConsent(w http.ResponseWriter, r *http.Request) {
	// Stub behaviour — unchanged when IDBI_AA_ENABLED is off.
	if h.aa == nil {
		apiresponse.OK(w, map[string]any{
			"consent_id": "CONSENT-" + uuid.New().String()[:8],
			"status":     "ACTIVE",
			"message":    "Consent created successfully",
		})
		return
	}

	userID, ok := authmw.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	var req consentCreateRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req) // body is optional
	}
	mobile := req.Mobile
	if mobile == "" {
		mobile = h.userPhone(r, userID)
	}
	view, err := h.aa.RequestConsent(r.Context(), userID, mobile, req.AccountID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, view)
}

func (h *AAHandler) ListConsents(w http.ResponseWriter, r *http.Request) {
	userID, ok := authmw.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	views, err := h.aa.List(r.Context(), userID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, map[string]any{"consents": views, "count": len(views)})
}

func (h *AAHandler) GetConsent(w http.ResponseWriter, r *http.Request) {
	h.refreshOrGet(w, r, false)
}

func (h *AAHandler) RefreshConsent(w http.ResponseWriter, r *http.Request) {
	h.refreshOrGet(w, r, true)
}

func (h *AAHandler) refreshOrGet(w http.ResponseWriter, r *http.Request, _ bool) {
	userID, ok := authmw.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	handle := chi.URLParam(r, "consentHandle")
	view, err := h.aa.Refresh(r.Context(), userID, handle)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, view)
}

// consentCallbackRequest carries the encrypted blob the AA app hands back to
// our redirect URL in "live" mode.
type consentCallbackRequest struct {
	Ecres   string `json:"ecres"`
	Resdate string `json:"resdate"`
	Fi      string `json:"fi"`
}

func (h *AAHandler) ConsentCallback(w http.ResponseWriter, r *http.Request) {
	userID, ok := authmw.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	var req consentCallbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiresponse.Error(w, apiresponse.Validation("invalid request body: %v", err))
		return
	}
	if req.Ecres == "" {
		apiresponse.Error(w, apiresponse.Validation("ecres is required"))
		return
	}
	view, err := h.aa.CompleteRedirect(r.Context(), userID, req.Ecres, req.Resdate, req.Fi)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, view)
}

func (h *AAHandler) FetchConsentData(w http.ResponseWriter, r *http.Request) {
	userID, ok := authmw.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	handle := chi.URLParam(r, "consentHandle")
	n, err := h.aa.FetchStatements(r.Context(), userID, handle)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, map[string]any{"consent_handle": handle, "transactions_written": n})
}

func ackAA(w http.ResponseWriter) {
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"message": "Success"})
}

// consentNotification handles inbound 497 pushConsentNotification. The AA
// spec wants a fast ack, and processing makes its own outbound IDBI call,
// so the work runs detached with its own timeout and the request is acked
// immediately. The handler is idempotent; failures are logged, and the app
// can always recover state via POST /consents/{handle}/refresh.
func (h *AAHandler) consentNotification(w http.ResponseWriter, r *http.Request) {
	var n idbi.PushConsentNotification
	if err := json.NewDecoder(r.Body).Decode(&n); err != nil {
		ackAA(w)
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), aaWebhookProcessTimeout)
		defer cancel()
		if err := h.aa.HandleConsentNotification(ctx, n); err != nil {
			slog.Error("aa: 497 consent notification processing failed",
				"consent_handle", n.ConsentHandle, "event_status", n.EventStatus, "error", err)
		}
	}()
	ackAA(w)
}

// dataNotification handles inbound 498 pushDataNotification. Same fast-ack,
// detached-processing model as consentNotification.
func (h *AAHandler) dataNotification(w http.ResponseWriter, r *http.Request) {
	var n idbi.PushDataNotification
	if err := json.NewDecoder(r.Body).Decode(&n); err != nil {
		ackAA(w)
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), aaWebhookProcessTimeout)
		defer cancel()
		if err := h.aa.HandleDataNotification(ctx, n); err != nil {
			slog.Error("aa: 498 data notification processing failed",
				"consent_handle", n.ConsentHandle, "event_status", n.EventStatus, "error", err)
		}
	}()
	ackAA(w)
}

// userPhone looks up the user's registered phone; "" if not found.
func (h *AAHandler) userPhone(r *http.Request, userID uuid.UUID) string {
	var phone string
	_ = h.pool.QueryRow(r.Context(), `SELECT COALESCE(phone_number, '') FROM users WHERE id = $1`, userID).Scan(&phone)
	return phone
}
