package handler

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/yourusername/astra-backend/internal/service"
)

type ChatRequest struct {
	Messages []map[string]interface{} `json:"messages"`
}

type ChatHandler struct {
	aiService service.AIService
}

func NewChatHandler(aiService service.AIService) *ChatHandler {
	return &ChatHandler{
		aiService: aiService,
	}
}

func (h *ChatHandler) HandleChat(w http.ResponseWriter, r *http.Request) {
	var chatReq ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&chatReq); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	responseBytes, statusCode, err := h.aiService.GetChatCompletion(r.Context(), chatReq.Messages)
	if err != nil {
		log.Printf("AI Service Error: %v", err)
		respondWithError(w, statusCode, "Error processing chat request")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, _ = w.Write(responseBytes)
}

func respondWithError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
