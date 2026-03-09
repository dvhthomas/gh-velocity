package github

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

type releaseResponse struct {
	TagName      string     `json:"tag_name"`
	Name         string     `json:"name"`
	Body         string     `json:"body"`
	CreatedAt    time.Time  `json:"created_at"`
	PublishedAt  *time.Time `json:"published_at"`
	HTMLURL      string     `json:"html_url"`
	Draft        bool       `json:"draft"`
	Prerelease   bool       `json:"prerelease"`
}

// GetRelease fetches a release by tag name.
func (c *Client) GetRelease(ctx context.Context, tag string) (*model.Release, error) {
	var resp releaseResponse
	path := fmt.Sprintf("repos/%s/%s/releases/tags/%s", url.PathEscape(c.owner), url.PathEscape(c.repo), url.PathEscape(tag))
	if err := c.rest.DoWithContext(ctx, "GET", path, nil, &resp); err != nil {
		return nil, fmt.Errorf("get release %s: %w", tag, err)
	}

	return &model.Release{
		TagName:      resp.TagName,
		Name:         resp.Name,
		Body:         resp.Body,
		CreatedAt:    resp.CreatedAt,
		PublishedAt:  resp.PublishedAt,
		URL:          resp.HTMLURL,
		IsDraft:      resp.Draft,
		IsPrerelease: resp.Prerelease,
	}, nil
}
