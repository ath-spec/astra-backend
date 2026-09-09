package advisortips

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/yourusername/astra-backend/internal/provider/llm"
)

// LLM is the subset of llm.Provider this service needs.
type LLM interface {
	Complete(ctx context.Context, req llm.Request) (*llm.Response, error)
	Name() string
}

// Config tunes the service.
type Config struct {
	CacheTTL    time.Duration // per (subject, topic, payload) cache; default 15m
	CallTimeout time.Duration // hard cap on one LLM call; default 15s
	MaxTokens   int           // per call; default 200
	Temperature float64       // default 0.4
	MaxCache    int           // cache entry ceiling; default 5000
	// AgentIDs optionally overrides each topic's BedrockAgentID (used only
	// when the active LLM provider is Bedrock). Wired from BEDROCK_AGENT_*_ID.
	AgentIDs map[Topic]string
}

// Tip is the outward shape returned to the client.
type Tip struct {
	Topic       Topic     `json:"topic"`
	Agent       string    `json:"agent"`    // display name for the FE
	Text        string    `json:"tip"`      // the tip itself
	Model       string    `json:"model"`    // model / agent that produced it
	Provider    string    `json:"provider"` // "groq" | "bedrock"
	Cached      bool      `json:"cached"`
	GeneratedAt time.Time `json:"generated_at"`
}

type Service struct {
	llm LLM
	cfg Config
	log *slog.Logger

	mu    sync.Mutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	tip Tip
	exp time.Time
}

func New(provider LLM, cfg Config, log *slog.Logger) *Service {
	if cfg.CacheTTL == 0 {
		cfg.CacheTTL = 15 * time.Minute
	}
	if cfg.CallTimeout == 0 {
		cfg.CallTimeout = 15 * time.Second
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 200
	}
	if cfg.Temperature == 0 {
		cfg.Temperature = 0.4
	}
	if cfg.MaxCache == 0 {
		cfg.MaxCache = 5000
	}
	if log == nil {
		log = slog.Default()
	}
	return &Service{llm: provider, cfg: cfg, cache: map[string]cacheEntry{}, log: log}
}

// Generate returns the tip for one topic. `subject` scopes the cache (use the
// user id, or the scheme code for a fund profile). `payload` is the computed
// analysis result — it is JSON-encoded and handed to the agent as context.
func (s *Service) Generate(ctx context.Context, subject string, topic Topic, payload any) (Tip, error) {
	agent, ok := AgentFor(topic)
	if !ok {
		return Tip{}, fmt.Errorf("advisortips: unknown topic %q", topic)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return Tip{}, fmt.Errorf("advisortips: encode payload: %w", err)
	}

	key := cacheKey(subject, topic, body)
	if t, ok := s.getCached(key); ok {
		t.Cached = true
		return t, nil
	}

	agentID := agent.BedrockAgentID
	if v, ok := s.cfg.AgentIDs[topic]; ok && v != "" {
		agentID = v
	}

	callCtx, cancel := context.WithTimeout(ctx, s.cfg.CallTimeout)
	defer cancel()

	resp, err := s.llm.Complete(callCtx, llm.Request{
		System: agent.Persona,
		Messages: []llm.Message{{
			Role:    llm.RoleUser,
			Content: "Here is the analysis for this user. Write the tip.\n\n" + string(body),
		}},
		Agent:       agentID,
		MaxTokens:   s.cfg.MaxTokens,
		Temperature: llm.Temp(s.cfg.Temperature),
	})
	if err != nil {
		return Tip{}, err
	}
	if resp == nil || strings.TrimSpace(resp.Text) == "" {
		return Tip{}, fmt.Errorf("advisortips: %s agent returned an empty tip", agent.Name)
	}

	tip := Tip{
		Topic:       topic,
		Agent:       agent.Name,
		Text:        cleanTip(resp.Text),
		Model:       resp.Model,
		Provider:    resp.Provider,
		GeneratedAt: time.Now().UTC(),
	}
	s.setCached(key, tip)
	return tip, nil
}

func (s *Service) getCached(key string) (Tip, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.cache[key]
	if !ok || time.Now().After(e.exp) {
		return Tip{}, false
	}
	return e.tip, true
}

func (s *Service) setCached(key string, t Tip) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	// Opportunistically drop expired entries so the map cannot grow without
	// bound across many users / payload variants.
	if len(s.cache) >= s.cfg.MaxCache {
		for k, e := range s.cache {
			if now.After(e.exp) {
				delete(s.cache, k)
			}
		}
		// Still full of live entries — evict arbitrary ones to stay bounded.
		for k := range s.cache {
			if len(s.cache) < s.cfg.MaxCache {
				break
			}
			delete(s.cache, k)
		}
	}
	s.cache[key] = cacheEntry{tip: t, exp: now.Add(s.cfg.CacheTTL)}
}

func cacheKey(subject string, topic Topic, payload []byte) string {
	h := sha1.Sum(payload)
	return subject + "|" + string(topic) + "|" + hex.EncodeToString(h[:8])
}

// cleanTip strips stray markdown / quoting an LLM sometimes adds despite the
// persona rules.
func cleanTip(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "\"'`")
	s = strings.TrimPrefix(s, "Tip:")
	return strings.TrimSpace(s)
}
