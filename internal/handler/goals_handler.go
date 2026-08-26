package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/yourusername/astra-backend/internal/apiresponse"
	goalsdomain "github.com/yourusername/astra-backend/internal/domain/goals"
	"github.com/yourusername/astra-backend/internal/httpx"
	"github.com/yourusername/astra-backend/internal/middleware"
	"github.com/yourusername/astra-backend/internal/service"
)

type GoalsHandler struct {
	svc *service.GoalsService
}

func NewGoalsHandler(svc *service.GoalsService) *GoalsHandler {
	return &GoalsHandler{svc: svc}
}

func (h *GoalsHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", h.createGoal)
	r.Get("/", h.listGoals)
	r.Get("/summary", h.summary)
	r.Get("/{goalID}", h.getGoal)
	return r
}

func (h *GoalsHandler) createGoal(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	var req goalsdomain.CreateGoalRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		apiresponse.Error(w, apiresponse.Validation("invalid request body: %v", err))
		return
	}
	goal, err := h.svc.CreateGoal(r.Context(), userID, req)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.Created(w, goal)
}

func (h *GoalsHandler) listGoals(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	goalsList, err := h.svc.ListGoals(r.Context(), userID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, goalsList)
}

func (h *GoalsHandler) getGoal(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	goalID := chi.URLParam(r, "goalID")
	goal, err := h.svc.GetGoal(r.Context(), userID, goalID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, goal)
}

func (h *GoalsHandler) summary(w http.ResponseWriter, r *http.Request) {
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
