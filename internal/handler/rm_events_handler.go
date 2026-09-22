package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/yourusername/astra-backend/internal/events"
	"github.com/yourusername/astra-backend/internal/middleware"
	"github.com/yourusername/astra-backend/internal/repository"
)

// rmEventsWriteWait bounds a single frame write so one stalled TCP
// connection can't leak a goroutine forever.
const rmEventsWriteWait = 10 * time.Second

// rmEventsPingInterval keeps the connection alive through idle proxies
// (Cloudflare/nginx) that silently drop quiet WebSocket connections.
const rmEventsPingInterval = 25 * time.Second

// RMEventsHandler streams live "something changed, refetch" invalidation
// events to the RM portal over a single long-lived WebSocket per session —
// the event-driven push layer the RM portal previously had none of (every
// tab only ever fetched once, on mount).
type RMEventsHandler struct {
	hub    *events.Hub
	assign repository.AssignmentRepository
}

func NewRMEventsHandler(hub *events.Hub, assign repository.AssignmentRepository) *RMEventsHandler {
	return &RMEventsHandler{hub: hub, assign: assign}
}

// clientControlMsg is the small control protocol the portal sends over the
// same socket to scope which specific client's Client360 view it currently
// has open — pushing every client change to every RM connection would work,
// but scales badly once a book has hundreds of clients and wastes bandwidth
// on views the RM isn't even looking at.
type clientControlMsg struct {
	Subscribe   string `json:"subscribe,omitempty"`
	Unsubscribe string `json:"unsubscribe,omitempty"`
}

func (h *RMEventsHandler) Stream(w http.ResponseWriter, r *http.Request) {
	rmID, ok := middleware.GetRMID(r.Context())
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	isAdmin := middleware.IsAdmin(r.Context())

	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// Every RM session auto-subscribes to their own book (or, for an admin,
	// every book) — no client-side opt-in needed for dashboard/list-level
	// totals to stay live.
	ownTopic := events.TopicRM(rmID)
	if isAdmin {
		ownTopic = events.TopicAdmin
	}
	ownCh, cancelOwn := h.hub.Subscribe(ownTopic)
	defer cancelOwn()

	// Per-client detail subscriptions the portal adds/removes as the RM
	// navigates Client360 tabs. Each one gets its own forwarding goroutine
	// (fanned into out) so Stream's select loop only ever reads one place.
	clientSubs := make(map[string]func())
	defer func() {
		for _, cancel := range clientSubs {
			cancel()
		}
	}()
	out := make(chan events.Event, 32)
	fwdDone := make(chan struct{})
	defer close(fwdDone)

	readErr := make(chan struct{})
	go func() {
		defer close(readErr)
		for {
			_, raw, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var msg clientControlMsg
			if err := json.Unmarshal(raw, &msg); err != nil {
				continue
			}
			if msg.Subscribe != "" {
				h.addClientSub(r.Context(), rmID, isAdmin, msg.Subscribe, clientSubs, out, fwdDone)
			}
			if msg.Unsubscribe != "" {
				if cancel, found := clientSubs[msg.Unsubscribe]; found {
					cancel()
					delete(clientSubs, msg.Unsubscribe)
				}
			}
		}
	}()

	ticker := time.NewTicker(rmEventsPingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-readErr:
			return
		case ev, ok := <-ownCh:
			if !ok {
				return
			}
			if err := h.write(conn, ev); err != nil {
				return
			}
		case ev := <-out:
			if err := h.write(conn, ev); err != nil {
				return
			}
		case <-ticker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(rmEventsWriteWait))
			if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"ping"}`)); err != nil {
				return
			}
		}
	}
}

// addClientSub subscribes to one client's topic after confirming the
// requesting RM actually owns that client (admins may subscribe to anyone)
// — without this check, any authenticated RM could snoop on another RM's
// client just by sending a subscribe frame with a guessed user ID.
func (h *RMEventsHandler) addClientSub(
	ctx context.Context,
	rmID uuid.UUID,
	isAdmin bool,
	userIDStr string,
	subs map[string]func(),
	out chan<- events.Event,
	streamDone <-chan struct{},
) {
	if _, found := subs[userIDStr]; found {
		return // already subscribed
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return
	}
	if !isAdmin {
		owner, found, err := h.assign.OwnerOf(ctx, userID)
		if err != nil || !found || owner == nil || *owner != rmID {
			return
		}
	}
	ch, cancel := h.hub.Subscribe(events.TopicUser(userID))
	subs[userIDStr] = cancel
	go func() {
		for {
			select {
			case ev, ok := <-ch:
				if !ok {
					return
				}
				select {
				case out <- ev:
				default:
					slog.Warn("rm events: dropped event, subscriber channel full", "user_id", userIDStr)
				}
			case <-streamDone:
				return
			}
		}
	}()
}

func (h *RMEventsHandler) write(conn *websocket.Conn, ev events.Event) error {
	_ = conn.SetWriteDeadline(time.Now().Add(rmEventsWriteWait))
	body, err := json.Marshal(ev)
	if err != nil {
		return nil
	}
	return conn.WriteMessage(websocket.TextMessage, body)
}
