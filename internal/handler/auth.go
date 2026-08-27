package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/yourusername/astra-backend/internal/middleware"
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
	WantsRM     bool                     `json:"wants_rm"` // advisory opt-in from the signup form
	Banks       []repository.BankAccount `json:"banks"`    // Dynamic UI accounts
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
	middleware.L(r.Context()).Info("mock OTP sent", "phone", req.PhoneNumber, "otp", "123456")

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

	// TODO: no real OTP provider is wired in yet (SendOTP below is a mock
	// that doesn't actually send anything) — every non-empty OTP is accepted
	// for now so the client's real login flow can be built and tested ahead
	// of that integration. Replace this with a real check once a real
	// SMS/OTP provider is chosen.
	if req.OTP == "" {
		respondAuthError(w, http.StatusBadRequest, "Missing otp")
		return
	}

	// 1. Find or Create User
	user, isNewUser, err := h.userRepo.FindOrCreateUser(r.Context(), req.AstraUserID, req.PhoneNumber, req.Name, req.WantsRM, req.Banks)
	if err != nil {
		middleware.L(r.Context()).Error("find or create user", "error", err)
		respondAuthError(w, http.StatusInternalServerError, "Error identifying user")
		return
	}

	// 2. Generate JWT for the verified user
	tokenString, err := h.authService.GenerateToken(user.ID)
	if err != nil {
		middleware.L(r.Context()).Error("generate access token", "error", err)
		respondAuthError(w, http.StatusInternalServerError, "Error generating token")
		return
	}

	// 2b. Also issue a long-lived refresh token, so the client can silently
	// obtain a new access token after the short-lived JWT expires instead of
	// forcing the user through OTP verification again on every app launch.
	refreshToken, refreshHash, err := h.authService.GenerateRefreshToken()
	if err != nil {
		middleware.L(r.Context()).Error("generate refresh token", "error", err)
		respondAuthError(w, http.StatusInternalServerError, "Error generating refresh token")
		return
	}
	if err := h.userRepo.CreateRefreshToken(r.Context(), user.ID, refreshHash, time.Now().Add(service.RefreshTokenTTL)); err != nil {
		middleware.L(r.Context()).Error("persist refresh token", "error", err)
		respondAuthError(w, http.StatusInternalServerError, "Error persisting refresh token")
		return
	}

	// 3. Return the tokens plus is_new_user, so the client can route to
	// onboarding (new user) or straight to home (returning user) instead of
	// always showing the onboarding flow.
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"token":         tokenString,
		"refresh_token": refreshToken,
		"is_new_user":   isNewUser,
	})
}

// RefreshTokenRequest is the POST /api/auth/refresh payload.
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// Refresh handles POST /api/auth/refresh — exchanges a still-valid refresh
// token for a new access token. The refresh token itself is rotated (the
// old one is revoked, a new one issued) on every use, so a refresh token can
// only ever be replayed once even if it leaks.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req RefreshTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RefreshToken == "" {
		respondAuthError(w, http.StatusBadRequest, "Missing refresh_token")
		return
	}

	hash := service.HashRefreshToken(req.RefreshToken)
	rt, err := h.userRepo.GetRefreshToken(r.Context(), hash)
	if err != nil {
		middleware.L(r.Context()).Error("get refresh token", "error", err)
		respondAuthError(w, http.StatusInternalServerError, "Error validating refresh token")
		return
	}
	if rt == nil || rt.RevokedAt != nil || time.Now().After(rt.ExpiresAt) {
		respondAuthError(w, http.StatusUnauthorized, "Invalid or expired refresh token")
		return
	}

	// Rotate: revoke the token that was just used before issuing its
	// replacement, so it can't be exchanged a second time.
	if err := h.userRepo.RevokeRefreshToken(r.Context(), hash); err != nil {
		middleware.L(r.Context()).Error("revoke refresh token", "error", err)
		respondAuthError(w, http.StatusInternalServerError, "Error rotating refresh token")
		return
	}

	accessToken, err := h.authService.GenerateToken(rt.UserID)
	if err != nil {
		middleware.L(r.Context()).Error("generate access token (refresh)", "error", err)
		respondAuthError(w, http.StatusInternalServerError, "Error generating token")
		return
	}
	newRefreshToken, newRefreshHash, err := h.authService.GenerateRefreshToken()
	if err != nil {
		middleware.L(r.Context()).Error("generate new refresh token", "error", err)
		respondAuthError(w, http.StatusInternalServerError, "Error generating refresh token")
		return
	}
	if err := h.userRepo.CreateRefreshToken(r.Context(), rt.UserID, newRefreshHash, time.Now().Add(service.RefreshTokenTTL)); err != nil {
		middleware.L(r.Context()).Error("persist new refresh token", "error", err)
		respondAuthError(w, http.StatusInternalServerError, "Error persisting refresh token")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"access_token":  accessToken,
		"refresh_token": newRefreshToken,
	})
}

// Logout handles POST /api/auth/logout — revokes the given refresh token so
// it can no longer be used to mint new access tokens. The client is still
// responsible for discarding its locally stored tokens; this just makes
// logout meaningful server-side too, not merely a client-side forget.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	var req RefreshTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RefreshToken == "" {
		respondAuthError(w, http.StatusBadRequest, "Missing refresh_token")
		return
	}
	if err := h.userRepo.RevokeRefreshToken(r.Context(), service.HashRefreshToken(req.RefreshToken)); err != nil {
		middleware.L(r.Context()).Error("revoke refresh token on logout", "error", err)
		respondAuthError(w, http.StatusInternalServerError, "Error revoking refresh token")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": "logged out"})
}

// Me handles GET /api/auth/me — a lightweight session-check endpoint the
// client calls on app launch: if a token is already stored, this confirms
// it's still valid and returns the user's basic profile, so the app can
// route straight to home instead of showing the login screen again.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		respondAuthError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	user, err := h.userRepo.GetByID(r.Context(), userID)
	if err != nil {
		respondAuthError(w, http.StatusUnauthorized, "Session is no longer valid")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"user_id":       user.ID,
		"astra_user_id": user.AstraUserID,
		"phone_number":  user.PhoneNumber,
		"name":          user.Name,
		"wants_rm":      user.WantsRM,
		"created_at":    user.CreatedAt,
	})
}

// UpdateMe handles PATCH /api/auth/me — lets the authenticated user update
// their profile. Currently just the display name, which the app collects
// during onboarding (after the account row is already created on OTP verify).
func (h *AuthHandler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserID(r.Context())
	if !ok {
		respondAuthError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondAuthError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}
	name := strings.TrimSpace(req.Name)
	if len(name) < 2 {
		respondAuthError(w, http.StatusBadRequest, "Name must be at least 2 characters")
		return
	}

	if err := h.userRepo.UpdateUserName(r.Context(), userID, name); err != nil {
		middleware.L(r.Context()).Error("update user name", "error", err)
		respondAuthError(w, http.StatusInternalServerError, "Error updating profile")
		return
	}

	user, err := h.userRepo.GetByID(r.Context(), userID)
	if err != nil {
		respondAuthError(w, http.StatusInternalServerError, "Error reloading profile")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"user_id":       user.ID,
		"astra_user_id": user.AstraUserID,
		"phone_number":  user.PhoneNumber,
		"name":          user.Name,
		"wants_rm":      user.WantsRM,
		"created_at":    user.CreatedAt,
	})
}

// ResetUser handles POST /api/auth/reset — resets and deletes a user by phone number
// so they can log in fresh and trigger new archetype seeding with rich subscriptions.
func (h *AuthHandler) ResetUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PhoneNumber string `json:"phone_number"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.PhoneNumber != "" {
		_ = h.userRepo.DeleteUserByPhone(r.Context(), req.PhoneNumber)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"message": "User reset successfully. Next login will re-seed fresh data.",
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
