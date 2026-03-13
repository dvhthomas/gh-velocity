package github

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/log"
	"github.com/bitsbyme/gh-velocity/internal/model"
)

// searchItemToIssue converts a search API item to a model.Issue.
func searchItemToIssue(item searchIssueResponse) model.Issue {
	labels := make([]string, len(item.Labels))
	for i, l := range item.Labels {
		labels[i] = l.Name
	}
	issue := model.Issue{
		Number:      item.Number,
		Title:       item.Title,
		State:       item.State,
		StateReason: item.StateReason,
		Labels:      labels,
		CreatedAt:   item.CreatedAt.UTC(),
		UpdatedAt:   item.UpdatedAt.UTC(),
		ClosedAt:    item.ClosedAt,
		URL:         item.HTMLURL,
	}
	if issue.ClosedAt != nil {
		utc := issue.ClosedAt.UTC()
		issue.ClosedAt = &utc
	}
	return issue
}

// searchItemToPR converts a search API item to a model.PR.
func searchItemToPR(item searchIssueResponse) model.PR {
	labels := make([]string, len(item.Labels))
	for i, l := range item.Labels {
		labels[i] = l.Name
	}
	var author string
	if item.User != nil {
		author = item.User.Login
	}
	pr := model.PR{
		Number:    item.Number,
		Title:     item.Title,
		State:     item.State,
		Labels:    labels,
		Author:    author,
		CreatedAt: item.CreatedAt.UTC(),
		URL:       item.HTMLURL,
	}
	if item.PullRequest != nil {
		pr.MergedAt = item.PullRequest.MergedAt
	}
	return pr
}

// searchPaginated executes a paginated GitHub search API query and returns raw items.
// The query must be a complete, pre-assembled search string.
// Returns at most 1000 results (GitHub search API limit of 10 pages × 100 per page).
func (c *Client) searchPaginated(ctx context.Context, query string) ([]searchIssueResponse, error) {
	var allItems []searchIssueResponse
	page := 1

	for {
		if err := c.throttleSearch(ctx); err != nil {
			return nil, err
		}

		var resp searchResponse
		path := fmt.Sprintf("search/issues?q=%s&per_page=100&page=%d",
			url.QueryEscape(query), page)
		if err := c.rest.DoWithContext(ctx, "GET", path, nil, &resp); err != nil {
			kind, wait := rateLimitDetect(err)
			if kind != rateLimitNone {
				if kind == rateLimitSecondary {
					log.Warn("GitHub secondary rate limit hit; waiting %s before retry", wait)
				} else {
					log.Warn("search rate-limited; waiting %s before retry", wait)
				}
				select {
				case <-time.After(wait):
				case <-ctx.Done():
					return nil, ctx.Err()
				}
				// Retry once after waiting.
				if retryErr := c.rest.DoWithContext(ctx, "GET", path, nil, &resp); retryErr != nil {
					return nil, rateLimitError(retryErr)
				}
			} else {
				return nil, err
			}
		}

		allItems = append(allItems, resp.Items...)

		if len(resp.Items) < 100 {
			break
		}
		page++
		if page > 10 { // search API returns max 1000 results
			log.Warn("results capped at 1000; narrow the date range or scope for complete data")
			break
		}
	}

	return allItems, nil
}

// rateLimitError wraps a rate limit failure into an actionable AppError
// with concrete guidance for both humans and AI agents.
func rateLimitError(err error) error {
	kind, _ := rateLimitDetect(err)

	if kind == rateLimitSecondary {
		return &model.AppError{
			Code: model.ErrRateLimited,
			Message: "GitHub secondary rate limit exceeded (too many search requests in a short burst). " +
				"Retried once but still blocked.",
			Details: map[string]any{
				"type": "secondary",
				"suggestions": []string{
					"Wait 2-5 minutes before retrying.",
					"Use --current to fetch only the current iteration (1 API call instead of many).",
					"Use a board-based strategy (project_url in config) which uses GraphQL instead of search API.",
					"Reduce --iterations to lower the number of search API calls.",
					"In CI, space commands apart with 60+ second gaps between gh velocity invocations.",
				},
			},
		}
	}

	if kind == rateLimitPrimary {
		return &model.AppError{
			Code: model.ErrRateLimited,
			Message: "GitHub search API primary rate limit exceeded (30 requests/minute). " +
				"Retried once but still blocked.",
			Details: map[string]any{
				"type": "primary",
				"suggestions": []string{
					"Wait for the rate limit to reset (usually under 60 seconds).",
					"Use --current to minimize API calls.",
					"Use a board-based strategy (project_url in config) which uses GraphQL instead of search API.",
				},
			},
		}
	}

	// Not a rate limit error — return as-is.
	return err
}

// SearchIssues executes a GitHub search API query and returns issues.
// The query must be a complete, pre-assembled search string (e.g., from scope.Query.Build()).
// The Client's owner/repo are NOT injected — the query is used as-is.
// Results are cached per-process: identical queries return cached results.
func (c *Client) SearchIssues(ctx context.Context, query string) ([]model.Issue, error) {
	key := CacheKey("search-issues", query)
	hit := true
	v, err := c.cache.Do(key, func() (any, error) {
		hit = false
		items, err := c.searchPaginated(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("search issues: %w", err)
		}
		issues := make([]model.Issue, 0, len(items))
		for _, item := range items {
			issues = append(issues, searchItemToIssue(item))
		}
		return issues, nil
	})
	if err != nil {
		return nil, err
	}
	if hit {
		log.Debug("cache hit: search-issues key=%s", key[:8])
	} else {
		log.Debug("cache miss: search-issues key=%s (%d results)", key[:8], len(v.([]model.Issue)))
	}
	return v.([]model.Issue), nil
}

// SearchPRs executes a GitHub search API query and returns PRs.
// The query must be a complete, pre-assembled search string (e.g., from scope.Query.Build()).
// The Client's owner/repo are NOT injected — the query is used as-is.
// Results are cached per-process: identical queries return cached results.
func (c *Client) SearchPRs(ctx context.Context, query string) ([]model.PR, error) {
	key := CacheKey("search-prs", query)
	hit := true
	v, err := c.cache.Do(key, func() (any, error) {
		hit = false
		items, err := c.searchPaginated(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("search PRs: %w", err)
		}
		prs := make([]model.PR, 0, len(items))
		for _, item := range items {
			prs = append(prs, searchItemToPR(item))
		}
		return prs, nil
	})
	if err != nil {
		return nil, err
	}
	if hit {
		log.Debug("cache hit: search-prs key=%s", key[:8])
	} else {
		log.Debug("cache miss: search-prs key=%s (%d results)", key[:8], len(v.([]model.PR)))
	}
	return v.([]model.PR), nil
}

// SearchClosedIssues finds all issues closed in the given date range.
// Deprecated: Use SearchIssues with a pre-assembled query from scope.Query.Build().
func (c *Client) SearchClosedIssues(ctx context.Context, since, until time.Time) ([]model.Issue, error) {
	sinceStr := since.UTC().Format("2006-01-02T15:04:05Z")
	untilStr := until.UTC().Format("2006-01-02T15:04:05Z")
	query := fmt.Sprintf("repo:%s/%s is:issue is:closed closed:%s..%s",
		c.owner, c.repo, sinceStr, untilStr)
	return c.SearchIssues(ctx, query)
}

// SearchOpenPRsAwaitingReview finds open PRs with pending review requests.
// Uses review:required search qualifier to find PRs needing reviews.
func (c *Client) SearchOpenPRsAwaitingReview(ctx context.Context) ([]model.PR, error) {
	query := fmt.Sprintf("repo:%s/%s is:pr is:open review:required", c.owner, c.repo)
	return c.SearchPRs(ctx, query)
}

// SearchOpenIssuesWithLabels finds open issues with the given labels.
// Deprecated: Use SearchIssues with a pre-assembled query from scope.Query.Build().
func (c *Client) SearchOpenIssuesWithLabels(ctx context.Context, labels []string) ([]model.Issue, error) {
	var query strings.Builder
	query.WriteString(fmt.Sprintf("repo:%s/%s is:issue is:open", c.owner, c.repo))
	for _, l := range labels {
		query.WriteString(fmt.Sprintf(` label:"%s"`, l))
	}
	return c.SearchIssues(ctx, query.String())
}
