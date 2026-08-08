package handler

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/yourusername/astra-backend/internal/repository"
	"github.com/yourusername/astra-backend/internal/service"
)

type OTPRequest struct {
	PhoneNumber string `json:"phone_number"`
}

type VerifyRequest struct {
	AstraUserID string                   `json:"astra_user_id"`
	PhoneNumber string                   `json:"phone_number"`
	OTP         string                   `json:"otp"`
	Name        string                   `json:"name"`
	Banks       []repository.BankAccount `json:"banks"` // Dynamic UI accounts
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

// SendOTP handles the POST /api/auth/otp/send endpoint
func (h *AuthHandler) SendOTP(w http.ResponseWriter, r *http.Request) {
	var req OTPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondAuthError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	if req.PhoneNumber == "" {
		respondAuthError(w, http.StatusBadRequest, "Missing phone_number")
		return
	}

	// Mock OTP send for demo
	log.Printf("MOCK OTP: Sent OTP 1234 to %s", req.PhoneNumber)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "OTP sent successfully",
	})
}

// VerifyOTP handles the POST /api/auth/otp/verify endpoint
func (h *AuthHandler) VerifyOTP(w http.ResponseWriter, r *http.Request) {
	var req VerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondAuthError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	if req.AstraUserID == "" || req.PhoneNumber == "" {
		respondAuthError(w, http.StatusBadRequest, "Missing astra_user_id or phone_number")
		return
	}

	if req.OTP != "1234" {
		respondAuthError(w, http.StatusUnauthorized, "Invalid OTP")
		return
	}

	// 1. Find or Create User
	user, err := h.userRepo.FindOrCreateUser(r.Context(), req.AstraUserID, req.PhoneNumber, req.Name, req.Banks)
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
