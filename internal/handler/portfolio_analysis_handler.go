package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/yourusername/astra-backend/internal/apiresponse"
	"github.com/yourusername/astra-backend/internal/middleware"
	"github.com/yourusername/astra-backend/internal/service"
)

// PortfolioAnalysisHandler serves the Portfolio Analysis screen's tabs.
// Only Allocation is implemented so far — it's computed live from the
// user's real MF/Stocks/FD holdings. Discipline and Performance need
// further design (SIP schedule history, benchmark data) and aren't wired
// yet.
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
