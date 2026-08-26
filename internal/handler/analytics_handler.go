package handler

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/yourusername/astra-backend/internal/apiresponse"
	"github.com/yourusername/astra-backend/internal/middleware"
	analyticsservice "github.com/yourusername/astra-backend/internal/service/analytics"
)

type AnalyticsHandler struct {
	svc *analyticsservice.Service
}

func NewAnalyticsHandler(svc *analyticsservice.Service) *AnalyticsHandler {
	return &AnalyticsHandler{svc: svc}
}

// Routes mounts the spend analytics engine under /api/v1/analytics/spend.
// See the plan doc for the mapping from these 11 analyzers back to the
// z-backend items they consolidate.
func (h *AnalyticsHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/weekday-weekend", h.weekdayWeekend)
	r.Get("/trends", h.trends)
	r.Get("/categories", h.categories)
	r.Get("/category-momentum", h.categoryMomentum)
	r.Get("/average", h.average)
	r.Get("/merchants", h.merchants)
	r.Get("/recurring", h.recurring)
	r.Get("/impulse", h.impulse)
	r.Get("/summary", h.summary)
	r.Get("/snapshot", h.snapshot)
	r.Get("/compare", h.compare)
	return r
}

func (h *AnalyticsHandler) weekdayWeekend(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	result, err := h.svc.WeekdayVsWeekend(r.Context(), userID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, result)
}

func (h *AnalyticsHandler) trends(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	period := r.URL.Query().Get("period")
	switch period {
	case "", "daily", "weekly", "monthly":
	default:
		apiresponse.Error(w, apiresponse.Validation("period must be one of daily, weekly, monthly"))
		return
	}
	result, err := h.svc.Trends(r.Context(), userID, period)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, result)
}

func (h *AnalyticsHandler) categories(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	result, err := h.svc.CategoryTrend(r.Context(), userID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, result)
}

func (h *AnalyticsHandler) categoryMomentum(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	result, err := h.svc.CategoryMomentum(r.Context(), userID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, result)
}

func (h *AnalyticsHandler) average(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	result, err := h.svc.AverageStats(r.Context(), userID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, result)
}

func (h *AnalyticsHandler) merchants(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	result, err := h.svc.MerchantAnalysis(r.Context(), userID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, result)
}

func (h *AnalyticsHandler) recurring(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	result, err := h.svc.RecurringDetection(r.Context(), userID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, result)
}

func (h *AnalyticsHandler) impulse(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	result, err := h.svc.NightAndImpulse(r.Context(), userID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, result)
}

func (h *AnalyticsHandler) summary(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	result, err := h.svc.PatternSummary(r.Context(), userID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, result)
}

func (h *AnalyticsHandler) snapshot(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	result, err := h.svc.Snapshot(r.Context(), userID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, result)
}

func (h *AnalyticsHandler) compare(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	by := r.URL.Query().Get("by")
	if by == "" {
		by = "category"
	}
	if by != "category" && by != "merchant" {
		apiresponse.Error(w, apiresponse.Validation("by must be category or merchant"))
		return
	}
	var names []string
	if raw := r.URL.Query().Get("names"); raw != "" {
		for _, n := range strings.Split(raw, ",") {
			if n = strings.TrimSpace(n); n != "" {
				names = append(names, n)
			}
		}
	}
	result, err := h.svc.Compare(r.Context(), userID, by, names)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, result)
}
