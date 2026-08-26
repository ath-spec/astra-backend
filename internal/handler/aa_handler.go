package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourusername/astra-backend/internal/apiresponse"
	authmw "github.com/yourusername/astra-backend/internal/middleware"
)

type AAHandler struct {
	pool *pgxpool.Pool
}

func NewAAHandler(pool *pgxpool.Pool) *AAHandler {
	return &AAHandler{pool: pool}
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
}

func (h *AAHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/accounts", h.GetAccounts)
	r.Post("/accounts", h.AddAccount)
	r.Post("/accounts/link", h.AddAccount)
	r.Delete("/accounts/{accountID}", h.UnlinkAccount)
	r.Get("/accounts/{accountID}/transactions", h.GetAccountTransactions)
	r.Post("/consent", h.CreateConsent)
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

func (h *AAHandler) CreateConsent(w http.ResponseWriter, r *http.Request) {
	apiresponse.OK(w, map[string]any{
		"consent_id": "CONSENT-" + uuid.New().String()[:8],
		"status":     "ACTIVE",
		"message":    "Consent created successfully",
	})
}
