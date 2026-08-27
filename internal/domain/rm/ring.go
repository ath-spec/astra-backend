package rm

import "github.com/google/uuid"

// Slot is one active RM's position in the assignment ring: their identity,
// client capacity, and how many clients they currently hold.
type Slot struct {
	RMID          uuid.UUID
	MaxPortfolios int
	ClientCount   int
}

func (s Slot) hasCapacity() bool { return s.ClientCount < s.MaxPortfolios }

// PickNextRM implements capacity-aware round-robin book allocation, the
// standard wealth-management pattern: walk the active desk starting just
// after whoever was assigned last, and hand the client to the first RM with
// spare capacity. If the whole desk is at capacity, fall back to the
// globally least-loaded RM (overflow=true) so no client is ever left
// unrouted — the admin console surfaces those for rebalancing.
//
// ring must be the active RMs in a stable order (by created_at). afterID is
// rm_queue_state.last_assigned_rm_id (uuid.Nil on first ever assignment or
// if that RM has since been deactivated). ok is false only when ring is
// empty.
func PickNextRM(ring []Slot, afterID uuid.UUID) (chosen uuid.UUID, overflow bool, ok bool) {
	if len(ring) == 0 {
		return uuid.Nil, false, false
	}

	// Start index: the position right after afterID in the ring, or 0 if
	// afterID isn't present any more.
	start := 0
	for i, s := range ring {
		if s.RMID == afterID {
			start = (i + 1) % len(ring)
			break
		}
	}

	// One full lap from start looking for spare capacity.
	for off := 0; off < len(ring); off++ {
		s := ring[(start+off)%len(ring)]
		if s.hasCapacity() {
			return s.RMID, false, true
		}
	}

	// Whole desk full: least-loaded wins, ties broken by ring order.
	least := ring[0]
	for _, s := range ring[1:] {
		if s.ClientCount < least.ClientCount {
			least = s
		}
	}
	return least.RMID, true, true
}
