// Package advisortips generates the short "AI tip" shown on the portfolio
// analysis tabs (Allocation, Discipline, Performance) and on fund profiles.
//
// Each surface has its own named agent with a focused persona. Today the
// tips are produced by a single LLM call per agent (persona as the system
// prompt, the computed analysis result as the user content). When Bedrock
// Agents are wired, an agent's BedrockAgentID routes the call to a
// provider-side agent instead — no change here or at the call sites.
package advisortips

// Topic identifies an analysis surface.
type Topic string

const (
	TopicAllocation  Topic = "allocation"
	TopicDiscipline  Topic = "discipline"
	TopicPerformance Topic = "performance"
	TopicFundProfile Topic = "fund_profile"
)

// Agent is the persona behind one surface's tip.
type Agent struct {
	Topic Topic
	// Name is what the frontend shows as the tip's author.
	Name string
	// Persona is the system prompt: role, scope, tone, hard limits.
	Persona string
	// BedrockAgentID, when set and the active LLM provider is Bedrock,
	// routes generation to that provider-side agent.
	BedrockAgentID string
}

// commonRules is appended to every persona so tips stay short, specific and
// compliant regardless of surface.
const commonRules = `
Rules:
- 2 to 3 sentences, under 60 words. Plain, encouraging, concrete.
- Refer only to the numbers provided. Never invent holdings, returns or fees.
- No greetings, no disclaimers, no markdown, no bullet points. One paragraph.
- This is educational information, not investment advice; do not tell the user to buy or sell a specific security.`

var registry = map[Topic]Agent{
	TopicAllocation: {
		Topic: TopicAllocation,
		Name:  "Allocation Advisor",
		Persona: `You are the Allocation Advisor for the Astra investing app. You explain how a
user's money is split across equity, debt and other assets, whether that split
suits their stated risk level, and the single most useful adjustment to consider.` + commonRules,
	},
	TopicDiscipline: {
		Topic: TopicDiscipline,
		Name:  "Discipline Coach",
		Persona: `You are the Discipline Coach for the Astra investing app. You comment on the
user's investing consistency — SIP regularity, staying invested through dips,
avoiding panic exits — and give one habit to reinforce or fix.` + commonRules,
	},
	TopicPerformance: {
		Topic: TopicPerformance,
		Name:  "Performance Analyst",
		Persona: `You are the Performance Analyst for the Astra investing app. You interpret the
user's returns versus the benchmark, flag drag from high-cost funds, and name
the one factor most responsible for the gap.` + commonRules,
	},
	TopicFundProfile: {
		Topic: TopicFundProfile,
		Name:  "Fund Profiler",
		Persona: `You are the Fund Profiler for the Astra investing app. Given one fund's
category, cost, risk and role in the portfolio, you explain in plain terms what
kind of fund it is and how it fits alongside the user's other holdings.` + commonRules,
	},
}

// Agents returns a copy of the agent registry (topic -> agent).
func Agents() map[Topic]Agent {
	out := make(map[Topic]Agent, len(registry))
	for k, v := range registry {
		out[k] = v
	}
	return out
}

// AgentFor returns the agent for a topic, ok=false if the topic is unknown.
func AgentFor(t Topic) (Agent, bool) {
	a, ok := registry[t]
	return a, ok
}

// ParseTopic validates a raw topic string.
func ParseTopic(s string) (Topic, bool) {
	t := Topic(s)
	_, ok := registry[t]
	return t, ok
}
