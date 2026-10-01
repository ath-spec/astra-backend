package middleware

import (
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
)

// IPBlocklist is a middleware that immediately rejects requests from known
// malicious IP addresses with a 403 Forbidden, before any further processing
// (auth, rate limiting, logging, etc.). This is the cheapest possible defence
// against persistent scanners and abusers.
//
// IPs can be added in two ways:
//  1. Hardcoded in the defaultBlockedIPs slice below (committed attackers).
//  2. At runtime via the BLOCKED_IPS environment variable (comma-separated),
//     so Ops can ban new IPs without a code change or redeploy.
//
// Position in the chain: FIRST — before RequestID, RealIP, and every other
// middleware, so blocked traffic never allocates request context, never hits
// the rate limiter bucket, and never appears in structured logs.

// defaultBlockedIPs contains IPs that are permanently blocked. Add new entries
// here when you identify persistent bad actors from the access logs.
var defaultBlockedIPs = []string{
	"169.58.252.152", // Automated vuln scanner — first observed 2026-10-01
}

// blocklist is the singleton that holds the merged set of blocked IPs.
var blocklist *ipBlocklist

func init() {
	blocklist = newIPBlocklist(defaultBlockedIPs)
}

type ipBlocklist struct {
	mu  sync.RWMutex
	ips map[string]struct{}
}

func newIPBlocklist(seeds []string) *ipBlocklist {
	bl := &ipBlocklist{ips: make(map[string]struct{}, len(seeds)+8)}

	// 1. Hardcoded IPs.
	for _, ip := range seeds {
		if ip = strings.TrimSpace(ip); ip != "" {
			bl.ips[ip] = struct{}{}
		}
	}

	// 2. IPs from the environment (comma-separated).
	if env := os.Getenv("BLOCKED_IPS"); env != "" {
		for _, ip := range strings.Split(env, ",") {
			if ip = strings.TrimSpace(ip); ip != "" {
				bl.ips[ip] = struct{}{}
			}
		}
	}

	return bl
}

// contains checks whether the given IP is blocked.
func (bl *ipBlocklist) contains(ip string) bool {
	bl.mu.RLock()
	defer bl.mu.RUnlock()
	_, ok := bl.ips[ip]
	return ok
}

// Add dynamically adds an IP to the blocklist at runtime.
func (bl *ipBlocklist) Add(ip string) {
	bl.mu.Lock()
	defer bl.mu.Unlock()
	bl.ips[ip] = struct{}{}
}

// stripPort extracts the IP address from an addr that may include a port
// (e.g. "169.58.252.152:45278" → "169.58.252.152").
func stripPort(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		// No port present — addr is already a bare IP.
		return addr
	}
	return host
}

// IPBlocklistMiddleware returns a 403 Forbidden for any request whose
// RemoteAddr (after stripping port) matches a blocked IP. It logs the block
// once at Warn level so you can track how often an attacker retries.
func IPBlocklistMiddleware(next http.Handler) http.Handler {
	if len(blocklist.ips) > 0 {
		slog.Info("IP blocklist active", "blocked_count", len(blocklist.ips))
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientIP := stripPort(r.RemoteAddr)

		if blocklist.contains(clientIP) {
			slog.Warn("blocked request from banned IP",
				"ip", clientIP,
				"method", r.Method,
				"path", r.URL.Path,
			)
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// BlockIP dynamically adds an IP to the blocklist at runtime. This can be
// called from an admin endpoint or an automated abuse-detection goroutine.
func BlockIP(ip string) {
	blocklist.Add(ip)
	slog.Info("IP added to blocklist", "ip", ip)
}
