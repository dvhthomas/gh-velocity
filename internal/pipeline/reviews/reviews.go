// Package reviews implements the review-pressure metric pipeline.
// It shows open PRs awaiting code review and flags stale ones.
package reviews

import (
	"context"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/format"
	gh "github.com/bitsbyme/gh-velocity/internal/github"
	"github.com/bitsbyme/gh-velocity/internal/model"
	"github.com/bitsbyme/gh-velocity/internal/scope"
)

// StaleThreshold is the duration after which a PR awaiting review
// is considered stale.
const StaleThreshold = 48 * time.Hour

// Pipeline implements pipeline.Pipeline for the reviews command.
type Pipeline struct {
	// Constructor params
	Client *gh.Client
	Owner  string
	Repo   string
	Now    time.Time

	// GatherData output
	prs []model.PR

	// ProcessData output
	Result    model.ReviewPressureResult
	SearchURL string
	Warnings  []string
}

// GatherData fetches open PRs awaiting review.
func (p *Pipeline) GatherData(ctx context.Context) error {
	prs, err := p.Client.SearchOpenPRsAwaitingReview(ctx)
	if err != nil {
		return &model.AppError{
			Code:    model.ErrRateLimited,
			Message: "failed to search for PRs awaiting review: " + err.Error(),
		}
	}
	p.prs = prs
	return nil
}

// ProcessData computes review age and staleness.
func (p *Pipeline) ProcessData() error {
	p.Result = model.ReviewPressureResult{
		Repository: p.Owner + "/" + p.Repo,
	}

	for _, pr := range p.prs {
		age := p.Now.Sub(pr.CreatedAt)
		p.Result.AwaitingReview = append(p.Result.AwaitingReview, model.PRAwaitingReview{
			Number:  pr.Number,
			Title:   pr.Title,
			URL:     pr.URL,
			Age:     age,
			IsStale: age > StaleThreshold,
		})
	}

	p.SearchURL = scope.OpenPRsAwaitingReviewSearchURL(p.Owner, p.Repo)
	return nil
}

// Render writes the review results in the requested format.
func (p *Pipeline) Render(rc format.RenderContext) error {
	switch rc.Format {
	case format.JSON:
		return WriteJSON(rc.Writer, p.Result, p.SearchURL, p.Warnings)
	case format.Markdown:
		return WriteMarkdown(rc, p.Result, p.SearchURL)
	default:
		return WritePretty(rc, p.Result, p.SearchURL)
	}
}
