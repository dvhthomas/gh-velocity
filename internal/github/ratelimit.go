package github

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	ghapi "github.com/cli/go-gh/v2/pkg/api"
)

// DefaultSecondaryBackoff is the wait duration when a secondary rate limit is hit.
// GitHub's secondary lockout is typically 1-5 minutes; 60s is at the aggressive end.
const DefaultSecondaryBackoff = 60 * time.Second

// rateLimitKind classifies which type of rate limit was hit.
type rateLimitKind int

const (
	rateLimitNone      rateLimitKind = iota
	rateLimitPrimary                 // HTTP 429 or 403 with X-RateLimit-Remaining: 0
	rateLimitSecondary               // HTTP 403 with "secondary rate limit" in body
)

// rateLimitDetect checks if an error is a GitHub rate limit error and returns
// the kind and recommended wait duration. It detects:
//   - HTTP 429 (Too Many Requests) → primary
//   - HTTP 403 with X-RateLimit-Remaining: 0 → primary
//   - HTTP 403 with "secondary rate limit" in message → secondary
func rateLimitDetect(err error) (kind rateLimitKind, wait time.Duration) {
	var httpErr *ghapi.HTTPError
	if !errors.As(err, &httpErr) {
		return rateLimitNone, 0
	}

	if httpErr.StatusCode == http.StatusTooManyRequests {
		return rateLimitPrimary, resetDuration(httpErr.Headers)
	}

	if httpErr.StatusCode == http.StatusForbidden {
		// Primary: explicit rate limit exhaustion.
		remaining := httpErr.Headers.Get("X-RateLimit-Remaining")
		if remaining == "0" {
			return rateLimitPrimary, resetDuration(httpErr.Headers)
		}

		// Secondary: abuse detection. GitHub includes "secondary rate limit"
		// in the error message body. These have no reset header — use a
		// fixed 60s backoff (typical lockout is 1-5 minutes).
		if isSecondaryRateLimit(httpErr) {
			return rateLimitSecondary, 60 * time.Second
		}
	}

	return rateLimitNone, 0
}

// rateLimitWait is a convenience wrapper that returns (wait, true) for any
// rate limit kind. Kept for backward compatibility with existing callers.
func rateLimitWait(err error) (time.Duration, bool) {
	kind, wait := rateLimitDetect(err)
	if kind == rateLimitNone {
		return 0, false
	}
	return wait, true
}

// isSecondaryRateLimit checks if a GitHub HTTP error is a secondary/abuse
// rate limit. GitHub includes specific text in the error message.
func isSecondaryRateLimit(httpErr *ghapi.HTTPError) bool {
	msg := strings.ToLower(httpErr.Message)
	return strings.Contains(msg, "secondary rate limit") ||
		strings.Contains(msg, "abuse detection")
}

// resetDuration computes how long to wait based on X-RateLimit-Reset header.
// Falls back to 5 seconds if the header is missing or unparseable.
func resetDuration(headers http.Header) time.Duration {
	const fallback = 5 * time.Second

	resetStr := headers.Get("X-RateLimit-Reset")
	if resetStr == "" {
		return fallback
	}

	resetUnix, err := strconv.ParseInt(resetStr, 10, 64)
	if err != nil {
		return fallback
	}

	resetTime := time.Unix(resetUnix, 0)
	wait := time.Until(resetTime) + 1*time.Second // add 1s buffer
	if wait <= 0 {
		return fallback
	}

	// Cap at 60 seconds to avoid excessively long waits
	if wait > 60*time.Second {
		wait = 60 * time.Second
	}

	return wait
}
