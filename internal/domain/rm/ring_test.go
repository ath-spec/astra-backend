package rm

import (
	"testing"

	"github.com/google/uuid"
)

func TestPickNextRM(t *testing.T) {
	a := uuid.MustParse("00000000-0000-0000-0000-0000000000a1")
	b := uuid.MustParse("00000000-0000-0000-0000-0000000000b2")
	c := uuid.MustParse("00000000-0000-0000-0000-0000000000c3")

	slot := func(id uuid.UUID, count, max int) Slot {
		return Slot{RMID: id, ClientCount: count, MaxPortfolios: max}
	}

	tests := []struct {
		name         string
		ring         []Slot
		after        uuid.UUID
		wantChosen   uuid.UUID
		wantOverflow bool
		wantOK       bool
	}{
		{
			name:   "empty ring",
			ring:   nil,
			after:  uuid.Nil,
			wantOK: false,
		},
		{
			name:       "single rm with capacity",
			ring:       []Slot{slot(a, 5, 150)},
			after:      uuid.Nil,
			wantChosen: a,
			wantOK:     true,
		},
		{
			name:       "first ever assignment starts at ring head",
			ring:       []Slot{slot(a, 0, 150), slot(b, 0, 150)},
			after:      uuid.Nil,
			wantChosen: a,
			wantOK:     true,
		},
		{
			name:       "advances past last assigned",
			ring:       []Slot{slot(a, 1, 150), slot(b, 1, 150), slot(c, 1, 150)},
			after:      a,
			wantChosen: b,
			wantOK:     true,
		},
		{
			name:       "wraps around the ring",
			ring:       []Slot{slot(a, 1, 150), slot(b, 1, 150), slot(c, 1, 150)},
			after:      c,
			wantChosen: a,
			wantOK:     true,
		},
		{
			name:       "skips rm at capacity",
			ring:       []Slot{slot(a, 1, 150), slot(b, 150, 150), slot(c, 1, 150)},
			after:      a,
			wantChosen: c,
			wantOK:     true,
		},
		{
			name:       "stale after id not in ring falls back to head",
			ring:       []Slot{slot(a, 1, 150), slot(b, 1, 150)},
			after:      uuid.MustParse("00000000-0000-0000-0000-0000000000ff"),
			wantChosen: a,
			wantOK:     true,
		},
		{
			name:         "whole desk full -> least loaded, overflow",
			ring:         []Slot{slot(a, 200, 150), slot(b, 160, 150), slot(c, 175, 150)},
			after:        a,
			wantChosen:   b,
			wantOverflow: true,
			wantOK:       true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			chosen, overflow, ok := PickNextRM(tc.ring, tc.after)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !tc.wantOK {
				return
			}
			if chosen != tc.wantChosen {
				t.Errorf("chosen = %v, want %v", chosen, tc.wantChosen)
			}
			if overflow != tc.wantOverflow {
				t.Errorf("overflow = %v, want %v", overflow, tc.wantOverflow)
			}
		})
	}
}

// Six sequential signups across three equal-capacity RMs must land 2/2/2.
func TestPickNextRM_RoundRobinEven(t *testing.T) {
	ids := []uuid.UUID{
		uuid.MustParse("00000000-0000-0000-0000-0000000000a1"),
		uuid.MustParse("00000000-0000-0000-0000-0000000000b2"),
		uuid.MustParse("00000000-0000-0000-0000-0000000000c3"),
	}
	counts := map[uuid.UUID]int{}
	after := uuid.Nil
	for i := 0; i < 6; i++ {
		ring := make([]Slot, len(ids))
		for j, id := range ids {
			ring[j] = Slot{RMID: id, ClientCount: counts[id], MaxPortfolios: 150}
		}
		chosen, _, ok := PickNextRM(ring, after)
		if !ok {
			t.Fatal("expected a pick")
		}
		counts[chosen]++
		after = chosen
	}
	for _, id := range ids {
		if counts[id] != 2 {
			t.Errorf("rm %v got %d clients, want 2", id, counts[id])
		}
	}
}
