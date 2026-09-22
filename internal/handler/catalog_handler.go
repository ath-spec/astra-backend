package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/yourusername/astra-backend/internal/apiresponse"
	catalogdomain "github.com/yourusername/astra-backend/internal/domain/catalog"
	"github.com/yourusername/astra-backend/internal/middleware"
	"github.com/yourusername/astra-backend/internal/service"
)

type CatalogHandler struct {
	svc *service.CatalogService
}

func NewCatalogHandler(svc *service.CatalogService) *CatalogHandler {
	return &CatalogHandler{svc: svc}
}

func (h *CatalogHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/funds", h.searchFunds)
	r.Get("/funds/{schemeCode}", h.getFund)
	r.Get("/funds/{schemeCode}/profile", h.getFundProfile)
	r.Get("/nfos", h.listNFOs)
	return r
}

func (h *CatalogHandler) searchFunds(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	params := catalogdomain.SearchParams{
		Category:  q.Get("category"),
		RiskLevel: q.Get("risk_level"),
	}

	if v := q.Get("min_investment_max"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			apiresponse.Error(w, apiresponse.Validation("min_investment_max must be a number"))
			return
		}
		params.MinInvestmentMax = &f
	}
	if v := q.Get("page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			apiresponse.Error(w, apiresponse.Validation("page must be a positive integer"))
			return
		}
		params.Page = n
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			apiresponse.Error(w, apiresponse.Validation("limit must be a positive integer"))
			return
		}
		params.Limit = n
	}

	funds, err := h.svc.SearchFunds(r.Context(), params)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, funds)
}

func (h *CatalogHandler) getFund(w http.ResponseWriter, r *http.Request) {
	schemeCode := chi.URLParam(r, "schemeCode")
	fund, err := h.svc.GetFund(r.Context(), schemeCode)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, fund)
}

func (h *CatalogHandler) getFundProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	schemeCode := chi.URLParam(r, "schemeCode")
	profile, err := h.svc.GetFundProfile(r.Context(), userID, schemeCode)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, profile)
}

func (h *CatalogHandler) listNFOs(w http.ResponseWriter, r *http.Request) {
	nfos, err := h.svc.ListNFOs(r.Context())
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, nfos)
}
