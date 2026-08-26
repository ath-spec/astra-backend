package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/yourusername/astra-backend/internal/apiresponse"
)

// AAHandler stubs the Account Aggregator domain (consent, linked accounts,
// transactions). The schema (aa_bank_accounts, aa_transactions — see
// migration 000006) is ready, but the routes are deliberately unimplemented
// until an AA gateway provider is selected; wiring one in only requires
// adding an internal/provider/aa package and swapping these bodies for real
// calls into a service — no route/contract changes.
type AAHandler struct{}

func NewAAHandler() *AAHandler {
	return &AAHandler{}
}

func (h *AAHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/consent", h.notConfigured)
	r.Get("/accounts", h.notConfigured)
	r.Get("/accounts/{accountID}/transactions", h.notConfigured)
	return r
}

func (h *AAHandler) notConfigured(w http.ResponseWriter, r *http.Request) {
	apiresponse.Error(w, apiresponse.NotImplemented("Account Aggregator provider is not yet configured"))
}
