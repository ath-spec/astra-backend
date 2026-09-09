package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/yourusername/astra-backend/internal/apiresponse"
	"github.com/yourusername/astra-backend/internal/middleware"
	"github.com/yourusername/astra-backend/internal/service"
	"github.com/yourusername/astra-backend/internal/service/advisortips"
)

type PortfolioAnalysisHandler struct {
	svc  *service.PortfolioAnalysisService
	tips *advisortips.Service // nil unless an LLM provider is configured
}

func NewPortfolioAnalysisHandler(svc *service.PortfolioAnalysisService) *PortfolioAnalysisHandler {
	return &PortfolioAnalysisHandler{svc: svc}
}

// WithTips attaches the AI pro-tip service (Allocation / Discipline /
// Performance / fund-profile agents). When absent, the /tip routes are not
// registered and the tabs simply render without a tip.
func (h *PortfolioAnalysisHandler) WithTips(t *advisortips.Service) *PortfolioAnalysisHandler {
	h.tips = t
	return h
}

func (h *PortfolioAnalysisHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/allocation", h.allocation)
	r.Get("/discipline", h.discipline)
	r.Get("/performance", h.performance)
	r.Get("/simulate", h.simulate)
	r.Post("/simulate", h.simulate)

	if h.tips != nil {
		r.Get("/allocation/tip", h.tip(advisortips.TopicAllocation))
		r.Get("/discipline/tip", h.tip(advisortips.TopicDiscipline))
		r.Get("/performance/tip", h.tip(advisortips.TopicPerformance))
		r.Post("/fund-tip", h.fundTip)
	}
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

// tip returns a handler that computes the given tab's analysis and asks that
// tab's agent for a short tip. A provider outage degrades to 200 with an
// empty tip rather than failing the tab.
func (h *PortfolioAnalysisHandler) tip(topic advisortips.Topic) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := middleware.GetUserID(r.Context())
		if !ok {
			apiresponse.Error(w, apiresponse.ErrUnauthorized)
			return
		}

		var (
			payload any
			err     error
		)
		switch topic {
		case advisortips.TopicAllocation:
			payload, err = h.svc.Allocation(r.Context(), userID)
		case advisortips.TopicDiscipline:
			payload, err = h.svc.Discipline(r.Context(), userID)
		case advisortips.TopicPerformance:
			payload, err = h.svc.Performance(r.Context(), userID)
		default:
			apiresponse.Error(w, apiresponse.Validation("unknown tip topic"))
			return
		}
		if err != nil {
			apiresponse.Error(w, err)
			return
		}

		tp, terr := h.tips.Generate(r.Context(), userID.String(), topic, payload)
		if terr != nil {
			// The tip is a non-essential embellishment on an analysis tab that
			// has already rendered. Any failure — unconfigured provider, model
			// outage, timeout — degrades to "no tip", never an error response.
			slog.Warn("advisortip generation failed", "topic", topic, "error", terr)
			apiresponse.OK(w, map[string]any{"topic": topic, "tip": "", "available": false})
			return
		}
		apiresponse.OK(w, tp)
	}
}

type fundTipRequest struct {
	SchemeCode string `json:"scheme_code"`
	// Profile is the fund-profile object the screen already renders (category,
	// expense ratio, risk band, role in portfolio, …). Passed straight to the
	// agent as context.
	Profile map[string]any `json:"profile"`
}

func (h *PortfolioAnalysisHandler) fundTip(w http.ResponseWriter, r *http.Request) {
	if _, ok := middleware.GetUserID(r.Context()); !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	var req fundTipRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiresponse.Error(w, apiresponse.Validation("invalid request body: %v", err))
		return
	}
	if req.SchemeCode == "" {
		apiresponse.Error(w, apiresponse.Validation("scheme_code is required"))
		return
	}
	subject := "fund:" + req.SchemeCode
	tp, err := h.tips.Generate(r.Context(), subject, advisortips.TopicFundProfile, req.Profile)
	if err != nil {
		slog.Warn("advisortip generation failed", "topic", advisortips.TopicFundProfile, "scheme", req.SchemeCode, "error", err)
		apiresponse.OK(w, map[string]any{"topic": advisortips.TopicFundProfile, "tip": "", "available": false})
		return
	}
	apiresponse.OK(w, tp)
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
