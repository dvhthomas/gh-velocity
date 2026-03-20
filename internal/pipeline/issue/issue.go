// Package issue implements the composite issue detail pipeline.
// It aggregates lead time, cycle time, category, and linked PRs
// into a single view for one issue.
package issue

import (
	"context"
	"fmt"

	"github.com/dvhthomas/gh-velocity/internal/classify"
	"github.com/dvhthomas/gh-velocity/internal/format"
	gh "github.com/dvhthomas/gh-velocity/internal/github"
	"github.com/dvhthomas/gh-velocity/internal/metrics"
	"github.com/dvhthomas/gh-velocity/internal/model"
)

// LinkedPR holds a closing PR and its cycle time.
type LinkedPR struct {
	PR        model.PR
	CycleTime model.Metric
}

// Pipeline implements pipeline.Pipeline for single-issue detail.
type Pipeline struct {
	// Constructor params
	Client            *gh.Client
	Owner             string
	Repo              string
	IssueNumber       int
	Strategy          metrics.CycleTimeStrategy
	Classifier        *classify.Classifier
	HasLifecycleMatch bool // true when lifecycle.in-progress.match is configured

	// GatherData output
	Issue     *model.Issue
	ClosingPRs []*model.PR
	Warnings  []string

	// ProcessData output
	LeadTime           model.Metric
	CycleTime          model.Metric
	CycleTimeFiltered  bool // true when cycle time was negative (bad data) and zeroed out
	Category           string
	LinkedPRs          []LinkedPR
}

// GatherData fetches the issue and its closing PRs from GitHub.
func (p *Pipeline) GatherData(ctx context.Context) error {
	issue, err := p.Client.GetIssue(ctx, p.IssueNumber)
	if err != nil {
		return err
	}
	p.Issue = issue

	// Fetch closing PRs (degrade gracefully on failure).
	prs, err := p.Client.GetClosingPRs(ctx, p.IssueNumber)
	if err != nil {
		p.Warnings = append(p.Warnings, fmt.Sprintf("could not fetch closing PRs: %v", err))
	} else {
		p.ClosingPRs = prs
	}

	return nil
}

// ProcessData computes all metrics from gathered data.
func (p *Pipeline) ProcessData() error {
	// Lead time
	p.LeadTime = metrics.LeadTime(*p.Issue)

	// Cycle time (via configured strategy)
	if p.Strategy != nil {
		input := metrics.CycleTimeInput{Issue: p.Issue}
		// Use the first closing PR if PR strategy
		if len(p.ClosingPRs) > 0 {
			input.PR = p.ClosingPRs[0]
		}
		p.CycleTime = p.Strategy.Compute(context.Background(), input)

		if p.CycleTime.Duration != nil && *p.CycleTime.Duration < 0 {
			p.Warnings = append(p.Warnings, "Negative cycle time detected — filtered from results.")
			p.CycleTime = model.Metric{}
			p.CycleTimeFiltered = true
		}
	}

	// Category classification
	if p.Classifier != nil {
		result := p.Classifier.Classify(classify.Input{
			Labels:    p.Issue.Labels,
			IssueType: p.Issue.IssueType,
			Title:     p.Issue.Title,
		})
		p.Category = result.Category()
	} else {
		p.Category = "other"
	}

	// Linked PR cycle times (created → merged)
	for _, pr := range p.ClosingPRs {
		lt := LinkedPR{PR: *pr}
		if pr.MergedAt != nil {
			d := pr.MergedAt.Sub(pr.CreatedAt)
			lt.CycleTime = model.NewMetric(
				&model.Event{Time: pr.CreatedAt, Signal: model.SignalPRCreated},
				&model.Event{Time: *pr.MergedAt, Signal: model.SignalPRMerged},
			)
			_ = d // duration computed by NewMetric
		}
		p.LinkedPRs = append(p.LinkedPRs, lt)
	}

	return nil
}

// Render writes the issue detail in the requested format.
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

