// Package middleware contains shared HTTP middleware for the Astra API.
package middleware

import (
	"log/slog"
	"net/http"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// StructuredLogger is a chi-compatible middleware that emits one JSON log
// line per request via the stdlib slog default logger (configured in main).
// It replaces chi's built-in middleware.Logger, which only emits plain text.
//
// Each line includes:
//   - method, path, status, latency_ms — the standard request summary
//   - request_id  — the X-Request-Id set by middleware.RequestID upstream
//   - ip          — the real client IP (after middleware.RealIP resolves it)
//
// The logger never reads or logs request/response bodies; use VerboseLogger
// (DEBUG_VERBOSE_LOG=true) for that during local development.
func StructuredLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// WrapResponseWriter intercepts the status code written by the handler
		// so we can log it after the fact. It passes through all other writes
		// unchanged — handlers are completely unaware of the wrapper.
		ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()

		defer func() {
			slog.Info("http",
				"method",     r.Method,
				"path",       r.URL.Path,
				"status",     ww.Status(),
				"latency_ms", time.Since(start).Milliseconds(),
				"request_id", chimw.GetReqID(r.Context()),
				"ip",         r.RemoteAddr,
			)
		}()

		next.ServeHTTP(ww, r)
	})
}
