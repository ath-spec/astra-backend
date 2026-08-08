package handler

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/yourusername/astra-backend/internal/repository"
	"github.com/yourusername/astra-backend/internal/service"
)

type TokenRequest struct {
	AstraUserID string `json:"astra_user_id"`
	PhoneNumber string `json:"phone_number"`
	Name        string `json:"name"`
}

type AuthHandler struct {
	authService *service.AuthService
	userRepo    repository.UserRepository
}

func NewAuthHandler(authService *service.AuthService, userRepo repository.UserRepository) *AuthHandler {
	return &AuthHandler{
		authService: authService,
		userRepo:    userRepo,
	}
}

// GenerateToken handles the POST /api/auth/token endpoint
func (h *AuthHandler) GenerateToken(w http.ResponseWriter, r *http.Request) {
	var req TokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondAuthError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	if req.AstraUserID == "" || req.PhoneNumber == "" {
		respondAuthError(w, http.StatusBadRequest, "Missing astra_user_id or phone_number")
		return
	}

	// 1. Find or Create User
	user, err := h.userRepo.FindOrCreateUser(r.Context(), req.AstraUserID, req.PhoneNumber, req.Name)
	if err != nil {
		log.Printf("User DB Error: %v", err)
		respondAuthError(w, http.StatusInternalServerError, "Error identifying user")
		return
	}

	// 2. Generate JWT for the verified user
	tokenString, err := h.authService.GenerateToken(user.ID)
	if err != nil {
		log.Printf("Auth Service Error: %v", err)
		respondAuthError(w, http.StatusInternalServerError, "Error generating token")
		return
	}

	// 3. Return the Token
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"token": tokenString,
	})
}

func respondAuthError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": message,
		"code":  code,
	})
}
