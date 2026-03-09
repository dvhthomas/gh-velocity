// Package github wraps go-gh REST client for GitHub API access.
package github

import (
	"fmt"

	ghapi "github.com/cli/go-gh/v2/pkg/api"
)

// Client wraps go-gh REST client.
type Client struct {
	rest  *ghapi.RESTClient
	owner string
	repo  string
}

// NewClient creates a Client for the given owner/repo.
func NewClient(owner, repo string) (*Client, error) {
	rest, err := ghapi.DefaultRESTClient()
	if err != nil {
		return nil, fmt.Errorf("github: create REST client: %w", err)
	}
	return &Client{
		rest:  rest,
		owner: owner,
		repo:  repo,
	}, nil
}
