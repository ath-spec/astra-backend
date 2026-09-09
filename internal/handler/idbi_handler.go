package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/yourusername/astra-backend/internal/apiresponse"
	"github.com/yourusername/astra-backend/internal/middleware"
	"github.com/yourusername/astra-backend/internal/repository"
	"github.com/yourusername/astra-backend/internal/service/creditscore"
	"github.com/yourusername/astra-backend/internal/service/idbiaccounts"
	"github.com/yourusername/astra-backend/internal/service/idbileads"
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
	leads    *idbileads.Service
}

func NewIDBIHandler(accounts *idbiaccounts.Service, spend *statementsync.Service, loans *idbiloans.Service, credit *creditscore.Service, leads *idbileads.Service) *IDBIHandler {
	return &IDBIHandler{accounts: accounts, spend: spend, loans: loans, credit: credit, leads: leads}
}

// Enabled reports whether at least one sub-feature is on (so main can decide
// whether to mount the router at all).
func (h *IDBIHandler) Enabled() bool {
	return h.accounts != nil || h.spend != nil || h.loans != nil || h.credit != nil || h.leads != nil
}

func (h *IDBIHandler) Routes() chi.Router {
	r := chi.NewRouter()
	if h.accounts != nil {
		r.Post("/link", h.linkCustomer)
		r.Get("/accounts", h.listAccounts)
		r.Post("/accounts/refresh", h.refreshAccounts)
		r.Get("/accounts/{accountNumber}/lien", h.accountLien)
	}
	if h.spend != nil {
		r.Post("/spend/refresh", h.refreshSpend)
	}
	if h.loans != nil {
		r.Get("/loans", h.listLoans)
		r.Post("/loans/refresh", h.refreshLoans)
		r.Get("/loans/{loanAccountID}/payoff", h.loanPayoff)
		r.Get("/loans/{loanAccountID}/limits", h.loanLimits)
		r.Get("/loans/{loanAccountID}/overdue-position", h.loanOverduePosition)
		r.Get("/loans/{loanAccountID}/schedule", h.loanSchedule)
	}
	if h.credit != nil {
		r.Get("/credit-score", h.creditScore)
	}
	if h.leads != nil {
		r.Post("/leads", h.createLead)
	}
	return r
}

type createLeadBody struct {
	Product         string  `json:"product"`
	Category        string  `json:"category"`
	SubCategory     string  `json:"sub_category"`
	EstimatedAmount float64 `json:"estimated_amount"`
	SolID           string  `json:"sol_id"`
	FirstName       string  `json:"first_name"`
	LastName        string  `json:"last_name"`
	Mobile          string  `json:"mobile"`
	Email           string  `json:"email"`
	PAN             string  `json:"pan"`
	Address         string  `json:"address"`
	Pincode         string  `json:"pincode"`
	State           string  `json:"state"`
}

func (h *IDBIHandler) createLead(w http.ResponseWriter, r *http.Request) {
	if _, ok := middleware.GetUserID(r.Context()); !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	var b createLeadBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		apiresponse.Error(w, apiresponse.Validation("invalid request body: %v", err))
		return
	}
	res, err := h.leads.Submit(r.Context(), idbileads.Lead{
		Applicant: idbileads.Applicant{
			FirstName: b.FirstName, LastName: b.LastName, Mobile: b.Mobile,
			Email: b.Email, PAN: b.PAN, Address: b.Address, Pincode: b.Pincode, State: b.State,
		},
		Product:         b.Product,
		Category:        b.Category,
		SubCategory:     b.SubCategory,
		EstimatedAmount: b.EstimatedAmount,
		SolID:           b.SolID,
	})
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, res)
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

type linkCustomerBody struct {
	CifID         string `json:"cif_id"`
	CustID        string `json:"cust_id"`
	AccountNumber string `json:"account_number"` // alternative: resolve cust_id via 365
	SkipRefresh   bool   `json:"skip_refresh"`
}

// linkCustomer binds the signed-in user to an IDBI customer identity
// (idbi_customer_link) — the prerequisite for every IDBI feature. Give a
// cif_id (+ optional cust_id), or an account_number to resolve cust_id from
// 365. Unless skip_refresh is set it then syncs the account mirror so data
// is immediately visible.
func (h *IDBIHandler) linkCustomer(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	var b linkCustomerBody
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		apiresponse.Error(w, apiresponse.Validation("invalid request body: %v", err))
		return
	}

	if b.CifID != "" {
		if err := h.accounts.SeedLink(r.Context(), userID, b.CifID, b.CustID); err != nil {
			apiresponse.Error(w, err)
			return
		}
	} else if b.AccountNumber != "" {
		if _, err := h.accounts.ResolveLinkByAccount(r.Context(), userID, b.AccountNumber); err != nil {
			apiresponse.Error(w, err)
			return
		}
	} else {
		apiresponse.Error(w, apiresponse.Validation("provide cif_id or account_number"))
		return
	}

	resp := map[string]any{"linked": true, "cif_id": b.CifID, "cust_id": b.CustID}
	if !b.SkipRefresh && b.CifID != "" {
		if err := h.accounts.Refresh(r.Context(), userID); err != nil {
			resp["refresh_error"] = err.Error()
		} else {
			accts, _ := h.accounts.List(r.Context(), userID)
			resp["accounts"] = accts
		}
	}
	apiresponse.OK(w, resp)
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

func (h *IDBIHandler) accountLien(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	res, err := h.accounts.AccountLien(r.Context(), userID, chi.URLParam(r, "accountNumber"))
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, res)
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

func (h *IDBIHandler) loanLimits(w http.ResponseWriter, r *http.Request) {
	if _, ok := middleware.GetUserID(r.Context()); !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	res, err := h.loans.LoanLimits(r.Context(), chi.URLParam(r, "loanAccountID"))
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, res)
}

func (h *IDBIHandler) loanOverduePosition(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	res, err := h.loans.OverduePosition(r.Context(), userID, chi.URLParam(r, "loanAccountID"))
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, res)
}

func (h *IDBIHandler) loanSchedule(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	res, err := h.loans.RepaymentSchedule(r.Context(), userID, chi.URLParam(r, "loanAccountID"))
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, res)
}
