package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourusername/astra-backend/internal/apiresponse"
	authmw "github.com/yourusername/astra-backend/internal/middleware"
	"github.com/yourusername/astra-backend/internal/provider/idbi"
	"github.com/yourusername/astra-backend/internal/service/idbiaa"
)

// aaWebhookProcessTimeout bounds the detached processing of an inbound AA
// notification (which itself makes an outbound IDBI call). The webhook is
// acked immediately; this only limits the background work.
const aaWebhookProcessTimeout = 30 * time.Second

type AAHandler struct {
	pool *pgxpool.Pool
	aa   *idbiaa.Service // nil unless IDBI_AA_ENABLED — then the consent flow is real
}

func NewAAHandler(pool *pgxpool.Pool) *AAHandler {
	return &AAHandler{pool: pool}
}

// WithIDBI attaches the real AA consent service (feature 4). When it is not
// attached, CreateConsent / GetAccountTransactions keep their original stub
// behaviour, so the app is unchanged with the flag off.
func (h *AAHandler) WithIDBI(svc *idbiaa.Service) *AAHandler {
	h.aa = svc
	return h
}

// IDBIEnabled reports whether the real AA flow is wired.
func (h *AAHandler) IDBIEnabled() bool { return h.aa != nil }

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
}

func (h *AAHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/accounts", h.GetAccounts)
	r.Post("/accounts", h.AddAccount)
	r.Post("/accounts/link", h.AddAccount)
	r.Delete("/accounts/{accountID}", h.UnlinkAccount)
	r.Get("/accounts/{accountID}/transactions", h.GetAccountTransactions)
	r.Post("/consent", h.CreateConsent)

	// Real AA consent flow — only routed when IDBI_AA_ENABLED. Existing
	// routes above are untouched.
	if h.aa != nil {
		r.Get("/consents", h.ListConsents)
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

	rows, err := h.pool.Query(r.Context(), `
		SELECT id, bank_name, account_type, balance, created_at
		FROM bank_accounts
		WHERE user_id = $1
		ORDER BY created_at ASC
	`, userID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	defer rows.Close()

	var accounts []BankAccountResponse
	for rows.Next() {
		var acc BankAccountResponse
		if err := rows.Scan(&acc.ID, &acc.BankName, &acc.AccountType, &acc.Balance, &acc.CreatedAt); err != nil {
			apiresponse.Error(w, err)
			return
		}
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

	_, err = h.pool.Exec(r.Context(), `
		DELETE FROM bank_accounts
		WHERE id = $1 AND user_id = $2
	`, accountID, userID)
	if err != nil {
		apiresponse.Error(w, err)
		return
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
