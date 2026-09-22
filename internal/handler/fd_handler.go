package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/yourusername/astra-backend/internal/apiresponse"
	fddomain "github.com/yourusername/astra-backend/internal/domain/fd"
	"github.com/yourusername/astra-backend/internal/httpx"
	"github.com/yourusername/astra-backend/internal/middleware"
	"github.com/yourusername/astra-backend/internal/service"
)

type FDHandler struct {
	svc *service.FDService
}

func NewFDHandler(svc *service.FDService) *FDHandler {
	return &FDHandler{svc: svc}
}

func (h *FDHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", h.openFD)
	r.Get("/", h.listFDs)
	r.Get("/{fdAccountNumber}", h.getFD)
	r.Post("/{fdAccountNumber}/withdraw", h.withdraw)
	return r
}

func (h *FDHandler) openFD(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	var req fddomain.OpenRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		apiresponse.Error(w, apiresponse.Validation("invalid request body: %v", err))
		return
	}
	account, err := h.svc.OpenFD(r.Context(), userID, req)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.Created(w, account)
}

func (h *FDHandler) listFDs(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	accounts, err := h.svc.ListFDs(r.Context(), userID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, accounts)
}

func (h *FDHandler) getFD(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	fdAccountNumber := chi.URLParam(r, "fdAccountNumber")
	account, err := h.svc.GetFD(r.Context(), userID, fdAccountNumber)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, account)
}

func (h *FDHandler) withdraw(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	fdAccountNumber := chi.URLParam(r, "fdAccountNumber")

	var req fddomain.WithdrawRequest
	if r.ContentLength != 0 {
		if err := httpx.DecodeJSON(w, r, &req); err != nil {
			apiresponse.Error(w, apiresponse.Validation("invalid request body: %v", err))
			return
		}
	}

	result, err := h.svc.Withdraw(r.Context(), userID, fdAccountNumber, req)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, result)
}
