package middleware

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

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

// existenceCacheTTL bounds how stale a "this user still exists" (or doesn't)
// answer is allowed to be. Every protected request was paying a full DB
// round-trip for this check alone — on top of whatever the handler itself
// queries — which adds up across every single API call and competes for
// pool connections under load. A user_id is never reused after deletion
// (UUIDs, not reused serial ids), so caching a negative result is exactly as
// safe as caching a positive one: there's no scenario where the same ID
// becomes valid again after being invalid. 90s bounds how long a just-wiped
// account keeps working to about the same order of magnitude as the JWT's
// own short lifetime, which is an acceptable tradeoff for cutting a
// mandatory DB call out of every request.
const existenceCacheTTL = 90 * time.Second

type existenceCacheEntry struct {
	exists    bool
	expiresAt time.Time
}

// existenceCache is a small in-process cache, the same short-TTL pattern
// internal/service/budget/service.go's categoryIndex already uses — no
// external cache dependency needed for something this cheap to bound.
type existenceCache struct {
	mu      sync.Mutex
	entries map[uuid.UUID]existenceCacheEntry
}

var sharedExistenceCache = &existenceCache{entries: make(map[uuid.UUID]existenceCacheEntry)}

// checkExists returns whether userID exists, using the cache when fresh and
// falling back to a real DB check otherwise. Shared by RequireAuth and
// RequireAuthWS so a user hitting both REST and WS routes only pays the DB
// round-trip once per TTL window, not once per middleware.
func (c *existenceCache) checkExists(ctx context.Context, users userExistenceChecker, userID uuid.UUID) bool {
	now := time.Now()
	c.mu.Lock()
	if e, ok := c.entries[userID]; ok && now.Before(e.expiresAt) {
		c.mu.Unlock()
		return e.exists
	}
	c.mu.Unlock()

	_, err := users.GetByID(ctx, userID)
	exists := err == nil

	c.mu.Lock()
	c.entries[userID] = existenceCacheEntry{exists: exists, expiresAt: now.Add(existenceCacheTTL)}
	// Opportunistic eviction on write so a long-running process doesn't
	// accumulate one entry per distinct user forever — cheap relative to the
	// DB round-trip we just paid, and only runs on a cache miss.
	if len(c.entries) > 10_000 {
		for k, v := range c.entries {
			if now.After(v.expiresAt) {
				delete(c.entries, k)
			}
		}
	}
	c.mu.Unlock()
	return exists
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

			if !sharedExistenceCache.checkExists(r.Context(), users, claims.UserID) {
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

			if !sharedExistenceCache.checkExists(r.Context(), users, claims.UserID) {
				http.Error(w, `{"error": "Account no longer exists"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), UserIDKey, claims.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
