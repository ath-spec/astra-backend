package middleware

import (
	"net/http"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// Interceptor is the global response interceptor. It runs on every request and:
//
//  1. Sets security-hardening headers before the handler writes the body, so
//     they are present on every response regardless of which handler runs
//     (including 404s, 429s, and panic-recovered 500s).
//
//  2. Echoes X-Request-Id back in the response so Flutter/Dio clients (and
//     browser DevTools) can correlate a failed response with the server log
//     line that explains it.
//
//  3. Logs unexpected 5xx responses at Error level after the handler returns,
//     correlated to the same request_id as all other log lines for that
//     request. This is distinct from StructuredLogger which logs every request
//     at Info regardless of outcome.
//
// Position in the chain: after RequestID + WithRequestLogger (so the
// request_id is available), before StructuredLogger (so the 5xx error log
// line appears before the summary line in time-ordered logs).
func Interceptor(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// --- Pre-handler: set response headers that must be present on every
		// response, including error responses. These are written to the
		// underlying header map before any handler calls WriteHeader, so they
		// are always flushed to the client.

		// Security hardening — prevents a class of web attacks.
		// Safe for a pure JSON API; no HTML content is ever served.
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Echo the request ID back to the client so Dio/Flutter can read it
		// from the response and include it in crash reports or support tickets.
		reqID := chimw.GetReqID(r.Context())
		if reqID != "" {
			w.Header().Set("X-Request-Id", reqID)
		}

		// Wrap the ResponseWriter so we can inspect the status code after the
		// handler returns, without interfering with the response body.
		ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)

		// --- Run the handler chain.
		next.ServeHTTP(ww, r)

		// --- Post-handler: log unexpected server errors at Error level.
		// 4xx are client errors — expected, already reflected in the summary
		// log line. Only 5xx (and unwritten responses that default to 200 but
		// indicate a Recoverer-caught panic) warrant an Error-level log.
		if ww.Status() >= http.StatusInternalServerError {
			L(r.Context()).Error("server error response",
				"status", ww.Status(),
				"method", r.Method,
				"path",   r.URL.Path,
			)
		}
	})
}
