// Package github wraps go-gh REST client for GitHub API access.
package github

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/log"
	ghapi "github.com/cli/go-gh/v2/pkg/api"
)

// projectGQL returns a GraphQL client for project board queries.
// If GH_VELOCITY_TOKEN is set, it creates a dedicated client using that token
// (GITHUB_TOKEN cannot access Projects v2). Otherwise returns nil and callers
// fall back to the default client.
func projectGQL() *ghapi.GraphQLClient {
	token := os.Getenv("GH_VELOCITY_TOKEN")
	if token == "" {
		return nil
	}
	gql, err := ghapi.NewGraphQLClient(ghapi.ClientOptions{AuthToken: token})
	if err != nil {
		return nil
	}
	return gql
}

// Client wraps go-gh REST and GraphQL clients.
type Client struct {
	rest       *ghapi.RESTClient
	gql        *ghapi.GraphQLClient
	projGQL    *ghapi.GraphQLClient // separate client for project queries (GH_VELOCITY_TOKEN)
	owner      string
	repo       string
	repoNodeID string       // cached GraphQL node ID, populated lazily by repoID()
	cache      *QueryCache  // deduplicates identical API calls within a single invocation

	// Search throttle: serializes search API calls with a minimum gap to
	// avoid triggering GitHub's secondary (abuse) rate limits.
	searchMu       sync.Mutex
	searchDelay    time.Duration // minimum gap between search API calls
	searchLastCall time.Time     // last time a search call was issued

}

// NewClient creates a Client for the given owner/repo.
// searchDelay is the minimum gap between search API calls to avoid triggering
// GitHub's secondary rate limits. Pass 0 to disable throttling.
func NewClient(owner, repo string, searchDelay time.Duration) (*Client, error) {
	rest, err := ghapi.DefaultRESTClient()
	if err != nil {
		return nil, fmt.Errorf("github: create REST client: %w", err)
	}
	gql, err := ghapi.DefaultGraphQLClient()
	if err != nil {
		return nil, fmt.Errorf("github: create GraphQL client: %w", err)
	}
	return &Client{
		rest:        rest,
		gql:         gql,
		projGQL:     projectGQL(),
		owner:       owner,
		repo:        repo,
		cache:       NewQueryCache(10 * time.Minute),
		searchDelay: searchDelay,
	}, nil
}

// throttleSearch enforces a minimum gap between search API calls.
// Concurrent goroutines serialize through the mutex, ensuring only one
// search call is in-flight at a time with spacing between them.
// Returns a context error if the context is cancelled during the wait.
func (c *Client) throttleSearch(ctx context.Context) error {
	c.searchMu.Lock()
	defer c.searchMu.Unlock()

	if c.searchDelay <= 0 {
		return nil
	}

	elapsed := time.Since(c.searchLastCall)
	if wait := c.searchDelay - elapsed; wait > 0 {
		log.Debug("throttle: waiting %s before next search call", wait.Round(time.Millisecond))
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	c.searchLastCall = time.Now()
	return nil
}

// projectClient returns the project-specific GraphQL client if available,
// otherwise the default client.
func (c *Client) projectClient() *ghapi.GraphQLClient {
	if c.projGQL != nil {
		return c.projGQL
	}
	return c.gql
}

// RateLimitStatus holds the current GraphQL rate limit state.
type RateLimitStatus struct {
	Limit     int
	Remaining int
	Used      int
	ResetAt   time.Time
}

// RateLimit queries the current GraphQL rate limit status.
// Costs 1 point itself, so call sparingly (e.g., once at command end).
func (c *Client) RateLimit(ctx context.Context) (*RateLimitStatus, error) {
	var resp struct {
		RateLimit struct {
			Limit     int    `json:"limit"`
			Remaining int    `json:"remaining"`
			Used      int    `json:"used"`
			ResetAt   string `json:"resetAt"`
		} `json:"rateLimit"`
	}
	if err := c.gql.DoWithContext(ctx, `{ rateLimit { limit remaining used resetAt } }`, nil, &resp); err != nil {
		return nil, err
	}
	resetAt, _ := time.Parse(time.RFC3339, resp.RateLimit.ResetAt)
	return &RateLimitStatus{
		Limit:     resp.RateLimit.Limit,
		Remaining: resp.RateLimit.Remaining,
		Used:      resp.RateLimit.Used,
		ResetAt:   resetAt,
	}, nil
}

// Owner returns the repository owner.
func (c *Client) Owner() string { return c.owner }

// Repo returns the repository name.
func (c *Client) Repo() string { return c.repo }

// CanonicalRepo checks whether the configured repository is accessible and
// returns the canonical owner/name (correct casing). Returns empty strings
// and false if the repo is not found.
func (c *Client) CanonicalRepo(ctx context.Context) (owner, name string, ok bool, err error) {
	var repo struct {
		Owner struct {
			Login string `json:"login"`
		} `json:"owner"`
		Name string `json:"name"`
	}
	if err := c.rest.DoWithContext(ctx, "GET", fmt.Sprintf("repos/%s/%s", c.owner, c.repo), nil, &repo); err != nil {
		// go-gh returns *api.HTTPError for 404
		if strings.Contains(err.Error(), "404") {
			return "", "", false, nil
		}
		return "", "", false, err
	}
	return repo.Owner.Login, repo.Name, true, nil
}

// GetAuthenticatedUser returns the login of the authenticated GitHub user.
// Works with all token types (classic PAT, fine-grained PAT, GitHub App).
func (c *Client) GetAuthenticatedUser(ctx context.Context) (string, error) {
	var user struct {
		Login string `json:"login"`
	}
	if err := c.rest.DoWithContext(ctx, "GET", "user", nil, &user); err != nil {
		return "", fmt.Errorf("get authenticated user: %w", err)
	}
	if user.Login == "" {
		return "", fmt.Errorf("get authenticated user: empty login returned")
	}
	return user.Login, nil
}

// TokenScopes returns the OAuth scopes granted to the current token.
// Uses GET /user and reads the X-OAuth-Scopes response header.
// Fine-grained PATs may return an empty list (they don't use OAuth scopes).
func (c *Client) TokenScopes(ctx context.Context) ([]string, error) {
	resp, err := c.rest.RequestWithContext(ctx, "GET", "user", nil)
	if err != nil {
		return nil, fmt.Errorf("check token scopes: %w", err)
	}
	resp.Body.Close()

	header := resp.Header.Get("X-OAuth-Scopes")
	if header == "" {
		return nil, nil
	}

	var scopes []string
	for s := range strings.SplitSeq(header, ",") {
		if trimmed := strings.TrimSpace(s); trimmed != "" {
			scopes = append(scopes, trimmed)
		}
	}
	return scopes, nil
}
