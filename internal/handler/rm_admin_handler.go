package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/yourusername/astra-backend/internal/apiresponse"
	rmdomain "github.com/yourusername/astra-backend/internal/domain/rm"
	"github.com/yourusername/astra-backend/internal/httpx"
	"github.com/yourusername/astra-backend/internal/middleware"
	"github.com/yourusername/astra-backend/internal/service"
)

// RMAdminHandler serves the admin-only console: RM lifecycle, book
// oversight, and assignment operations. Mounted at /api/rm/admin behind
// RequireRMAuth + RequireAdmin.
type RMAdminHandler struct {
	svc *service.RMAdminService
}

func NewRMAdminHandler(svc *service.RMAdminService) *RMAdminHandler {
	return &RMAdminHandler{svc: svc}
}

// Register mounts the admin routes onto an already-authenticated,
// admin-gated router (see cmd/api/main.go).
func (h *RMAdminHandler) Register(r chi.Router) {
	r.Get("/overview", h.overview)

	r.Get("/rms", h.listRMs)
	r.Post("/rms", h.createRM)
	r.Get("/rms/{rmID}", h.rmDetail)
	r.Patch("/rms/{rmID}", h.updateRM)
	r.Get("/rms/{rmID}/clients", h.rmClients)
	r.Post("/rms/{rmID}/offboard", h.offboard)

	r.Get("/clients", h.listClients)

	r.Post("/assignments/assign", h.assign)
	r.Post("/assignments/transfer", h.transfer)
	r.Post("/assignments/remove", h.remove)
	r.Get("/assignments/history", h.history)
}

func (h *RMAdminHandler) actorID(r *http.Request) uuid.UUID {
	id, _ := middleware.GetRMID(r.Context())
	return id
}

func (h *RMAdminHandler) overview(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.Overview(r.Context())
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, res)
}

func (h *RMAdminHandler) listRMs(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.Roster(r.Context())
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, res)
}

func (h *RMAdminHandler) createRM(w http.ResponseWriter, r *http.Request) {
	var req rmdomain.CreateRMRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		apiresponse.Error(w, apiresponse.Validation("invalid request body: %v", err))
		return
	}
	res, err := h.svc.CreateRM(r.Context(), req)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.Created(w, res)
}

func (h *RMAdminHandler) rmDetail(w http.ResponseWriter, r *http.Request) {
	rmID, err := uuid.Parse(chi.URLParam(r, "rmID"))
	if err != nil {
		apiresponse.Error(w, apiresponse.Validation("invalid rm id"))
		return
	}
	res, err := h.svc.RMDetail(r.Context(), rmID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, res)
}

func (h *RMAdminHandler) updateRM(w http.ResponseWriter, r *http.Request) {
	rmID, err := uuid.Parse(chi.URLParam(r, "rmID"))
	if err != nil {
		apiresponse.Error(w, apiresponse.Validation("invalid rm id"))
		return
	}
	var req rmdomain.UpdateRMRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		apiresponse.Error(w, apiresponse.Validation("invalid request body: %v", err))
		return
	}
	res, err := h.svc.UpdateRM(r.Context(), rmID, req)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, res)
}

func (h *RMAdminHandler) rmClients(w http.ResponseWriter, r *http.Request) {
	rmID, err := uuid.Parse(chi.URLParam(r, "rmID"))
	if err != nil {
		apiresponse.Error(w, apiresponse.Validation("invalid rm id"))
		return
	}
	res, err := h.svc.RMClients(r.Context(), rmID, parseListFilters(r))
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, res)
}

func (h *RMAdminHandler) offboard(w http.ResponseWriter, r *http.Request) {
	rmID, err := uuid.Parse(chi.URLParam(r, "rmID"))
	if err != nil {
		apiresponse.Error(w, apiresponse.Validation("invalid rm id"))
		return
	}
	var req rmdomain.OffboardRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		apiresponse.Error(w, apiresponse.Validation("invalid request body: %v", err))
		return
	}
	moved, err := h.svc.Offboard(r.Context(), h.actorID(r), rmID, req.Reason)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, map[string]int{"clients_moved": moved})
}

func (h *RMAdminHandler) listClients(w http.ResponseWriter, r *http.Request) {
	var assigned *bool
	if v := r.URL.Query().Get("assigned"); v != "" {
		b := v == "true" || v == "1"
		assigned = &b
	}
	res, err := h.svc.ListClients(r.Context(), assigned, parseListFilters(r))
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, res)
}

func (h *RMAdminHandler) assign(w http.ResponseWriter, r *http.Request) {
	var req rmdomain.AssignRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		apiresponse.Error(w, apiresponse.Validation("invalid request body: %v", err))
		return
	}
	if err := h.svc.Assign(r.Context(), h.actorID(r), req); err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, map[string]string{"message": "assigned"})
}

func (h *RMAdminHandler) transfer(w http.ResponseWriter, r *http.Request) {
	var req rmdomain.TransferRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		apiresponse.Error(w, apiresponse.Validation("invalid request body: %v", err))
		return
	}
	if err := h.svc.Transfer(r.Context(), h.actorID(r), req); err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, map[string]string{"message": "transferred"})
}

func (h *RMAdminHandler) remove(w http.ResponseWriter, r *http.Request) {
	var req rmdomain.RemoveRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		apiresponse.Error(w, apiresponse.Validation("invalid request body: %v", err))
		return
	}
	if err := h.svc.Remove(r.Context(), h.actorID(r), req); err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, map[string]string{"message": "removed"})
}

func (h *RMAdminHandler) history(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var userID, rmID *uuid.UUID
	if v := q.Get("user_id"); v != "" {
		if id, err := uuid.Parse(v); err == nil {
			userID = &id
		}
	}
	if v := q.Get("rm_id"); v != "" {
		if id, err := uuid.Parse(v); err == nil {
			rmID = &id
		}
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	res, err := h.svc.History(r.Context(), userID, rmID, limit, offset)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, res)
}
