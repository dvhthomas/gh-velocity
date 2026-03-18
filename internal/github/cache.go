package github

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/log"
	"golang.org/x/sync/singleflight"
)

// QueryCache deduplicates identical GitHub API calls within a single CLI
// invocation and optionally persists results to a DiskCache for cross-
// invocation reuse.
//
// Read path: in-memory → disk → API call → write to both.
// The in-memory layer uses singleflight to coalesce concurrent requests
// for the same key (e.g., when report's 3 pipelines all search for the
// same issues).
type QueryCache struct {
	mu      sync.RWMutex
	entries map[string]cacheEntry
	ttl     time.Duration
	now     func() time.Time // injectable clock for testing; defaults to time.Now
	flight  singleflight.Group
	disk    *DiskCache // optional; nil when --no-cache or unavailable
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
// the others block and receive the same result. Successful results are cached
// in memory only (no disk persistence). Use DoJSON for disk-backed caching.
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

// DoJSON is like Do but also checks/populates the disk cache.
// The typeName parameter identifies the cached data type (e.g., "search-issues")
// and is stored alongside the data for debugging. The unmarshal function
// deserializes disk-cached JSON back into the typed Go value.
//
// Read path: in-memory → disk → fn() → write to memory + disk.
func (c *QueryCache) DoJSON(key, typeName string, fn func() (any, error), unmarshal func(json.RawMessage) (any, error)) (any, error) {
	// Fast path: in-memory cache.
	if v, ok := c.Get(key); ok {
		log.Debug("cache hit (memory): %s key=%s", typeName, key[:min(8, len(key))])
		return v, nil
	}

	// Singleflight: coalesce concurrent calls.
	v, err, _ := c.flight.Do(key, func() (any, error) {
		// Double-check memory after winning singleflight.
		if v, ok := c.Get(key); ok {
			log.Debug("cache hit (memory): %s key=%s", typeName, key[:min(8, len(key))])
			return v, nil
		}

		// Check disk cache.
		if c.disk != nil {
			if raw, _, ok := c.disk.Get(key); ok {
				v, err := unmarshal(raw)
				if err == nil {
					log.Debug("cache hit (disk): %s key=%s", typeName, key[:min(8, len(key))])
					c.Set(key, v) // promote to memory
					return v, nil
				}
				// Deserialization failed — treat as miss, continue to API.
			}
		}

		// Cache miss — execute the function.
		result, err := fn()
		if err != nil {
			return nil, err
		}

		// Store in memory.
		c.Set(key, result)

		// Store on disk (best-effort, don't fail the call).
		if c.disk != nil {
			if raw, jsonErr := json.Marshal(result); jsonErr == nil {
				if diskErr := c.disk.Set(key, typeName, raw); diskErr != nil {
					log.Debug("disk cache write failed: %v", diskErr)
				}
			}
		}

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
