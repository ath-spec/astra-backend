package middleware

import (
	"log"
	"net/http"
	"net/http/httputil"
)

// VerboseLogger dumps the entire HTTP request (headers and body) for debugging purposes.
func VerboseLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// DumpRequest reads the body and restores it so subsequent handlers can still read it.
		dump, err := httputil.DumpRequest(r, true)
		if err == nil {
			log.Printf("\n--- INCOMING REQUEST ---\n%s\n------------------------\n", string(dump))
		} else {
			log.Printf("Error dumping request: %v", err)
		}

		next.ServeHTTP(w, r)
	})
}
