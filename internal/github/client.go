// Package github wraps go-gh REST client for GitHub API access.
package github

import (
	"fmt"

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
