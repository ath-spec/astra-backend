package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/yourusername/astra-backend/internal/apiresponse"
	"github.com/yourusername/astra-backend/internal/service/rmcreditrisk"
)

// IDBIRMHandler serves the RM-portal IDBI features (feature 5: credit risk).
// Registered into the RM protected group only when its service is wired
// (IDBI_RM_RISK_ENABLED=true). Routes live under /api/rm/idbi.
type IDBIRMHandler struct {
	risk *rmcreditrisk.Service
}

func NewIDBIRMHandler(risk *rmcreditrisk.Service) *IDBIRMHandler {
	return &IDBIRMHandler{risk: risk}
}

func (h *IDBIRMHandler) Enabled() bool { return h.risk != nil }

// Register mounts routes on an already-authed RM router.
func (h *IDBIRMHandler) Register(r chi.Router) {
	if h.risk == nil {
		return
	}
	r.Route("/idbi", func(r chi.Router) {
		r.Get("/clients/{userID}/credit-risk", h.clientCreditRisk)
		r.Post("/clients/{userID}/credit-risk/refresh", h.refreshClientCreditRisk)
	})
}

func (h *IDBIRMHandler) clientCreditRisk(w http.ResponseWriter, r *http.Request) {
	uid, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		apiresponse.Error(w, apiresponse.Validation("invalid userID"))
		return
	}
	e, err := h.risk.Get(r.Context(), uid)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, e)
}

func (h *IDBIRMHandler) refreshClientCreditRisk(w http.ResponseWriter, r *http.Request) {
	uid, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		apiresponse.Error(w, apiresponse.Validation("invalid userID"))
		return
	}
	if err := h.risk.Refresh(r.Context(), uid); err != nil {
		apiresponse.Error(w, err)
		return
	}
	e, err := h.risk.Get(r.Context(), uid)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, map[string]any{"exposure": e, "refreshed": true})
}
