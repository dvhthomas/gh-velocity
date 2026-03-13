package github

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestQueryCache_SetGet(t *testing.T) {
	c := NewQueryCache(time.Minute)

	// Miss on empty cache.
	if _, ok := c.Get("k1"); ok {
		t.Fatal("expected miss on empty cache")
	}

	// Set and hit.
	c.Set("k1", "hello")
	v, ok := c.Get("k1")
	if !ok {
		t.Fatal("expected hit after Set")
	}
	if v.(string) != "hello" {
		t.Fatalf("got %v, want hello", v)
	}

	// Different key is still a miss.
	if _, ok := c.Get("k2"); ok {
		t.Fatal("expected miss for different key")
	}
}

func TestQueryCache_TTLExpiry(t *testing.T) {
	now := time.Now()
	c := NewQueryCache(5 * time.Minute)
	c.now = func() time.Time { return now }

	c.Set("k1", "val")
	if _, ok := c.Get("k1"); !ok {
		t.Fatal("expected hit before TTL")
	}

	// Advance clock past TTL without sleeping.
	now = now.Add(6 * time.Minute)

	if _, ok := c.Get("k1"); ok {
		t.Fatal("expected miss after TTL expiry")
	}
}

func TestQueryCache_Do_CachesResult(t *testing.T) {
	c := NewQueryCache(time.Minute)
	var calls atomic.Int32

	fn := func() (any, error) {
		calls.Add(1)
		return "result", nil
	}

	// First call executes fn.
	v, err := c.Do("k1", fn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.(string) != "result" {
		t.Fatalf("got %v, want result", v)
	}
	if calls.Load() != 1 {
		t.Fatalf("fn called %d times, want 1", calls.Load())
	}

	// Second call returns cached result.
	v, err = c.Do("k1", fn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.(string) != "result" {
		t.Fatalf("got %v, want result", v)
	}
	if calls.Load() != 1 {
		t.Fatalf("fn called %d times, want 1 (should be cached)", calls.Load())
	}
}

func TestQueryCache_Do_DoesNotCacheErrors(t *testing.T) {
	c := NewQueryCache(time.Minute)
	var calls atomic.Int32

	fn := func() (any, error) {
		calls.Add(1)
		return nil, errors.New("fail")
	}

	// First call returns error.
	_, err := c.Do("k1", fn)
	if err == nil {
		t.Fatal("expected error")
	}

	// Second call retries (error was not cached).
	_, err = c.Do("k1", fn)
	if err == nil {
		t.Fatal("expected error on retry")
	}
	if calls.Load() != 2 {
		t.Fatalf("fn called %d times, want 2 (errors should not be cached)", calls.Load())
	}
}

func TestQueryCache_Do_Singleflight(t *testing.T) {
	c := NewQueryCache(time.Minute)
	var calls atomic.Int32

	// Slow function to ensure concurrent calls overlap.
	fn := func() (any, error) {
		calls.Add(1)
		time.Sleep(50 * time.Millisecond)
		return "shared", nil
	}

	// Launch 5 concurrent calls with the same key.
	var wg sync.WaitGroup
	results := make([]any, 5)
	errs := make([]error, 5)
	for i := range 5 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = c.Do("k1", fn)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d error: %v", i, err)
		}
		if results[i].(string) != "shared" {
			t.Fatalf("goroutine %d got %v, want shared", i, results[i])
		}
	}

	// singleflight should coalesce to 1 call.
	if calls.Load() != 1 {
		t.Fatalf("fn called %d times, want 1 (singleflight should coalesce)", calls.Load())
	}
}

func TestQueryCache_ConcurrentAccess(t *testing.T) {
	c := NewQueryCache(time.Minute)

	// Hammer the cache from many goroutines to trigger race detector.
	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := CacheKey("method", string(rune('a'+n%10)))
			c.Set(key, n)
			c.Get(key)
		}(i)
	}
	wg.Wait()
}

func TestCacheKey_Deterministic(t *testing.T) {
	k1 := CacheKey("search-issues", "repo:foo/bar is:issue")
	k2 := CacheKey("search-issues", "repo:foo/bar is:issue")
	if k1 != k2 {
		t.Fatalf("same inputs produced different keys: %s vs %s", k1, k2)
	}
}

func TestCacheKey_DifferentInputs(t *testing.T) {
	k1 := CacheKey("search-issues", "repo:foo/bar")
	k2 := CacheKey("search-prs", "repo:foo/bar")
	if k1 == k2 {
		t.Fatalf("different methods produced same key: %s", k1)
	}
}

func TestCacheKey_NullSeparator(t *testing.T) {
	// "ab" + "" vs "a" + "b" should differ due to null separators.
	k1 := CacheKey("ab", "")
	k2 := CacheKey("a", "b")
	if k1 == k2 {
		t.Fatal("null separator failed to disambiguate")
	}
}
