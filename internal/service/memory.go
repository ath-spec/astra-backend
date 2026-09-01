package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/yourusername/astra-backend/internal/repository"
)

// MemoryService gives the consumer AI chat a durable, cross-session memory:
// facts, preferences, standing instructions and codewords the user stated in
// earlier conversations, plus one rolling summary of older dialogue.
//
// Two guarantees make stale vs. fresh unambiguous for the model:
//   - Every remembered item is rendered with the date the user said it.
//   - The prompt states that the LIVE GROUND-TRUTH block (recomputed each turn)
//     wins over memory on any number, holding, balance, price or date.
//
// Writes are extracted by a cheap model call after the reply is already sent, so
// they add no user-facing latency. Reads are one indexed query.
type MemoryService struct {
	repo    repository.ChatMemoryRepository
	groqKey string
	client  *http.Client
	model   string
}

func NewMemoryService(repo repository.ChatMemoryRepository, groqKey string) *MemoryService {
	return &MemoryService{
		repo:    repo,
		groqKey: groqKey,
		client:  &http.Client{Timeout: 25 * time.Second},
		model:   "openai/gpt-oss-20b", // small/cheap — extraction is a classify-and-condense job
	}
}

// ---- read path -------------------------------------------------------------

type RecallResult struct {
	Facts     []repository.ChatMemory
	Summary   string
	SummaryAt time.Time
}

func (r RecallResult) Empty() bool {
	return len(r.Facts) == 0 && strings.TrimSpace(r.Summary) == ""
}

const maxInjectedFacts = 8

// Recall loads the memory block for a user. Never errors out the caller — a
// memory failure must not break chat; it just yields an empty block.
func (s *MemoryService) Recall(ctx context.Context, userID uuid.UUID) RecallResult {
	if s == nil || s.repo == nil {
		return RecallResult{}
	}
	var res RecallResult
	if facts, err := s.repo.TopFacts(ctx, userID, maxInjectedFacts); err == nil {
		res.Facts = facts
	} else {
		slog.Warn("memory recall: top facts", "user_id", userID, "error", err)
	}
	if sum, at, err := s.repo.GetSummary(ctx, userID); err == nil {
		res.Summary, res.SummaryAt = sum, at
	}
	return res
}

// FormatForPrompt renders the memory block appended to the system prompt. Pure —
// no I/O — so it is unit-tested directly. Returns "" when there is nothing.
func (s *MemoryService) FormatForPrompt(r RecallResult) string {
	if r.Empty() {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n### PERSISTENT MEMORY (stated by the user in earlier sessions - NOT live data):\n")
	for _, m := range r.Facts {
		date := m.CapturedAt.UTC().Format("2006-01-02")
		tag := m.Kind
		if m.Source == "user" {
			tag += ", user-pinned"
		}
		fmt.Fprintf(&b, "[%s | %s] %s\n", date, tag, strings.TrimSpace(m.Content))
	}
	if strings.TrimSpace(r.Summary) != "" {
		if !r.SummaryAt.IsZero() {
			fmt.Fprintf(&b, "Earlier-conversation summary (as of %s): %s\n",
				r.SummaryAt.UTC().Format("2006-01-02"), strings.TrimSpace(r.Summary))
		} else {
			fmt.Fprintf(&b, "Earlier-conversation summary: %s\n", strings.TrimSpace(r.Summary))
		}
	}
	b.WriteString("Each item is dated to when the user said it. Use them to recall details on request " +
		"(e.g. a codeword) and to personalise. If any item conflicts with the LIVE GROUND-TRUTH block " +
		"above, the LIVE data is current and correct - rely on it and, if useful, note that the earlier value changed.\n")
	return b.String()
}

// ---- inspect / manual edit (API) ----------------------------------------

func (s *MemoryService) List(ctx context.Context, userID uuid.UUID) ([]repository.ChatMemory, error) {
	return s.repo.ListAll(ctx, userID)
}

func (s *MemoryService) Forget(ctx context.Context, userID, id uuid.UUID) (bool, error) {
	return s.repo.DeleteByID(ctx, userID, id)
}

// AddUserMemory pins a fact the user asked to be remembered, verbatim-ish.
func (s *MemoryService) AddUserMemory(ctx context.Context, userID uuid.UUID, content, kind string) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return fmt.Errorf("empty memory")
	}
	if len(content) > 400 {
		content = content[:400]
	}
	if !validMemoryKind(kind) {
		kind = "fact"
	}
	return s.repo.Upsert(ctx, repository.ChatMemory{
		UserID:   userID,
		Kind:     kind,
		Content:  content,
		KeyNorm:  normalizeKey(content),
		Salience: 5,
		Source:   "user",
	})
}

// ---- write path (async) -------------------------------------------------

// Observe runs after a reply is sent: it asks a small model what, if anything,
// in this exchange is worth remembering long-term, then upserts those memories
// and refreshes the rolling summary. Safe to call in a goroutine with a
// background context.
func (s *MemoryService) Observe(ctx context.Context, userID uuid.UUID, userMsg, assistantMsg, priorSummary string) {
	if s == nil || s.repo == nil || s.groqKey == "" {
		return
	}
	userMsg = strings.TrimSpace(userMsg)
	if userMsg == "" {
		return
	}

	ext, err := s.extract(ctx, userMsg, assistantMsg, priorSummary)
	if err != nil {
		slog.Warn("memory observe: extract", "user_id", userID, "error", err)
		return
	}

	kept := 0
	for _, m := range ext.Memories {
		content := strings.TrimSpace(m.Content)
		if content == "" || len([]rune(content)) > 300 {
			continue
		}
		if looksLikePII(content) || looksLikePII(m.Key) {
			continue
		}
		if !validMemoryKind(m.Kind) {
			m.Kind = "fact"
		}
		key := normalizeKey(firstNonEmpty(m.Key, content))
		if key == "" {
			continue
		}
		if err := s.repo.Upsert(ctx, repository.ChatMemory{
			UserID:   userID,
			Kind:     m.Kind,
			Content:  content,
			KeyNorm:  key,
			Salience: clampSalience(m.Salience),
			Source:   "auto",
		}); err != nil {
			slog.Warn("memory observe: upsert", "user_id", userID, "error", err)
			continue
		}
		kept++
		if kept >= 6 { // anti-spam: at most 6 new/updated memories per exchange
			break
		}
	}

	newSummary := strings.TrimSpace(ext.Summary)
	if newSummary != "" && newSummary != strings.TrimSpace(priorSummary) {
		if len([]rune(newSummary)) > 600 {
			newSummary = string([]rune(newSummary)[:600])
		}
		if err := s.repo.SetSummary(ctx, userID, newSummary); err != nil {
			slog.Warn("memory observe: set summary", "user_id", userID, "error", err)
		}
	}
}

// ---- extraction model call --------------------------------------------

type extraction struct {
	Memories []extractedMemory `json:"memories"`
	Summary  string            `json:"summary"`
}

type extractedMemory struct {
	Key      string `json:"key"`
	Kind     string `json:"kind"`
	Content  string `json:"content"`
	Salience int    `json:"salience"`
}

const memoryExtractSystemPrompt = `You maintain the long-term memory of a wealth-app assistant.
From ONE user/assistant exchange, extract only what is durable and user-specific.

Return STRICT JSON:
{"memories":[{"key":"<snake_case topic>","kind":"fact|preference|personal|instruction|goal_note","content":"<one sentence, <=20 words, no first person>","salience":1-5}],"summary":"<=40 words, running summary of the whole conversation; empty string to keep the existing one"}

KEEP: preferences, personal facts (family, job, plans, dates), standing instructions, codewords, anything the user says to "remember".
DROP: one-off questions, portfolio values / prices / balances / returns / holdings (these are tracked live elsewhere), greetings, small talk, and anything transient.
NEVER store: PAN, Aadhaar, phone numbers, account numbers, card numbers, OTP, passwords, tokens.
If the user updates an earlier fact, reuse the SAME key with the new content.
If nothing is durable, return {"memories":[],"summary":""}. Output JSON only, no prose, no code fences.`

func (s *MemoryService) extract(ctx context.Context, userMsg, assistantMsg, priorSummary string) (extraction, error) {
	var exchange strings.Builder
	if strings.TrimSpace(priorSummary) != "" {
		fmt.Fprintf(&exchange, "EXISTING SUMMARY: %s\n\n", strings.TrimSpace(priorSummary))
	}
	fmt.Fprintf(&exchange, "USER: %s\n\nASSISTANT: %s", truncateForExtract(userMsg, 1500), truncateForExtract(assistantMsg, 1500))

	payload := map[string]any{
		"model":       s.model,
		"temperature": 0,
		"messages": []map[string]any{
			{"role": "system", "content": memoryExtractSystemPrompt},
			{"role": "user", "content": exchange.String()},
		},
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, groqAPIURL, bytes.NewReader(body))
	if err != nil {
		return extraction{}, err
	}
	req.Header.Set("Authorization", "Bearer "+s.groqKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return extraction{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return extraction{}, fmt.Errorf("extract model status %d: %s", resp.StatusCode, truncateForExtract(string(raw), 200))
	}

	var chat struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &chat); err != nil || len(chat.Choices) == 0 {
		return extraction{}, fmt.Errorf("extract model: unreadable response")
	}
	return parseExtraction(chat.Choices[0].Message.Content)
}

// ---- pure helpers (unit-tested) -------------------------------------

var (
	keyNonWord    = regexp.MustCompile(`[^a-z0-9]+`)
	rePAN         = regexp.MustCompile(`\b[A-Z]{5}[0-9]{4}[A-Z]\b`)
	reLongDigits  = regexp.MustCompile(`\b\d{10,}\b`) // phone / account / aadhaar / card
	reSecretWord  = regexp.MustCompile(`(?i)\b(otp|password|passcode|cvv|pin|token|api[_ -]?key)\b`)
	reAadhaarSpce = regexp.MustCompile(`\b\d{4}\s\d{4}\s\d{4}\b`)
)

func normalizeKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = keyNonWord.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_")
	if len(s) > 120 {
		s = s[:120]
		s = strings.Trim(s, "_")
	}
	return s
}

// looksLikePII is a coarse guard so the extractor can never persist obvious
// identifiers even if the model misbehaves.
func looksLikePII(s string) bool {
	if s == "" {
		return false
	}
	return rePAN.MatchString(s) ||
		reLongDigits.MatchString(s) ||
		reAadhaarSpce.MatchString(s) ||
		reSecretWord.MatchString(s)
}

func clampSalience(n int) int {
	if n < 1 {
		return 1
	}
	if n > 5 {
		return 5
	}
	return n
}

func validMemoryKind(k string) bool {
	switch k {
	case "fact", "preference", "personal", "instruction", "goal_note":
		return true
	default:
		return false
	}
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func truncateForExtract(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}

// parseExtraction tolerates a bare JSON object, a ```json fenced block, or JSON
// with leading/trailing prose, and returns a zero extraction for "nothing".
func parseExtraction(raw string) (extraction, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return extraction{}, nil
	}
	if i := strings.Index(raw, "```"); i >= 0 {
		raw = raw[i+3:]
		raw = strings.TrimPrefix(raw, "json")
		if j := strings.Index(raw, "```"); j >= 0 {
			raw = raw[:j]
		}
		raw = strings.TrimSpace(raw)
	}
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return extraction{}, fmt.Errorf("no json object in extractor output")
	}
	var ext extraction
	if err := json.Unmarshal([]byte(raw[start:end+1]), &ext); err != nil {
		return extraction{}, fmt.Errorf("extractor json: %w", err)
	}
	return ext, nil
}
