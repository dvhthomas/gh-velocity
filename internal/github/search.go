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

// SearchClosedIssues finds all issues closed in the given date range using the search API.
// Uses: GET /search/issues?q=repo:{owner}/{repo}+is:issue+is:closed+closed:{start}..{end}
// Returns at most 1000 results (GitHub search API limit).
// Warns on stderr if results are capped.
// Returns model.Issue with fields populated from search results (number, title, state, createdAt, closedAt).
func (c *Client) SearchClosedIssues(ctx context.Context, since, until time.Time) ([]model.Issue, error) {
	// NOTE: GitHub Search API requires the query as a single string parameter via REST.
	// This is not GraphQL — string interpolation is the correct approach here.
	sinceStr := since.UTC().Format("2006-01-02T15:04:05Z")
	untilStr := until.UTC().Format("2006-01-02T15:04:05Z")

	query := fmt.Sprintf("repo:%s/%s is:issue is:closed closed:%s..%s",
		c.owner, c.repo, sinceStr, untilStr)

	var allIssues []model.Issue
	page := 1

	for {
		var resp searchResponse
		path := fmt.Sprintf("search/issues?q=%s&per_page=100&page=%d",
			url.QueryEscape(query), page)
		if err := c.rest.DoWithContext(ctx, "GET", path, nil, &resp); err != nil {
			return nil, fmt.Errorf("search closed issues: %w", err)
		}

		for _, item := range resp.Items {
			allIssues = append(allIssues, searchItemToIssue(item))
		}

		if len(resp.Items) < 100 {
			break
		}
		page++
		if page > 10 { // search API returns max 1000 results (10 pages of 100)
			log.Warn("results capped at 1000; narrow the date range for complete data")
			break
		}
	}

	return allIssues, nil
}

// SearchOpenIssuesWithLabels finds open issues that have at least one of the given labels.
// Uses: GET /search/issues?q=repo:{owner}/{repo}+is:issue+is:open+label:{label1},label:{label2},...
// Returns at most 1000 results (GitHub search API limit).
func (c *Client) SearchOpenIssuesWithLabels(ctx context.Context, labels []string) ([]model.Issue, error) {
	var query strings.Builder
	query.WriteString(fmt.Sprintf("repo:%s/%s is:issue is:open", c.owner, c.repo))
	for _, l := range labels {
		query.WriteString(fmt.Sprintf(` label:"%s"`, l))
	}

	var allIssues []model.Issue
	page := 1

	for {
		var resp searchResponse
		path := fmt.Sprintf("search/issues?q=%s&per_page=100&page=%d",
			url.QueryEscape(query.String()), page)
		if err := c.rest.DoWithContext(ctx, "GET", path, nil, &resp); err != nil {
			return nil, fmt.Errorf("search open issues with labels: %w", err)
		}

		for _, item := range resp.Items {
			allIssues = append(allIssues, searchItemToIssue(item))
		}

		if len(resp.Items) < 100 {
			break
		}
		page++
		if page > 10 {
			log.Warn("results capped at 1000; consider narrowing your label filters")
			break
		}
	}

	return allIssues, nil
}
