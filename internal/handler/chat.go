package handler

import (
	"encoding/json"
	"log"
	"net/http"

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
	// 1. Extract identification headers from the app
	astraUserID := r.Header.Get("X-Astra-User-Id")
	phoneNumber := r.Header.Get("X-Astra-Phone-Number")

	if astraUserID == "" || phoneNumber == "" {
		respondWithError(w, http.StatusBadRequest, "Missing X-Astra-User-Id or X-Astra-Phone-Number headers")
		return
	}

	// 2. Decode the incoming chat messages
	var chatReq ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&chatReq); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	// 3. Find or Create the User in the Database
	user, err := h.userRepo.FindOrCreateUser(r.Context(), astraUserID, phoneNumber)
	if err != nil {
		log.Printf("User DB Error: %v", err)
		respondWithError(w, http.StatusInternalServerError, "Error identifying user")
		return
	}

	// 4. Get the AI response and let the service handle saving the chat history
	responseBytes, statusCode, err := h.aiService.GetChatCompletion(r.Context(), user.ID, chatReq.Messages)
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
