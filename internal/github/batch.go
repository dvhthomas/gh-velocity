package github

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

// FetchIssues fetches multiple issues in batched GraphQL queries using aliases.
// Batches up to 20 issues per query to stay within GraphQL complexity limits.
// Returns a map of successfully fetched issues and a map of per-issue errors.
func (c *Client) FetchIssues(ctx context.Context, numbers []int) (map[int]*model.Issue, map[int]error) {
	const batchSize = 20
	issues := make(map[int]*model.Issue)
	fetchErrors := make(map[int]error)

	for i := 0; i < len(numbers); i += batchSize {
		end := i + batchSize
		if end > len(numbers) {
			end = len(numbers)
		}
		batch := numbers[i:end]

		batchIssues, err := c.fetchIssuesBatch(ctx, batch)
		if err != nil {
			// If the entire batch fails, record error for each issue.
			for _, num := range batch {
				fetchErrors[num] = err
			}
			continue
		}
		for num, issue := range batchIssues {
			issues[num] = issue
		}
	}

	return issues, fetchErrors
}

// fetchIssuesBatch fetches a single batch of issues via GraphQL aliases.
func (c *Client) fetchIssuesBatch(ctx context.Context, numbers []int) (map[int]*model.Issue, error) {
	// Build aliased query fragments — one alias per issue number.
	queryFragments := ""
	for _, num := range numbers {
		queryFragments += fmt.Sprintf(`
    issue%d: issue(number: %d) {
      number
      title
      state
      createdAt
      closedAt
      url
      labels(first: 20) {
        nodes { name }
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

	// Dynamic aliases require map-based response.
	var resp struct {
		Repository map[string]json.RawMessage
	}
	if err := c.gql.DoWithContext(ctx, query, variables, &resp); err != nil {
		return nil, fmt.Errorf("fetch issues batch: %w", err)
	}

	result := make(map[int]*model.Issue)
	for _, num := range numbers {
		alias := fmt.Sprintf("issue%d", num)
		raw, ok := resp.Repository[alias]
		if !ok {
			continue
		}
		var node gqlIssueNode
		if err := json.Unmarshal(raw, &node); err != nil {
			return nil, fmt.Errorf("unmarshal issue #%d: %w", num, err)
		}

		labels := make([]string, len(node.Labels.Nodes))
		for i, l := range node.Labels.Nodes {
			labels[i] = l.Name
		}
		result[num] = &model.Issue{
			Number:    node.Number,
			Title:     node.Title,
			State:     node.State,
			Labels:    labels,
			CreatedAt: node.CreatedAt,
			ClosedAt:  node.ClosedAt,
			URL:       node.URL,
		}
	}

	return result, nil
}
