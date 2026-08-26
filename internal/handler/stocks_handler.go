package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/yourusername/astra-backend/internal/apiresponse"
	stocksdomain "github.com/yourusername/astra-backend/internal/domain/stocks"
	"github.com/yourusername/astra-backend/internal/httpx"
	"github.com/yourusername/astra-backend/internal/middleware"
	"github.com/yourusername/astra-backend/internal/service"
)

type StocksHandler struct {
	svc *service.StocksService
}

func NewStocksHandler(svc *service.StocksService) *StocksHandler {
	return &StocksHandler{svc: svc}
}

// Routes mounts the Demat & Exchange endpoints. Every route here requires
// JWT auth via the parent router group (see cmd/api/main.go).
func (h *StocksHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/holdings", h.getHoldings)
	r.Get("/quote", h.getQuote)
	r.Post("/orders", h.placeOrder)
	r.Get("/orders", h.listOrders)
	r.Get("/orders/{orderID}", h.getOrder)
	r.Put("/orders/{orderID}", h.modifyOrder)
	r.Delete("/orders/{orderID}", h.cancelOrder)
	return r
}

func (h *StocksHandler) getHoldings(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	holdings, err := h.svc.GetHoldings(r.Context(), userID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, holdings)
}

func (h *StocksHandler) getQuote(w http.ResponseWriter, r *http.Request) {
	symbol := r.URL.Query().Get("trading_symbol")
	if symbol == "" {
		apiresponse.Error(w, apiresponse.Validation("trading_symbol query parameter is required"))
		return
	}
	exchange := r.URL.Query().Get("exchange")

	quote, err := h.svc.GetQuote(r.Context(), exchange, symbol)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, quote)
}

func (h *StocksHandler) placeOrder(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	var req stocksdomain.OrderRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		apiresponse.Error(w, apiresponse.Validation("invalid request body: %v", err))
		return
	}
	order, err := h.svc.PlaceOrder(r.Context(), userID, req)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.Created(w, order)
}

func (h *StocksHandler) modifyOrder(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	orderID := chi.URLParam(r, "orderID")

	var req stocksdomain.OrderRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		apiresponse.Error(w, apiresponse.Validation("invalid request body: %v", err))
		return
	}
	order, err := h.svc.ModifyOrder(r.Context(), userID, orderID, req)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, order)
}

func (h *StocksHandler) cancelOrder(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	orderID := chi.URLParam(r, "orderID")

	order, err := h.svc.CancelOrder(r.Context(), userID, orderID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, order)
}

func (h *StocksHandler) listOrders(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	statusFilter := r.URL.Query().Get("status_filter")
	orders, err := h.svc.ListOrders(r.Context(), userID, statusFilter)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, orders)
}

func (h *StocksHandler) getOrder(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	orderID := chi.URLParam(r, "orderID")

	order, err := h.svc.GetOrder(r.Context(), userID, orderID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, order)
}
