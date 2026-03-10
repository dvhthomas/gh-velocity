package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

// searchIssueResponse represents a single item from the GitHub search API.
type searchIssueResponse struct {
	Number    int        `json:"number"`
	Title     string     `json:"title"`
	State     string     `json:"state"`
	CreatedAt time.Time  `json:"created_at"`
	ClosedAt  *time.Time `json:"closed_at"`
	HTMLURL   string     `json:"html_url"`
	Labels    []struct {
		Name string `json:"name"`
	} `json:"labels"`
	PullRequest *struct {
		MergedAt *time.Time `json:"merged_at"`
	} `json:"pull_request"`
}

type searchResponse struct {
	TotalCount        int                   `json:"total_count"`
	IncompleteResults bool                  `json:"incomplete_results"`
	Items             []searchIssueResponse `json:"items"`
}

// SearchMergedPRs finds all PRs merged in the given date range using the search API.
// Uses: GET /search/issues?q=repo:{owner}/{repo}+is:pr+is:merged+merged:{start}..{end}
// Returns at most 1000 results (GitHub search API limit).
func (c *Client) SearchMergedPRs(ctx context.Context, start, end time.Time) ([]model.PR, error) {
	startStr := start.UTC().Format("2006-01-02T15:04:05Z")
	endStr := end.UTC().Format("2006-01-02T15:04:05Z")

	query := fmt.Sprintf("repo:%s/%s is:pr is:merged merged:%s..%s",
		c.owner, c.repo, startStr, endStr)

	var allPRs []model.PR
	page := 1

	for {
		var resp searchResponse
		path := fmt.Sprintf("search/issues?q=%s&per_page=100&page=%d",
			url.QueryEscape(query), page)
		if err := c.rest.DoWithContext(ctx, "GET", path, nil, &resp); err != nil {
			return nil, fmt.Errorf("search merged PRs: %w", err)
		}

		for _, item := range resp.Items {
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
			allPRs = append(allPRs, pr)
		}

		if len(resp.Items) < 100 {
			break
		}
		page++
		if page > 10 { // search API returns max 1000 results (10 pages of 100)
			break
		}
	}

	return allPRs, nil
}

type prNode struct {
	Number                  int        `json:"number"`
	Title                   string     `json:"title"`
	MergedAt                *time.Time `json:"mergedAt"`
	ClosingIssuesReferences struct {
		Nodes []gqlIssueNode `json:"nodes"`
	} `json:"closingIssuesReferences"`
}

type gqlIssueNode struct {
	Number    int        `json:"number"`
	Title     string     `json:"title"`
	State     string     `json:"state"`
	CreatedAt time.Time  `json:"createdAt"`
	ClosedAt  *time.Time `json:"closedAt"`
	URL       string     `json:"url"`
	Labels    struct {
		Nodes []struct {
			Name string `json:"name"`
		} `json:"nodes"`
	} `json:"labels"`
}

// FetchPRLinkedIssues fetches linked issues for multiple PRs in batched GraphQL queries.
// Batches up to 20 PRs per query. Returns a map of PR number → linked issues.
func (c *Client) FetchPRLinkedIssues(ctx context.Context, prNumbers []int) (map[int][]model.Issue, error) {
	const batchSize = 20
	result := make(map[int][]model.Issue)

	for i := 0; i < len(prNumbers); i += batchSize {
		end := i + batchSize
		if end > len(prNumbers) {
			end = len(prNumbers)
		}
		batch := prNumbers[i:end]

		batchResult, err := c.fetchPRLinkedIssuesBatch(ctx, batch)
		if err != nil {
			return nil, err
		}
		for k, v := range batchResult {
			result[k] = v
		}
	}

	return result, nil
}

// fetchPRLinkedIssuesBatch fetches linked issues for a single batch of PRs.
func (c *Client) fetchPRLinkedIssuesBatch(ctx context.Context, prNumbers []int) (map[int][]model.Issue, error) {
	// Build aliased query fragments
	queryFragments := ""
	for _, num := range prNumbers {
		queryFragments += fmt.Sprintf(`
    pr%d: pullRequest(number: %d) {
      number
      title
      mergedAt
      closingIssuesReferences(first: 10) {
        nodes {
          number
          title
          state
          createdAt
          closedAt
          url
          labels(first: 20) {
            nodes { name }
          }
        }
      }
    }`, num, num)
	}

	query := fmt.Sprintf(`query($owner: String!, $name: String!) {
  repository(owner: $owner, name: $name) {%s
  }
}`, queryFragments)

	variables := map[string]interface{}{
		"owner": c.owner,
		"name":  c.repo,
	}

	// The go-gh GraphQL client unmarshals into a struct.
	// For dynamic aliases we use a map-based response.
	var resp struct {
		Repository map[string]json.RawMessage
	}
	if err := c.gql.DoWithContext(ctx, query, variables, &resp); err != nil {
		return nil, fmt.Errorf("fetch PR linked issues: %w", err)
	}

	result := make(map[int][]model.Issue)
	for _, num := range prNumbers {
		alias := fmt.Sprintf("pr%d", num)
		raw, ok := resp.Repository[alias]
		if !ok {
			continue
		}
		var node prNode
		if err := json.Unmarshal(raw, &node); err != nil {
			return nil, fmt.Errorf("unmarshal PR %d linked issues: %w", num, err)
		}

		var issues []model.Issue
		for _, issueNode := range node.ClosingIssuesReferences.Nodes {
			labels := make([]string, len(issueNode.Labels.Nodes))
			for i, l := range issueNode.Labels.Nodes {
				labels[i] = l.Name
			}
			issues = append(issues, model.Issue{
				Number:    issueNode.Number,
				Title:     issueNode.Title,
				State:     issueNode.State,
				Labels:    labels,
				CreatedAt: issueNode.CreatedAt,
				ClosedAt:  issueNode.ClosedAt,
				URL:       issueNode.URL,
			})
		}
		result[num] = issues
	}

	return result, nil
}
