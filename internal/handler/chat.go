package handler

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/google/uuid"
	"github.com/yourusername/astra-backend/internal/middleware"
	"github.com/yourusername/astra-backend/internal/repository"
	"github.com/yourusername/astra-backend/internal/service"
)

type ChatRequest struct {
	Messages []map[string]interface{} `json:"messages"`
}

type ChatHandler struct {
	aiService service.AIService
	userRepo  repository.UserRepository
}

func NewChatHandler(aiService service.AIService, userRepo repository.UserRepository) *ChatHandler {
	return &ChatHandler{
		aiService: aiService,
		userRepo:  userRepo,
	}
}

func (h *ChatHandler) HandleChat(w http.ResponseWriter, r *http.Request) {
	// 1. Securely extract the User ID from the Context (set by JWT middleware)
	userIDValue := r.Context().Value(middleware.UserIDKey)
	if userIDValue == nil {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized: No user in context")
		return
	}
	
	userID, ok := userIDValue.(uuid.UUID)
	if !ok {
		respondWithError(w, http.StatusInternalServerError, "Internal Error: Invalid user ID format")
		return
	}

	// 2. Decode the incoming chat messages
	var chatReq ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&chatReq); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	// 3. Get the AI response and let the service handle saving the chat history
	responseBytes, statusCode, err := h.aiService.GetChatCompletion(r.Context(), userID, chatReq.Messages)
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
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": message,
		"code":  code,
	})
}
