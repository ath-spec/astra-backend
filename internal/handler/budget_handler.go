package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	budgetdomain "github.com/yourusername/astra-backend/internal/domain/budget"
	"github.com/yourusername/astra-backend/internal/middleware"
	budgetprovider "github.com/yourusername/astra-backend/internal/provider/budget"
	"github.com/yourusername/astra-backend/internal/repository"
	budgetservice "github.com/yourusername/astra-backend/internal/service/budget"
)

// BudgetHandler serves the budget feature under /api/v1/analytics/budgets.
// Route names and JSON bodies match the reference budget API exactly (raw
// bodies, no apiresponse envelope) so the ported Flutter client is a drop-in.
type BudgetHandler struct {
	svc *budgetservice.Service
}

func NewBudgetHandler(svc *budgetservice.Service) *BudgetHandler {
	return &BudgetHandler{svc: svc}
}

func (h *BudgetHandler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Post("/diagnosis", h.diagnosis)
	r.Post("/suggest/categories", h.suggestCategories)

	r.Post("/sessions", h.createSession)
	r.Get("/sessions/{id}", h.getSession)
	r.Patch("/sessions/{id}", h.updateSession)
	r.Post("/sessions/{id}/finalize", h.finalizeSession)
	r.Delete("/sessions/{id}", h.deleteSession)

	r.Get("/status", h.status)
	r.Get("/latest", h.latest)
	r.Patch("/latest/categories", h.patchLatestCategory)

	r.Get("/settings", h.getSettings)
	r.Patch("/settings", h.updateSettings)

	r.Delete("/active", h.resetActive)

	r.Get("/insights", h.insights)
	r.Get("/global-categories", h.globalCategories)

	// Smart Rebalance (reallocation) + ML feedback loop.
	r.Post("/reallocation/run", h.reallocationRun)
	r.Post("/reallocation/apply", h.reallocationApply)
	r.Get("/ml/suggestions", h.listMLSuggestions)
	r.Post("/ml/suggestions/{id}/respond", h.respondMLSuggestion)
	r.Get("/ml/cooldowns", h.listMLCooldowns)

	// Month-rollover triggers, scoped per-user (the user API has no admin
	// role).
	r.Post("/admin/trigger-rollover", h.triggerRollover)
	r.Post("/admin/retry-fallback-rollovers", h.retryFallbackRollovers)

	return r
}

// ─── helpers ───────────────────────────────────────────────────────────

func budgetUser(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	uid, ok := middleware.GetUserID(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return uuid.Nil, false
	}
	return uid, true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func sessionID(r *http.Request) (uuid.UUID, error) {
	return uuid.Parse(chi.URLParam(r, "id"))
}

// ─── diagnosis / suggest ───────────────────────────────────────────────

func (h *BudgetHandler) diagnosis(w http.ResponseWriter, r *http.Request) {
	uid, ok := budgetUser(w, r)
	if !ok {
		return
	}
	var req budgetdomain.DiagnosisRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	resp, err := h.svc.Diagnosis(r.Context(), uid, req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "diagnosis failed"})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *BudgetHandler) suggestCategories(w http.ResponseWriter, r *http.Request) {
	uid, ok := budgetUser(w, r)
	if !ok {
		return
	}
	var req budgetdomain.SuggestCategoriesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.TotalBudget <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "total_budget must be positive"})
		return
	}

	resp, err := h.svc.SuggestCategories(r.Context(), uid, req)
	if err != nil {
		var conflict *budgetservice.ConflictError
		if errors.As(err, &conflict) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write(conflict.RawBody)
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "suggestion failed"})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// ─── sessions ──────────────────────────────────────────────────────────

func (h *BudgetHandler) createSession(w http.ResponseWriter, r *http.Request) {
	uid, ok := budgetUser(w, r)
	if !ok {
		return
	}
	var req budgetdomain.CreateSessionRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	resp, err := h.svc.CreateSession(r.Context(), uid, req.Month)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not create session"})
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (h *BudgetHandler) getSession(w http.ResponseWriter, r *http.Request) {
	uid, ok := budgetUser(w, r)
	if !ok {
		return
	}
	sid, err := sessionID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid session id"})
		return
	}
	resp, err := h.svc.GetSession(r.Context(), uid, sid)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success", "data": resp})
}

func (h *BudgetHandler) updateSession(w http.ResponseWriter, r *http.Request) {
	uid, ok := budgetUser(w, r)
	if !ok {
		return
	}
	sid, err := sessionID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid session id"})
		return
	}
	var req budgetdomain.UpdateSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	resp, err := h.svc.UpdateSession(r.Context(), uid, sid, req)
	if err != nil {
		if errors.Is(err, repository.ErrBudgetSessionNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not update session"})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *BudgetHandler) finalizeSession(w http.ResponseWriter, r *http.Request) {
	uid, ok := budgetUser(w, r)
	if !ok {
		return
	}
	sid, err := sessionID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid session id"})
		return
	}
	if err := h.svc.FinalizeSession(r.Context(), uid, sid); err != nil {
		if errors.Is(err, repository.ErrBudgetSessionNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not finalize session"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"session_id": sid.String(),
		"status":     "finalized",
		"_links":     "/api/v1/analytics/budgets/latest",
	})
}

func (h *BudgetHandler) deleteSession(w http.ResponseWriter, r *http.Request) {
	uid, ok := budgetUser(w, r)
	if !ok {
		return
	}
	sid, err := sessionID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid session id"})
		return
	}
	if err := h.svc.DeleteSession(r.Context(), uid, sid); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not delete session"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "Session deleted successfully"})
}

// ─── status / latest / patch / reset ───────────────────────────────────

func (h *BudgetHandler) status(w http.ResponseWriter, r *http.Request) {
	uid, ok := budgetUser(w, r)
	if !ok {
		return
	}
	resp, err := h.svc.Status(r.Context(), uid)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load status"})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *BudgetHandler) latest(w http.ResponseWriter, r *http.Request) {
	uid, ok := budgetUser(w, r)
	if !ok {
		return
	}
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			limit = n
		}
	}
	resp, err := h.svc.Latest(r.Context(), uid, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load budget"})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *BudgetHandler) patchLatestCategory(w http.ResponseWriter, r *http.Request) {
	uid, ok := budgetUser(w, r)
	if !ok {
		return
	}
	var req budgetdomain.PatchCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if err := h.svc.PatchLatestCategory(r.Context(), uid, req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "success", "message": "Category budget updated successfully"})
}

func (h *BudgetHandler) resetActive(w http.ResponseWriter, r *http.Request) {
	uid, ok := budgetUser(w, r)
	if !ok {
		return
	}
	msg, err := h.svc.ResetActive(r.Context(), uid)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not reset budget"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "reset", "message": msg})
}

// ─── settings / insights / categories ──────────────────────────────────

func (h *BudgetHandler) getSettings(w http.ResponseWriter, r *http.Request) {
	uid, ok := budgetUser(w, r)
	if !ok {
		return
	}
	resp, err := h.svc.GetSettings(r.Context(), uid)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load settings"})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *BudgetHandler) updateSettings(w http.ResponseWriter, r *http.Request) {
	uid, ok := budgetUser(w, r)
	if !ok {
		return
	}
	var req budgetdomain.UpdateSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if err := h.svc.UpdateSettings(r.Context(), uid, req); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not update settings"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

func (h *BudgetHandler) insights(w http.ResponseWriter, r *http.Request) {
	uid, ok := budgetUser(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, h.svc.Insights(r.Context(), uid))
}

func (h *BudgetHandler) globalCategories(w http.ResponseWriter, r *http.Request) {
	if _, ok := budgetUser(w, r); !ok {
		return
	}
	resp, err := h.svc.GlobalCategories(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not load categories"})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// ─── Smart Rebalance + ML feedback ─────────────────────────────────────

func (h *BudgetHandler) reallocationRun(w http.ResponseWriter, r *http.Request) {
	uid, ok := budgetUser(w, r)
	if !ok {
		return
	}
	var req budgetdomain.RunReallocationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	resp, err := h.svc.RunReallocation(r.Context(), uid, req)
	if err != nil {
		if errors.Is(err, budgetprovider.ErrUnavailable) {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"error": "Budget Manager service unavailable", "code": "SERVICE_UNAVAILABLE",
			})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *BudgetHandler) reallocationApply(w http.ResponseWriter, r *http.Request) {
	uid, ok := budgetUser(w, r)
	if !ok {
		return
	}
	var req budgetdomain.ApplyReallocationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	resp, err := h.svc.ApplyReallocation(r.Context(), uid, req)
	if err != nil {
		if errors.Is(err, budgetprovider.ErrUnavailable) {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"error": "Budget Manager service unavailable", "code": "SERVICE_UNAVAILABLE",
			})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *BudgetHandler) listMLSuggestions(w http.ResponseWriter, r *http.Request) {
	uid, ok := budgetUser(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, h.svc.ListSuggestions(r.Context(), uid))
}

func (h *BudgetHandler) listMLCooldowns(w http.ResponseWriter, r *http.Request) {
	uid, ok := budgetUser(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, h.svc.ListCooldowns(r.Context(), uid))
}

func (h *BudgetHandler) triggerRollover(w http.ResponseWriter, r *http.Request) {
	uid, ok := budgetUser(w, r)
	if !ok {
		return
	}
	draftID, err := h.svc.RolloverForUser(r.Context(), uid)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "rollover failed"})
		return
	}
	if draftID == "" {
		writeJSON(w, http.StatusOK, map[string]string{"status": "noop", "message": "no rollover needed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status":                    "created",
		"pending_rollover_draft_id": draftID,
		"_links":                    "/api/v1/analytics/budgets/sessions/" + draftID,
	})
}

func (h *BudgetHandler) retryFallbackRollovers(w http.ResponseWriter, r *http.Request) {
	if _, ok := budgetUser(w, r); !ok {
		return
	}
	// Async — respond immediately.
	go func() {
		h.svc.RunRollover(context.Background())
	}()
	writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

func (h *BudgetHandler) respondMLSuggestion(w http.ResponseWriter, r *http.Request) {
	uid, ok := budgetUser(w, r)
	if !ok {
		return
	}
	sid, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid suggestion id"})
		return
	}
	var req budgetdomain.RespondSuggestionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if err := h.svc.RespondToSuggestion(r.Context(), uid, sid, req); err != nil {
		if errors.Is(err, repository.ErrBudgetSuggestionNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "suggestion not found"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "success", "message": "Feedback recorded: " + req.Action})
}
