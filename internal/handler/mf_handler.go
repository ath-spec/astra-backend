package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/yourusername/astra-backend/internal/apiresponse"
	mfdomain "github.com/yourusername/astra-backend/internal/domain/mf"
	"github.com/yourusername/astra-backend/internal/httpx"
	"github.com/yourusername/astra-backend/internal/middleware"
	"github.com/yourusername/astra-backend/internal/service"
)

// MFHandler serves the mock Mutual Fund investment domain (holdings,
// purchase/SIP, redemption, transaction history) — backed by a stateful
// mock provider today, swappable for a real CAMS/KFintech-style RTA later.
//
// GET /cas remains a deliberate 501: importing a Consolidated Account
// Statement means pulling holdings the user bought through OTHER apps/AMCs
// via an external RTA aggregator — a genuinely different capability from
// this app's own mock investment ledger below, and one that still needs a
// real vendor chosen before it can do anything.
type MFHandler struct {
	svc *service.MFService
}

func NewMFHandler(svc *service.MFService) *MFHandler {
	return &MFHandler{svc: svc}
}

func (h *MFHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/cas", h.notConfigured)
	r.Get("/holdings", h.getHoldings)
	r.Post("/purchase", h.purchase)
	r.Post("/redeem", h.redeem)
	r.Get("/transactions", h.getTransactions)
	return r
}

func (h *MFHandler) notConfigured(w http.ResponseWriter, r *http.Request) {
	apiresponse.Error(w, apiresponse.NotImplemented("MF Central/CAS import provider is not yet configured"))
}

func (h *MFHandler) getHoldings(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	result, err := h.svc.GetHoldings(r.Context(), userID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, result)
}

func (h *MFHandler) purchase(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	var req mfdomain.PurchaseRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		apiresponse.Error(w, apiresponse.Validation("invalid request body: %v", err))
		return
	}
	txn, err := h.svc.Purchase(r.Context(), userID, req)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.Created(w, txn)
}

func (h *MFHandler) redeem(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	var req mfdomain.RedeemRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		apiresponse.Error(w, apiresponse.Validation("invalid request body: %v", err))
		return
	}
	result, err := h.svc.Redeem(r.Context(), userID, req)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, result)
}

func (h *MFHandler) getTransactions(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	schemeCode := r.URL.Query().Get("scheme_code")
	txns, err := h.svc.GetTransactions(r.Context(), userID, schemeCode)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, txns)
}
