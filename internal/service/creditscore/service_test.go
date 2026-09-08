package creditscore

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestMockSource_StableAndInRange(t *testing.T) {
	uid := uuid.New()
	s1, err := MockSource{}.FetchScore(context.Background(), uid)
	if err != nil {
		t.Fatal(err)
	}
	s2, _ := MockSource{}.FetchScore(context.Background(), uid)
	if s1.Score != s2.Score {
		t.Errorf("non-deterministic: %d vs %d", s1.Score, s2.Score)
	}
	if s1.Score < 300 || s1.Score > 900 {
		t.Errorf("score %d out of 300-900", s1.Score)
	}
	if s1.Band == "" || s1.Source != "mock" || s1.Bureau != "CIBIL" {
		t.Errorf("bad metadata: %+v", s1)
	}
	if len(s1.Factors) == 0 {
		t.Error("no factors")
	}
}

func TestService_Caches(t *testing.T) {
	svc := New(MockSource{}, time.Hour)
	uid := uuid.New()
	a, _ := svc.Get(context.Background(), uid)
	b, _ := svc.Get(context.Background(), uid)
	if a.GeneratedAt != b.GeneratedAt {
		t.Error("expected cached result (same GeneratedAt)")
	}
}

func TestBand(t *testing.T) {
	cases := map[int]string{820: "Excellent", 760: "Very Good", 710: "Good", 660: "Fair", 620: "Poor"}
	for score, want := range cases {
		if got := band(score); got != want {
			t.Errorf("band(%d) = %q, want %q", score, got, want)
		}
	}
}
