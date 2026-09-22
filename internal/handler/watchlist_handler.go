package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/yourusername/astra-backend/internal/apiresponse"
	"github.com/yourusername/astra-backend/internal/middleware"
	"github.com/yourusername/astra-backend/internal/service"
)

// WatchlistHandler serves the user's saved/bookmarked funds — the bookmark
// button on the fund profile screen and the Watchlist screen's list.
type WatchlistHandler struct {
	svc *service.WatchlistService
}

func NewWatchlistHandler(svc *service.WatchlistService) *WatchlistHandler {
	return &WatchlistHandler{svc: svc}
}

func (h *WatchlistHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.list)
	r.Put("/{schemeCode}", h.add)
	r.Delete("/{schemeCode}", h.remove)
	return r
}

func (h *WatchlistHandler) list(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	items, err := h.svc.List(r.Context(), userID)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, items)
}

func (h *WatchlistHandler) add(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	schemeCode := chi.URLParam(r, "schemeCode")
	if err := h.svc.Add(r.Context(), userID, schemeCode); err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, map[string]bool{"watched": true})
}

func (h *WatchlistHandler) remove(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	schemeCode := chi.URLParam(r, "schemeCode")
	if err := h.svc.Remove(r.Context(), userID, schemeCode); err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, map[string]bool{"watched": false})
}
