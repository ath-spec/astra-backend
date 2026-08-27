package handler

import (
	"net/http"

	"github.com/yourusername/astra-backend/internal/apiresponse"
	rmdomain "github.com/yourusername/astra-backend/internal/domain/rm"
	"github.com/yourusername/astra-backend/internal/httpx"
	"github.com/yourusername/astra-backend/internal/middleware"
	"github.com/yourusername/astra-backend/internal/service"
)

// RMAuthHandler serves the RM/Admin console's email+password auth flow. It
// is mounted at /api/rm/auth and is completely independent of the user OTP
// auth in auth.go.
type RMAuthHandler struct {
	svc *service.RMAuthService
}

func NewRMAuthHandler(svc *service.RMAuthService) *RMAuthHandler {
	return &RMAuthHandler{svc: svc}
}

func (h *RMAuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req rmdomain.LoginRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		apiresponse.Error(w, apiresponse.Validation("invalid request body: %v", err))
		return
	}
	pair, err := h.svc.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, pair)
}

func (h *RMAuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req rmdomain.RefreshRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		apiresponse.Error(w, apiresponse.Validation("invalid request body: %v", err))
		return
	}
	pair, err := h.svc.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, pair)
}

func (h *RMAuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	var req rmdomain.RefreshRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		apiresponse.Error(w, apiresponse.Validation("invalid request body: %v", err))
		return
	}
	if err := h.svc.Logout(r.Context(), req.RefreshToken); err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, map[string]string{"message": "logged out"})
}

func (h *RMAuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	rmID, ok := middleware.GetRMID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	profile, err := h.svc.Me(r.Context(), rmID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, profile)
}
