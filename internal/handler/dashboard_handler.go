package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/yourusername/astra-backend/internal/apiresponse"
	"github.com/yourusername/astra-backend/internal/middleware"
	"github.com/yourusername/astra-backend/internal/service"
)

// DashboardHandler serves the cross-domain portfolio summary the Home
// screen needs — total wealth and a per-asset-class breakdown across
// Stocks, MF, Fixed Deposits and bank accounts.
type DashboardHandler struct {
	svc *service.DashboardService
}

func NewDashboardHandler(svc *service.DashboardService) *DashboardHandler {
	return &DashboardHandler{svc: svc}
}

func (h *DashboardHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/summary", h.summary)
	r.Get("/growth", h.growth)
	return r
}

func (h *DashboardHandler) summary(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	result, err := h.svc.Summary(r.Context(), userID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, result)
}

func (h *DashboardHandler) growth(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	days := 180
	if v := r.URL.Query().Get("days"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			apiresponse.Error(w, apiresponse.Validation("days must be a positive integer"))
			return
		}
		days = n
	}
	points, err := h.svc.GrowthHistory(r.Context(), userID, days)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, points)
}
