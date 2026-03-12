package metric

import (
	"context"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/format"
	gh "github.com/bitsbyme/gh-velocity/internal/github"
	"github.com/bitsbyme/gh-velocity/internal/model"
	"github.com/bitsbyme/gh-velocity/internal/scope"
)

// ReviewStaleThreshold is the duration after which a PR awaiting review
// is considered stale.
const ReviewStaleThreshold = 48 * time.Hour

// ReviewsPipeline implements Pipeline for the reviews command.
type ReviewsPipeline struct {
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
}

// GatherData fetches open PRs awaiting review.
func (p *ReviewsPipeline) GatherData(ctx context.Context) error {
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
func (p *ReviewsPipeline) ProcessData() error {
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
			IsStale: age > ReviewStaleThreshold,
		})
	}

	p.SearchURL = scope.OpenPRsAwaitingReviewSearchURL(p.Owner, p.Repo)
	return nil
}

// Render writes the review results in the requested format.
func (p *ReviewsPipeline) Render(rc format.RenderContext) error {
	switch rc.Format {
	case format.JSON:
		return format.WriteReviewsJSON(rc.Writer, p.Result, p.SearchURL)
	case format.Markdown:
		return format.WriteReviewsMarkdown(rc, p.Result, p.SearchURL)
	default:
		return format.WriteReviewsPretty(rc, p.Result, p.SearchURL)
	}
}
