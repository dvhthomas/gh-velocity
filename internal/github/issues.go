package github

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/model"
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
}

// GetIssue fetches an issue by number.
func (c *Client) GetIssue(ctx context.Context, number int) (*model.Issue, error) {
	var resp issueResponse
	path := fmt.Sprintf("repos/%s/%s/issues/%d", url.PathEscape(c.owner), url.PathEscape(c.repo), number)
	if err := c.rest.DoWithContext(ctx, "GET", path, nil, &resp); err != nil {
		return nil, fmt.Errorf("get issue #%d: %w", number, err)
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
