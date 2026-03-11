package github

import (
	"context"
	"fmt"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

// projectItemsResponse is the GraphQL response for project items queries.
type projectItemsResponse struct {
	Node struct {
		Items struct {
			PageInfo struct {
				HasNextPage bool   `json:"hasNextPage"`
				EndCursor   string `json:"endCursor"`
			} `json:"pageInfo"`
			Nodes []projectItemContentNode `json:"nodes"`
		} `json:"items"`
	} `json:"node"`
}

type projectItemContentNode struct {
	Content     projectContent     `json:"content"`
	FieldValues projectFieldValues `json:"fieldValues"`
}

type projectContent struct {
	Typename   string     `json:"__typename"`
	Number     int        `json:"number,omitempty"`
	Title      string     `json:"title,omitempty"`
	State      string     `json:"state,omitempty"`
	CreatedAt  *time.Time `json:"createdAt,omitempty"`
	Repository *struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository,omitempty"`
	Labels *struct {
		Nodes []struct {
			Name string `json:"name"`
		} `json:"nodes"`
	} `json:"labels,omitempty"`
}

type projectFieldValues struct {
	Nodes []projectFieldValue `json:"nodes"`
}

type projectFieldValue struct {
	Typename  string     `json:"__typename"`
	Name      string     `json:"name,omitempty"`
	UpdatedAt *time.Time `json:"updatedAt,omitempty"`
	Field     *struct {
		ID string `json:"id"`
	} `json:"field,omitempty"`
}

const projectItemsQuery = `query($projectId: ID!, $cursor: String) {
  node(id: $projectId) {
    ... on ProjectV2 {
      items(first: 100, after: $cursor) {
        pageInfo { hasNextPage endCursor }
        nodes {
          content {
            __typename
            ... on Issue {
              number title state createdAt
              repository { nameWithOwner }
              labels(first: 20) { nodes { name } }
            }
            ... on PullRequest {
              number title state createdAt
              repository { nameWithOwner }
            }
            ... on DraftIssue { title createdAt }
          }
          fieldValues(first: 20) {
            nodes {
              __typename
              ... on ProjectV2ItemFieldSingleSelectValue {
                name updatedAt
                field { ... on ProjectV2SingleSelectField { id } }
              }
            }
          }
        }
      }
    }
  }
}`

// ListProjectItems returns all items on a Projects v2 board.
// Uses cursor-based pagination with node(id:) query.
func (c *Client) ListProjectItems(ctx context.Context, projectID, statusFieldID string) ([]model.ProjectItem, error) {
	var allItems []model.ProjectItem
	var cursor *string

	for {
		variables := map[string]any{
			"projectId": projectID,
		}
		if cursor != nil {
			variables["cursor"] = *cursor
		}

		var resp projectItemsResponse
		if err := c.gql.DoWithContext(ctx, projectItemsQuery, variables, &resp); err != nil {
			return nil, fmt.Errorf("list project items: %w", err)
		}

		for _, node := range resp.Node.Items.Nodes {
			item := model.ProjectItem{
				ContentType: node.Content.Typename,
				Number:      node.Content.Number,
				Title:       node.Content.Title,
			}

			if node.Content.CreatedAt != nil {
				item.CreatedAt = node.Content.CreatedAt.UTC()
			}

			if node.Content.Repository != nil {
				item.Repo = node.Content.Repository.NameWithOwner
			}

			if node.Content.Labels != nil {
				for _, l := range node.Content.Labels.Nodes {
					item.Labels = append(item.Labels, l.Name)
				}
			}

			// Find the status field value.
			for _, fv := range node.FieldValues.Nodes {
				if fv.Typename != "ProjectV2ItemFieldSingleSelectValue" {
					continue
				}
				// Match by field ID if configured, otherwise use first single-select.
				if statusFieldID != "" && (fv.Field == nil || fv.Field.ID != statusFieldID) {
					continue
				}
				item.Status = fv.Name
				if fv.UpdatedAt != nil {
					utc := fv.UpdatedAt.UTC()
					item.StatusAt = &utc
				}
				break
			}

			allItems = append(allItems, item)
		}

		if !resp.Node.Items.PageInfo.HasNextPage {
			break
		}
		cursor = &resp.Node.Items.PageInfo.EndCursor
	}

	return allItems, nil
}
