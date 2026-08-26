package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/yourusername/astra-backend/internal/apiresponse"
	paymentsdomain "github.com/yourusername/astra-backend/internal/domain/payments"
	"github.com/yourusername/astra-backend/internal/httpx"
	"github.com/yourusername/astra-backend/internal/middleware"
	"github.com/yourusername/astra-backend/internal/service"
)

type PaymentsHandler struct {
	svc *service.PaymentsService
}

func NewPaymentsHandler(svc *service.PaymentsService) *PaymentsHandler {
	return &PaymentsHandler{svc: svc}
}

func (h *PaymentsHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/payments", h.initiatePayment)
	r.Get("/payments/{paymentID}", h.getPayment)
	r.Post("/mandates", h.createMandate)
	r.Get("/mandates", h.listMandates)
	r.Get("/mandates/summary", h.recurringSummary)
	r.Post("/mandates/{mandateID}/action", h.mandateAction)
	r.Get("/mandates/{mandateID}/history", h.mandateHistory)
	return r
}

func (h *PaymentsHandler) initiatePayment(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	var req paymentsdomain.PaymentRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		apiresponse.Error(w, apiresponse.Validation("invalid request body: %v", err))
		return
	}
	payment, err := h.svc.InitiatePayment(r.Context(), userID, req)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.Created(w, payment)
}

func (h *PaymentsHandler) getPayment(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	paymentID := chi.URLParam(r, "paymentID")
	payment, err := h.svc.GetPayment(r.Context(), userID, paymentID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, payment)
}

func (h *PaymentsHandler) createMandate(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	var req paymentsdomain.MandateRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		apiresponse.Error(w, apiresponse.Validation("invalid request body: %v", err))
		return
	}
	mandate, err := h.svc.CreateMandate(r.Context(), userID, req)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.Created(w, mandate)
}

func (h *PaymentsHandler) listMandates(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	statusFilter := r.URL.Query().Get("status_filter")
	mandates, err := h.svc.ListMandates(r.Context(), userID, statusFilter)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, mandates)
}

func (h *PaymentsHandler) mandateAction(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	mandateID := chi.URLParam(r, "mandateID")

	var req paymentsdomain.MandateActionRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		apiresponse.Error(w, apiresponse.Validation("invalid request body: %v", err))
		return
	}
	result, err := h.svc.MandateAction(r.Context(), userID, mandateID, req)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, result)
}

func (h *PaymentsHandler) mandateHistory(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	mandateID := chi.URLParam(r, "mandateID")
	history, err := h.svc.MandateHistory(r.Context(), userID, mandateID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, history)
}

func (h *PaymentsHandler) recurringSummary(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	summary, err := h.svc.RecurringSummary(r.Context(), userID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, summary)
}
