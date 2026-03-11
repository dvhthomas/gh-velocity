package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// Comment represents a GitHub issue or PR comment.
type Comment struct {
	ID   int64  `json:"id"`
	Body string `json:"body"`
	User string `json:"user"`
}

type commentResponse struct {
	ID   int64  `json:"id"`
	Body string `json:"body"`
	User struct {
		Login string `json:"login"`
	} `json:"user"`
}

// ListComments fetches all comments on an issue or PR.
// Uses: GET /repos/{owner}/{repo}/issues/{number}/comments
func (c *Client) ListComments(ctx context.Context, number int) ([]Comment, error) {
	var all []Comment
	page := 1

	for {
		var resp []commentResponse
		path := fmt.Sprintf("repos/%s/%s/issues/%d/comments?per_page=100&page=%d",
			url.PathEscape(c.owner), url.PathEscape(c.repo), number, page)
		if err := c.rest.DoWithContext(ctx, "GET", path, nil, &resp); err != nil {
			return nil, fmt.Errorf("list comments on #%d: %w", number, err)
		}

		for _, r := range resp {
			all = append(all, Comment{
				ID:   r.ID,
				Body: r.Body,
				User: r.User.Login,
			})
		}

		if len(resp) < 100 {
			break
		}
		page++
	}

	return all, nil
}

// CreateComment creates a new comment on an issue or PR.
// Uses: POST /repos/{owner}/{repo}/issues/{number}/comments
func (c *Client) CreateComment(ctx context.Context, number int, body string) error {
	path := fmt.Sprintf("repos/%s/%s/issues/%d/comments",
		url.PathEscape(c.owner), url.PathEscape(c.repo), number)
	payload := struct {
		Body string `json:"body"`
	}{Body: body}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal comment body: %w", err)
	}
	if err := c.rest.DoWithContext(ctx, "POST", path, bytes.NewReader(data), nil); err != nil {
		return fmt.Errorf("create comment on #%d: %w", number, err)
	}
	return nil
}

// UpdateComment updates an existing comment by its ID.
// Uses: PATCH /repos/{owner}/{repo}/issues/comments/{id}
func (c *Client) UpdateComment(ctx context.Context, commentID int64, body string) error {
	path := fmt.Sprintf("repos/%s/%s/issues/comments/%d",
		url.PathEscape(c.owner), url.PathEscape(c.repo), commentID)
	payload := struct {
		Body string `json:"body"`
	}{Body: body}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal comment body: %w", err)
	}
	if err := c.rest.DoWithContext(ctx, "PATCH", path, bytes.NewReader(data), nil); err != nil {
		return fmt.Errorf("update comment %d: %w", commentID, err)
	}
	return nil
}
