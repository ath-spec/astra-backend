package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/yourusername/astra-backend/internal/apiresponse"
)

// KYCHandler stubs the KYC/PAN Verification domain. The schema
// (kyc_verifications — see migration 000006) is ready, but the route is
// deliberately unimplemented until a KYC/PAN verification vendor is
// selected.
type KYCHandler struct{}

func NewKYCHandler() *KYCHandler {
	return &KYCHandler{}
}

func (h *KYCHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/pan/verify", h.notConfigured)
	return r
}

func (h *KYCHandler) notConfigured(w http.ResponseWriter, r *http.Request) {
	apiresponse.Error(w, apiresponse.NotImplemented("KYC/PAN verification provider is not yet configured"))
}
