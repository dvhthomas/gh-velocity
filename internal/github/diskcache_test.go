package github

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDiskCache_SetAndGet(t *testing.T) {
	dir := t.TempDir()
	dc := NewDiskCache(dir, 5*time.Minute)
	if dc == nil {
		t.Fatal("expected non-nil DiskCache")
	}

	data := json.RawMessage(`["hello","world"]`)
	if err := dc.Set("testkey", "search-issues", data); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, typeName, ok := dc.Get("testkey")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if typeName != "search-issues" {
		t.Errorf("type = %q, want %q", typeName, "search-issues")
	}
	if string(got) != string(data) {
		t.Errorf("data = %s, want %s", got, data)
	}
}

func TestDiskCache_Expiry(t *testing.T) {
	dir := t.TempDir()
	dc := NewDiskCache(dir, 5*time.Minute)
	now := time.Now()
	dc.now = func() time.Time { return now }

	data := json.RawMessage(`{"count":1}`)
	if err := dc.Set("expkey", "test", data); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Still valid at now + 4 min.
	dc.now = func() time.Time { return now.Add(4 * time.Minute) }
	_, _, ok := dc.Get("expkey")
	if !ok {
		t.Error("expected cache hit within TTL")
	}

	// Expired at now + 6 min.
	dc.now = func() time.Time { return now.Add(6 * time.Minute) }
	_, _, ok = dc.Get("expkey")
	if ok {
		t.Error("expected cache miss after TTL")
	}

	// File should be lazily deleted.
	path := dc.path("expkey")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("expected file to be deleted after expiry")
	}
}

func TestDiskCache_CorruptFile(t *testing.T) {
	dir := t.TempDir()
	dc := NewDiskCache(dir, 5*time.Minute)

	// Write garbage to a cache file.
	path := dc.path("corrupt")
	os.WriteFile(path, []byte("not json"), 0o644)

	_, _, ok := dc.Get("corrupt")
	if ok {
		t.Error("expected cache miss for corrupt entry")
	}

	// File should be deleted.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("expected corrupt file to be deleted")
	}
}

func TestDiskCache_MissingKey(t *testing.T) {
	dir := t.TempDir()
	dc := NewDiskCache(dir, 5*time.Minute)

	_, _, ok := dc.Get("nonexistent")
	if ok {
		t.Error("expected cache miss for nonexistent key")
	}
}

func TestDiskCache_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	dc := NewDiskCache(dir, 5*time.Minute)

	data := json.RawMessage(`{"test":true}`)
	if err := dc.Set("atomic", "test", data); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Verify no temp files remain.
	entries, _ := os.ReadDir(filepath.Join(dir, cacheVersion))
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("temp file not cleaned up: %s", e.Name())
		}
	}
}

func TestDiskCache_VersionedDir(t *testing.T) {
	dir := t.TempDir()
	dc := NewDiskCache(dir, 5*time.Minute)

	expected := filepath.Join(dir, cacheVersion)
	if dc.dir != expected {
		t.Errorf("dir = %q, want %q", dc.dir, expected)
	}
}

func TestDefaultCacheDir(t *testing.T) {
	dir := DefaultCacheDir()
	if dir == "" {
		t.Skip("os.UserCacheDir() not available on this platform")
	}
	if !filepath.IsAbs(dir) {
		t.Errorf("expected absolute path, got %q", dir)
	}
	if filepath.Base(dir) != "gh-velocity" {
		t.Errorf("expected dir ending in gh-velocity, got %q", dir)
	}
}
