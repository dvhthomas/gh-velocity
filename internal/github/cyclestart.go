package github

import (
	"context"
	"fmt"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

// CycleStart represents the detected start of active work on an issue.
type CycleStart struct {
	Time   time.Time
	Signal string // "status-change"
	Detail string // e.g., "Backlog → In progress"
}

// closingPRNode is a timeline node for ClosedEvent queries.
type closingPRNode struct {
	Typename string     `json:"__typename"`
	Closer   *closerRef `json:"closer,omitempty"`
}

type closerRef struct {
	Typename  string     `json:"__typename"`
	Number    int        `json:"number,omitempty"`
	Title     string     `json:"title,omitempty"`
	CreatedAt *time.Time `json:"createdAt,omitempty"`
	MergedAt  *time.Time `json:"mergedAt,omitempty"`
}

// closingPRResponse is the GraphQL response for finding the PR that closed an issue.
type closingPRResponse struct {
	Repository struct {
		Issue struct {
			TimelineItems struct {
				Nodes []closingPRNode `json:"nodes"`
			} `json:"timelineItems"`
		} `json:"issue"`
	} `json:"repository"`
}

// GetClosingPR finds the PR that closed an issue via timeline events.
// Returns nil (not an error) if no closing PR was found.
func (c *Client) GetClosingPR(ctx context.Context, issueNumber int) (*model.PR, error) {
	query := `query($owner: String!, $repo: String!, $number: Int!) {
  repository(owner: $owner, name: $repo) {
    issue(number: $number) {
      timelineItems(first: 20, itemTypes: [CLOSED_EVENT]) {
        nodes {
          __typename
          ... on ClosedEvent {
            closer {
              __typename
              ... on PullRequest {
                number
                title
                createdAt
                mergedAt
                url
              }
            }
          }
        }
      }
    }
  }
}`
	variables := map[string]any{
		"owner":  c.owner,
		"repo":   c.repo,
		"number": issueNumber,
	}

	var resp closingPRResponse
	if err := c.gql.DoWithContext(ctx, query, variables, &resp); err != nil {
		return nil, fmt.Errorf("get closing PR for issue #%d: %w", issueNumber, err)
	}

	for _, node := range resp.Repository.Issue.TimelineItems.Nodes {
		if node.Closer != nil && node.Closer.Typename == "PullRequest" && node.Closer.CreatedAt != nil {
			pr := &model.PR{
				Number:    node.Closer.Number,
				Title:     node.Closer.Title,
				CreatedAt: *node.Closer.CreatedAt,
			}
			if node.Closer.MergedAt != nil {
				pr.MergedAt = node.Closer.MergedAt
			}
			return pr, nil
		}
	}

	return nil, nil
}

// projectStatusResponse is the GraphQL response for project item queries.
type projectStatusResponse struct {
	Repository struct {
		Issue struct {
			ProjectItems struct {
				Nodes []projectItemNode `json:"nodes"`
			} `json:"projectItems"`
		} `json:"issue"`
	} `json:"repository"`
}

type projectItemNode struct {
	Project struct {
		ID string `json:"id"`
	} `json:"project"`
	FieldValues struct {
		Nodes []fieldValueNode `json:"nodes"`
	} `json:"fieldValues"`
}

type fieldValueNode struct {
	Typename  string     `json:"__typename"`
	Name      string     `json:"name,omitempty"`
	UpdatedAt *time.Time `json:"updatedAt,omitempty"`
	Field     *struct {
		ID string `json:"id"`
	} `json:"field,omitempty"`
}

// ProjectStatus represents the result of checking an issue's project board status.
type ProjectStatus struct {
	// CycleStart is set when the issue has moved out of backlog.
	CycleStart *CycleStart
	// InBacklog is true when the issue is explicitly in the backlog status.
	// When true, cycle time should be suppressed even if other signals exist
	// (e.g., the issue was assigned then moved back to backlog).
	InBacklog bool
}

// GetProjectStatus checks the issue's status on a GitHub Projects v2 board.
//
// It queries the issue's project items, finds the status field matching
// statusFieldID, and determines whether the issue is in backlog or has
// moved out. Returns a ProjectStatus with either a CycleStart (if work
// has started) or InBacklog=true (if the issue is in the backlog status).
//
// Returns an empty ProjectStatus if the issue is not in the project or
// no matching status field is found.
func (c *Client) GetProjectStatus(ctx context.Context, issueNumber int, projectID, statusFieldID, backlogStatus string) (*ProjectStatus, error) {
	query := `query($owner: String!, $repo: String!, $number: Int!) {
  repository(owner: $owner, name: $repo) {
    issue(number: $number) {
      projectItems(first: 20) {
        nodes {
          project { id }
          fieldValues(first: 20) {
            nodes {
              __typename
              ... on ProjectV2ItemFieldSingleSelectValue {
                name
                updatedAt
                field { ... on ProjectV2SingleSelectField { id } }
              }
            }
          }
        }
      }
    }
  }
}`

	variables := map[string]any{
		"owner":  c.owner,
		"repo":   c.repo,
		"number": issueNumber,
	}

	var resp projectStatusResponse
	if err := c.gql.DoWithContext(ctx, query, variables, &resp); err != nil {
		return nil, fmt.Errorf("get project status for issue #%d: %w", issueNumber, err)
	}

	for _, item := range resp.Repository.Issue.ProjectItems.Nodes {
		// If a specific project ID is configured, only check that project.
		if projectID != "" && item.Project.ID != projectID {
			continue
		}

		for _, fv := range item.FieldValues.Nodes {
			if fv.Typename != "ProjectV2ItemFieldSingleSelectValue" {
				continue
			}
			// Match by field ID if configured, otherwise check all single-select fields.
			if statusFieldID != "" && (fv.Field == nil || fv.Field.ID != statusFieldID) {
				continue
			}

			if fv.Name == backlogStatus {
				// Issue is explicitly in backlog — work has not started
				// (or was moved back). Cycle time should be suppressed.
				return &ProjectStatus{InBacklog: true}, nil
			}

			// Status is not backlog — work has started.
			if fv.UpdatedAt != nil {
				return &ProjectStatus{
					CycleStart: &CycleStart{
						Time:   *fv.UpdatedAt,
						Signal: "status-change",
						Detail: fmt.Sprintf("%s → %s", backlogStatus, fv.Name),
					},
				}, nil
			}
		}
	}

	// Issue not found in the project or no matching status field.
	return &ProjectStatus{}, nil
}
