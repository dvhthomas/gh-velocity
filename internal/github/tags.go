package github

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/model"
)

const maxPages = 50

type tagResponse struct {
	Name   string `json:"name"`
	Commit struct {
		SHA string `json:"sha"`
	} `json:"commit"`
}

// gitRefResponse matches the GitHub git/ref API response shape.
type gitRefResponse struct {
	Object struct {
		SHA  string `json:"sha"`
		Type string `json:"type"`
	} `json:"object"`
}

// gitTagObject matches the GitHub git/tags/{sha} API for annotated tags.
type gitTagObject struct {
	Object struct {
		SHA  string `json:"sha"`
		Type string `json:"type"`
	} `json:"object"`
}

type commitDetailResponse struct {
	Commit struct {
		Author struct {
			Date time.Time `json:"date"`
		} `json:"author"`
	} `json:"commit"`
}

// GetTagDate returns the commit date for a tag by resolving the tag ref.
// This works for both annotated and lightweight tags.
func (c *Client) GetTagDate(ctx context.Context, tag string) (time.Time, error) {
	var ref gitRefResponse
	path := fmt.Sprintf("repos/%s/%s/git/ref/tags/%s",
		url.PathEscape(c.owner), url.PathEscape(c.repo), url.PathEscape(tag))
	if err := c.rest.DoWithContext(ctx, "GET", path, nil, &ref); err != nil {
		// Fall back to listing tags and finding the SHA
		return c.getTagDateViaList(ctx, tag)
	}

	sha := ref.Object.SHA

	// Annotated tags point to a tag object; dereference to get the commit SHA.
	if ref.Object.Type == "tag" {
		var tagObj gitTagObject
		tagPath := fmt.Sprintf("repos/%s/%s/git/tags/%s",
			url.PathEscape(c.owner), url.PathEscape(c.repo), url.PathEscape(sha))
		if err := c.rest.DoWithContext(ctx, "GET", tagPath, nil, &tagObj); err == nil {
			sha = tagObj.Object.SHA
		}
	}

	return c.getCommitDate(ctx, sha)
}

func (c *Client) getTagDateViaList(ctx context.Context, tag string) (time.Time, error) {
	var resp []tagResponse
	path := fmt.Sprintf("repos/%s/%s/tags?per_page=100",
		url.PathEscape(c.owner), url.PathEscape(c.repo))
	if err := c.rest.DoWithContext(ctx, "GET", path, nil, &resp); err != nil {
		return time.Time{}, fmt.Errorf("list tags for date: %w", err)
	}
	for _, t := range resp {
		if t.Name == tag {
			return c.getCommitDate(ctx, t.Commit.SHA)
		}
	}
	return time.Time{}, fmt.Errorf("tag %q not found", tag)
}

func (c *Client) getCommitDate(ctx context.Context, sha string) (time.Time, error) {
	var resp commitDetailResponse
	path := fmt.Sprintf("repos/%s/%s/commits/%s",
		url.PathEscape(c.owner), url.PathEscape(c.repo), url.PathEscape(sha))
	if err := c.rest.DoWithContext(ctx, "GET", path, nil, &resp); err != nil {
		return time.Time{}, fmt.Errorf("get commit %s: %w", sha, err)
	}
	return resp.Commit.Author.Date, nil
}

// ListTags fetches repository tags via the GitHub API.
// Returns tag names sorted by the API default (most recent first).
func (c *Client) ListTags(ctx context.Context) ([]string, error) {
	var allTags []string
	page := 1
	for {
		var resp []tagResponse
		path := fmt.Sprintf("repos/%s/%s/tags?per_page=100&page=%d",
			url.PathEscape(c.owner), url.PathEscape(c.repo), page)
		if err := c.rest.DoWithContext(ctx, "GET", path, nil, &resp); err != nil {
			return nil, fmt.Errorf("list tags: %w", err)
		}
		if len(resp) == 0 {
			break
		}
		for _, t := range resp {
			allTags = append(allTags, t.Name)
		}
		if len(resp) < 100 {
			break
		}
		page++
		if page > maxPages {
			log.Printf("warning: ListTags: reached max page limit (%d), returning partial results", maxPages)
			break
		}
	}
	return allTags, nil
}

type compareResponse struct {
	Commits []compareCommit `json:"commits"`
}

type compareCommit struct {
	SHA    string `json:"sha"`
	Commit struct {
		Message string `json:"message"`
		Author  struct {
			Date time.Time `json:"date"`
		} `json:"author"`
	} `json:"commit"`
	HTMLURL string `json:"html_url"`
}

// CompareCommits fetches the commits between two refs via the GitHub API.
// Returns commits in chronological order (oldest first), matching local git log behavior.
func (c *Client) CompareCommits(ctx context.Context, base, head string) ([]model.Commit, error) {
	var allCommits []model.Commit
	page := 1
	for {
		var resp compareResponse
		path := fmt.Sprintf("repos/%s/%s/compare/%s...%s?per_page=100&page=%d",
			url.PathEscape(c.owner), url.PathEscape(c.repo),
			url.PathEscape(base), url.PathEscape(head), page)
		if err := c.rest.DoWithContext(ctx, "GET", path, nil, &resp); err != nil {
			return nil, fmt.Errorf("compare %s...%s: %w", base, head, err)
		}
		for _, c := range resp.Commits {
			// Use first line of commit message to match local git --format=%s behavior.
			msg := c.Commit.Message
			if idx := strings.IndexByte(msg, '\n'); idx >= 0 {
				msg = msg[:idx]
			}
			allCommits = append(allCommits, model.Commit{
				SHA:        c.SHA,
				AuthoredAt: c.Commit.Author.Date,
				Message:    msg,
				URL:        c.HTMLURL,
			})
		}
		if len(resp.Commits) < 100 {
			break
		}
		page++
		if page > maxPages {
			log.Printf("warning: CompareCommits: reached max page limit (%d), returning partial results", maxPages)
			break
		}
	}
	return allCommits, nil
}
