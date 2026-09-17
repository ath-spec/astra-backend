package test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"github.com/yourusername/astra-backend/internal/commons/middlewares"
	"github.com/yourusername/astra-backend/internal/commons/util"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestCase(t *testing.T, method, route string, handler http.HandlerFunc, body []byte, expected int, headers ...string) {
	eventid, _ := uuid.NewRandom()
	req, _ := http.NewRequest(method, route, bytes.NewReader(body))
	req = middlewares.AddDefaultProfileToContext(req, util.AccessTokenType)
	req = AddChiURLParams(req, map[string]string{
		"id":         "X-1241515",
		"locationId": "1",
		"goalId":     "1",
		"eventId":    eventid.String(),
	})
	rr := httptest.NewRecorder()

	for _, header := range headers {
		parts := strings.Split(header, ":")
		if len(parts) != 2 {
			continue
		}
		req.Header.Add(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
	}

	handler.ServeHTTP(rr, req)

	if rr.Code != expected {
		t.Errorf("expected status code %d, got %d with body %s", expected, rr.Code, rr.Body)
	}
}
