# AI Chat Memory & Zero-PII Pipeline Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement an enterprise-grade AI chat architecture featuring a programmatic Zero-PII redaction firewall, token-dense live financial ground-truth context, and multi-turn episodic conversation memory with entity extraction.

**Architecture:** Concurrently gathers all non-sensitive user portfolio and analytics domains into a token-dense bracketed state vector (~250 tokens), scrubs all outbound payloads through a regex PII firewall, maintains a clean sliding window of dialogue turns, and extracts persistent user preferences/pinned entities asynchronously across sessions.

**Tech Stack:** Go 1.22+, PostgreSQL (pgxpool), Groq LLM API (LLaMA-3.3-70B), Chi Router.

---

## File Structure

- Create: `internal/service/pii_redactor.go` — Regex-based PII scrubber for PAN, Aadhaar, Phone numbers, and Bank account numbers.
- Create: `internal/service/pii_redactor_test.go` — Unit tests validating redaction of all sensitive PII patterns.
- Modify: `internal/repository/chat_repo.go` — Extend `ChatSession` struct with `Summary` and `PinnedEntities` metadata and update SQL queries.
- Modify: `internal/service/ai.go` — Add async background summarizer and entity extractor to `GroqAIService`.
- Modify: `internal/handler/chat.go` — Connect PII redactor, episodic memory tags, token-dense context builder, and sliding-window memory management.

---

### Task 1: Zero-PII Redaction Engine

**Files:**
- Create: `internal/service/pii_redactor.go`
- Create: `internal/service/pii_redactor_test.go`

- [ ] **Step 1: Write the failing unit tests for PIIRedactor**

```go
// internal/service/pii_redactor_test.go
package service

import (
	"testing"
)

func TestPIIRedactor_Redact(t *testing.T) {
	redactor := NewPIIRedactor()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Redacts PAN Card",
			input:    "My PAN number is ABCDE1234F for verification",
			expected: "My PAN number is [REDACTED_PAN] for verification",
		},
		{
			name:     "Redacts 12-digit Aadhaar Number",
			input:    "Aadhaar: 1234 5678 9012 and 987654321098",
			expected: "Aadhaar: [REDACTED_AADHAAR] and [REDACTED_AADHAAR]",
		},
		{
			name:     "Redacts 12-18 digit Bank Account Numbers",
			input:    "Transfer to account 123456789012345 please",
			expected: "Transfer to account [REDACTED_ACCT] please",
		},
		{
			name:     "Redacts Indian Phone Numbers",
			input:    "Call me at +919876543210 or 9123456789",
			expected: "Call me at [REDACTED_PHONE] or [REDACTED_PHONE]",
		},
		{
			name:     "Preserves Non-Sensitive Currency and Dates",
			input:    "Invested ₹1,50,000 in Parag Parikh Flexi Cap Fund on 12 Jan 2026",
			expected: "Invested ₹1,50,000 in Parag Parikh Flexi Cap Fund on 12 Jan 2026",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := redactor.Redact(tt.input)
			if got != tt.expected {
				t.Errorf("Redact() = %q, want %q", got, tt.expected)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -v ./internal/service -run TestPIIRedactor_Redact`  
Expected: FAIL with undefined `NewPIIRedactor`

- [ ] **Step 3: Implement PIIRedactor**

```go
// internal/service/pii_redactor.go
package service

import (
	"regexp"
)

// PIIRedactor sanitizes high-risk PII identifiers before prompts leave to external LLM providers.
type PIIRedactor struct {
	panRegex     *regexp.Regexp
	aadhaarRegex *regexp.Regexp
	acctRegex    *regexp.Regexp
	phoneRegex   *regexp.Regexp
}

func NewPIIRedactor() *PIIRedactor {
	return &PIIRedactor{
		// Indian PAN format (5 uppercase letters, 4 digits, 1 uppercase letter)
		panRegex: regexp.MustCompile(`\b[A-Z]{5}[0-9]{4}[A-Z]{1}\b`),
		// 12-digit Aadhaar format with optional spaces
		aadhaarRegex: regexp.MustCompile(`\b\d{4}\s?\d{4}\s?\d{4}\b`),
		// 12 to 18 digit bank account numbers
		acctRegex: regexp.MustCompile(`\b\d{12,18}\b`),
		// Indian 10-digit mobile numbers with optional +91/91 prefix
		phoneRegex: regexp.MustCompile(`(?:\+91[\-\s]?)?[6-9]\d{9}\b`),
	}
}

// Redact scrubs sensitive PII tokens from input text.
func (r *PIIRedactor) Redact(text string) string {
	text = r.panRegex.ReplaceAllString(text, "[REDACTED_PAN]")
	text = r.aadhaarRegex.ReplaceAllString(text, "[REDACTED_AADHAAR]")
	text = r.acctRegex.ReplaceAllString(text, "[REDACTED_ACCT]")
	text = r.phoneRegex.ReplaceAllString(text, "[REDACTED_PHONE]")
	return text
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test -v ./internal/service -run TestPIIRedactor_Redact`  
Expected: PASS

---

### Task 2: Episodic Memory & Pinned Entity Persistence

**Files:**
- Modify: `internal/repository/chat_repo.go`

- [ ] **Step 1: Extend ChatSession Model and SQL operations**

```go
// In internal/repository/chat_repo.go

type ChatSession struct {
	ID             uuid.UUID
	UserID         uuid.UUID
	Messages       []map[string]interface{}
	Summary        string
	PinnedEntities []string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
```

Update `GetSessionForUser` and `SaveSession` in `PostgresChatRepository` to read/write `summary` and `pinned_entities` with backward compatibility.

- [ ] **Step 2: Run repository compilation and tests**

Run: `go build ./internal/repository/...`  
Expected: PASS

---

### Task 3: Async Memory Summarization & Entity Extractor

**Files:**
- Modify: `internal/service/ai.go`

- [ ] **Step 1: Implement background summarizer and entity extractor**

In `GroqAIService`:
1. When a session reaches $\ge 8$ dialogue turns and no summary was generated in the last 10 minutes, spawn an async goroutine.
2. Call Groq with a low-temperature summarization prompt:
   - Input: Recent conversation dialogue.
   - Output JSON: `{"summary": "...", "pinned_entities": ["[GOAL] Invest 20k/mo in flexi cap", "[PREFERENCE] Low risk debt tilt"]}`.
3. Save the `summary` and `pinned_entities` into `session` via `chatRepo.SaveSession`.

- [ ] **Step 2: Run unit and package tests**

Run: `go test -v ./internal/service/...`  
Expected: PASS

---

### Task 4: Handler Integration with PII Firewall & Live Context

**Files:**
- Modify: `internal/handler/chat.go`

- [ ] **Step 1: Connect PIIRedactor and Episodic Memory into HandleChat**

1. Instantiate `piiRedactor := service.NewPIIRedactor()`.
2. In `buildUserLiveContext`, retrieve session summary and pinned entities if available, and append:
   ```text
   [PINNED_ENTITIES: Goal="Retirement 2045" | Pref="Tax-saving equity" | Plan="Increase SIP by 10%"]
   [CONVERSATION_SUMMARY: User previously reviewed sector drift and planned to allocate deployable cash into liquid funds.]
   ```
3. Run `promptContent = piiRedactor.Redact(promptContent)` to ensure zero PII leakage.
4. Filter incoming user messages through `piiRedactor.Redact(userMsg)`.
5. Prepend the single, fresh system prompt to clean dialogue history and call `aiService.GetChatCompletion`.

- [ ] **Step 2: Verify package builds**

Run: `go build ./...`  
Expected: PASS

---

### Task 5: End-to-End Verification

- [ ] **Step 1: Run complete test suite**

Run: `go test ./...`  
Expected: All tests pass with exit code 0.

- [ ] **Step 2: Run backend binary check**

Run: `go build -o tmp_api.exe ./cmd/api`  
Expected: Clean compilation with exit code 0.
