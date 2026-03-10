package github

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	ghapi "github.com/cli/go-gh/v2/pkg/api"
)

// rateLimitWait checks if an error is a GitHub rate limit error and returns
// the recommended wait duration. It detects:
//   - HTTP 429 (Too Many Requests)
//   - HTTP 403 with X-RateLimit-Remaining: 0
//
// If X-RateLimit-Reset is present, the wait is computed from that timestamp.
// Otherwise, exponential backoff durations are used (5s, 15s, 45s).
func rateLimitWait(err error) (time.Duration, bool) {
	var httpErr *ghapi.HTTPError
	if !errors.As(err, &httpErr) {
		return 0, false
	}

	if httpErr.StatusCode == http.StatusTooManyRequests {
		return resetDuration(httpErr.Headers), true
	}

	if httpErr.StatusCode == http.StatusForbidden {
		remaining := httpErr.Headers.Get("X-RateLimit-Remaining")
		if remaining == "0" {
			return resetDuration(httpErr.Headers), true
		}
	}

	return 0, false
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
