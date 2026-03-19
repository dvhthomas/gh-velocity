package github

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/classify"
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
	URL       string     `json:"url,omitempty"`
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
				URL:       node.Closer.URL,
			}
			if node.Closer.MergedAt != nil {
				pr.MergedAt = node.Closer.MergedAt
			}
			return pr, nil
		}
	}

	return nil, nil
}

// GetClosingPRs finds all PRs that closed an issue via timeline events.
// Returns an empty slice (not an error) if no closing PRs were found.
// An issue may have multiple closing PRs if it was reopened and closed again.
func (c *Client) GetClosingPRs(ctx context.Context, issueNumber int) ([]*model.PR, error) {
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
		return nil, fmt.Errorf("get closing PRs for issue #%d: %w", issueNumber, err)
	}

	seen := make(map[int]bool)
	var prs []*model.PR
	for _, node := range resp.Repository.Issue.TimelineItems.Nodes {
		if node.Closer != nil && node.Closer.Typename == "PullRequest" && node.Closer.CreatedAt != nil {
			if seen[node.Closer.Number] {
				continue
			}
			seen[node.Closer.Number] = true
			pr := &model.PR{
				Number:    node.Closer.Number,
				Title:     node.Closer.Title,
				CreatedAt: *node.Closer.CreatedAt,
				URL:       node.Closer.URL,
			}
			if node.Closer.MergedAt != nil {
				pr.MergedAt = node.Closer.MergedAt
			}
			prs = append(prs, pr)
		}
	}

	return prs, nil
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
