package github

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/model"
)

type releaseResponse struct {
	TagName     string     `json:"tag_name"`
	Name        string     `json:"name"`
	Body        string     `json:"body"`
	CreatedAt   time.Time  `json:"created_at"`
	PublishedAt *time.Time `json:"published_at"`
	HTMLURL     string     `json:"html_url"`
	Draft       bool       `json:"draft"`
	Prerelease  bool       `json:"prerelease"`
}

// ListReleases fetches recent releases (up to 30) and filters to those
// published within [since, until]. Excludes drafts.
func (c *Client) ListReleases(ctx context.Context, since, until time.Time) ([]model.Release, error) {
	var resp []releaseResponse
	path := fmt.Sprintf("repos/%s/%s/releases?per_page=30", url.PathEscape(c.owner), url.PathEscape(c.repo))
	if err := c.rest.DoWithContext(ctx, "GET", path, nil, &resp); err != nil {
		return nil, fmt.Errorf("list releases: %w", err)
	}

	var releases []model.Release
	for _, r := range resp {
		if r.Draft {
			continue
		}
		pub := r.CreatedAt
		if r.PublishedAt != nil {
			pub = *r.PublishedAt
		}
		if pub.Before(since) || !pub.Before(until) {
			continue
		}
		releases = append(releases, model.Release{
			TagName:      r.TagName,
			Name:         r.Name,
			Body:         r.Body,
			CreatedAt:    r.CreatedAt,
			PublishedAt:  r.PublishedAt,
			URL:          r.HTMLURL,
			IsDraft:      r.Draft,
			IsPrerelease: r.Prerelease,
		})
	}
	return releases, nil
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
