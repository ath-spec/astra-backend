package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/yourusername/astra-backend/internal/repository"
	"github.com/yourusername/astra-backend/internal/service"
)

// userExistenceChecker is the minimal repository seam RequireAuth needs to
// confirm a token's subject is still a real account. A JWT is a bearer
// credential valid purely by signature + expiry — it says nothing about
// whether that user row still exists (deleted account, or in dev, a wiped
// database). Without this check, a device holding an old token keeps
// "authenticating" successfully against a user_id that is gone, and every
// query scoped to it silently returns empty results instead of the 401 that
// should send the app back to login.
type userExistenceChecker interface {
	GetByID(ctx context.Context, userID uuid.UUID) (*repository.User, error)
}

// contextKey is a custom type to prevent context key collisions
type contextKey string

const UserIDKey contextKey = "user_id"

// GetUserID extracts the authenticated user's ID from a request context that
// has passed through RequireAuth. The bool is false if the context has no
// user ID (e.g. called from an unprotected route by mistake) — callers must
// check it rather than using the zero UUID.
func GetUserID(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(UserIDKey).(uuid.UUID)
	return id, ok
}

// RequireAuth validates the JWT token in the Authorization header and
// confirms its subject still exists (see userExistenceChecker).
func RequireAuth(authService *service.AuthService, users userExistenceChecker) func(http.Handler) http.Handler {
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

			if _, err := users.GetByID(r.Context(), claims.UserID); err != nil {
				http.Error(w, `{"error": "Account no longer exists"}`, http.StatusUnauthorized)
				return
			}

			// Add UserID to Context
			ctx := context.WithValue(r.Context(), UserIDKey, claims.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAuthWS is RequireAuth's counterpart for WebSocket upgrade routes.
// Native clients can set the Authorization header on the upgrade request,
// but browsers' native WebSocket API cannot — so this also accepts the JWT
// as a `?token=` query parameter, same as the reference realtime-STT
// implementation. Kept as a separate middleware (rather than changing
// RequireAuth globally) so plain REST endpoints never log tokens via query
// string, only the handful of streaming routes that need this fallback.
func RequireAuthWS(authService *service.AuthService, users userExistenceChecker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenString := ""
			if authHeader := r.Header.Get("Authorization"); authHeader != "" {
				parts := strings.Split(authHeader, " ")
				if len(parts) == 2 && parts[0] == "Bearer" {
					tokenString = parts[1]
				}
			}
			if tokenString == "" {
				tokenString = r.URL.Query().Get("token")
			}
			if tokenString == "" {
				http.Error(w, `{"error": "Missing auth token"}`, http.StatusUnauthorized)
				return
			}

			claims, err := authService.ValidateToken(tokenString)
			if err != nil {
				http.Error(w, `{"error": "Invalid or expired token"}`, http.StatusUnauthorized)
				return
			}

			if _, err := users.GetByID(r.Context(), claims.UserID); err != nil {
				http.Error(w, `{"error": "Account no longer exists"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), UserIDKey, claims.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
