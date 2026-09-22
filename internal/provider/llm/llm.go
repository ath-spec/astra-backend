// Package llm is the provider-agnostic seam for large-language-model calls.
//
// Today the only wired backend is Groq (OpenAI-compatible chat completions).
// A Bedrock backend is provisioned behind the same interface and becomes the
// active provider by setting LLM_PROVIDER=bedrock once the AWS networking /
// IAM is in place — no call-site changes.
//
// Keep this package free of business logic: it turns a Request into text.
package llm

import (
	"context"
	"errors"
	"strconv"
)

// Role values for a chat message.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// Message is one turn in a chat-style prompt.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Request is a single completion call. System is optional and, when set, is
// prepended as a system message.
type Request struct {
	System   string
	Messages []Message
	// Model is a single per-call model override. Ignored when Models is set.
	Model string
	// Models is an ordered fallback list; the first that returns a usable
	// response wins. Empty => the provider's configured default list.
	Models    []string
	MaxTokens int // 0 => provider default
	// Temperature is optional: nil => provider default, non-nil => sent as-is
	// (so an explicit 0.0 is honoured, which matters for extraction/JSON calls).
	Temperature *float64
	// ResponseFormat, when "json_object", asks the provider to constrain the
	// reply to valid JSON. Providers that cannot honour it ignore it.
	ResponseFormat string
	// Agent, when set, routes the call to a named provider-side agent
	// (e.g. a Bedrock Agent) instead of a raw model. Providers that do not
	// support agents ignore it.
	Agent string
}

// Validate checks the request is well-formed before it reaches a backend.
func (r Request) Validate() error {
	if r.System == "" && len(r.Messages) == 0 {
		return errors.New("llm: request has no system prompt and no messages")
	}
	for i, m := range r.Messages {
		if m.Role == "" {
			return errors.New("llm: message " + strconv.Itoa(i) + " has no role")
		}
	}
	return nil
}

// Temp returns a pointer to f, for setting Request.Temperature inline.
func Temp(f float64) *float64 { return &f }

// Usage is token accounting, when the provider reports it.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// Response is the normalised result of a completion.
type Response struct {
	Text     string `json:"text"`
	Model    string `json:"model"`
	Provider string `json:"provider"`
	Usage    Usage  `json:"usage"`
}

// Provider is implemented by each LLM backend.
type Provider interface {
	// Complete runs one completion and returns the assistant text.
	Complete(ctx context.Context, req Request) (*Response, error)
	// Name is the backend identifier ("groq", "bedrock").
	Name() string
}

// ErrNotConfigured means the selected provider has no usable credentials /
// wiring yet. Callers should treat it as "AI unavailable", not a hard error.
var ErrNotConfigured = errors.New("llm: provider not configured")

// messages assembles System + Messages into a single slice.
func (r Request) allMessages() []Message {
	out := make([]Message, 0, len(r.Messages)+1)
	if r.System != "" {
		out = append(out, Message{Role: RoleSystem, Content: r.System})
	}
	return append(out, r.Messages...)
}
