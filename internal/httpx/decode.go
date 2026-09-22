// Package httpx holds small HTTP request helpers shared across handlers.
package httpx

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// maxBodyBytes caps request bodies at 1MiB. Every request payload in this
// API is a small financial-form JSON object; anything larger is either a
// mistake or an attempt to exhaust memory, so it's rejected outright rather
// than read in full.
const maxBodyBytes = 1 << 20

// DecodeJSON reads and decodes a JSON request body into dst, rejecting
// unknown fields and bodies over maxBodyBytes. It returns a plain error
// (never wrapped in a sentinel) — callers should wrap it with
// apiresponse.ErrValidation to report it to the client.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("decode request body: %w", err)
	}
	return nil
}
