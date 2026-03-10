package github

import (
	"context"
	"fmt"
)

// ListLabels returns all label names for the repository.
func (c *Client) ListLabels(ctx context.Context) ([]string, error) {
	var allLabels []string
	page := 1

	for {
		var labels []struct {
			Name string `json:"name"`
		}

		err := c.rest.Get(fmt.Sprintf("repos/%s/%s/labels?per_page=100&page=%d", c.owner, c.repo, page), &labels)
		if err != nil {
			return nil, fmt.Errorf("list labels for %s/%s: %w", c.owner, c.repo, err)
		}

		for _, l := range labels {
			allLabels = append(allLabels, l.Name)
		}

		if len(labels) < 100 {
			break
		}
		page++
	}

	return allLabels, nil
}
