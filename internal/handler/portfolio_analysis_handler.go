package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/yourusername/astra-backend/internal/apiresponse"
	"github.com/yourusername/astra-backend/internal/middleware"
	"github.com/yourusername/astra-backend/internal/service"
)

type PortfolioAnalysisHandler struct {
	svc *service.PortfolioAnalysisService
}

func NewPortfolioAnalysisHandler(svc *service.PortfolioAnalysisService) *PortfolioAnalysisHandler {
	return &PortfolioAnalysisHandler{svc: svc}
}

func (h *PortfolioAnalysisHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/allocation", h.allocation)
	r.Get("/discipline", h.discipline)
	r.Get("/performance", h.performance)
	r.Get("/simulate", h.simulate)
	r.Post("/simulate", h.simulate)
	return r
}

func (h *PortfolioAnalysisHandler) allocation(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	result, err := h.svc.Allocation(r.Context(), userID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, result)
}

func (h *PortfolioAnalysisHandler) discipline(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	result, err := h.svc.Discipline(r.Context(), userID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, result)
}

func (h *PortfolioAnalysisHandler) performance(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	result, err := h.svc.Performance(r.Context(), userID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, result)
}

func (h *PortfolioAnalysisHandler) simulate(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}

	schemeCode := r.URL.Query().Get("scheme_code")
	if schemeCode == "" {
		schemeCode = "PARAG-FLX-G"
	}

	amountStr := r.URL.Query().Get("amount")
	amount, err := strconv.ParseFloat(amountStr, 64)
	if err != nil || amount <= 0 {
		amount = 10000.00
	}

	result, err := h.svc.SimulatePurchase(r.Context(), userID, schemeCode, amount)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}

	apiresponse.OK(w, result)
}
