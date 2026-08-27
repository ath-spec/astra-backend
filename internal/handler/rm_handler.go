package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/yourusername/astra-backend/internal/apiresponse"
	rmdomain "github.com/yourusername/astra-backend/internal/domain/rm"
	"github.com/yourusername/astra-backend/internal/middleware"
	"github.com/yourusername/astra-backend/internal/service"
)

// RMHandler serves the RM-facing console endpoints (an admin token is also
// accepted — admins can view any RM's book). Mounted at /api/rm behind
// RequireRMAuth.
type RMHandler struct {
	svc *service.RMService
}

func NewRMHandler(svc *service.RMService) *RMHandler {
	return &RMHandler{svc: svc}
}

// Register mounts the RM-facing routes onto an already-authenticated
// router (see cmd/api/main.go). Not a standalone sub-router, so it can sit
// alongside the /admin sub-tree without a chi mount collision.
func (h *RMHandler) Register(r chi.Router) {
	r.Get("/dashboard/summary", h.bookSummary)
	r.Get("/clients", h.listClients)
	r.Get("/clients/{userID}", h.getClient)
	r.Get("/clients/{userID}/growth", h.clientGrowth)
	r.Get("/clients/{userID}/portfolio-history", h.portfolioHistory)
}

func parseListFilters(r *http.Request) rmdomain.ListFilters {
	q := r.URL.Query()
	f := rmdomain.ListFilters{
		Search: q.Get("search"),
		Sort:   q.Get("sort"),
		Order:  q.Get("order"),
	}
	if v, err := strconv.Atoi(q.Get("limit")); err == nil {
		f.Limit = v
	}
	if v, err := strconv.Atoi(q.Get("offset")); err == nil {
		f.Offset = v
	}
	return f
}

func (h *RMHandler) bookSummary(w http.ResponseWriter, r *http.Request) {
	rmID, ok := middleware.GetRMID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	res, err := h.svc.BookSummary(r.Context(), rmID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, res)
}

func (h *RMHandler) listClients(w http.ResponseWriter, r *http.Request) {
	rmID, ok := middleware.GetRMID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	res, err := h.svc.ListClients(r.Context(), rmID, parseListFilters(r))
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, res)
}

func (h *RMHandler) getClient(w http.ResponseWriter, r *http.Request) {
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
	days := 0
	if v, err := strconv.Atoi(r.URL.Query().Get("days")); err == nil {
		days = v
	}
	res, err := h.svc.GetClient(r.Context(), rmID, middleware.IsAdmin(r.Context()), userID, days)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, res)
}

func (h *RMHandler) portfolioHistory(w http.ResponseWriter, r *http.Request) {
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
	days := 365
	if v, err := strconv.Atoi(r.URL.Query().Get("days")); err == nil && v > 0 {
		days = v
	}
	res, err := h.svc.PortfolioHistory(r.Context(), rmID, middleware.IsAdmin(r.Context()), userID, days)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, res)
}

func (h *RMHandler) clientGrowth(w http.ResponseWriter, r *http.Request) {
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
	days := 180
	if v, err := strconv.Atoi(r.URL.Query().Get("days")); err == nil && v > 0 {
		days = v
	}
	res, err := h.svc.ClientGrowth(r.Context(), rmID, middleware.IsAdmin(r.Context()), userID, days)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, res)
}
