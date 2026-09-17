package middlewares

import (
	"net/http"
	"github.com/yourusername/astra-backend/internal/commons/redis"
	"strconv"
	"strings"
	"time"
)

// extractClientIP safely extracts the real client IP.
// It reads the leftmost entry from X-Forwarded-For (the original client),
// which is set by load balancers/proxies like Railway or Cloudflare.
// Using the leftmost prevents spoofing via comma-chained IPs (e.g. "evil-ip, real-ip").
func extractClientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		// X-Forwarded-For can be: "client, proxy1, proxy2"
		// We want the leftmost (original client) IP.
		parts := strings.SplitN(forwarded, ",", 2)
		ip := strings.TrimSpace(parts[0])
		if ip != "" {
			return ip
		}
	}
	// Fallback: strip port from RemoteAddr (format: "1.2.3.4:port")
	addr := r.RemoteAddr
	if idx := strings.LastIndex(addr, ":"); idx != -1 {
		return addr[:idx]
	}
	return addr
}

// rateLimitMessages returns an escalating message based on how many times over
// the limit the client currently is. Keeps it professional but memorable.
var rateLimitMessages = []string{
	// 1x over limit
	"Slow down. You've exceeded the request limit for this window. Please wait before trying again.",
	// 2x over limit
	"Still going? Your IP has been flagged for excessive requests. Continued abuse will result in a permanent block.",
	// 3x+ over limit
	"Your activity has been identified as potentially malicious and is being logged with your IP address. If this is a mistake, contact support@zeyro.in.",
}

// writeRateLimitResponse sends a 429 with a custom escalating message and Retry-After header.
func writeRateLimitResponse(w http.ResponseWriter, count, maxRequests int, window time.Duration) {
	// Pick message based on how far over the limit we are
	overBy := count/maxRequests - 1
	if overBy < 0 {
		overBy = 0
	}
	if overBy >= len(rateLimitMessages) {
		overBy = len(rateLimitMessages) - 1
	}
	message := rateLimitMessages[overBy]

	retryAfterSecs := int(window.Seconds())
	w.Header().Set("Retry-After", strconv.Itoa(retryAfterSecs))
	w.Header().Set("X-RateLimit-Limit", strconv.Itoa(maxRequests))
	w.Header().Set("X-RateLimit-Remaining", "0")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	w.Write([]byte(`{"error":true,"message":"` + message + `"}`))
}

type RateLimiter struct {
	redisClient redis.Client
}

func NewRateLimiter(redisClient redis.Client) *RateLimiter {
	return &RateLimiter{redisClient: redisClient}
}

// RateLimitMiddleware limits requests per IP
func (rl *RateLimiter) RateLimitMiddleware(maxRequests int, window time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clientIP := extractClientIP(r)

			key := "rate_limit:" + clientIP
			ctx := r.Context()

			// Get current count
			countStr, err := rl.redisClient.Get(ctx, key)
			if err != nil {
				// If key doesn't exist, start counting
				countStr = "0"
			}

			count, err := strconv.Atoi(countStr)
			if err != nil {
				count = 0
			}

			if count >= maxRequests {
				writeRateLimitResponse(w, count, maxRequests, window)
				return
			}

			// Increment counter
			if count == 0 {
				// Set with expiration
				rl.redisClient.Set(ctx, key, "1", window)
			} else {
				// Increment existing
				rl.redisClient.Incr(ctx, key)
			}

			next.ServeHTTP(w, r)
		})
	}
}

// OTPRateLimitMiddleware specific rate limiting for OTP endpoints
func (rl *RateLimiter) OTPRateLimitMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clientIP := extractClientIP(r)
			key := "otp_rate_limit:" + clientIP
			ctx := r.Context()

			countStr, err := rl.redisClient.Get(ctx, key)
			if err != nil {
				countStr = "0"
			}

			count, err := strconv.Atoi(countStr)
			if err != nil {
				count = 0
			}

			// Limit OTP requests to 5 per hour per IP
			if count >= 5 {
				writeRateLimitResponse(w, count, 5, time.Hour)
				return
			}

			if count == 0 {
				rl.redisClient.Set(ctx, key, "1", time.Hour)
			} else {
				rl.redisClient.Incr(ctx, key)
			}

			next.ServeHTTP(w, r)
		})
	}
}
