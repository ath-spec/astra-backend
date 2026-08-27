package handler

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/yourusername/astra-backend/internal/apiresponse"
	"github.com/yourusername/astra-backend/internal/middleware"
	"github.com/yourusername/astra-backend/internal/service"
)

// RMChatHandler serves the staff copilot. It is mounted inside the
// RM-authenticated route group; the scope (rm vs admin) and system prompt are
// chosen from the caller's staff role, and history is stored per (staff,
// scope) — completely separate from the user app's /api/chat.
type RMChatHandler struct {
	svc *service.RMChatService
}

func NewRMChatHandler(svc *service.RMChatService) *RMChatHandler {
	return &RMChatHandler{svc: svc}
}

func (h *RMChatHandler) Register(r chi.Router) {
	r.Post("/chat", h.chat)
	r.Get("/chat/history", h.history)
	r.Post("/chat/new", h.newSession)
	r.Post("/chat/tts", h.tts)
	r.Post("/chat/stt", h.stt)
}

func (h *RMChatHandler) scope(r *http.Request) string {
	if middleware.IsAdmin(r.Context()) {
		return service.ScopeAdmin
	}
	return service.ScopeRM
}

type rmChatRequest struct {
	Messages []map[string]interface{} `json:"messages"`
	ClientID string                   `json:"client_id,omitempty"`
}

func (h *RMChatHandler) chat(w http.ResponseWriter, r *http.Request) {
	rmID, ok := middleware.GetRMID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	var req rmChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		apiresponse.Error(w, apiresponse.Validation("invalid request body"))
		return
	}
	if len(req.Messages) == 0 {
		apiresponse.Error(w, apiresponse.Validation("messages are required"))
		return
	}
	var clientID *uuid.UUID
	if req.ClientID != "" {
		if id, err := uuid.Parse(req.ClientID); err == nil {
			clientID = &id
		}
	}

	reply, err := h.svc.Chat(r.Context(), rmID, h.scope(r), clientID, req.Messages)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, map[string]string{"reply": reply})
}

func (h *RMChatHandler) history(w http.ResponseWriter, r *http.Request) {
	rmID, ok := middleware.GetRMID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	msgs, err := h.svc.History(r.Context(), rmID, h.scope(r))
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, map[string]interface{}{"messages": msgs})
}

func (h *RMChatHandler) newSession(w http.ResponseWriter, r *http.Request) {
	rmID, ok := middleware.GetRMID(r.Context())
	if !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	if err := h.svc.NewSession(r.Context(), rmID, h.scope(r)); err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, map[string]bool{"ok": true})
}

func (h *RMChatHandler) tts(w http.ResponseWriter, r *http.Request) {
	if _, ok := middleware.GetRMID(r.Context()); !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	var body struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Text == "" {
		apiresponse.Error(w, apiresponse.Validation("text is required"))
		return
	}
	raw, status, err := h.svc.TTS(r.Context(), body.Text)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(raw)
}

func (h *RMChatHandler) stt(w http.ResponseWriter, r *http.Request) {
	if _, ok := middleware.GetRMID(r.Context()); !ok {
		apiresponse.Error(w, apiresponse.ErrUnauthorized)
		return
	}
	// Accept either a multipart "file" upload or a raw audio body.
	var (
		audio    []byte
		filename = "speech.webm"
		err      error
	)
	if file, hdr, ferr := r.FormFile("file"); ferr == nil {
		defer file.Close()
		if hdr != nil && hdr.Filename != "" {
			filename = hdr.Filename
		}
		audio, err = io.ReadAll(io.LimitReader(file, 20<<20)) // 20 MB cap
	} else {
		audio, err = io.ReadAll(io.LimitReader(r.Body, 20<<20))
	}
	if err != nil {
		apiresponse.Error(w, apiresponse.Validation("could not read audio"))
		return
	}

	transcript, err := h.svc.Transcribe(r.Context(), audio, filename)
	if err != nil {
		apiresponse.Error(w, err)
		return
	}
	apiresponse.OK(w, map[string]string{"transcript": transcript})
}
