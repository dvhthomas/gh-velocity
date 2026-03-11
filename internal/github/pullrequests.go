package github

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

// searchIssueResponse represents a single item from the GitHub search API.
type searchIssueResponse struct {
	Number    int        `json:"number"`
	Title     string     `json:"title"`
	State     string     `json:"state"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
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

// SearchMergedPRs finds all PRs merged in the given date range.
// Deprecated: Use SearchPRs with a pre-assembled query from scope.Query.Build().
func (c *Client) SearchMergedPRs(ctx context.Context, start, end time.Time) ([]model.PR, error) {
	startStr := start.UTC().Format("2006-01-02T15:04:05Z")
	endStr := end.UTC().Format("2006-01-02T15:04:05Z")
	query := fmt.Sprintf("repo:%s/%s is:pr is:merged merged:%s..%s",
		c.owner, c.repo, startStr, endStr)
	return c.SearchPRs(ctx, query)
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
	IssueType *struct {
		Name string `json:"name"`
	} `json:"issueType"`
}

// FetchPRLinkedIssues fetches linked issues for multiple PRs in batched GraphQL queries.
// Batches up to 20 PRs per query. Returns a map of PR number → linked issues.
func (c *Client) FetchPRLinkedIssues(ctx context.Context, prNumbers []int) (map[int][]model.Issue, error) {
	const batchSize = 20
	result := make(map[int][]model.Issue)

	for i := 0; i < len(prNumbers); i += batchSize {
		end := min(i+batchSize, len(prNumbers))
		batch := prNumbers[i:end]

		batchResult, err := c.fetchPRLinkedIssuesBatch(ctx, batch)
		if err != nil {
			return nil, err
		}
		maps.Copy(result, batchResult)
	}

	return result, nil
}

// fetchPRLinkedIssuesBatch fetches linked issues for a single batch of PRs.
func (c *Client) fetchPRLinkedIssuesBatch(ctx context.Context, prNumbers []int) (map[int][]model.Issue, error) {
	// Build aliased query fragments
	var queryFragments strings.Builder
	for _, num := range prNumbers {
		queryFragments.WriteString(fmt.Sprintf(`
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
          issueType { name }
        }
      }
    }`, num, num))
	}

	query := fmt.Sprintf(`query($owner: String!, $name: String!) {
  repository(owner: $owner, name: $name) {%s
  }
}`, queryFragments.String())

	variables := map[string]any{
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
			issue := model.Issue{
				Number:    issueNode.Number,
				Title:     issueNode.Title,
				State:     issueNode.State,
				Labels:    labels,
				CreatedAt: issueNode.CreatedAt,
				ClosedAt:  issueNode.ClosedAt,
				URL:       issueNode.URL,
			}
			if issueNode.IssueType != nil {
				issue.IssueType = issueNode.IssueType.Name
			}
			issues = append(issues, issue)
		}
		result[num] = issues
	}

	return result, nil
}
