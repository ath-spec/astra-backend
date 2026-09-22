package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// groqChatURL is a var (not const) so tests can point it at a stub server.
var groqChatURL = "https://api.groq.com/openai/v1/chat/completions"

// maxGroqResponseBytes caps how much of an upstream response we will read into
// memory, so a misbehaving or hostile endpoint cannot exhaust the process.
const maxGroqResponseBytes = 4 << 20 // 4 MiB

// GroqProvider talks to Groq's OpenAI-compatible chat-completions endpoint.
// It mirrors the behaviour of the existing inline Groq calls in
// internal/service (model fallback list, plain bearer auth).
type GroqProvider struct {
	apiKey string
	models []string
	http   *http.Client
}

// GroqConfig configures the Groq backend.
type GroqConfig struct {
	APIKey string
	// Models is the ordered fallback list; the first that returns 200 wins.
	Models  []string
	Timeout time.Duration
	HTTP    *http.Client
}

// DefaultGroqModels is used when GroqConfig.Models is empty.
var DefaultGroqModels = []string{"openai/gpt-oss-120b", "openai/gpt-oss-20b"}

func NewGroqProvider(cfg GroqConfig) *GroqProvider {
	models := cfg.Models
	if len(models) == 0 {
		models = DefaultGroqModels
	}
	hc := cfg.HTTP
	if hc == nil {
		to := cfg.Timeout
		if to == 0 {
			to = 30 * time.Second
		}
		hc = &http.Client{Timeout: to}
	}
	return &GroqProvider{apiKey: cfg.APIKey, models: models, http: hc}
}

func (p *GroqProvider) Name() string { return "groq" }

func (p *GroqProvider) Complete(ctx context.Context, req Request) (*Response, error) {
	if p.apiKey == "" {
		return nil, ErrNotConfigured
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}

	models := p.models
	switch {
	case len(req.Models) > 0:
		models = req.Models
	case req.Model != "":
		models = []string{req.Model}
	}

	payloadBase := map[string]any{"messages": toOpenAIMessages(req.allMessages())}
	if req.MaxTokens > 0 {
		payloadBase["max_tokens"] = req.MaxTokens
	}
	if req.Temperature != nil {
		payloadBase["temperature"] = *req.Temperature
	}
	if req.ResponseFormat == "json_object" {
		payloadBase["response_format"] = map[string]string{"type": "json_object"}
	}

	var lastErr error
	for _, model := range models {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		payload := make(map[string]any, len(payloadBase)+1)
		for k, v := range payloadBase {
			payload[k] = v
		}
		payload["model"] = model

		body, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("groq: encode request: %w", err)
		}
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, groqChatURL, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("groq: build request: %w", err)
		}
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := p.http.Do(httpReq)
		if err != nil {
			lastErr = fmt.Errorf("groq: call model %s: %w", model, err)
			continue
		}
		raw, readErr := io.ReadAll(io.LimitReader(resp.Body, maxGroqResponseBytes))
		resp.Body.Close()
		if readErr != nil {
			lastErr = fmt.Errorf("groq: read model %s response: %w", model, readErr)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			err := fmt.Errorf("groq: model %s returned %d: %s", model, resp.StatusCode, trunc(string(raw), 300))
			// Auth / permission failures are the same for every model — stop
			// rather than replay the whole fallback list against a 401.
			if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
				return nil, err
			}
			lastErr = err
			continue
		}

		var parsed struct {
			Model   string `json:"model"`
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
			Usage struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(raw, &parsed); err != nil {
			lastErr = fmt.Errorf("groq: decode response: %w", err)
			continue
		}
		if len(parsed.Choices) == 0 {
			lastErr = fmt.Errorf("groq: model %s returned no choices", model)
			continue
		}
		return &Response{
			Text:     strings.TrimSpace(parsed.Choices[0].Message.Content),
			Model:    firstNonEmpty(parsed.Model, model),
			Provider: "groq",
			Usage:    Usage{PromptTokens: parsed.Usage.PromptTokens, CompletionTokens: parsed.Usage.CompletionTokens},
		}, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("groq: no models configured")
	}
	return nil, lastErr
}

func toOpenAIMessages(ms []Message) []map[string]string {
	out := make([]map[string]string, len(ms))
	for i, m := range ms {
		out[i] = map[string]string{"role": m.Role, "content": m.Content}
	}
	return out
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
