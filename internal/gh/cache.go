package gh

import (
	"sync"
	"time"
)

// cache is a minimal in-memory TTL cache keyed by request URL. It is safe
// for concurrent use since fetches run as tea.Cmd goroutines.
type cache struct {
	mu      sync.Mutex
	ttl     time.Duration
	entries map[string]cacheEntry
}

type cacheEntry struct {
	value   interface{}
	expires time.Time
}

func newCache(ttl time.Duration) *cache {
	return &cache{ttl: ttl, entries: make(map[string]cacheEntry)}
}

// get returns the cached value for key if present and not expired.
func (c *cache) get(key string) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok || time.Now().After(e.expires) {
		return nil, false
	}
	return e.value, true
}

func (c *cache) set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = cacheEntry{value: value, expires: time.Now().Add(c.ttl)}
}

// bust removes key from the cache, forcing the next fetch to hit the
// network. Used by the "r" refresh binding.
func (c *cache) bust(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}
