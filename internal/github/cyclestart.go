package github

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/classify"
	"github.com/dvhthomas/gh-velocity/internal/log"
	"github.com/dvhthomas/gh-velocity/internal/model"
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
	key := projectStatusCacheKey(c.owner, c.repo, issueNumber, projectID, statusFieldID, backlogStatus)
	v, err := c.cache.DoJSON(key, "project-status",
		func() (any, error) {
			return c.fetchProjectStatus(ctx, issueNumber, projectID, statusFieldID, backlogStatus)
		},
		func(raw json.RawMessage) (any, error) {
			var ps ProjectStatus
			return &ps, json.Unmarshal(raw, &ps)
		},
	)
	if err != nil {
		return nil, err
	}
	return v.(*ProjectStatus), nil
}

// projectStatusCacheKey builds a cache key for project status lookups.
// Includes a token flag to prevent cross-token cache poisoning.
func projectStatusCacheKey(owner, repo string, number int, projectID, statusFieldID, backlogStatus string) string {
	return CacheKey("project-status", owner, repo, fmt.Sprintf("%d", number), projectID, statusFieldID, backlogStatus, hasProjectToken())
}

// hasProjectToken returns "1" if GH_VELOCITY_TOKEN is set, "0" otherwise.
// Used in cache keys to prevent cross-token cache poisoning.
func hasProjectToken() string {
	if os.Getenv("GH_VELOCITY_TOKEN") != "" {
		return "1"
	}
	return "0"
}

// fetchProjectStatus makes the actual GraphQL call to get an issue's project status.
func (c *Client) fetchProjectStatus(ctx context.Context, issueNumber int, projectID, statusFieldID, backlogStatus string) (*ProjectStatus, error) {
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
	if err := c.projectClient().DoWithContext(ctx, query, variables, &resp); err != nil {
		return nil, fmt.Errorf("get project status for issue #%d: %w", issueNumber, err)
	}

	return matchProjectStatus(resp.Repository.Issue.ProjectItems.Nodes, projectID, statusFieldID, backlogStatus), nil
}

// matchProjectStatus extracts a ProjectStatus from raw project item nodes.
// Shared by single and batch fetchers.
func matchProjectStatus(items []projectItemNode, projectID, statusFieldID, backlogStatus string) *ProjectStatus {
	for _, item := range items {
		if projectID != "" && item.Project.ID != projectID {
			continue
		}

		for _, fv := range item.FieldValues.Nodes {
			if fv.Typename != "ProjectV2ItemFieldSingleSelectValue" {
				continue
			}
			if statusFieldID != "" && (fv.Field == nil || fv.Field.ID != statusFieldID) {
				continue
			}

			if fv.Name == backlogStatus {
				return &ProjectStatus{InBacklog: true}
			}

			if fv.UpdatedAt != nil {
				return &ProjectStatus{
					CycleStart: &CycleStart{
						Time:   *fv.UpdatedAt,
						Signal: "status-change",
						Detail: fmt.Sprintf("%s → %s", backlogStatus, fv.Name),
					},
				}
			}
		}
	}

	return &ProjectStatus{}
}

// BatchGetProjectStatuses fetches project status for multiple issues in batched
// GraphQL queries using aliases. Results are written to the cache so subsequent
// individual GetProjectStatus calls hit cache instead of making API calls.
// Batch size: 20 issues per query (matching FetchIssues convention).
func (c *Client) BatchGetProjectStatuses(ctx context.Context, numbers []int, projectID, statusFieldID, backlogStatus string) {
	const batchSize = 20
	if len(numbers) == 0 {
		return
	}

	for i := 0; i < len(numbers); i += batchSize {
		end := min(i+batchSize, len(numbers))
		batch := numbers[i:end]

		results, err := c.fetchProjectStatusBatch(ctx, batch, projectID, statusFieldID, backlogStatus)
		if err != nil {
			log.Debug("batch project status failed: %v", err)
			// Individual GetProjectStatus calls will still work as fallback.
			continue
		}

		// Warm cache with individual results.
		for num, ps := range results {
			key := projectStatusCacheKey(c.owner, c.repo, num, projectID, statusFieldID, backlogStatus)
			c.cache.Set(key, ps)

			// Also write to disk cache.
			if c.cache.disk != nil {
				if raw, err := json.Marshal(ps); err == nil {
					c.cache.disk.Set(key, "project-status", raw)
				}
			}
		}
	}

	log.Debug("batch project status: warmed cache for %d issues", len(numbers))
}

// fetchProjectStatusBatch fetches project status for a batch of issues via
// GraphQL aliases (one alias per issue number in a single query).
func (c *Client) fetchProjectStatusBatch(ctx context.Context, numbers []int, projectID, statusFieldID, backlogStatus string) (map[int]*ProjectStatus, error) {
	var fragments strings.Builder
	for _, num := range numbers {
		fragments.WriteString(fmt.Sprintf(`
    issue%d: issue(number: %d) {
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
    }`, num, num))
	}

	query := fmt.Sprintf(`query($owner: String!, $repo: String!) {
  repository(owner: $owner, name: $repo) {%s
  }
}`, fragments.String())

	variables := map[string]any{
		"owner": c.owner,
		"repo":  c.repo,
	}

	var resp struct {
		Repository map[string]json.RawMessage
	}
	if err := c.projectClient().DoWithContext(ctx, query, variables, &resp); err != nil {
		return nil, fmt.Errorf("batch project status: %w", err)
	}

	result := make(map[int]*ProjectStatus, len(numbers))
	for _, num := range numbers {
		alias := fmt.Sprintf("issue%d", num)
		raw, ok := resp.Repository[alias]
		if !ok {
			result[num] = &ProjectStatus{}
			continue
		}

		var issueResp struct {
			ProjectItems struct {
				Nodes []projectItemNode `json:"nodes"`
			} `json:"projectItems"`
		}
		if err := json.Unmarshal(raw, &issueResp); err != nil {
			result[num] = &ProjectStatus{}
			continue
		}

		result[num] = matchProjectStatus(issueResp.ProjectItems.Nodes, projectID, statusFieldID, backlogStatus)
	}

	return result, nil
}

// labeledEventResponse is the GraphQL response for label timeline queries.
type labeledEventResponse struct {
	Repository struct {
		Issue struct {
			TimelineItems struct {
				Nodes []labeledEventNode `json:"nodes"`
			} `json:"timelineItems"`
		} `json:"issue"`
	} `json:"repository"`
}

type labeledEventNode struct {
	Typename  string    `json:"__typename"`
	CreatedAt time.Time `json:"createdAt"`
	Label     struct {
		Name string `json:"name"`
	} `json:"label"`
}

// GetLabelCycleStart finds the earliest label event on an issue that matches
// any of the given matchers. This is used by the issue cycle time strategy
// when no project board is configured but lifecycle.in-progress.match is set.
//
// matcherStrings uses classify.Matcher syntax (e.g., "label:in-progress").
// Only label: matchers are meaningful here; type: and title: matchers are
// silently skipped since we're matching against label events.
func (c *Client) GetLabelCycleStart(ctx context.Context, issueNumber int, matcherStrings []string) (*CycleStart, error) {
	// Parse matchers and extract label names for matching.
	var matchers []classify.Matcher
	for _, s := range matcherStrings {
		m, err := classify.ParseMatcher(s)
		if err != nil {
			return nil, fmt.Errorf("parse lifecycle matcher %q: %w", s, err)
		}
		matchers = append(matchers, m)
	}

	query := `query($owner: String!, $repo: String!, $number: Int!) {
  repository(owner: $owner, name: $repo) {
    issue(number: $number) {
      timelineItems(first: 100, itemTypes: [LABELED_EVENT]) {
        nodes {
          __typename
          ... on LabeledEvent {
            createdAt
            label { name }
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

	key := CacheKey("label-cycle-start", c.owner, c.repo, fmt.Sprintf("%d", issueNumber))
	v, err := c.cache.Do(key, func() (any, error) {
		var resp labeledEventResponse
		if err := c.gql.DoWithContext(ctx, query, variables, &resp); err != nil {
			return nil, fmt.Errorf("get label timeline for issue #%d: %w", issueNumber, err)
		}
		return resp.Repository.Issue.TimelineItems.Nodes, nil
	})
	if err != nil {
		return nil, err
	}
	nodes := v.([]labeledEventNode)

	// Find the earliest label event that matches any of the matchers.
	var earliest *CycleStart
	for _, node := range nodes {
		input := classify.Input{Labels: []string{node.Label.Name}}
		for _, m := range matchers {
			if m.Matches(input) {
				if earliest == nil || node.CreatedAt.Before(earliest.Time) {
					earliest = &CycleStart{
						Time:   node.CreatedAt,
						Signal: "label-added",
						Detail: fmt.Sprintf("labeled %q", strings.ToLower(node.Label.Name)),
					}
				}
				break
			}
		}
	}

	return earliest, nil
}
