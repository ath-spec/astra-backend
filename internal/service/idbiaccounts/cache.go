package idbiaccounts

import (
	"sync"
	"time"
)

// ttlCache is a tiny in-process value cache with per-entry expiry — the same
// shape as service/budget's ttlCache, but storing decoded values (any) rather
// than JSON bytes since this service caches its own []Account.
type ttlEntry struct {
	val any
	exp time.Time
}

type ttlCache struct {
	mu sync.Mutex
	m  map[string]ttlEntry
}

func newTTLCache() *ttlCache { return &ttlCache{m: map[string]ttlEntry{}} }

func (c *ttlCache) get(k string) (any, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[k]
	if !ok || time.Now().After(e.exp) {
		delete(c.m, k)
		return nil, false
	}
	return e.val, true
}

func (c *ttlCache) set(k string, v any, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[k] = ttlEntry{val: v, exp: time.Now().Add(ttl)}
}

func (c *ttlCache) delete(k string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.m, k)
}
