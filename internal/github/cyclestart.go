package github

import (
	"context"
	"fmt"
	"time"
)

// CycleStart represents the detected start of active work on an issue.
type CycleStart struct {
	Time   time.Time
	Signal string // "status-change", "label", "pr-created", "assigned", "commit"
	Detail string // e.g., "Backlog → In progress" or "PR #42: title"
}

// cycleStartWithLabelsResponse is the GraphQL response for issue timeline + labels queries.
type cycleStartWithLabelsResponse struct {
	Repository struct {
		Issue struct {
			Labels struct {
				Nodes []labelRef `json:"nodes"`
			} `json:"labels"`
			TimelineItems struct {
				Nodes []timelineNode `json:"nodes"`
			} `json:"timelineItems"`
		} `json:"issue"`
	} `json:"repository"`
}

type timelineNode struct {
	Typename string `json:"__typename"`

	// ClosedEvent fields
	Closer *closerRef `json:"closer,omitempty"`

	// AssignedEvent / LabeledEvent fields
	CreatedAt *time.Time `json:"createdAt,omitempty"`
	Assignee  *actor     `json:"assignee,omitempty"`

	// LabeledEvent fields
	Label *labelRef `json:"label,omitempty"`

	// CrossReferencedEvent fields
	Source *crossRefSource `json:"source,omitempty"`
}

type labelRef struct {
	Name string `json:"name"`
}

type closerRef struct {
	Typename  string     `json:"__typename"`
	Number    int        `json:"number,omitempty"`
	Title     string     `json:"title,omitempty"`
	CreatedAt *time.Time `json:"createdAt,omitempty"`
}

type crossRefSource struct {
	Typename  string     `json:"__typename"`
	Number    int        `json:"number,omitempty"`
	Title     string     `json:"title,omitempty"`
	CreatedAt *time.Time `json:"createdAt,omitempty"`
}

type actor struct {
	Login string `json:"login"`
}

// CycleStartResult holds the cycle start signal and backlog status from timeline analysis.
type CycleStartResult struct {
	// CycleStart is set when a start signal was found.
	CycleStart *CycleStart
	// InBacklog is true when the issue currently has a backlog label,
	// which suppresses cycle time even if other signals exist.
	InBacklog bool
}

// GetCycleStart determines when active work started on an issue.
// It checks, in priority order:
//  1. Label signal — the first time an activeLabels label was added
//  2. PR created — any PR referencing the issue (including drafts)
//  3. First assigned — earliest assignment event
//
// If the issue currently has any backlogLabels, InBacklog is set to true
// and cycle time should be suppressed by the caller.
//
// The caller may also query GetProjectStatus for a higher-priority
// status-change signal when Projects v2 is configured.
func (c *Client) GetCycleStart(ctx context.Context, issueNumber int, activeLabels, backlogLabels []string) (*CycleStartResult, error) {
	// Single GraphQL query fetching ClosedEvent (for PR closer),
	// CrossReferencedEvent (for any PR referencing the issue, including drafts),
	// LabeledEvent (for label-based status), and AssignedEvent (for assignment).
	query := `query($owner: String!, $repo: String!, $number: Int!) {
  repository(owner: $owner, name: $repo) {
    issue(number: $number) {
      labels(first: 100) {
        nodes { name }
      }
      timelineItems(first: 100, itemTypes: [CLOSED_EVENT, ASSIGNED_EVENT, CROSS_REFERENCED_EVENT, LABELED_EVENT]) {
        nodes {
          __typename
          ... on ClosedEvent {
            closer {
              __typename
              ... on PullRequest {
                number
                title
                createdAt
              }
            }
          }
          ... on AssignedEvent {
            createdAt
            assignee {
              ... on User { login }
              ... on Bot { login }
            }
          }
          ... on CrossReferencedEvent {
            source {
              __typename
              ... on PullRequest {
                number
                title
                createdAt
              }
            }
          }
          ... on LabeledEvent {
            createdAt
            label { name }
          }
        }
      }
    }
  }
}`

	variables := map[string]interface{}{
		"owner":  c.owner,
		"repo":   c.repo,
		"number": issueNumber,
	}

	var resp cycleStartWithLabelsResponse
	if err := c.gql.DoWithContext(ctx, query, variables, &resp); err != nil {
		return nil, fmt.Errorf("get cycle start for issue #%d: %w", issueNumber, err)
	}

	result := &CycleStartResult{}

	// Check if the issue currently has a backlog label.
	if len(backlogLabels) > 0 {
		blSet := make(map[string]bool, len(backlogLabels))
		for _, bl := range backlogLabels {
			blSet[bl] = true
		}
		for _, l := range resp.Repository.Issue.Labels.Nodes {
			if blSet[l.Name] {
				result.InBacklog = true
				break
			}
		}
	}

	// Build active label set for matching.
	activeSet := make(map[string]bool, len(activeLabels))
	for _, al := range activeLabels {
		activeSet[al] = true
	}

	var labelStart *CycleStart
	var prStart *CycleStart
	var assignedStart *CycleStart

	for _, node := range resp.Repository.Issue.TimelineItems.Nodes {
		switch node.Typename {
		case "LabeledEvent":
			// Label matching active_labels was added.
			if node.Label != nil && activeSet[node.Label.Name] && node.CreatedAt != nil {
				if labelStart == nil || node.CreatedAt.Before(labelStart.Time) {
					labelStart = &CycleStart{
						Time:   *node.CreatedAt,
						Signal: "label",
						Detail: fmt.Sprintf("labeled %q", node.Label.Name),
					}
				}
			}
		case "ClosedEvent":
			// PR that closed the issue — use its creation date.
			if node.Closer != nil && node.Closer.Typename == "PullRequest" && node.Closer.CreatedAt != nil {
				if prStart == nil || node.Closer.CreatedAt.Before(prStart.Time) {
					prStart = &CycleStart{
						Time:   *node.Closer.CreatedAt,
						Signal: "pr-created",
						Detail: fmt.Sprintf("PR #%d: %s", node.Closer.Number, node.Closer.Title),
					}
				}
			}
		case "CrossReferencedEvent":
			// Any PR referencing the issue (including open/draft PRs).
			if node.Source != nil && node.Source.Typename == "PullRequest" && node.Source.CreatedAt != nil {
				if prStart == nil || node.Source.CreatedAt.Before(prStart.Time) {
					prStart = &CycleStart{
						Time:   *node.Source.CreatedAt,
						Signal: "pr-created",
						Detail: fmt.Sprintf("PR #%d: %s", node.Source.Number, node.Source.Title),
					}
				}
			}
		case "AssignedEvent":
			if node.CreatedAt != nil {
				if assignedStart == nil || node.CreatedAt.Before(assignedStart.Time) {
					login := ""
					if node.Assignee != nil {
						login = node.Assignee.Login
					}
					assignedStart = &CycleStart{
						Time:   *node.CreatedAt,
						Signal: "assigned",
						Detail: fmt.Sprintf("assigned to %s", login),
					}
				}
			}
		}
	}

	// Priority: label > PR created > first assigned
	switch {
	case labelStart != nil:
		result.CycleStart = labelStart
	case prStart != nil:
		result.CycleStart = prStart
	case assignedStart != nil:
		result.CycleStart = assignedStart
	}

	return result, nil
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

	variables := map[string]interface{}{
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
