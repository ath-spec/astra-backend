package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/yourusername/astra-backend/internal/service"
)

// contextKey is a custom type to prevent context key collisions
type contextKey string

const UserIDKey contextKey = "user_id"

// RequireAuth validates the JWT token in the Authorization header
func RequireAuth(authService *service.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, `{"error": "Missing Authorization header"}`, http.StatusUnauthorized)
				return
			}

			// Expecting "Bearer <token>"
			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || parts[0] != "Bearer" {
				http.Error(w, `{"error": "Invalid Authorization format"}`, http.StatusUnauthorized)
				return
			}

			tokenString := parts[1]

			// Validate Token
			claims, err := authService.ValidateToken(tokenString)
			if err != nil {
				http.Error(w, `{"error": "Invalid or expired token"}`, http.StatusUnauthorized)
				return
			}

			// Add UserID to Context
			ctx := context.WithValue(r.Context(), UserIDKey, claims.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
