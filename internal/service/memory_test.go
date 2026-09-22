package service

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/yourusername/astra-backend/internal/repository"
)

func TestNormalizeKey(t *testing.T) {
	cases := map[string]string{
		"Codeword":                       "codeword",
		"  Wedding Plan!! ":              "wedding_plan",
		"crypto / gold preference":       "crypto_gold_preference",
		"__leading and trailing__":       "leading_and_trailing",
		strings.Repeat("x", 200):         strings.Repeat("x", 120),
		"日本語 only":                        "only",
	}
	for in, want := range cases {
		if got := normalizeKey(in); got != want {
			t.Errorf("normalizeKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLooksLikePII(t *testing.T) {
	pii := []string{
		"his PAN is ABCDE1234F",
		"call 9876543210",
		"account 123456789012345",
		"aadhaar 1234 5678 9012",
		"the OTP was 8890",
		"password is hunter2",
	}
	for _, s := range pii {
		if !looksLikePII(s) {
			t.Errorf("looksLikePII(%q) = false, want true", s)
		}
	}
	clean := []string{
		"user is planning a wedding in December 2026",
		"prefers low-risk debt-tilted portfolios",
		"codeword is bluefish",
		"wants to retire by 2045",
	}
	for _, s := range clean {
		if looksLikePII(s) {
			t.Errorf("looksLikePII(%q) = true, want false", s)
		}
	}
}

func TestClampSalience(t *testing.T) {
	for in, want := range map[int]int{-5: 1, 0: 1, 1: 1, 3: 3, 5: 5, 9: 5} {
		if got := clampSalience(in); got != want {
			t.Errorf("clampSalience(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestParseExtraction(t *testing.T) {
	t.Run("bare json", func(t *testing.T) {
		ext, err := parseExtraction(`{"memories":[{"key":"codeword","kind":"fact","content":"codeword is bluefish","salience":5}],"summary":"talked codewords"}`)
		if err != nil {
			t.Fatal(err)
		}
		if len(ext.Memories) != 1 || ext.Memories[0].Key != "codeword" || ext.Summary != "talked codewords" {
			t.Fatalf("bad parse: %+v", ext)
		}
	})
	t.Run("fenced with prose", func(t *testing.T) {
		raw := "Here you go:\n```json\n{\"memories\":[],\"summary\":\"\"}\n```\ndone"
		ext, err := parseExtraction(raw)
		if err != nil || len(ext.Memories) != 0 || ext.Summary != "" {
			t.Fatalf("parse = %+v err=%v", ext, err)
		}
	})
	t.Run("empty", func(t *testing.T) {
		if ext, err := parseExtraction("   "); err != nil || len(ext.Memories) != 0 {
			t.Fatalf("empty should be zero extraction, got %+v err %v", ext, err)
		}
	})
	t.Run("garbage", func(t *testing.T) {
		if _, err := parseExtraction("no json here"); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestFormatForPrompt(t *testing.T) {
	svc := &MemoryService{}

	if svc.FormatForPrompt(RecallResult{}) != "" {
		t.Fatal("empty recall should render empty string")
	}

	at := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	out := svc.FormatForPrompt(RecallResult{
		Facts: []repository.ChatMemory{
			{ID: uuid.New(), Kind: "fact", Content: `codeword is "bluefish"`, Source: "user", CapturedAt: at},
			{ID: uuid.New(), Kind: "preference", Content: "dislikes crypto and gold", Source: "auto", CapturedAt: at},
		},
		Summary:   "User reviewed sector drift and idle cash.",
		SummaryAt: at,
	})

	for _, want := range []string{
		"PERSISTENT MEMORY",
		"2026-08-12",
		"user-pinned",
		`codeword is "bluefish"`,
		"dislikes crypto and gold",
		"Earlier-conversation summary (as of 2026-08-12):",
		"LIVE data is current and correct",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("FormatForPrompt output missing %q\n---\n%s", want, out)
		}
	}
}

func TestValidMemoryKind(t *testing.T) {
	for _, k := range []string{"fact", "preference", "personal", "instruction", "goal_note"} {
		if !validMemoryKind(k) {
			t.Errorf("validMemoryKind(%q) = false", k)
		}
	}
	for _, k := range []string{"summary", "", "random", "note"} {
		if validMemoryKind(k) {
			t.Errorf("validMemoryKind(%q) = true", k)
		}
	}
}
