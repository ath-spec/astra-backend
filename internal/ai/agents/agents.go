// Package agents is the routing catalog for every LLM-backed surface in the
// app: the user chat assistants, the RM/Admin copilots, the RM client-narrative
// writer, and the memory-extraction pass. The four portfolio pro-tip agents
// live in internal/service/advisortips but follow the same contract.
//
// An Agent carries only *routing* metadata — which model fallback list, which
// sampling params, and (when the active llm.Provider is Bedrock) which
// provider-side Bedrock Agent to invoke. The persona / system prompt stays at
// the call site that owns it. This keeps a later Groq -> Bedrock switch a
// config change (set BEDROCK_AGENT_<KEY>_ID, flip LLM_PROVIDER) with no drop in
// behaviour: until an id is set the call runs on the model list exactly as it
// does today.
package agents

import (
	"strings"

	"github.com/yourusername/astra-backend/internal/provider/llm"
)

// Key identifies one agent.
type Key string

const (
	KeyAppChat      Key = "app_chat"      // ASTRA in-app wealth advisor (full)
	KeyAppQuickChat Key = "app_quickchat" // ASTRA floating nav-pill (terse)
	KeyRMCopilot    Key = "rm_copilot"    // RM console copilot
	KeyAdminCopilot Key = "admin_copilot" // Admin console copilot
	KeyRMNarrator   Key = "rm_narrator"   // RM client-review narrative writer
	KeyMemory       Key = "memory"        // durable-fact extraction from a chat turn
)

// Agent is the routing profile for one surface.
type Agent struct {
	Key   Key
	Label string // human-readable, for logs / future display

	// Models is the ordered Groq fallback list — the first that answers wins.
	// This mirrors today's inline behaviour verbatim.
	Models []string
	// Temperature: nil leaves it at the provider default; non-nil is sent
	// as-is (an explicit 0 is honoured).
	Temperature *float64
	// MaxTokens: 0 means "leave unset" (provider default).
	MaxTokens int
	// ResponseFormat is "json_object" for agents whose reply must be JSON.
	ResponseFormat string

	// BedrockAgentID, when non-empty and the active provider is Bedrock,
	// routes the call to that provider-side agent instead of a raw model.
	BedrockAgentID string
}

// Request builds the llm.Request scaffold for this agent. The caller supplies
// the system prompt (persona) and the dialogue messages; the agent contributes
// model routing and sampling. The returned value is a copy — safe to mutate.
func (a Agent) Request(system string, msgs []llm.Message) llm.Request {
	return llm.Request{
		System:         system,
		Messages:       msgs,
		Models:         append([]string(nil), a.Models...),
		Temperature:    a.Temperature,
		MaxTokens:      a.MaxTokens,
		ResponseFormat: a.ResponseFormat,
		Agent:          a.BedrockAgentID,
	}
}

// chatModels is the shared fallback list for the conversational surfaces —
// identical to the lists currently hard-coded in service/ai.go and
// service/rm_ai.go.
var chatModels = []string{"openai/gpt-oss-120b", "openai/gpt-oss-20b"}

// defaults is the built-in catalog. Every entry encodes the behaviour that
// surface has in production today.
var defaults = map[Key]Agent{
	KeyAppChat: {
		Key: KeyAppChat, Label: "ASTRA Advisor",
		Models: chatModels,
	},
	KeyAppQuickChat: {
		Key: KeyAppQuickChat, Label: "ASTRA Quick Chat",
		Models: chatModels,
	},
	KeyRMCopilot: {
		Key: KeyRMCopilot, Label: "RM Copilot",
		Models: chatModels,
	},
	KeyAdminCopilot: {
		Key: KeyAdminCopilot, Label: "Admin Copilot",
		Models: chatModels,
	},
	KeyRMNarrator: {
		Key: KeyRMNarrator, Label: "RM Narrative Writer",
		Models:         []string{"llama-3.3-70b-versatile", "llama-3.1-8b-instant", "openai/gpt-oss-120b"},
		Temperature:    llm.Temp(0.2),
		MaxTokens:      2048,
		ResponseFormat: "json_object",
	},
	KeyMemory: {
		Key: KeyMemory, Label: "Memory Extractor",
		Models:      []string{"openai/gpt-oss-20b"},
		Temperature: llm.Temp(0),
	},
}

// Catalog is the resolved set of agents for a running process.
type Catalog struct {
	byKey map[Key]Agent
}

// Overrides carries deployment-time configuration for the catalog.
type Overrides struct {
	// BedrockAgentIDs maps an agent key to its Bedrock Agent id
	// (BEDROCK_AGENT_<KEY>_ID). Empty / missing entries leave the agent on
	// its model list.
	BedrockAgentIDs map[Key]string
	// Models optionally replaces an agent's fallback model list.
	Models map[Key][]string
}

// New resolves the built-in catalog against the given overrides.
func New(ov Overrides) *Catalog {
	c := &Catalog{byKey: make(map[Key]Agent, len(defaults))}
	for k, a := range defaults {
		if ms, ok := ov.Models[k]; ok && len(ms) > 0 {
			a.Models = append([]string(nil), ms...)
		}
		if id := strings.TrimSpace(ov.BedrockAgentIDs[k]); id != "" {
			a.BedrockAgentID = id
		}
		c.byKey[k] = a
	}
	return c
}

// Get returns the agent for key. It always returns a usable Agent: an unknown
// key falls back to the app-chat profile so a lookup typo degrades to a working
// call rather than an empty model list.
func (c *Catalog) Get(key Key) Agent {
	if a, ok := c.byKey[key]; ok {
		return a
	}
	return c.byKey[KeyAppChat]
}

// All returns every resolved agent (order unspecified) for logging / debug.
func (c *Catalog) All() []Agent {
	out := make([]Agent, 0, len(c.byKey))
	for _, a := range c.byKey {
		out = append(out, a)
	}
	return out
}
