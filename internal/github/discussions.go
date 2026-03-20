package github

import (
	"context"
	"fmt"
	"strings"
)

// Discussion represents a GitHub Discussion.
type Discussion struct {
	ID    string `json:"id"` // GraphQL node ID
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url"`
}

// repoID fetches and caches the GraphQL node ID of the repository,
// required for mutation inputs like createDiscussion.
func (c *Client) repoID(ctx context.Context) (string, error) {
	if c.repoNodeID != "" {
		return c.repoNodeID, nil
	}
	query := `query($owner: String!, $repo: String!) {
		repository(owner: $owner, name: $repo) { id }
	}`
	variables := map[string]any{
		"owner": c.owner,
		"repo":  c.repo,
	}
	var resp struct {
		Repository struct {
			ID string `json:"id"`
		} `json:"repository"`
	}
	if err := c.gql.DoWithContext(ctx, query, variables, &resp); err != nil {
		return "", fmt.Errorf("fetch repository ID: %w", err)
	}
	c.repoNodeID = resp.Repository.ID
	return c.repoNodeID, nil
}

// CheckDiscussionsEnabled checks whether Discussions are enabled on the repository.
// Uses: GET /repos/{owner}/{repo} → has_discussions field.
func (c *Client) CheckDiscussionsEnabled(ctx context.Context) (bool, error) {
	var resp struct {
		HasDiscussions bool `json:"has_discussions"`
	}
	path := fmt.Sprintf("repos/%s/%s", c.owner, c.repo)
	if err := c.rest.DoWithContext(ctx, "GET", path, nil, &resp); err != nil {
		return false, fmt.Errorf("check discussions enabled: %w", err)
	}
	return resp.HasDiscussions, nil
}

// ResolveDiscussionCategoryID resolves a human-readable category name
// (e.g. "General") to its GraphQL node ID (e.g. "DIC_kwDO...").
// The match is case-insensitive. Uses the client's default owner/repo.
func (c *Client) ResolveDiscussionCategoryID(ctx context.Context, name string) (string, error) {
	return c.ResolveDiscussionCategoryIDForRepo(ctx, c.owner, c.repo, name)
}

// ResolveDiscussionCategoryIDForRepo resolves a category name on a specific repo.
func (c *Client) ResolveDiscussionCategoryIDForRepo(ctx context.Context, owner, repo, name string) (string, error) {
	query := `query($owner: String!, $repo: String!) {
		repository(owner: $owner, name: $repo) {
			discussionCategories(first: 50) {
				nodes { id name }
			}
		}
	}`
	variables := map[string]any{
		"owner": owner,
		"repo":  repo,
	}
	var resp struct {
		Repository struct {
			DiscussionCategories struct {
				Nodes []struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				} `json:"nodes"`
			} `json:"discussionCategories"`
		} `json:"repository"`
	}
	if err := c.gql.DoWithContext(ctx, query, variables, &resp); err != nil {
		return "", fmt.Errorf("resolve discussion category: %w", err)
	}

	lower := strings.ToLower(name)
	for _, cat := range resp.Repository.DiscussionCategories.Nodes {
		if strings.ToLower(cat.Name) == lower {
			return cat.ID, nil
		}
	}

	// Build list of available categories for the error message.
	var names []string
	for _, cat := range resp.Repository.DiscussionCategories.Nodes {
		names = append(names, cat.Name)
	}
	return "", fmt.Errorf("discussion category %q not found; available: %s", name, strings.Join(names, ", "))
}

// SearchDiscussions searches for discussions in the given category.
// Uses the client's default owner/repo.
func (c *Client) SearchDiscussions(ctx context.Context, categoryID string, limit int) ([]Discussion, error) {
	return c.SearchDiscussionsForRepo(ctx, c.owner, c.repo, categoryID, limit)
}

// SearchDiscussionsForRepo searches for discussions in a specific repo's category,
// ordered by most recently updated first.
// Returns at most `limit` discussions.
func (c *Client) SearchDiscussionsForRepo(ctx context.Context, owner, repo, categoryID string, limit int) ([]Discussion, error) {
	if limit <= 0 {
		limit = 50
	}

	query := `query($owner: String!, $repo: String!, $categoryID: ID!, $limit: Int!) {
		repository(owner: $owner, name: $repo) {
			discussions(
				first: $limit
				categoryId: $categoryID
				orderBy: { field: UPDATED_AT, direction: DESC }
			) {
				nodes {
					id
					title
					body
					url
				}
			}
		}
	}`
	variables := map[string]any{
		"owner":      owner,
		"repo":       repo,
		"categoryID": categoryID,
		"limit":      limit,
	}

	var resp struct {
		Repository struct {
			Discussions struct {
				Nodes []Discussion `json:"nodes"`
			} `json:"discussions"`
		} `json:"repository"`
	}
	if err := c.gql.DoWithContext(ctx, query, variables, &resp); err != nil {
		return nil, fmt.Errorf("search discussions: %w", err)
	}
	return resp.Repository.Discussions.Nodes, nil
}

// CreateDiscussion creates a new Discussion in the given category.
// Uses the client's default owner/repo.
func (c *Client) CreateDiscussion(ctx context.Context, categoryID, title, body string) (string, error) {
	return c.CreateDiscussionForRepo(ctx, c.owner, c.repo, categoryID, title, body)
}

// CreateDiscussionForRepo creates a new Discussion in a specific repo's category.
// Returns the URL of the created discussion.
func (c *Client) CreateDiscussionForRepo(ctx context.Context, owner, repo, categoryID, title, body string) (string, error) {
	repoID, err := c.fetchRepoID(ctx, owner, repo)
	if err != nil {
		return "", err
	}

	query := `mutation($repoID: ID!, $categoryID: ID!, $title: String!, $body: String!) {
		createDiscussion(input: {
			repositoryId: $repoID
			categoryId: $categoryID
			title: $title
			body: $body
		}) {
			discussion {
				url
			}
		}
	}`
	variables := map[string]any{
		"repoID":     repoID,
		"categoryID": categoryID,
		"title":      title,
		"body":       body,
	}

	var resp struct {
		CreateDiscussion struct {
			Discussion struct {
				URL string `json:"url"`
			} `json:"discussion"`
		} `json:"createDiscussion"`
	}
	if err := c.gql.DoWithContext(ctx, query, variables, &resp); err != nil {
		return "", fmt.Errorf("create discussion: %w", err)
	}
	return resp.CreateDiscussion.Discussion.URL, nil
}

// fetchRepoID fetches the GraphQL node ID for a given owner/repo.
// Uses the cached value when targeting the client's own repo.
func (c *Client) fetchRepoID(ctx context.Context, owner, repo string) (string, error) {
	if owner == c.owner && repo == c.repo {
		return c.repoID(ctx)
	}
	query := `query($owner: String!, $repo: String!) {
		repository(owner: $owner, name: $repo) { id }
	}`
	variables := map[string]any{
		"owner": owner,
		"repo":  repo,
	}
	var resp struct {
		Repository struct {
			ID string `json:"id"`
		} `json:"repository"`
	}
	if err := c.gql.DoWithContext(ctx, query, variables, &resp); err != nil {
		return "", fmt.Errorf("fetch repository ID for %s/%s: %w", owner, repo, err)
	}
	return resp.Repository.ID, nil
}

// UpdateDiscussion updates the body of an existing Discussion.
func (c *Client) UpdateDiscussion(ctx context.Context, discussionID, body string) error {
	query := `mutation($id: ID!, $body: String!) {
		updateDiscussion(input: {
			discussionId: $id
			body: $body
		}) {
			discussion { id }
		}
	}`
	variables := map[string]any{
		"id":   discussionID,
		"body": body,
	}

	if err := c.gql.DoWithContext(ctx, query, variables, nil); err != nil {
		return fmt.Errorf("update discussion %s: %w", discussionID, err)
	}
	return nil
}
