// Package apiresponse defines the single JSON response envelope used by
// every HTTP handler in this service, so callers never have to guess the
// shape of a success or error body.
package apiresponse

import (
	"encoding/json"
	"log"
	"net/http"
)

// Envelope is the standard response shape: {"error": bool, "message"?: string, "data"?: any}.
type Envelope struct {
	Error   bool   `json:"error"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data,omitempty"`
}

// OK writes a 200 success envelope with the given payload.
func OK(w http.ResponseWriter, data any) {
	Write(w, http.StatusOK, Envelope{Error: false, Data: data})
}

// Created writes a 201 success envelope with the given payload.
func Created(w http.ResponseWriter, data any) {
	Write(w, http.StatusCreated, Envelope{Error: false, Data: data})
}

// NoContent writes a 204 with no body.
func NoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// Write encodes an arbitrary envelope with the given HTTP status code. It is
// the low-level primitive OK/Created/Error all build on.
func Write(w http.ResponseWriter, status int, env Envelope) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(env); err != nil {
		// Status/headers are already flushed at this point, so all we can do
		// is record that the client got a truncated/empty body.
		log.Printf("apiresponse: failed to encode response body: %v", err)
	}
}
