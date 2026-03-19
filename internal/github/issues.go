package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/model"
)

type issueResponse struct {
	Number    int        `json:"number"`
	Title     string     `json:"title"`
	State     string     `json:"state"`
	CreatedAt time.Time  `json:"created_at"`
	ClosedAt  *time.Time `json:"closed_at"`
	HTMLURL   string     `json:"html_url"`
	Labels    []struct {
		Name string `json:"name"`
	} `json:"labels"`
	// PullRequest is non-nil when the "issue" is actually a pull request.
	// GitHub's REST /issues endpoint returns PRs too.
	PullRequest *struct{} `json:"pull_request,omitempty"`
}

// GetIssue fetches an issue by number.
func (c *Client) GetIssue(ctx context.Context, number int) (*model.Issue, error) {
	var resp issueResponse
	path := fmt.Sprintf("repos/%s/%s/issues/%d", url.PathEscape(c.owner), url.PathEscape(c.repo), number)
	if err := c.rest.DoWithContext(ctx, "GET", path, nil, &resp); err != nil {
		return nil, fmt.Errorf("get issue #%d: %w", number, err)
	}

	if resp.PullRequest != nil {
		return nil, fmt.Errorf("#%d is a pull request, not an issue; use --pr for PR cycle time", number)
	}

	labels := make([]string, len(resp.Labels))
	for i, l := range resp.Labels {
		labels[i] = l.Name
	}

	return &model.Issue{
		Number:    resp.Number,
		Title:     resp.Title,
		State:     resp.State,
		Labels:    labels,
		CreatedAt: resp.CreatedAt,
		ClosedAt:  resp.ClosedAt,
		URL:       resp.HTMLURL,
	}, nil
}

// issueBodyResponse is a minimal response for reading issue/PR body.
type issueBodyResponse struct {
	Body string `json:"body"`
}

// GetIssueBody fetches just the body of an issue or PR by number.
func (c *Client) GetIssueBody(ctx context.Context, number int) (string, error) {
	var resp issueBodyResponse
	path := fmt.Sprintf("repos/%s/%s/issues/%d", url.PathEscape(c.owner), url.PathEscape(c.repo), number)
	if err := c.rest.DoWithContext(ctx, "GET", path, nil, &resp); err != nil {
		return "", fmt.Errorf("get body for #%d: %w", number, err)
	}
	return resp.Body, nil
}

// UpdateIssueBody patches the body of an issue or PR by number.
func (c *Client) UpdateIssueBody(ctx context.Context, number int, body string) error {
	path := fmt.Sprintf("repos/%s/%s/issues/%d", url.PathEscape(c.owner), url.PathEscape(c.repo), number)
	payload := struct {
		Body string `json:"body"`
	}{Body: body}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal body: %w", err)
	}

	return c.rest.DoWithContext(ctx, "PATCH", path, bytes.NewReader(data), nil)
}
