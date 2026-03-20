// Package pr implements the composite PR detail pipeline.
// It aggregates cycle time, review metrics, author type, and closed issues
// into a single view for one pull request.
package pr

import (
	"context"
	"fmt"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/format"
	gh "github.com/dvhthomas/gh-velocity/internal/github"
	"github.com/dvhthomas/gh-velocity/internal/model"
)

// Pipeline implements pipeline.Pipeline for single-PR detail.
type Pipeline struct {
	// Constructor params
	Client   *gh.Client
	Owner    string
	Repo     string
	PRNumber int

	// GatherData output
	PR             *model.PR
	Reviews        []model.Review
	ClosedIssues   []model.Issue
	CommitMessages []string
	Warnings       []string

	// ProcessData output
	CycleTime     model.Metric
	ReviewSummary model.ReviewSummary
	AuthorType    model.AuthorType
}

// GatherData fetches the PR, its reviews, linked issues, and commit messages.
func (p *Pipeline) GatherData(ctx context.Context) error {
	pr, err := p.Client.GetPR(ctx, p.PRNumber)
	if err != nil {
		return err
	}
	p.PR = pr

	// Fetch reviews (degrade gracefully).
	reviews, err := p.Client.GetPRReviews(ctx, p.PRNumber)
	if err != nil {
		p.Warnings = append(p.Warnings, fmt.Sprintf("could not fetch reviews: %v", err))
	} else {
		p.Reviews = reviews
	}

	// Fetch linked issues (degrade gracefully).
	linked, err := p.Client.FetchPRLinkedIssues(ctx, []int{p.PRNumber})
	if err != nil {
		p.Warnings = append(p.Warnings, fmt.Sprintf("could not fetch closed issues: %v", err))
	} else if issues, ok := linked[p.PRNumber]; ok {
		p.ClosedIssues = issues
	}

	// Fetch commit messages for author type detection (degrade gracefully).
	messages, err := p.Client.GetPRCommitTrailers(ctx, p.PRNumber)
	if err != nil {
		p.Warnings = append(p.Warnings, fmt.Sprintf("could not fetch commit messages: %v", err))
	} else {
		p.CommitMessages = messages
	}

	return nil
}

// ProcessData computes all metrics from gathered data.
func (p *Pipeline) ProcessData() error {
	// Cycle time: created → merged
	if p.PR.MergedAt != nil {
		p.CycleTime = model.NewMetric(
			&model.Event{Time: p.PR.CreatedAt, Signal: model.SignalPRCreated},
			&model.Event{Time: *p.PR.MergedAt, Signal: model.SignalPRMerged},
		)
	}

	// Review metrics
	p.ReviewSummary = computeReviewSummary(p.PR.CreatedAt, p.Reviews)

	// Author type detection
	p.AuthorType = model.DetectAuthorType(p.PR.Author, p.CommitMessages...)

	return nil
}

// Render writes the PR detail in the requested format.
func (p *Pipeline) Render(rc format.RenderContext) error {
	switch rc.Format {
	case format.JSON:
		return WriteJSON(rc.Writer, p)
	case format.Markdown:
		return WriteMarkdown(rc, p)
	default:
		return WritePretty(rc, p)
	}
}

// computeReviewSummary computes time-to-first-review and review rounds.
func computeReviewSummary(prCreated time.Time, reviews []model.Review) model.ReviewSummary {
	summary := model.ReviewSummary{Reviews: reviews}

	var firstSubstantive *time.Time
	rounds := 0

	for _, r := range reviews {
		isSubstantive := r.State == "APPROVED" || r.State == "CHANGES_REQUESTED"
		if isSubstantive {
			rounds++
			if firstSubstantive == nil {
				t := r.SubmittedAt
				firstSubstantive = &t
			}
		}
	}

	summary.ReviewRounds = rounds
	if firstSubstantive != nil {
		d := firstSubstantive.Sub(prCreated)
		summary.TimeToFirstReview = &d
	}

	return summary
}


