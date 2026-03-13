package github

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

// iterationFieldResponse is the GraphQL response for iteration field queries.
type iterationFieldResponse struct {
	Node struct {
		Fields struct {
			Nodes []iterationFieldNode `json:"nodes"`
		} `json:"fields"`
	} `json:"node"`
}

type iterationFieldNode struct {
	Typename      string                    `json:"__typename"`
	ID            string                    `json:"id"`
	Name          string                    `json:"name"`
	Configuration *iterationFieldConfigNode `json:"configuration,omitempty"`
}

type iterationFieldConfigNode struct {
	Duration            int                     `json:"duration"`
	Iterations          []iterationValueNode    `json:"iterations"`
	CompletedIterations []iterationValueNode    `json:"completedIterations"`
}

type iterationValueNode struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	StartDate string `json:"startDate"` // "YYYY-MM-DD"
	Duration  int    `json:"duration"`  // days
}

const iterationFieldQuery = `query($projectId: ID!) {
  node(id: $projectId) {
    ... on ProjectV2 {
      fields(first: 30) {
        nodes {
          ... on ProjectV2IterationField {
            __typename
            id
            name
            configuration {
              duration
              iterations {
                id
                title
                startDate
                duration
              }
              completedIterations {
                id
                title
                startDate
                duration
              }
            }
          }
        }
      }
    }
  }
}`

// ListIterationField fetches iteration field configuration from a project board.
// Returns the field config matching the given field name.
func (c *Client) ListIterationField(ctx context.Context, projectID, fieldName string) (*model.IterationFieldConfig, error) {
	variables := map[string]any{
		"projectId": projectID,
	}

	var resp iterationFieldResponse
	if err := c.projectClient().DoWithContext(ctx, iterationFieldQuery, variables, &resp); err != nil {
		return nil, fmt.Errorf("list iteration field: %w", err)
	}

	for _, f := range resp.Node.Fields.Nodes {
		if f.Typename != "ProjectV2IterationField" {
			continue
		}
		if !strings.EqualFold(f.Name, fieldName) {
			continue
		}
		if f.Configuration == nil {
			return nil, fmt.Errorf("iteration field %q has no configuration", fieldName)
		}

		cfg := &model.IterationFieldConfig{}
		for _, it := range f.Configuration.Iterations {
			iter, err := parseIteration(it)
			if err != nil {
				return nil, err
			}
			cfg.Iterations = append(cfg.Iterations, iter)
		}
		for _, it := range f.Configuration.CompletedIterations {
			iter, err := parseIteration(it)
			if err != nil {
				return nil, err
			}
			cfg.CompletedIterations = append(cfg.CompletedIterations, iter)
		}
		return cfg, nil
	}

	return nil, fmt.Errorf("iteration field %q not found on project %s", fieldName, projectID)
}

func parseIteration(n iterationValueNode) (model.Iteration, error) {
	start, err := time.Parse(time.DateOnly, n.StartDate)
	if err != nil {
		return model.Iteration{}, fmt.Errorf("parse iteration start date %q: %w", n.StartDate, err)
	}
	return model.Iteration{
		ID:        n.ID,
		Title:     n.Title,
		StartDate: start,
		Duration:  n.Duration,
		EndDate:   start.AddDate(0, 0, n.Duration),
	}, nil
}

// velocityItemsResponse is the GraphQL response for velocity item queries.
type velocityItemsResponse struct {
	Node struct {
		Items struct {
			PageInfo struct {
				HasNextPage bool   `json:"hasNextPage"`
				EndCursor   string `json:"endCursor"`
			} `json:"pageInfo"`
			Nodes []velocityItemNode `json:"nodes"`
		} `json:"items"`
	} `json:"node"`
}

type velocityItemNode struct {
	Content   velocityContent       `json:"content"`
	Iteration *velocityFieldIter    `json:"iteration"`
	Effort    *velocityFieldNumber  `json:"effort"`
}

type velocityContent struct {
	Typename    string     `json:"__typename"`
	Number      int        `json:"number,omitempty"`
	Title       string     `json:"title,omitempty"`
	URL         string     `json:"url,omitempty"`
	State       string     `json:"state,omitempty"`
	StateReason string     `json:"stateReason,omitempty"`
	ClosedAt    *time.Time `json:"closedAt,omitempty"`
	MergedAt    *time.Time `json:"mergedAt,omitempty"`
	CreatedAt   *time.Time `json:"createdAt,omitempty"`
	Repository  *struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository,omitempty"`
	Labels *struct {
		Nodes []struct {
			Name string `json:"name"`
		} `json:"nodes"`
	} `json:"labels,omitempty"`
	IssueType *struct {
		Name string `json:"name"`
	} `json:"issueType,omitempty"`
}

type velocityFieldIter struct {
	IterationID string `json:"iterationId,omitempty"`
	Title       string `json:"title,omitempty"`
	StartDate   string `json:"startDate,omitempty"`
	Duration    int    `json:"duration,omitempty"`
}

type velocityFieldNumber struct {
	Number *float64 `json:"number"`
}

// buildVelocityItemsQuery builds the GraphQL query for velocity items.
// Field names are inserted as string literals in fieldValueByName() calls
// since GraphQL does not support variables in alias/field-name positions.
// The field names come from validated user config, not raw user input.
func buildVelocityItemsQuery(iterFieldName, numFieldName string) string {
	var iterFragment, numFragment string
	if iterFieldName != "" {
		iterFragment = fmt.Sprintf(`
          iteration: fieldValueByName(name: %q) {
            ... on ProjectV2ItemFieldIterationValue {
              iterationId
              title
              startDate
              duration
            }
          }`, iterFieldName)
	}
	if numFieldName != "" {
		numFragment = fmt.Sprintf(`
          effort: fieldValueByName(name: %q) {
            ... on ProjectV2ItemFieldNumberValue {
              number
            }
          }`, numFieldName)
	}

	return fmt.Sprintf(`query($projectId: ID!, $cursor: String) {
  node(id: $projectId) {
    ... on ProjectV2 {
      items(first: 100, after: $cursor) {
        pageInfo { hasNextPage endCursor }
        nodes {
          content {
            __typename
            ... on Issue {
              number title url state stateReason closedAt createdAt
              repository { nameWithOwner }
              labels(first: 20) { nodes { name } }
              issueType { name }
            }
            ... on PullRequest {
              number title url state mergedAt closedAt createdAt
              repository { nameWithOwner }
              labels(first: 20) { nodes { name } }
            }
          }%s%s
        }
      }
    }
  }
}`, iterFragment, numFragment)
}

// ListProjectItemsWithFields returns project items with iteration and number field values.
// Pass empty string for either field name to skip that field.
func (c *Client) ListProjectItemsWithFields(ctx context.Context, projectID, iterFieldName, numFieldName string) ([]model.VelocityItem, error) {
	query := buildVelocityItemsQuery(iterFieldName, numFieldName)

	var allItems []model.VelocityItem
	var cursor *string

	for {
		variables := map[string]any{
			"projectId": projectID,
		}
		if cursor != nil {
			variables["cursor"] = *cursor
		}

		var resp velocityItemsResponse
		if err := c.projectClient().DoWithContext(ctx, query, variables, &resp); err != nil {
			return nil, fmt.Errorf("list project items with fields: %w", err)
		}

		for _, node := range resp.Node.Items.Nodes {
			ct := node.Content
			if ct.Typename != "Issue" && ct.Typename != "PullRequest" {
				continue // skip DraftIssues
			}

			item := model.VelocityItem{
				ContentType: ct.Typename,
				Number:      ct.Number,
				Title:       ct.Title,
				State:       ct.State,
				StateReason: ct.StateReason,
				ClosedAt:    ct.ClosedAt,
				MergedAt:    ct.MergedAt,
			}

			if ct.CreatedAt != nil {
				item.CreatedAt = ct.CreatedAt.UTC()
			}
			if ct.ClosedAt != nil {
				utc := ct.ClosedAt.UTC()
				item.ClosedAt = &utc
			}
			if ct.MergedAt != nil {
				utc := ct.MergedAt.UTC()
				item.MergedAt = &utc
			}

			if ct.Repository != nil {
				item.Repo = ct.Repository.NameWithOwner
			}

			if ct.Labels != nil {
				for _, l := range ct.Labels.Nodes {
					item.Labels = append(item.Labels, l.Name)
				}
			}

			if ct.IssueType != nil {
				item.IssueType = ct.IssueType.Name
			}

			if node.Iteration != nil && node.Iteration.IterationID != "" {
				item.IterationID = node.Iteration.IterationID
			}

			if node.Effort != nil && node.Effort.Number != nil {
				v := *node.Effort.Number
				item.Effort = &v
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
