package middlewares

import (
	"net/http"
	"strings"
)

// knownScannerPaths is a set of paths that are exclusively probed by bots and
// vulnerability scanners. They have no legitimate use on this API server.
// Requests to these paths are rejected immediately with 400 to avoid polluting logs.
var knownScannerPaths = map[string]struct{}{
	"/.env":                     {},
	"/.git/config":              {},
	"/.git/HEAD":                {},
	"/.vscode/sftp.json":        {},
	"/.DS_Store":                {},
	"/config.json":              {},
	"/actuator/env":             {},
	"/actuator/health":          {},
	"/actuator/info":            {},
	"/telescope/requests":       {},
	"/debug/default/view":       {},
	"/swagger-ui.html":          {},
	"/v2/api-docs":              {},
	"/v3/api-docs":              {},
	"/api-docs/swagger.json":    {},
	"/api/swagger.json":         {},
	"/swagger.json":             {},
	"/swagger/v1/swagger.json":  {},
	"/swagger/properties.json":  {},
	"/trace.axd":                {},
	"/@vite/env":                {},
	"/info.php":                 {},
	"/phpinfo.php":              {},
	"/wp-login.php":             {},
	"/wp-admin/":                {},
	"/login.action":             {},
	"/v2/_catalog":              {},
	"/graphql":                  {},
	"/api/graphql":              {},
	"/graphql/api":              {},
	"/api/gql":                  {},
	"/api":                      {},
	"/.well-known/security.txt": {},
	"/version":                  {},
	"/server-status":            {},
	"/server":                   {},
	"/console/":                 {},
}

// knownScannerPrefixes are path prefixes used by bots that should always be blocked.
var knownScannerPrefixes = []string{
	"/s/031313e22323e23333e26363/",
	"/ecp/Current/",
	"/?rest_route=/wp/",
}

// BotBlockerMiddleware drops requests to paths known to be scanner/bot probes.
// These paths have no legitimate business purpose on this server.
// Returns 400 Bad Request to avoid information leakage from structured 404 responses.
func BotBlockerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Check exact path matches
		if _, blocked := knownScannerPaths[path]; blocked {
			http.Error(w, "", http.StatusBadRequest)
			return
		}

		// Check prefix matches
		for _, prefix := range knownScannerPrefixes {
			if strings.HasPrefix(path, prefix) {
				http.Error(w, "", http.StatusBadRequest)
				return
			}
		}

		// Block query strings used by WordPress scanners
		if r.URL.RawQuery != "" && strings.Contains(r.URL.RawQuery, "rest_route=/wp/") {
			http.Error(w, "", http.StatusBadRequest)
			return
		}

		next.ServeHTTP(w, r)
	})
}
