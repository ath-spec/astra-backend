package handler

import (
	"net/http"
	"os"
	"strings"

	"github.com/gorilla/websocket"
)

// wsAllowedOrigins mirrors the REST CORS allowlist in cmd/api/main.go — kept
// as its own list because gorilla/websocket's CheckOrigin is a separate gate
// from the cors middleware (which never runs for a hijacked connection).
var wsAllowedOrigins = map[string]bool{
	"https://astrafin.netlify.app":   true,
	"https://astrafinrm.netlify.app": true,
}

// wsCheckOrigin allows known production frontends, localhost/127.0.0.1 (any
// port, for local dev), anything listed in FRONTEND_URL, and requests with
// no Origin header at all — native mobile app WebSocket clients don't send
// one, only browsers do.
func wsCheckOrigin(r *http.Request) bool {
	origin := strings.TrimRight(r.Header.Get("Origin"), "/")
	if origin == "" {
		return true
	}
	if wsAllowedOrigins[origin] {
		return true
	}
	if strings.HasPrefix(origin, "http://localhost:") || strings.HasPrefix(origin, "http://127.0.0.1:") {
		return true
	}
	if frontendURL := os.Getenv("FRONTEND_URL"); frontendURL != "" {
		for _, o := range strings.Split(frontendURL, ",") {
			if strings.TrimRight(strings.TrimSpace(o), "/") == origin {
				return true
			}
		}
	}
	return false
}

var wsUpgrader = websocket.Upgrader{
	CheckOrigin:     wsCheckOrigin,
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
}
