package github

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

const maxPages = 50

type tagResponse struct {
	Name   string `json:"name"`
	Commit struct {
		SHA string `json:"sha"`
	} `json:"commit"`
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
