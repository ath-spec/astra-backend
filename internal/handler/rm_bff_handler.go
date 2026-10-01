package handler

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/yourusername/astra-backend/internal/apiresponse"
	"github.com/yourusername/astra-backend/internal/middleware"
	"github.com/yourusername/astra-backend/internal/service/rmbff"
)

// RMBFFHandler serves the view-model endpoints the RM console renders
// directly, one call per screen. Mounted at /api/rm/bff behind RequireRMAuth.
type RMBFFHandler struct {
	svc *rmbff.Service
}

func NewRMBFFHandler(svc *rmbff.Service) *RMBFFHandler {
	return &RMBFFHandler{svc: svc}
}

func (h *RMBFFHandler) Register(r chi.Router) {
	r.Get("/bff/dashboard", h.dashboard)
	r.Get("/bff/portfolio", h.portfolio)
	r.Get("/bff/early-warnings", h.earlyWarnings)
	r.Get("/bff/clients/{userID}/customer-360", h.customer360)
	r.Get("/bff/clients/{userID}/cashflow", h.cashflow)
	r.Get("/bff/clients/{userID}/credit-risk", h.creditRisk)
}

func (h *RMBFFHandler) dashboard(w http.ResponseWriter, r *http.Request) {
	rmID, ok := middleware.GetRMID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	res, err := h.svc.Dashboard(r.Context(), rmID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, res)
}

func (h *RMBFFHandler) portfolio(w http.ResponseWriter, r *http.Request) {
	rmID, ok := middleware.GetRMID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	res, err := h.svc.Portfolio(r.Context(), rmID, rmbff.PortfolioQuery{Page: page, Tab: q.Get("tab"), Search: q.Get("search")})
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, res)
}

func (h *RMBFFHandler) customer360(w http.ResponseWriter, r *http.Request) {
	rmID, ok := middleware.GetRMID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	userID, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		apiresponse.Error(w, apiresponse.Validation("invalid user id"))
		return
	}
	res, err := h.svc.Customer360(r.Context(), rmID, middleware.IsAdmin(r.Context()), userID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, res)
}

func (h *RMBFFHandler) earlyWarnings(w http.ResponseWriter, r *http.Request) {
	rmID, ok := middleware.GetRMID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	res, err := h.svc.EarlyWarnings(r.Context(), rmID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, res)
}

// clientView serves a per-client view-model: it parses the {userID} path
// param and the caller's identity, then delegates to build.
func clientView[T any](w http.ResponseWriter, r *http.Request, build func(ctx context.Context, rmID uuid.UUID, isAdmin bool, userID uuid.UUID) (T, error)) {
	rmID, ok := middleware.GetRMID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	userID, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		apiresponse.Error(w, apiresponse.Validation("invalid user id"))
		return
	}
	res, err := build(r.Context(), rmID, middleware.IsAdmin(r.Context()), userID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, res)
}

func (h *RMBFFHandler) cashflow(w http.ResponseWriter, r *http.Request) {
	clientView(w, r, h.svc.CashFlow)
}
func (h *RMBFFHandler) creditRisk(w http.ResponseWriter, r *http.Request) {
	clientView(w, r, h.svc.CreditRisk)
}
