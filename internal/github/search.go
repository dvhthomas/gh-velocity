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
		Number:    item.Number,
		Title:     item.Title,
		State:     item.State,
		Labels:    labels,
		CreatedAt: item.CreatedAt.UTC(),
		ClosedAt:  item.ClosedAt,
		URL:       item.HTMLURL,
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
	pr := model.PR{
		Number:    item.Number,
		Title:     item.Title,
		State:     item.State,
		Labels:    labels,
		CreatedAt: item.CreatedAt,
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
		var resp searchResponse
		path := fmt.Sprintf("search/issues?q=%s&per_page=100&page=%d",
			url.QueryEscape(query), page)
		if err := c.rest.DoWithContext(ctx, "GET", path, nil, &resp); err != nil {
			return nil, err
		}

		allItems = append(allItems, resp.Items...)

		if len(resp.Items) < 100 {
			break
		}
		page++
		if page > 10 { // search API returns max 1000 results
			log.Warn("results capped at 1000; narrow the query for complete data: %s", query)
			break
		}
	}

	return allItems, nil
}

// SearchIssues executes a GitHub search API query and returns issues.
// The query must be a complete, pre-assembled search string (e.g., from scope.Query.Build()).
// The Client's owner/repo are NOT injected — the query is used as-is.
func (c *Client) SearchIssues(ctx context.Context, query string) ([]model.Issue, error) {
	items, err := c.searchPaginated(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("search issues: %w", err)
	}

	issues := make([]model.Issue, 0, len(items))
	for _, item := range items {
		issues = append(issues, searchItemToIssue(item))
	}
	return issues, nil
}

// SearchPRs executes a GitHub search API query and returns PRs.
// The query must be a complete, pre-assembled search string (e.g., from scope.Query.Build()).
// The Client's owner/repo are NOT injected — the query is used as-is.
func (c *Client) SearchPRs(ctx context.Context, query string) ([]model.PR, error) {
	items, err := c.searchPaginated(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("search PRs: %w", err)
	}

	prs := make([]model.PR, 0, len(items))
	for _, item := range items {
		prs = append(prs, searchItemToPR(item))
	}
	return prs, nil
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
