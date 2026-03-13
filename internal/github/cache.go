package github

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// QueryCache deduplicates identical GitHub API calls within a single CLI
// invocation. It uses singleflight to coalesce concurrent requests for the
// same key (e.g., when report's 3 pipelines all search for the same issues).
//
// Cache is per-process, in-memory only. TTL is a safety net — CLI processes
// complete in seconds, so expiry is effectively irrelevant.
type QueryCache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
	ttl     time.Duration
	now     func() time.Time // injectable clock for testing; defaults to time.Now
	flight  singleflight.Group
}

type cacheEntry struct {
	value     any
	createdAt time.Time
}

// NewQueryCache creates a cache with the given TTL.
func NewQueryCache(ttl time.Duration) *QueryCache {
	return &QueryCache{
		entries: make(map[string]cacheEntry),
		ttl:     ttl,
		now:     time.Now,
	}
}

// Get returns a cached value if present and not expired.
func (c *QueryCache) Get(key string) (any, bool) {
	c.mu.RLock()
	e, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if c.now().Sub(e.createdAt) > c.ttl {
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		return nil, false
	}
	return e.value, true
}

// Set stores a value in the cache.
func (c *QueryCache) Set(key string, value any) {
	c.mu.Lock()
	c.entries[key] = cacheEntry{value: value, createdAt: c.now()}
	c.mu.Unlock()
}

// Do executes fn at most once for a given key concurrently.
// If multiple goroutines call Do with the same key, only the first executes fn;
// the others block and receive the same result. Successful results are cached.
func (c *QueryCache) Do(key string, fn func() (any, error)) (any, error) {
	// Fast path: check cache.
	if v, ok := c.Get(key); ok {
		return v, nil
	}

	// Singleflight: coalesce concurrent calls for the same key.
	v, err, _ := c.flight.Do(key, func() (any, error) {
		// Double-check cache after winning the singleflight race.
		if v, ok := c.Get(key); ok {
			return v, nil
		}
		result, err := fn()
		if err != nil {
			return nil, err
		}
		c.Set(key, result)
		return result, nil
	})
	return v, err
}

// CacheKey builds a cache key by hashing the given parts.
func CacheKey(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0}) // null separator to avoid collisions
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}
