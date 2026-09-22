package middleware

import (
	"context"
	"log/slog"
	"net/http"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// loggerKey is the unexported context key for the per-request slog.Logger.
// Using a private type prevents key collisions with any other package.
type loggerKey struct{}

// WithRequestLogger injects a *slog.Logger pre-seeded with the request's
// request_id into the context. It must run after middleware.RequestID so that
// GetReqID returns a non-empty value.
//
// Usage in handlers and services:
//
//	middleware.L(ctx).Info("order placed", "order_id", id)
//	// → {"time":1234567890,"level":"INFO","msg":"order placed",
//	//     "request_id":"abc123","order_id":"xyz"}
func WithRequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := slog.Default().With("request_id", chimw.GetReqID(r.Context()))
		ctx := context.WithValue(r.Context(), loggerKey{}, logger)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// L retrieves the per-request *slog.Logger from ctx. If the context has no
// logger (e.g. called from a background job or a test that skips the middleware
// chain), it falls back to the global default logger so callers never need to
// nil-check.
func L(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}
