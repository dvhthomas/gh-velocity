// Package github wraps go-gh REST client for GitHub API access.
package github

import (
	"context"
	"fmt"
	"strings"

	ghapi "github.com/cli/go-gh/v2/pkg/api"
)

// Client wraps go-gh REST and GraphQL clients.
type Client struct {
	rest       *ghapi.RESTClient
	gql        *ghapi.GraphQLClient
	owner      string
	repo       string
	repoNodeID string // cached GraphQL node ID, populated lazily by repoID()
}

// NewClient creates a Client for the given owner/repo.
func NewClient(owner, repo string) (*Client, error) {
	rest, err := ghapi.DefaultRESTClient()
	if err != nil {
		return nil, fmt.Errorf("github: create REST client: %w", err)
	}
	gql, err := ghapi.DefaultGraphQLClient()
	if err != nil {
		return nil, fmt.Errorf("github: create GraphQL client: %w", err)
	}
	return &Client{
		rest:  rest,
		gql:   gql,
		owner: owner,
		repo:  repo,
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
