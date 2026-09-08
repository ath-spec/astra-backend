package handler

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/yourusername/astra-backend/internal/apiresponse"
	"github.com/yourusername/astra-backend/internal/middleware"
	"github.com/yourusername/astra-backend/internal/repository"
	"github.com/yourusername/astra-backend/internal/service/creditscore"
	"github.com/yourusername/astra-backend/internal/service/idbiaccounts"
	"github.com/yourusername/astra-backend/internal/service/idbiloans"
	"github.com/yourusername/astra-backend/internal/service/statementsync"
)

// IDBIHandler serves the IDBI mirror APIs (features 1+). Each sub-service is
// optional — a route is registered only when its service is wired (its feature
// flag is on). Mounted at /api/v1/idbi.
type IDBIHandler struct {
	accounts *idbiaccounts.Service
	spend    *statementsync.Service
	loans    *idbiloans.Service
	credit   *creditscore.Service
}

func NewIDBIHandler(accounts *idbiaccounts.Service, spend *statementsync.Service, loans *idbiloans.Service, credit *creditscore.Service) *IDBIHandler {
	return &IDBIHandler{accounts: accounts, spend: spend, loans: loans, credit: credit}
}

// Enabled reports whether at least one sub-feature is on (so main can decide
// whether to mount the router at all).
func (h *IDBIHandler) Enabled() bool {
	return h.accounts != nil || h.spend != nil || h.loans != nil || h.credit != nil
}

func (h *IDBIHandler) Routes() chi.Router {
	r := chi.NewRouter()
	if h.accounts != nil {
		r.Get("/accounts", h.listAccounts)
		r.Post("/accounts/refresh", h.refreshAccounts)
	}
	if h.spend != nil {
		r.Post("/spend/refresh", h.refreshSpend)
	}
	if h.loans != nil {
		r.Get("/loans", h.listLoans)
		r.Post("/loans/refresh", h.refreshLoans)
		r.Get("/loans/{loanAccountID}/payoff", h.loanPayoff)
	}
	if h.credit != nil {
		r.Get("/credit-score", h.creditScore)
	}
	return r
}

func (h *IDBIHandler) creditScore(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	sc, err := h.credit.Get(r.Context(), userID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, sc)
}

func (h *IDBIHandler) listAccounts(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	accts, err := h.accounts.List(r.Context(), userID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, map[string]any{"accounts": accts})
}

func (h *IDBIHandler) refreshAccounts(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	if err := h.accounts.Refresh(r.Context(), userID); err != nil {
		if errors.Is(err, repository.ErrNoCustomerLink) {
			apiresponse.Error(w, apiresponse.Validation("this account is not linked to an IDBI customer yet"))
			return
		}
		apiresponse.Error(w, err)
		return
	}
	accts, err := h.accounts.List(r.Context(), userID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, map[string]any{"accounts": accts, "refreshed": true})
}

func (h *IDBIHandler) refreshSpend(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	n, err := h.spend.SyncUser(r.Context(), userID)
	if err != nil {
		if errors.Is(err, repository.ErrNoCustomerLink) {
			apiresponse.Error(w, apiresponse.Validation("this account is not linked to an IDBI customer yet"))
			return
		}
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, map[string]any{"synced": true, "transactions_written": n})
}

func (h *IDBIHandler) listLoans(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	loans, err := h.loans.List(r.Context(), userID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, map[string]any{"loans": loans})
}

func (h *IDBIHandler) refreshLoans(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	if err := h.loans.Refresh(r.Context(), userID); err != nil {
		if errors.Is(err, repository.ErrNoCustomerLink) {
			apiresponse.Error(w, apiresponse.Validation("this account is not linked to an IDBI customer yet"))
			return
		}
		apiresponse.Error(w, err)
		return
	}
	loans, err := h.loans.List(r.Context(), userID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, map[string]any{"loans": loans, "refreshed": true})
}

func (h *IDBIHandler) loanPayoff(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	loanID := chi.URLParam(r, "loanAccountID")
	q, err := h.loans.PayoffQuote(r.Context(), userID, loanID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, q)
}
