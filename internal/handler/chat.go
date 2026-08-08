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
	chatRepo  repository.ChatRepository
}

func NewChatHandler(aiService service.AIService, userRepo repository.UserRepository, chatRepo repository.ChatRepository) *ChatHandler {
	return &ChatHandler{
		aiService: aiService,
		userRepo:  userRepo,
		chatRepo:  chatRepo,
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

	// 3. Context Enrichment! (Backend Patterns)
	// We inject a powerful financial system prompt hidden from the frontend.
	systemPrompt := map[string]interface{}{
		"role": "system",
		"content": `You are Astra, an elite financial and investment advisor AI. 
The user is asking you for investment advice, portfolio analysis, or budget tracking. 
Be concise, highly professional, and use specific numbers. 
Context: The user has linked 2 bank accounts. They hold ₹2,50,000 in Mutual Funds and ₹1,20,000 in stocks.`,
	}
	
	// Prepend the system prompt to the messages
	messagesWithContext := append([]map[string]interface{}{systemPrompt}, chatReq.Messages...)

	// 4. Get the AI response
	responseBytes, statusCode, err := h.aiService.GetChatCompletion(r.Context(), userID, messagesWithContext)
	if err != nil {
		log.Printf("AI Service Error: %v", err)
		respondWithError(w, statusCode, "Error processing chat request")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, _ = w.Write(responseBytes)
}

// GetHistory returns the user's existing chat messages
func (h *ChatHandler) GetHistory(w http.ResponseWriter, r *http.Request) {
	userIDValue := r.Context().Value(middleware.UserIDKey)
	if userIDValue == nil {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}
	
	userID := userIDValue.(uuid.UUID)
	session, err := h.chatRepo.GetSessionForUser(r.Context(), userID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to load chat history")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"messages": session.Messages,
	})
}

func respondWithError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": message,
		"code":  code,
	})
}
