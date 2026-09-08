// Package idbi is the HTTP client for the IDBI Atlas API gateway.
//
// Gateway (sandbox):  https://sandboxpocgatewayprod.idbi.bank.in
// Path shape:          POST /Development/<name>test        (dev/sandbox stage)
// Auth:                none at the app layer — the gateway allow-lists the
//
//	caller's egress IP. Nothing is sent in a header.
//	Production path prefix and the "test" suffix will differ.
//
// Every method here was shaped against a live capture of the sandbox
// (scripts/idbi-capture-fixtures.py -> internal/provider/idbi/testdata/).
// The response envelope is NOT consistent across endpoints — some are flat,
// some wrap the payload in "result", some in "data". Each Fetch* method knows
// its own shape; there is no shared unwrap.
//
// Known-unusable in the sandbox (return ErrNotAvailable, caller falls back to
// a mock/computed value):
//
//	408 fetchCibilScore        - bureau credentials in the sample are rejected
//	                             (idVision IDVErr001 InvalidUserIdOrPassword);
//	                             no score field in the response at all.
//	433 fetchLoanInterestRates - sandbox returns an unrelated kitchen-sink
//	                             object, not a rate table.
//	456 performCustomerMasterDedupeCheck - no OpenAPI spec published; endpoint
//	                             path unknown.
//
// Simulated (the call succeeds and the shape is real, but there is no live
// counterparty): the whole AA chain — 590 requestConsent, 592 redirect URL
// (the returned webrd.onemoney.in link is a dummy), 593 decrypt, and the
// 497/498 webhooks. Build the flow; do not expect a real consent screen.
package idbi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// SandboxBaseURL is the IP-allow-listed dev gateway.
const SandboxBaseURL = "https://sandboxpocgatewayprod.idbi.bank.in"

// devStagePrefix is prepended to every operation name on the sandbox stage.
const devStagePrefix = "/Development/"

var (
	// ErrUnavailable — the gateway could not be reached or returned a 5xx /
	// transport error. Callers should retry or degrade.
	ErrUnavailable = errors.New("idbi gateway unavailable")

	// ErrNotAvailable — the endpoint exists but does not return usable data in
	// this environment (see the package doc). Callers fall back to a mock.
	ErrNotAvailable = errors.New("idbi data not available in this environment")
)

// APIError is a decoded IDBI error response. The gateway uses at least three
// shapes; this flattens them:
//
//	{"errors":[{"code","title","description"}]}      most APIs
//	{"message":"...", "failedFields":[...]}          field validation
//	{"message":"..."}                                simple validation
//
// A bare 403 with an HTML body (WAF, on an empty/garbage payload) surfaces as
// Status=403, Message="forbidden (WAF)".
type APIError struct {
	Status       int
	Code         string
	Title        string
	Message      string
	FailedFields []string
	RawBody      []byte
}

func (e *APIError) Error() string {
	b := &strings.Builder{}
	fmt.Fprintf(b, "idbi: HTTP %d", e.Status)
	if e.Code != "" {
		fmt.Fprintf(b, " [%s]", e.Code)
	}
	msg := e.Message
	if msg == "" {
		msg = e.Title
	}
	if msg != "" {
		fmt.Fprintf(b, " %s", msg)
	}
	if len(e.FailedFields) > 0 {
		fmt.Fprintf(b, " (fields: %s)", strings.Join(e.FailedFields, ", "))
	}
	return b.String()
}

// Config is the client's tunable surface. Only BaseURL matters today.
type Config struct {
	BaseURL string        // default SandboxBaseURL
	Timeout time.Duration // default 30s
	HTTP    *http.Client  // optional; overrides Timeout
}

// Client talks to the IDBI Atlas gateway. Use NewClient; the zero value is not
// usable.
type Client struct {
	baseURL string
	http    *http.Client
	log     *slog.Logger
}

func NewClient(cfg Config) *Client {
	base := strings.TrimSuffix(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		base = SandboxBaseURL
	}
	hc := cfg.HTTP
	if hc == nil {
		to := cfg.Timeout
		if to == 0 {
			to = 30 * time.Second
		}
		hc = &http.Client{Timeout: to}
	}
	return &Client{baseURL: base, http: hc, log: slog.Default()}
}

// call POSTs body as JSON to /Development/<op>test and unmarshals a successful
// (2xx) response into dst. op is the bare operation name, e.g.
// "performAccountEnquiry" — the stage prefix and "test" suffix are added here.
func (c *Client) call(ctx context.Context, op string, body, dst any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("%w: marshal %s request: %v", ErrUnavailable, op, err)
	}
	url := c.baseURL + devStagePrefix + op + "test"

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("%w: build %s request: %v", ErrUnavailable, op, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		c.log.Warn("idbi request failed", "op", op, "error", err)
		return fmt.Errorf("%w: %s: %v", ErrUnavailable, op, err)
	}
	defer resp.Body.Close()

	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))

	if resp.StatusCode >= 500 {
		c.log.Warn("idbi 5xx", "op", op, "status", resp.StatusCode, "body", trunc(payload))
		return fmt.Errorf("%w: %s: status %d", ErrUnavailable, op, resp.StatusCode)
	}
	if resp.StatusCode >= 400 {
		return parseAPIError(resp.StatusCode, payload)
	}
	if dst == nil {
		return nil
	}
	if err := json.Unmarshal(payload, dst); err != nil {
		c.log.Warn("idbi decode failed", "op", op, "body", trunc(payload))
		return fmt.Errorf("%w: decode %s response: %v", ErrUnavailable, op, err)
	}
	return nil
}

func parseAPIError(status int, body []byte) *APIError {
	e := &APIError{Status: status, RawBody: body}

	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] == '<' {
		e.Message = "forbidden (WAF)"
		if status != http.StatusForbidden {
			e.Message = "non-JSON error body"
		}
		return e
	}

	var env struct {
		Errors []struct {
			Code        string `json:"code"`
			Title       string `json:"title"`
			Description string `json:"description"`
		} `json:"errors"`
		Message      string   `json:"message"`
		FailedFields []string `json:"failedFields"`
	}
	if err := json.Unmarshal(trimmed, &env); err == nil {
		if len(env.Errors) > 0 {
			e.Code = env.Errors[0].Code
			e.Title = env.Errors[0].Title
			e.Message = env.Errors[0].Description
		}
		if env.Message != "" {
			e.Message = env.Message
		}
		e.FailedFields = env.FailedFields
	}
	if e.Message == "" && e.Title == "" {
		e.Message = string(trunc(trimmed))
	}
	return e
}

func trunc(b []byte) []byte {
	const n = 2048
	if len(b) > n {
		return append(b[:n:n], []byte("…")...)
	}
	return b
}

// AsAPIError extracts an *APIError from err, if present.
func AsAPIError(err error) (*APIError, bool) {
	var ae *APIError
	if errors.As(err, &ae) {
		return ae, true
	}
	return nil, false
}
