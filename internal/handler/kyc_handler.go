package handler

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/yourusername/astra-backend/internal/apiresponse"
	"github.com/yourusername/astra-backend/internal/events"
	"github.com/yourusername/astra-backend/internal/httpx"
	"github.com/yourusername/astra-backend/internal/middleware"
	"github.com/yourusername/astra-backend/internal/service/idbikyc"
)

// KYCHandler serves KYC / PAN verification. When an idbikyc.Service is wired
// (IDBI_KYC_ENABLED=true) POST /pan/verify runs a real CKYC search via IDBI
// 415; otherwise it returns notConfigured as before.
type KYCHandler struct {
	svc    *idbikyc.Service // nil => not configured
	events *events.Publisher
}

func NewKYCHandler(svc *idbikyc.Service) *KYCHandler {
	return &KYCHandler{svc: svc}
}

// WithEvents attaches the live-update publisher so a newly verified PAN
// shows up on the RM's account-details view without a manual refresh.
func (h *KYCHandler) WithEvents(pub *events.Publisher) *KYCHandler {
	h.events = pub
	return h
}

func (h *KYCHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/pan/verify", h.verifyPAN)
	return r
}

type verifyPANRequest struct {
	PAN string `json:"pan"`
}

func (h *KYCHandler) verifyPAN(w http.ResponseWriter, r *http.Request) {
	if h.svc == nil {
		apiresponse.Error(w, apiresponse.NotImplemented("KYC/PAN verification provider is not yet configured"))
		return
	}
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	var req verifyPANRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		apiresponse.Error(w, apiresponse.Validation("invalid request body: %v", err))
		return
	}
	res, err := h.svc.VerifyPAN(r.Context(), userID, req.PAN)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	if h.events != nil {
		go h.events.UserChanged(context.Background(), userID, events.TypeProfileUpdated)
	}
	apiresponse.OK(w, res)
}
