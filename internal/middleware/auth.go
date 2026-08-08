package middleware

import (
	"net/http"
)

func RequireAuth(validToken string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := r.Header.Get("X-Astra-Auth")
			
			// If validToken is empty (development), we might allow it, but for prod it should be enforced.
			if validToken != "" && token != validToken {
				http.Error(w, `{"success": false, "error": "Unauthorized"}`, http.StatusUnauthorized)
				return
			}
			
			next.ServeHTTP(w, r)
		})
	}
}
