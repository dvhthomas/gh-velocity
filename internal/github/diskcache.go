package github

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/log"
)

// cacheVersion is the cache directory version. Bump this when the cache
// format changes to avoid deserialization errors from stale entries.
const cacheVersion = "v1"

// DiskCache provides a filesystem-backed query cache with short TTL.
// It is an internal optimization — the on-disk format is opaque and may
// change between versions without notice. Users should never rely on
// cached data directly.
//
// Concurrent processes are safe: writes use atomic rename (write to temp
// file, then os.Rename). "Last writer wins" semantics — no file locking.
type DiskCache struct {
	dir string
	ttl time.Duration
	now func() time.Time // injectable clock for testing
}

// diskEntry wraps cached data with metadata for type-safe deserialization.
type diskEntry struct {
	Type      string          `json:"type"` // cache key prefix (e.g., "search-issues")
	CreatedAt time.Time       `json:"created_at"`
	Data      json.RawMessage `json:"data"`
}

// NewDiskCache creates a disk cache in the given directory with the specified TTL.
// The directory is created if it does not exist.
// Returns nil if the directory cannot be created.
func NewDiskCache(baseDir string, ttl time.Duration) *DiskCache {
	dir := filepath.Join(baseDir, cacheVersion)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Debug("disk cache disabled: cannot create %s: %v", dir, err)
		return nil
	}
	return &DiskCache{
		dir: dir,
		ttl: ttl,
		now: time.Now,
	}
}

// DefaultCacheDir returns the platform-appropriate cache directory for gh-velocity.
// Uses os.UserCacheDir() which returns:
//   - macOS: ~/Library/Caches/gh-velocity
//   - Linux: $XDG_CACHE_HOME/gh-velocity or ~/.cache/gh-velocity
//   - Windows: %LocalAppData%/gh-velocity
func DefaultCacheDir() string {
	base, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "gh-velocity")
}

// Get reads a cached value from disk. Returns the raw JSON data, the type
// prefix, and whether the entry was found and valid (not expired).
// Expired entries are lazily deleted.
func (dc *DiskCache) Get(key string) (json.RawMessage, string, bool) {
	path := dc.path(key)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", false // not found or unreadable
	}

	var entry diskEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		// Corrupt entry — delete and treat as miss.
		os.Remove(path)
		return nil, "", false
	}

	if dc.now().Sub(entry.CreatedAt) > dc.ttl {
		// Expired — lazy delete.
		os.Remove(path)
		return nil, "", false
	}

	return entry.Data, entry.Type, true
}

// Set writes a value to disk atomically. The value is wrapped with type
// metadata for safe deserialization. Uses write-to-temp + rename for
// atomic writes (safe for concurrent processes).
func (dc *DiskCache) Set(key, typeName string, data json.RawMessage) error {
	entry := diskEntry{
		Type:      typeName,
		CreatedAt: dc.now(),
		Data:      data,
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		return err
	}

	// Atomic write: temp file + rename.
	tmp, err := os.CreateTemp(dc.dir, "cache-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}

	return os.Rename(tmpPath, dc.path(key))
}

// path returns the filesystem path for a cache key.
func (dc *DiskCache) path(key string) string {
	return filepath.Join(dc.dir, key+".json")
}
