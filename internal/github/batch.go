package github

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	ghapi "github.com/cli/go-gh/v2/pkg/api"
	"golang.org/x/sync/errgroup"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

// FetchIssues fetches multiple issues concurrently with bounded parallelism.
// It returns a map of successfully fetched issues and a map of per-issue errors.
// Rate limit errors (HTTP 403 with X-RateLimit-Remaining: 0, or HTTP 429) are
// detected and handled with backoff before retrying.
func (c *Client) FetchIssues(ctx context.Context, numbers []int) (map[int]*model.Issue, map[int]error) {
	issues := make(map[int]*model.Issue)
	fetchErrors := make(map[int]error)
	var mu sync.Mutex

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(5)

	for _, num := range numbers {
		num := num // capture loop variable
		g.Go(func() error {
			issue, err := c.fetchIssueWithRetry(ctx, num)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				fetchErrors[num] = err
			} else {
				issues[num] = issue
			}
			// Never return an error to errgroup — we want partial results,
			// not cancellation on first failure.
			return nil
		})
	}

	// All goroutines return nil, so Wait never returns an error.
	_ = g.Wait()

	return issues, fetchErrors
}

// fetchIssueWithRetry fetches a single issue, retrying on rate limit errors
// with exponential backoff (up to 2 retries).
func (c *Client) fetchIssueWithRetry(ctx context.Context, number int) (*model.Issue, error) {
	const maxRetries = 2

	for attempt := 0; attempt <= maxRetries; attempt++ {
		issue, err := c.GetIssue(ctx, number)
		if err == nil {
			return issue, nil
		}

		waitDur, isRateLimit := rateLimitWait(err)
		if !isRateLimit {
			return nil, err
		}

		log.Printf("WARNING: rate limited fetching issue #%d (attempt %d/%d), waiting %s",
			number, attempt+1, maxRetries+1, waitDur)

		if attempt == maxRetries {
			return nil, &model.AppError{
				Code:    model.ErrRateLimited,
				Message: fmt.Sprintf("rate limited fetching issue #%d after %d attempts", number, maxRetries+1),
			}
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(waitDur):
			// retry
		}
	}

	// unreachable
	return nil, fmt.Errorf("unreachable: issue #%d", number)
}

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
