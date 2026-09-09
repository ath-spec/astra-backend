package llm

import "strings"

// Config selects and configures the active LLM provider.
type Config struct {
	Provider string // "groq" (default) | "bedrock"
	Groq     GroqConfig
	Bedrock  BedrockConfig
}

// New builds the provider named by cfg.Provider. Unknown / empty => groq.
func New(cfg Config) Provider {
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "bedrock":
		return NewBedrockProvider(cfg.Bedrock)
	default:
		return NewGroqProvider(cfg.Groq)
	}
}
