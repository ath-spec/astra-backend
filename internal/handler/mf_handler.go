package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/yourusername/astra-backend/internal/apiresponse"
)

// MFHandler stubs the MF Central / CAS domain (holdings, purchase/SIP,
// redemption). The schema (mf_folios, mf_transactions — see migration
// 000006) is ready, but the routes are deliberately unimplemented until a
// CAMS/KFintech-style RTA integration is selected.
type MFHandler struct{}

func NewMFHandler() *MFHandler {
	return &MFHandler{}
}

func (h *MFHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/cas", h.notConfigured)
	r.Post("/purchase", h.notConfigured)
	r.Post("/redeem", h.notConfigured)
	return r
}

func (h *MFHandler) notConfigured(w http.ResponseWriter, r *http.Request) {
	apiresponse.Error(w, apiresponse.NotImplemented("MF Central/CAS provider is not yet configured"))
}
