package github

import (
	"context"
	"fmt"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/model"
)

// reviewsResponse is the GraphQL response for PR reviews and review requests.
type reviewsResponse struct {
	Repository struct {
		PullRequest struct {
			Reviews struct {
				Nodes []reviewNode `json:"nodes"`
			} `json:"reviews"`
		} `json:"pullRequest"`
	} `json:"repository"`
}

type reviewNode struct {
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
	State       string    `json:"state"`
	SubmittedAt time.Time `json:"submittedAt"`
}

// GetPRReviews fetches review data for a pull request.
// Returns the list of reviews ordered by submission time.
func (c *Client) GetPRReviews(ctx context.Context, number int) ([]model.Review, error) {
	query := `query($owner: String!, $repo: String!, $number: Int!) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $number) {
      reviews(first: 100) {
        nodes {
          author { login }
          state
          submittedAt
        }
      }
    }
  }
}`
	variables := map[string]any{
		"owner":  c.owner,
		"repo":   c.repo,
		"number": number,
	}

	var resp reviewsResponse
	if err := c.gql.DoWithContext(ctx, query, variables, &resp); err != nil {
		return nil, fmt.Errorf("get PR #%d reviews: %w", number, err)
	}

	var reviews []model.Review
	for _, node := range resp.Repository.PullRequest.Reviews.Nodes {
		reviews = append(reviews, model.Review{
			Author:      node.Author.Login,
			State:       node.State,
			SubmittedAt: node.SubmittedAt,
		})
	}

	return reviews, nil
}

// GetPRCommitTrailers fetches commit messages for a PR to detect Co-Authored-By trailers.
// Returns the raw commit messages.
func (c *Client) GetPRCommitTrailers(ctx context.Context, number int) ([]string, error) {
	query := `query($owner: String!, $repo: String!, $number: Int!) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $number) {
      commits(first: 100) {
        nodes {
          commit {
            message
          }
        }
      }
    }
  }
}`
	variables := map[string]any{
		"owner":  c.owner,
		"repo":   c.repo,
		"number": number,
	}

	var resp struct {
		Repository struct {
			PullRequest struct {
				Commits struct {
					Nodes []struct {
						Commit struct {
							Message string `json:"message"`
						} `json:"commit"`
					} `json:"nodes"`
				} `json:"commits"`
			} `json:"pullRequest"`
		} `json:"repository"`
	}

	if err := c.gql.DoWithContext(ctx, query, variables, &resp); err != nil {
		return nil, fmt.Errorf("get PR #%d commits: %w", number, err)
	}

	var messages []string
	for _, node := range resp.Repository.PullRequest.Commits.Nodes {
		messages = append(messages, node.Commit.Message)
	}

	return messages, nil
}
