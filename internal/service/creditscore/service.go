// Package creditscore is IDBI integration feature 8: a credit-score service.
//
// The real source (408 fetchCibilScore) is DEAD in the sandbox — the bureau
// credentials in the shipped sample are rejected and no score is returned
// (idbi.ErrNotAvailable). Until ACC supplies working credentials this service
// returns a deterministic MOCK score per user so the "credit health" screen
// has something real-shaped to render. Swap Source for the live 408 parse when
// it works; the handler and response shape stay the same.
package creditscore

import (
	"context"
	"errors"
	"hash/fnv"
	"time"

	"github.com/google/uuid"

	"github.com/yourusername/astra-backend/internal/provider/idbi"
)

// Score is the outward shape (modelled on a CIBIL report summary).
type Score struct {
	Score       int       `json:"score"`   // 300-900
	Band        string    `json:"band"`    // Poor / Fair / Good / Very Good / Excellent
	Bureau      string    `json:"bureau"`  // "CIBIL"
	Source      string    `json:"source"`  // "mock" | "idbi"
	Factors     []string  `json:"factors"` // plain-language drivers
	GeneratedAt time.Time `json:"generated_at"`
}

// Source is the swappable score provider.
type Source interface {
	FetchScore(ctx context.Context, userID uuid.UUID) (Score, error)
}

// Service is a thin cache + passthrough over a Source.
type Service struct {
	src Source
	ttl time.Duration
	// per-user cache
	cache map[uuid.UUID]cacheEntry
}

type cacheEntry struct {
	s   Score
	exp time.Time
}

func New(src Source, ttl time.Duration) *Service {
	if ttl == 0 {
		ttl = 24 * time.Hour
	}
	return &Service{src: src, ttl: ttl, cache: map[uuid.UUID]cacheEntry{}}
}

func (s *Service) Get(ctx context.Context, userID uuid.UUID) (Score, error) {
	if e, ok := s.cache[userID]; ok && time.Now().Before(e.exp) {
		return e.s, nil
	}
	sc, err := s.src.FetchScore(ctx, userID)
	if err != nil {
		return Score{}, err
	}
	s.cache[userID] = cacheEntry{s: sc, exp: time.Now().Add(s.ttl)}
	return sc, nil
}

// --- Mock source -------------------------------------------------------

// MockSource returns a stable, plausible score derived from the user id.
type MockSource struct{}

func (MockSource) FetchScore(_ context.Context, userID uuid.UUID) (Score, error) {
	h := fnv.New32a()
	_, _ = h.Write(userID[:])
	// map hash into 640-820, a believable band
	score := 640 + int(h.Sum32()%181)
	return Score{
		Score:       score,
		Band:        band(score),
		Bureau:      "CIBIL",
		Source:      "mock",
		Factors:     factorsFor(score),
		GeneratedAt: time.Now().UTC(),
	}, nil
}

// --- Live source (408) ----------------------------------------------

// IDBISource wraps the real 408 call. It currently always returns
// ErrNotAvailable (see package doc); kept so the swap is a one-liner in main.
type IDBISource struct {
	Client interface {
		FetchCibilScore(ctx context.Context, req idbi.FetchCibilScoreRequest) (any, error)
	}
}

func (s IDBISource) FetchScore(ctx context.Context, _ uuid.UUID) (Score, error) {
	_, err := s.Client.FetchCibilScore(ctx, idbi.FetchCibilScoreRequest{})
	if err != nil {
		return Score{}, err // idbi.ErrNotAvailable today
	}
	return Score{}, errors.New("creditscore: live 408 parse not implemented")
}

func band(score int) string {
	switch {
	case score >= 800:
		return "Excellent"
	case score >= 750:
		return "Very Good"
	case score >= 700:
		return "Good"
	case score >= 650:
		return "Fair"
	default:
		return "Poor"
	}
}

func factorsFor(score int) []string {
	if score >= 750 {
		return []string{
			"On-time repayment history",
			"Low credit utilisation",
			"Healthy mix of secured and unsecured credit",
		}
	}
	if score >= 700 {
		return []string{
			"Mostly on-time repayments",
			"Moderate credit utilisation",
			"A few recent credit enquiries",
		}
	}
	return []string{
		"One or more late payments in the last 12 months",
		"High credit utilisation",
		"Short credit history",
	}
}
