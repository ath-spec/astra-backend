package util

import (
	"os"
	"strings"
)

// IsProduction reports whether APP_ENV is set to production/prod. It gates
// dev-only escape hatches (the "Bearer frontend" auth bypass, the mock-phone
// blocker) so they can never activate outside a non-prod environment,
// regardless of how a caller wires the middleware chain.
func IsProduction() bool {
	env := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
	return env == "production" || env == "prod"
}
