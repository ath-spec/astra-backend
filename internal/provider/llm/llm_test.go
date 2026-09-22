package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFactory_DefaultsToGroq(t *testing.T) {
	if got := New(Config{}).Name(); got != "groq" {
		t.Errorf("empty provider => %q, want groq", got)
	}
	if got := New(Config{Provider: "GROQ"}).Name(); got != "groq" {
		t.Errorf("case-insensitive groq => %q", got)
	}
	if got := New(Config{Provider: "bedrock"}).Name(); got != "bedrock" {
		t.Errorf("bedrock => %q", got)
	}
	if got := New(Config{Provider: "wat"}).Name(); got != "groq" {
		t.Errorf("unknown => %q, want groq fallback", got)
	}
}

func TestGroq_NoKey_NotConfigured(t *testing.T) {
	_, err := NewGroqProvider(GroqConfig{}).Complete(context.Background(), Request{
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	})
	if err != ErrNotConfigured {
		t.Fatalf("want ErrNotConfigured, got %v", err)
	}
}

func TestGroq_SendsSystemAndMessages_ParsesReply(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer k" {
			t.Errorf("auth header = %q", r.Header.Get("Authorization"))
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"m1","choices":[{"message":{"content":"  hello there  "}}],"usage":{"prompt_tokens":11,"completion_tokens":3}}`))
	}))
	defer srv.Close()

	p := NewGroqProvider(GroqConfig{APIKey: "k", Models: []string{"m1"}, HTTP: srv.Client()})
	old := groqChatURL
	groqChatURL = srv.URL
	defer func() { groqChatURL = old }()

	resp, err := p.Complete(context.Background(), Request{
		System:      "be terse",
		Messages:    []Message{{Role: RoleUser, Content: "hi"}},
		MaxTokens:   50,
		Temperature: Temp(0.3),
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Text != "hello there" {
		t.Errorf("text = %q (want trimmed)", resp.Text)
	}
	if resp.Provider != "groq" || resp.Model != "m1" {
		t.Errorf("resp = %+v", resp)
	}
	if resp.Usage.PromptTokens != 11 || resp.Usage.CompletionTokens != 3 {
		t.Errorf("usage = %+v", resp.Usage)
	}
	msgs, _ := gotBody["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("sent %d messages, want system+user", len(msgs))
	}
	first, _ := msgs[0].(map[string]any)
	if first["role"] != "system" || !strings.Contains(first["content"].(string), "terse") {
		t.Errorf("system message not first: %+v", first)
	}
	if gotBody["max_tokens"].(float64) != 50 || gotBody["temperature"].(float64) != 0.3 {
		t.Errorf("params not forwarded: %+v", gotBody)
	}
}

func TestGroq_ExplicitZeroTemperatureAndModelsList(t *testing.T) {
	var gotBody map[string]any
	var gotModels []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		if m, ok := gotBody["model"].(string); ok {
			gotModels = append(gotModels, m)
		}
		// First model 500s so the fallback list is exercised.
		if len(gotModels) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"boom"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"model":"m2","choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer srv.Close()

	p := NewGroqProvider(GroqConfig{APIKey: "k", HTTP: srv.Client()})
	old := groqChatURL
	groqChatURL = srv.URL
	defer func() { groqChatURL = old }()

	resp, err := p.Complete(context.Background(), Request{
		Messages:    []Message{{Role: RoleUser, Content: "hi"}},
		Models:      []string{"m1", "m2"},
		Temperature: Temp(0),
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if resp.Text != "ok" {
		t.Errorf("text = %q", resp.Text)
	}
	if len(gotModels) != 2 || gotModels[0] != "m1" || gotModels[1] != "m2" {
		t.Errorf("fallback list not followed: %v", gotModels)
	}
	tmp, ok := gotBody["temperature"]
	if !ok || tmp.(float64) != 0 {
		t.Errorf("explicit temperature 0 not sent: %v (present=%v)", tmp, ok)
	}
}

func TestGroq_StopsOnAuthFailure(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"bad key"}`))
	}))
	defer srv.Close()

	p := NewGroqProvider(GroqConfig{APIKey: "k", Models: []string{"m1", "m2", "m3"}, HTTP: srv.Client()})
	old := groqChatURL
	groqChatURL = srv.URL
	defer func() { groqChatURL = old }()

	_, err := p.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err == nil {
		t.Fatal("want error on 401")
	}
	if calls != 1 {
		t.Errorf("auth failure should not replay the fallback list, got %d calls", calls)
	}
}

func TestGroq_RejectsEmptyRequest(t *testing.T) {
	_, err := NewGroqProvider(GroqConfig{APIKey: "k"}).Complete(context.Background(), Request{})
	if err == nil {
		t.Fatal("want validation error for empty request")
	}
}

func TestBedrock_Provisioned_NotConnected(t *testing.T) {
	p := NewBedrockProvider(BedrockConfig{Region: "ap-south-1", ModelID: "anthropic.x"})
	if p.Name() != "bedrock" {
		t.Errorf("name = %q", p.Name())
	}
	if !p.Ready() {
		t.Error("Ready() should be true with region+model")
	}
	_, err := p.Complete(context.Background(), Request{Messages: []Message{{Role: RoleUser, Content: "hi"}}})
	if err == nil || !strings.Contains(err.Error(), "not connected") {
		t.Fatalf("want 'not connected' wrapping ErrNotConfigured, got %v", err)
	}
}
