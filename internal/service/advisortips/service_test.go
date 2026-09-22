package advisortips

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/yourusername/astra-backend/internal/provider/llm"
)

type fakeLLM struct {
	reply string
	err   error
	calls int
	last  llm.Request
}

func (f *fakeLLM) Name() string { return "fake" }
func (f *fakeLLM) Complete(_ context.Context, req llm.Request) (*llm.Response, error) {
	f.calls++
	f.last = req
	if f.err != nil {
		return nil, f.err
	}
	return &llm.Response{Text: f.reply, Model: "fake-1", Provider: "fake"}, nil
}

func TestGenerate_UsesAgentPersonaAndCaches(t *testing.T) {
	f := &fakeLLM{reply: `"Shift a little from mid-caps to large-caps to match your balanced risk level."`}
	s := New(f, Config{}, nil)

	payload := map[string]any{"level": "BALANCED", "equity_pct": 72.0}
	tip, err := s.Generate(context.Background(), "user-1", TopicAllocation, payload)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if tip.Agent != "Allocation Advisor" {
		t.Errorf("agent = %q", tip.Agent)
	}
	if strings.HasPrefix(tip.Text, `"`) || strings.HasSuffix(tip.Text, `"`) {
		t.Errorf("tip not de-quoted: %q", tip.Text)
	}
	if !strings.Contains(f.last.System, "Allocation Advisor") {
		t.Errorf("persona not sent as system: %q", f.last.System)
	}
	if !strings.Contains(f.last.Messages[0].Content, "BALANCED") {
		t.Errorf("payload not in user message: %q", f.last.Messages[0].Content)
	}

	// identical call -> cache hit, no second LLM call
	tip2, _ := s.Generate(context.Background(), "user-1", TopicAllocation, payload)
	if !tip2.Cached || f.calls != 1 {
		t.Errorf("expected cache hit, calls=%d cached=%v", f.calls, tip2.Cached)
	}

	// changed payload -> fresh call
	if _, err := s.Generate(context.Background(), "user-1", TopicAllocation, map[string]any{"level": "AGGRESSIVE"}); err != nil {
		t.Fatal(err)
	}
	if f.calls != 2 {
		t.Errorf("changed payload should re-call LLM, calls=%d", f.calls)
	}
}

func TestGenerate_PerTopicBedrockAgentOverride(t *testing.T) {
	f := &fakeLLM{reply: "ok"}
	s := New(f, Config{AgentIDs: map[Topic]string{TopicPerformance: "bdrk-perf-7"}}, nil)
	if _, err := s.Generate(context.Background(), "u", TopicPerformance, map[string]any{"x": 1}); err != nil {
		t.Fatal(err)
	}
	if f.last.Agent != "bdrk-perf-7" {
		t.Errorf("agent id override not applied: %q", f.last.Agent)
	}
}

func TestGenerate_UnknownTopic(t *testing.T) {
	s := New(&fakeLLM{reply: "x"}, Config{}, nil)
	if _, err := s.Generate(context.Background(), "u", Topic("nope"), nil); err == nil {
		t.Fatal("expected error for unknown topic")
	}
}

func TestGenerate_PropagatesProviderError(t *testing.T) {
	s := New(&fakeLLM{err: llm.ErrNotConfigured}, Config{}, nil)
	_, err := s.Generate(context.Background(), "u", TopicDiscipline, map[string]any{"score": 3})
	if !errors.Is(err, llm.ErrNotConfigured) {
		t.Fatalf("want ErrNotConfigured passthrough, got %v", err)
	}
}

func TestAgents_AllFourRegistered(t *testing.T) {
	got := Agents()
	for _, tp := range []Topic{TopicAllocation, TopicDiscipline, TopicPerformance, TopicFundProfile} {
		a, ok := got[tp]
		if !ok || a.Name == "" || a.Persona == "" {
			t.Errorf("agent for %q missing/incomplete: %+v", tp, a)
		}
	}
}
