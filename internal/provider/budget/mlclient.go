// Package budget holds the HTTP client for the budget-bloc ML service (the
// Hugging Face Space at zeyro87-budget-bloc). Wired endpoints — the set
// the budget flow calls:
//
//	POST {base}/ml/diagnosis              -> initial budget + insights
//	POST {base}/suggest/categories        -> per-category split for a chosen total
//	GET  {base}/analytics/recommendations -> dashboard insights
//	POST {base}/reallocation/run          -> Smart Rebalance proposal
//	POST {base}/reallocation/apply        -> apply an accepted rebalance
//	POST {base}/ml/learn                  -> feed accept/reject feedback back
//
// budget-bloc's remaining endpoints (/budget/create, /analytics/spending-summary,
// /ml/validate, /ml/optimize, /transactions, /export, /import, /statistics)
// are not used by any wired feature and are deliberately omitted.
package budget

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	budgetdomain "github.com/yourusername/astra-backend/internal/domain/budget"
)

const DefaultBaseURL = "https://zeyro87-budget-bloc.hf.space/api/v1"

// ErrUnavailable means the ML service could not be reached or returned a
// non-conflict error. Callers fall back to local heuristics.
var ErrUnavailable = errors.New("budget ML service unavailable")

// ConflictError carries a 409 body straight through to the HTTP client so
// the Flutter conflict sheet can render it verbatim.
type ConflictError struct {
	RawBody []byte
}

func (e *ConflictError) Error() string { return "budget ML returned 409 conflict" }

// Client talks to budget-bloc. The zero value is not usable; use NewClient.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

func NewClient(baseURL, token string) *Client {
	baseURL = strings.TrimSuffix(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	if !strings.HasSuffix(baseURL, "/api/v1") {
		baseURL += "/api/v1"
	}
	return &Client{
		baseURL: baseURL,
		token:   strings.TrimSpace(token),
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Diagnosis -> POST /ml/diagnosis
func (c *Client) Diagnosis(ctx context.Context, userID string, req budgetdomain.MLDiagnosisRequest) (*budgetdomain.MLDiagnosisResponse, error) {
	var out budgetdomain.MLDiagnosisResponse
	if err := c.post(ctx, "/ml/diagnosis", userID, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SuggestCategories -> POST /suggest/categories
func (c *Client) SuggestCategories(ctx context.Context, userID string, req budgetdomain.MLSuggestRequest) (*budgetdomain.SuggestCategoriesResponse, error) {
	var out budgetdomain.SuggestCategoriesResponse
	if err := c.post(ctx, "/suggest/categories", userID, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Recommendations -> GET /analytics/recommendations?budget_name=&user_id=
func (c *Client) Recommendations(ctx context.Context, userID, budgetName string) (*budgetdomain.MLRecommendationsResponse, error) {
	q := url.Values{}
	if budgetName != "" {
		q.Set("budget_name", budgetName)
	}
	if userID != "" {
		q.Set("user_id", userID)
	}
	var out budgetdomain.MLRecommendationsResponse
	if err := c.get(ctx, "/analytics/recommendations?"+q.Encode(), userID, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RunReallocation -> POST /reallocation/run
func (c *Client) RunReallocation(ctx context.Context, userID string, req budgetdomain.MLRunReallocationRequest) (*budgetdomain.MLRunReallocationResponse, error) {
	var out budgetdomain.MLRunReallocationResponse
	if err := c.post(ctx, "/reallocation/run", userID, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ApplyReallocation -> POST /reallocation/apply
func (c *Client) ApplyReallocation(ctx context.Context, userID string, req budgetdomain.MLApplyReallocationRequest) (*budgetdomain.MLApplyReallocationResponse, error) {
	var out budgetdomain.MLApplyReallocationResponse
	if err := c.post(ctx, "/reallocation/apply", userID, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Learn -> POST /ml/learn (feeds accept/reject feedback back to the model)
func (c *Client) Learn(ctx context.Context, userID string, req budgetdomain.MLLearnRequest) (*budgetdomain.MLLearnResponse, error) {
	var out budgetdomain.MLLearnResponse
	if err := c.post(ctx, "/ml/learn", userID, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) get(ctx context.Context, path, userID string, dst interface{}) error {
	return c.do(ctx, http.MethodGet, path, userID, nil, dst)
}

func (c *Client) post(ctx context.Context, path, userID string, body, dst interface{}) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("%w: marshal request: %v", ErrUnavailable, err)
	}
	return c.do(ctx, http.MethodPost, path, userID, raw, dst)
}

func (c *Client) do(ctx context.Context, method, path, userID string, body []byte, dst interface{}) error {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("%w: build request: %v", ErrUnavailable, err)
	}
	if body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.token)
	}
	if userID != "" {
		httpReq.Header.Set("X-User-Id", userID)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		slog.Warn("budget ML request failed", "path", path, "error", err)
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()

	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode == http.StatusConflict {
		return &ConflictError{RawBody: payload}
	}
	if resp.StatusCode >= 400 {
		slog.Warn("budget ML non-2xx", "path", path, "status", resp.StatusCode, "body", string(payload))
		return fmt.Errorf("%w: status %d", ErrUnavailable, resp.StatusCode)
	}

	if err := json.Unmarshal(payload, dst); err != nil {
		return fmt.Errorf("%w: decode response: %v", ErrUnavailable, err)
	}
	return nil
}
