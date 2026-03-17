package github

import (
	"context"
	"testing"
	"time"
)

func TestThrottleSearch_RespectsRateLimitUntil(t *testing.T) {
	c := &Client{
		searchDelay: 100 * time.Millisecond,
	}

	// Simulate a secondary rate limit detected by another goroutine:
	// set rateLimitUntil to 200ms from now.
	c.searchMu.Lock()
	c.rateLimitUntil = time.Now().Add(200 * time.Millisecond)
	c.searchMu.Unlock()

	start := time.Now()
	if err := c.throttleSearch(context.Background()); err != nil {
		t.Fatalf("throttleSearch returned error: %v", err)
	}
	elapsed := time.Since(start)

	// Should have waited at least 150ms (200ms minus scheduling jitter).
	if elapsed < 150*time.Millisecond {
		t.Errorf("throttleSearch returned too quickly: %s (expected ≥150ms)", elapsed)
	}
}

func TestThrottleSearch_ExpiredRateLimitUntilIsIgnored(t *testing.T) {
	c := &Client{
		searchDelay: 0, // no throttle delay
	}

	// Set rateLimitUntil in the past — should be ignored.
	c.searchMu.Lock()
	c.rateLimitUntil = time.Now().Add(-1 * time.Second)
	c.searchMu.Unlock()

	start := time.Now()
	if err := c.throttleSearch(context.Background()); err != nil {
		t.Fatalf("throttleSearch returned error: %v", err)
	}
	elapsed := time.Since(start)

	// Should return immediately (< 50ms).
	if elapsed > 50*time.Millisecond {
		t.Errorf("throttleSearch waited too long for expired rateLimitUntil: %s", elapsed)
	}
}

func TestThrottleSearch_RateLimitUntilTakesPrecedenceOverSearchDelay(t *testing.T) {
	c := &Client{
		searchDelay:    50 * time.Millisecond,
		searchLastCall: time.Now(), // just called — searchDelay would normally apply
	}

	// rateLimitUntil is further out than searchDelay — should use the longer wait.
	c.searchMu.Lock()
	c.rateLimitUntil = time.Now().Add(200 * time.Millisecond)
	c.searchMu.Unlock()

	start := time.Now()
	if err := c.throttleSearch(context.Background()); err != nil {
		t.Fatalf("throttleSearch returned error: %v", err)
	}
	elapsed := time.Since(start)

	if elapsed < 150*time.Millisecond {
		t.Errorf("should wait for rateLimitUntil (200ms), not searchDelay (50ms): waited %s", elapsed)
	}
}

func TestSetRateLimitPause(t *testing.T) {
	c := &Client{}

	before := time.Now()
	c.setRateLimitPause(60 * time.Second)

	c.searchMu.Lock()
	until := c.rateLimitUntil
	c.searchMu.Unlock()

	if until.Before(before.Add(59 * time.Second)) {
		t.Errorf("rateLimitUntil too early: %v (expected ≥ %v)", until, before.Add(59*time.Second))
	}
}
