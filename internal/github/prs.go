package github

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/model"
)

type prResponse struct {
	Number    int        `json:"number"`
	Title     string     `json:"title"`
	State     string     `json:"state"`
	CreatedAt time.Time  `json:"created_at"`
	MergedAt  *time.Time `json:"merged_at"`
	HTMLURL   string     `json:"html_url"`
	Labels    []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

// GetPR fetches a pull request by number.
func (c *Client) GetPR(ctx context.Context, number int) (*model.PR, error) {
	var resp prResponse
	path := fmt.Sprintf("repos/%s/%s/pulls/%d", url.PathEscape(c.owner), url.PathEscape(c.repo), number)
	if err := c.rest.DoWithContext(ctx, "GET", path, nil, &resp); err != nil {
		return nil, fmt.Errorf("get PR #%d: %w", number, err)
	}

	labels := make([]string, len(resp.Labels))
	for i, l := range resp.Labels {
		labels[i] = l.Name
	}

	return &model.PR{
		Number:    resp.Number,
		Title:     resp.Title,
		State:     resp.State,
		Labels:    labels,
		CreatedAt: resp.CreatedAt,
		MergedAt:  resp.MergedAt,
		URL:       resp.HTMLURL,
	}, nil
}
