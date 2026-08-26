package apiresponse

import (
	"errors"
	"fmt"
	"log"
	"net/http"
)

// Sentinel errors. Handlers/services wrap the concrete cause with one of
// these (fmt.Errorf("create order: %w", ErrValidation)) and Error() below
// maps it to the right HTTP status via errors.Is — handlers never set a
// status code by hand.
var (
	ErrValidation     = errors.New("validation failed")
	ErrNotFound       = errors.New("resource not found")
	ErrUnauthorized   = errors.New("unauthorized")
	ErrForbidden      = errors.New("forbidden")
	ErrConflict       = errors.New("conflict")
	ErrNotImplemented = errors.New("not implemented")
	ErrInternal       = errors.New("internal error")
)

// order matters only in that every sentinel must appear exactly once; a map
// would work equally well but a slice keeps errors.Is short-circuiting cheap
// for the common (small N) case without needing map iteration.
var mapped = []struct {
	err    error
	status int
}{
	{ErrValidation, http.StatusBadRequest},
	{ErrUnauthorized, http.StatusUnauthorized},
	{ErrForbidden, http.StatusForbidden},
	{ErrNotFound, http.StatusNotFound},
	{ErrConflict, http.StatusConflict},
	{ErrNotImplemented, http.StatusNotImplemented},
}

// Validation builds a client-safe, 400-mapped error from a formatted
// message, e.g. Validation("quantity must be positive") or
// Validation("invalid request body: %v", decodeErr).
func Validation(format string, args ...any) error {
	return fmt.Errorf(format+": %w", append(args, ErrValidation)...)
}

// NotFound builds a client-safe, 404-mapped error from a formatted message.
func NotFound(format string, args ...any) error {
	return fmt.Errorf(format+": %w", append(args, ErrNotFound)...)
}

// NotImplemented builds a client-safe, 501-mapped error from a formatted
// message — used by domains that are routed but not yet backed by a real or
// mock provider (see the aa/kyc/mf stub handlers).
func NotImplemented(format string, args ...any) error {
	return fmt.Errorf(format+": %w", append(args, ErrNotImplemented)...)
}

// Error writes an error envelope for err. Known sentinel errors (validation,
// not-found, conflict, ...) surface their message to the client since it
// only ever describes the wrapped business reason. Anything else — including
// ErrInternal and raw/unwrapped errors such as a bare DB or network failure —
// is logged server-side in full and reported to the client as a generic
// "internal server error", so implementation details never leak over HTTP.
func Error(w http.ResponseWriter, err error) {
	if err == nil {
		Write(w, http.StatusInternalServerError, Envelope{Error: true, Message: "internal server error"})
		return
	}

	for _, m := range mapped {
		if errors.Is(err, m.err) {
			Write(w, m.status, Envelope{Error: true, Message: err.Error()})
			return
		}
	}

	log.Printf("apiresponse: internal error: %v", err)
	Write(w, http.StatusInternalServerError, Envelope{Error: true, Message: "internal server error"})
}
