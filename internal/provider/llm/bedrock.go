package llm

import (
	"context"
	"fmt"
)

// BedrockProvider is the provisioned Amazon Bedrock backend. The interface,
// config, and wiring are in place; the actual SDK call is deliberately not
// implemented yet — it is switched on after the AWS networking / IAM review
// with ACC by setting LLM_PROVIDER=bedrock and filling BEDROCK_*.
//
// When implementing:
//   - use github.com/aws/aws-sdk-go-v2/service/bedrockruntime (Converse API)
//     for raw model calls, keyed by ModelID;
//   - use github.com/aws/aws-sdk-go-v2/service/bedrockagentruntime
//     (InvokeAgent) when Request.Agent is set, keyed by AgentID + AgentAliasID;
//   - map the response back into *Response exactly as GroqProvider does.
//
// Credentials come from the default AWS chain (task role in ECS, or the
// AWS_* env vars locally) — never hard-code them here.
type BedrockProvider struct {
	region       string
	modelID      string
	agentID      string
	agentAliasID string
}

// BedrockConfig configures the Bedrock backend.
type BedrockConfig struct {
	Region       string // BEDROCK_REGION, e.g. "ap-south-1"
	ModelID      string // BEDROCK_MODEL_ID, e.g. "anthropic.claude-3-5-sonnet-20240620-v1:0"
	AgentID      string // BEDROCK_AGENT_ID (optional — for Request.Agent routing)
	AgentAliasID string // BEDROCK_AGENT_ALIAS_ID
}

func NewBedrockProvider(cfg BedrockConfig) *BedrockProvider {
	return &BedrockProvider{
		region:       cfg.Region,
		modelID:      cfg.ModelID,
		agentID:      cfg.AgentID,
		agentAliasID: cfg.AgentAliasID,
	}
}

func (p *BedrockProvider) Name() string { return "bedrock" }

// Ready reports whether enough config is present to attempt a call once the
// SDK path is implemented.
func (p *BedrockProvider) Ready() bool {
	return p.region != "" && (p.modelID != "" || (p.agentID != "" && p.agentAliasID != ""))
}

func (p *BedrockProvider) Complete(ctx context.Context, req Request) (*Response, error) {
	// Provisioned but not connected yet — callers fall back to "AI
	// unavailable" on ErrNotConfigured, same as an unset API key.
	return nil, fmt.Errorf("bedrock backend not connected yet (region=%q model=%q agent=%q): %w",
		p.region, p.modelID, p.agentID, ErrNotConfigured)
}
