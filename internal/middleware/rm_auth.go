package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"

	rmdomain "github.com/yourusername/astra-backend/internal/domain/rm"
	"github.com/yourusername/astra-backend/internal/service"
)

const (
	rmIDKey   contextKey = "rm_id"
	rmRoleKey contextKey = "rm_role"
)

// GetRMID returns the authenticated staff id from a context that passed
// through RequireRMAuth.
func GetRMID(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(rmIDKey).(uuid.UUID)
	return id, ok
}

// GetRMRole returns the authenticated staff role ("rm" / "admin").
func GetRMRole(ctx context.Context) (string, bool) {
	role, ok := ctx.Value(rmRoleKey).(string)
	return role, ok
}

// IsAdmin is a convenience for handlers that widen behaviour for admins.
func IsAdmin(ctx context.Context) bool {
	role, ok := GetRMRole(ctx)
	return ok && role == rmdomain.RoleAdmin
}

// RequireRMAuth validates the staff access token (RM_JWT_SECRET) and puts
// the staff id + role in the request context. It is entirely separate from
// RequireAuth — a user JWT is never valid here and vice versa.
func RequireRMAuth(authService *service.RMAuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, `{"error": "Missing Authorization header"}`, http.StatusUnauthorized)
				return
			}
			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || parts[0] != "Bearer" {
				http.Error(w, `{"error": "Invalid Authorization format"}`, http.StatusUnauthorized)
				return
			}
			claims, err := authService.ValidateToken(parts[1])
			if err != nil {
				http.Error(w, `{"error": "Invalid or expired token"}`, http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), rmIDKey, claims.RMID)
			ctx = context.WithValue(ctx, rmRoleKey, claims.Role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAdmin must be chained after RequireRMAuth; it 403s any non-admin.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !IsAdmin(r.Context()) {
			http.Error(w, `{"error": "Admin access required"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
