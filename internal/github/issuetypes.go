package github

import (
	"context"
)

// ListIssueTypes returns the issue types configured on the repository.
// Returns nil (not error) if the repository or GitHub instance does not support issue types.
func (c *Client) ListIssueTypes(ctx context.Context) ([]string, error) {
	query := `query($owner: String!, $repo: String!) {
		repository(owner: $owner, name: $repo) {
			issueTypes(first: 20) {
				nodes { name }
			}
		}
	}`
	variables := map[string]any{
		"owner": c.owner,
		"repo":  c.repo,
	}

	var resp struct {
		Repository struct {
			IssueTypes *struct {
				Nodes []struct {
					Name string `json:"name"`
				} `json:"nodes"`
			} `json:"issueTypes"`
		} `json:"repository"`
	}

	if err := c.gql.DoWithContext(ctx, query, variables, &resp); err != nil {
		// Graceful degradation: old GHE without issueTypes field.
		return nil, nil
	}

	if resp.Repository.IssueTypes == nil {
		return nil, nil
	}

	var names []string
	for _, n := range resp.Repository.IssueTypes.Nodes {
		names = append(names, n.Name)
	}
	return names, nil
}
