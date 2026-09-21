// Package events is an in-process pub/sub used to push "something changed,
// refetch" invalidation hints to open RM portal WebSocket connections. It is
// deliberately not a message queue: events are best-effort hints (a dropped
// event just means the RM's next event or a manual refresh catches them up),
// there is no persistence, and it only fans out within a single API
// instance. That's the right tradeoff here — the portal already re-fetches
// full state from Postgres on every invalidation, so the event itself only
// ever needs to say "look again," never carry the actual data.
package events

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Event types the RM portal listens for.
const (
	TypeProfileUpdated      = "profile_updated"
	TypeTransaction         = "transaction"
	TypeBudgetChanged       = "budget_changed"
	TypePortfolioChanged    = "portfolio_changed"
	TypeSubscriptionChanged = "subscription_changed"
	TypeBankAccountChanged  = "bank_account_changed"
)

// Event is the JSON frame pushed down an RM's WebSocket connection.
type Event struct {
	Type   string    `json:"type"`
	UserID uuid.UUID `json:"user_id,omitempty"`
	RMID   uuid.UUID `json:"rm_id,omitempty"`
	At     int64     `json:"at"`
}

// Hub is a topic-keyed pub/sub. Topics in use:
//
//	"user:<uuid>" — one specific client's Client360 detail view
//	"rm:<uuid>"   — an RM's own book (dashboard totals, client list)
//	"admin"       — every admin console session
type Hub struct {
	mu   sync.RWMutex
	subs map[string]map[chan Event]struct{}
}

func NewHub() *Hub {
	return &Hub{subs: make(map[string]map[chan Event]struct{})}
}

const TopicAdmin = "admin"

func TopicUser(userID uuid.UUID) string { return "user:" + userID.String() }
func TopicRM(rmID uuid.UUID) string     { return "rm:" + rmID.String() }

// Subscribe registers a new listener on topic and returns a receive-only
// channel plus a cancel func the caller must call exactly once (typically
// via defer) to unregister and release the channel.
func (h *Hub) Subscribe(topic string) (<-chan Event, func()) {
	ch := make(chan Event, 16)
	h.mu.Lock()
	if h.subs[topic] == nil {
		h.subs[topic] = make(map[chan Event]struct{})
	}
	h.subs[topic][ch] = struct{}{}
	h.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			h.mu.Lock()
			delete(h.subs[topic], ch)
			if len(h.subs[topic]) == 0 {
				delete(h.subs, topic)
			}
			h.mu.Unlock()
			close(ch)
		})
	}
	return ch, cancel
}

// Publish fans ev out to every current subscriber of topic. A subscriber
// whose channel is full is skipped rather than blocked on — a slow RM
// connection must never stall the request that triggered the event.
func (h *Hub) Publish(topic string, ev Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subs[topic] {
		select {
		case ch <- ev:
		default:
		}
	}
}

// OwnerLookup resolves which RM (if any) currently owns userID, so a data
// change can also be pushed to that RM's book-level topic. Satisfied by
// repository.AssignmentRepository.OwnerOf.
type OwnerLookup func(ctx context.Context, userID uuid.UUID) (*uuid.UUID, bool, error)

// Publisher is the handler-facing entry point: one call fans a user's data
// change out to that client's detail view, their assigned RM's book, and
// every admin session, without each call site needing to know the topic
// scheme or look up RM ownership itself.
type Publisher struct {
	hub    *Hub
	lookup OwnerLookup
}

func NewPublisher(hub *Hub, lookup OwnerLookup) *Publisher {
	return &Publisher{hub: hub, lookup: lookup}
}

// UserChanged publishes eventType for userID. Safe to call fire-and-forget
// (e.g. `go pub.UserChanged(...)`) from a request handler after a mutation
// has already committed — never call it before the write is durable, or an
// RM's refetch could race the transaction and see stale data.
func (p *Publisher) UserChanged(ctx context.Context, userID uuid.UUID, eventType string) {
	if p == nil || p.hub == nil {
		return
	}
	ev := Event{Type: eventType, UserID: userID, At: time.Now().Unix()}
	p.hub.Publish(TopicUser(userID), ev)
	p.hub.Publish(TopicAdmin, ev)

	if p.lookup == nil {
		return
	}
	rmID, found, err := p.lookup(ctx, userID)
	if err != nil || !found || rmID == nil {
		return
	}
	rmEv := ev
	rmEv.RMID = *rmID
	p.hub.Publish(TopicRM(*rmID), rmEv)
}
