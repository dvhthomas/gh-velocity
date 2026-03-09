package github

import (
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	ghapi "github.com/cli/go-gh/v2/pkg/api"
)

func TestRateLimitWait_HTTP429(t *testing.T) {
	err := &ghapi.HTTPError{
		StatusCode: http.StatusTooManyRequests,
		Headers:    http.Header{},
	}
	wrapped := fmt.Errorf("get issue #1: %w", err)

	dur, isRL := rateLimitWait(wrapped)
	if !isRL {
		t.Fatal("expected rate limit detection for 429")
	}
	if dur != 5*time.Second {
		t.Errorf("expected 5s fallback, got %s", dur)
	}
}

func TestRateLimitWait_HTTP403_RemainingZero(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-RateLimit-Remaining", "0")

	err := &ghapi.HTTPError{
		StatusCode: http.StatusForbidden,
		Headers:    headers,
	}

	dur, isRL := rateLimitWait(err)
	if !isRL {
		t.Fatal("expected rate limit detection for 403 with remaining=0")
	}
	if dur != 5*time.Second {
		t.Errorf("expected 5s fallback, got %s", dur)
	}
}

func TestRateLimitWait_HTTP403_RemainingNonZero(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-RateLimit-Remaining", "100")

	err := &ghapi.HTTPError{
		StatusCode: http.StatusForbidden,
		Headers:    headers,
	}

	_, isRL := rateLimitWait(err)
	if isRL {
		t.Fatal("should not detect rate limit when remaining > 0")
	}
}

func TestRateLimitWait_HTTP404(t *testing.T) {
	err := &ghapi.HTTPError{
		StatusCode: http.StatusNotFound,
		Headers:    http.Header{},
	}

	_, isRL := rateLimitWait(err)
	if isRL {
		t.Fatal("should not detect rate limit for 404")
	}
}

func TestRateLimitWait_NonHTTPError(t *testing.T) {
	err := fmt.Errorf("some other error")

	_, isRL := rateLimitWait(err)
	if isRL {
		t.Fatal("should not detect rate limit for non-HTTP error")
	}
}

func TestRateLimitWait_WithResetHeader(t *testing.T) {
	resetTime := time.Now().Add(30 * time.Second)
	headers := http.Header{}
	headers.Set("X-RateLimit-Remaining", "0")
	headers.Set("X-RateLimit-Reset", strconv.FormatInt(resetTime.Unix(), 10))

	err := &ghapi.HTTPError{
		StatusCode: http.StatusForbidden,
		Headers:    headers,
	}

	dur, isRL := rateLimitWait(err)
	if !isRL {
		t.Fatal("expected rate limit detection")
	}
	// Should be approximately 31 seconds (30s + 1s buffer)
	if dur < 25*time.Second || dur > 35*time.Second {
		t.Errorf("expected ~31s wait, got %s", dur)
	}
}

func TestRateLimitWait_ResetInPast(t *testing.T) {
	resetTime := time.Now().Add(-10 * time.Second)
	headers := http.Header{}
	headers.Set("X-RateLimit-Remaining", "0")
	headers.Set("X-RateLimit-Reset", strconv.FormatInt(resetTime.Unix(), 10))

	err := &ghapi.HTTPError{
		StatusCode: http.StatusForbidden,
		Headers:    headers,
	}

	dur, isRL := rateLimitWait(err)
	if !isRL {
		t.Fatal("expected rate limit detection")
	}
	// Reset is in the past, should fallback to 5s
	if dur != 5*time.Second {
		t.Errorf("expected 5s fallback for past reset, got %s", dur)
	}
}

func TestResetDuration_EmptyHeader(t *testing.T) {
	dur := resetDuration(http.Header{})
	if dur != 5*time.Second {
		t.Errorf("expected 5s fallback, got %s", dur)
	}
}

func TestResetDuration_InvalidValue(t *testing.T) {
	headers := http.Header{}
	headers.Set("X-RateLimit-Reset", "not-a-number")

	dur := resetDuration(headers)
	if dur != 5*time.Second {
		t.Errorf("expected 5s fallback, got %s", dur)
	}
}

func TestResetDuration_CapsAt60s(t *testing.T) {
	resetTime := time.Now().Add(120 * time.Second)
	headers := http.Header{}
	headers.Set("X-RateLimit-Reset", strconv.FormatInt(resetTime.Unix(), 10))

	dur := resetDuration(headers)
	if dur != 60*time.Second {
		t.Errorf("expected 60s cap, got %s", dur)
	}
}
