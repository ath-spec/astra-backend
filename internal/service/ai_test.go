package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/yourusername/astra-backend/internal/ai/agents"
	"github.com/yourusername/astra-backend/internal/provider/llm"
	"github.com/yourusername/astra-backend/internal/provider/speech"
	"github.com/yourusername/astra-backend/internal/repository"
)

type fakeLLMProvider struct {
	resp    *llm.Response
	err     error
	lastReq llm.Request
}

func (f *fakeLLMProvider) Name() string { return "fake" }
func (f *fakeLLMProvider) Complete(_ context.Context, r llm.Request) (*llm.Response, error) {
	f.lastReq = r
	return f.resp, f.err
}

type fakeSpeech struct {
	tts *speech.TTSResult
	err error
}

func (f *fakeSpeech) Name() string { return "fake" }
func (f *fakeSpeech) TextToSpeech(_ context.Context, _ speech.TTSRequest) (*speech.TTSResult, error) {
	return f.tts, f.err
}
func (f *fakeSpeech) SpeechToText(_ context.Context, _ speech.STTRequest) (*speech.STTResult, error) {
	return nil, f.err
}

type memChatRepo struct{ saved *repository.ChatSession }

func (m *memChatRepo) GetSessionForUser(_ context.Context, uid uuid.UUID) (*repository.ChatSession, error) {
	return &repository.ChatSession{UserID: uid}, nil
}
func (m *memChatRepo) SaveSession(_ context.Context, s *repository.ChatSession) error {
	m.saved = s
	return nil
}

func newTestAI(p llm.Provider, sp speech.Provider, repo repository.ChatRepository) *GroqAIService {
	return NewGroqAIService(p, sp, agents.New(agents.Overrides{}), repo)
}

func TestGetChatCompletion_ReWrapsIntoOpenAIEnvelope(t *testing.T) {
	p := &fakeLLMProvider{resp: &llm.Response{
		Text: "Buy the dip, calmly.", Model: "openai/gpt-oss-120b",
		Usage: llm.Usage{PromptTokens: 40, CompletionTokens: 6},
	}}
	repo := &memChatRepo{}
	svc := newTestAI(p, &fakeSpeech{}, repo)

	msgs := []map[string]interface{}{
		{"role": "system", "content": "you are ASTRA"},
		{"role": "user", "content": "market fell 3%"},
	}
	body, status, err := svc.GetChatCompletion(context.Background(), uuid.New(), msgs, false)
	if err != nil || status != 200 {
		t.Fatalf("status=%d err=%v", status, err)
	}

	// System prompt is lifted out; only the dialogue turn is sent as a message.
	if p.lastReq.System != "you are ASTRA" {
		t.Errorf("system not lifted: %q", p.lastReq.System)
	}
	if len(p.lastReq.Messages) != 1 || p.lastReq.Messages[0].Role != "user" {
		t.Errorf("messages = %+v", p.lastReq.Messages)
	}

	// The response bytes must still be the exact envelope the FE + memory
	// pipeline parse: choices[0].message.content.
	var env struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens int `json:"prompt_tokens"`
		} `json:"usage"`
	}
	if e := json.Unmarshal(body, &env); e != nil {
		t.Fatalf("envelope not JSON: %v", e)
	}
	if len(env.Choices) != 1 || env.Choices[0].Message.Content != "Buy the dip, calmly." {
		t.Errorf("envelope content wrong: %s", body)
	}
	if env.Choices[0].Message.Role != "assistant" || env.Model != "openai/gpt-oss-120b" || env.Usage.PromptTokens != 40 {
		t.Errorf("envelope metadata wrong: %s", body)
	}

	// History persisted: dialogue turn + assistant reply, no system message.
	if repo.saved == nil || len(repo.saved.Messages) != 2 {
		t.Fatalf("history not saved as expected: %+v", repo.saved)
	}
	if repo.saved.Messages[1]["role"] != "assistant" {
		t.Errorf("assistant turn not appended: %+v", repo.saved.Messages)
	}
}

func TestGetChatCompletion_QuickSelectsNavPillAgent(t *testing.T) {
	p := &fakeLLMProvider{resp: &llm.Response{Text: "ok"}}
	svc := newTestAI(p, &fakeSpeech{}, &memChatRepo{})

	_, _, err := svc.GetChatCompletion(context.Background(), uuid.New(),
		[]map[string]interface{}{{"role": "user", "content": "hi"}}, true)
	if err != nil {
		t.Fatal(err)
	}
	want := agents.New(agents.Overrides{}).Get(agents.KeyAppQuickChat).Models
	if len(p.lastReq.Models) != len(want) || p.lastReq.Models[0] != want[0] {
		t.Errorf("quick chat did not use the nav-pill agent models: %v", p.lastReq.Models)
	}
}

func TestGetChatCompletion_UnconfiguredDegradesTo503(t *testing.T) {
	p := &fakeLLMProvider{err: llm.ErrNotConfigured}
	svc := newTestAI(p, &fakeSpeech{}, &memChatRepo{})

	body, status, err := svc.GetChatCompletion(context.Background(), uuid.New(),
		[]map[string]interface{}{{"role": "user", "content": "hi"}}, false)
	if err != nil {
		t.Errorf("ErrNotConfigured should not surface as a hard error, got %v", err)
	}
	if status != 503 {
		t.Errorf("status = %d, want 503", status)
	}
	if len(body) == 0 {
		t.Error("want a JSON error body")
	}
}

func TestGetChatCompletion_ProviderErrorIsReported(t *testing.T) {
	p := &fakeLLMProvider{err: errors.New("groq 500")}
	svc := newTestAI(p, &fakeSpeech{}, &memChatRepo{})

	_, status, err := svc.GetChatCompletion(context.Background(), uuid.New(),
		[]map[string]interface{}{{"role": "user", "content": "hi"}}, false)
	if err == nil || status != 502 {
		t.Errorf("want 502 + error, got status=%d err=%v", status, err)
	}
}

func TestGetTextToSpeech_ForwardsProviderBody(t *testing.T) {
	sp := &fakeSpeech{tts: &speech.TTSResult{Audio: []byte(`{"audios":["deadbeef"]}`)}}
	svc := newTestAI(&fakeLLMProvider{}, sp, &memChatRepo{})

	body, status, err := svc.GetTextToSpeech(context.Background(), "hello")
	if err != nil || status != 200 {
		t.Fatalf("status=%d err=%v", status, err)
	}
	if string(body) != `{"audios":["deadbeef"]}` {
		t.Errorf("body not forwarded verbatim: %s", body)
	}
}

func TestGetTextToSpeech_UnconfiguredDegrades(t *testing.T) {
	sp := &fakeSpeech{err: speech.ErrNotConfigured}
	svc := newTestAI(&fakeLLMProvider{}, sp, &memChatRepo{})

	_, status, err := svc.GetTextToSpeech(context.Background(), "hello")
	if err != nil || status != 503 {
		t.Errorf("want graceful 503, got status=%d err=%v", status, err)
	}
}
