// Package leadtime implements the lead-time metric pipeline.
// Lead time measures elapsed time from issue creation to close.
package leadtime

import (
	"context"
	"fmt"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/format"
	gh "github.com/bitsbyme/gh-velocity/internal/github"
	"github.com/bitsbyme/gh-velocity/internal/metrics"
	"github.com/bitsbyme/gh-velocity/internal/model"
)

// Compute calculates the lead time for an issue: created → closed.
// Returns a Metric with nil Duration if the issue is still open.
func Compute(issue model.Issue) model.Metric {
	return metrics.LeadTime(issue)
}

// BulkItem holds a single issue's lead time result for bulk output.
type BulkItem struct {
	Issue  model.Issue
	Metric model.Metric
}

// SinglePipeline implements pipeline.Pipeline for single-issue lead-time.
type SinglePipeline struct {
	// Constructor params
	Client      *gh.Client
	Owner       string
	Repo        string
	IssueNumber int

	// GatherData output
	Issue *model.Issue

	// ProcessData output
	LeadTime model.Metric
}

// GatherData fetches the issue from GitHub.
func (p *SinglePipeline) GatherData(ctx context.Context) error {
	issue, err := p.Client.GetIssue(ctx, p.IssueNumber)
	if err != nil {
		return err
	}
	p.Issue = issue
	return nil
}

// ProcessData computes lead time from the fetched issue.
func (p *SinglePipeline) ProcessData() error {
	p.LeadTime = Compute(*p.Issue)
	return nil
}

// Render writes the single-issue lead time in the requested format.
func (p *SinglePipeline) Render(rc format.RenderContext) error {
	repo := p.Owner + "/" + p.Repo
	switch rc.Format {
	case format.JSON:
		return WriteSingleJSON(rc.Writer, repo, p.IssueNumber, p.Issue.Title, p.Issue.State, p.Issue.URL, p.Issue.Labels, p.LeadTime, nil)
	case format.Markdown:
		fmt.Fprintf(rc.Writer, "| Issue | Title | Created (UTC) | Lead Time |\n")
		fmt.Fprintf(rc.Writer, "| ---: | --- | --- | --- |\n")
		fmt.Fprintf(rc.Writer, "| %s | %s | %s | %s |\n",
			format.FormatItemLink(p.IssueNumber, p.Issue.URL, rc),
			p.Issue.Title,
			p.Issue.CreatedAt.UTC().Format(time.DateOnly),
			format.FormatMetric(p.LeadTime))
		return nil
	default:
		fmt.Fprintf(rc.Writer, "Issue %s  %s\n",
			format.FormatItemLink(p.IssueNumber, p.Issue.URL, rc),
			p.Issue.Title)
		fmt.Fprintf(rc.Writer, "  Created:   %s UTC\n", p.Issue.CreatedAt.UTC().Format(time.RFC3339))
		fmt.Fprintf(rc.Writer, "  Lead Time: %s\n", format.FormatMetric(p.LeadTime))
		return nil
	}
}

// BulkPipeline implements pipeline.Pipeline for bulk lead-time queries.
type BulkPipeline struct {
	// Constructor params
	Client       *gh.Client
	Owner        string
	Repo         string
	Since        time.Time
	Until        time.Time
	SearchQuery  string
	SearchURL    string
	ExcludeUsers []string
	Debug        bool

	// GatherData output
	issues []model.Issue

	// ProcessData output
	Items    []BulkItem
	Stats    model.Stats
	Warnings []string
	Insights []model.Insight
}

// GatherData fetches issues from GitHub search.
func (p *BulkPipeline) GatherData(ctx context.Context) error {
	issues, err := p.Client.SearchIssues(ctx, p.SearchQuery)
	if err != nil {
		return err
	}
	p.issues = issues
	return nil
}

// ProcessData computes per-issue lead times and aggregate stats.
func (p *BulkPipeline) ProcessData() error {
	var durations []time.Duration

	for _, issue := range p.issues {
		lt := Compute(issue)
		p.Items = append(p.Items, BulkItem{Issue: issue, Metric: lt})
		if lt.Duration != nil {
			durations = append(durations, *lt.Duration)
		}
	}

	p.Stats = metrics.ComputeStats(durations)
	p.generateInsights()
	return nil
}

// generateInsights derives human-readable observations from the computed stats.
func (p *BulkPipeline) generateInsights() {
	items := make([]metrics.ItemRef, 0, len(p.Items))
	for _, bi := range p.Items {
		if bi.Metric.Duration != nil {
			items = append(items, metrics.ItemRef{
				Number:   bi.Issue.Number,
				Title:    bi.Issue.Title,
				Duration: *bi.Metric.Duration,
			})
		}
	}
	p.Insights = metrics.GenerateStatsInsights(p.Stats, "Lead Time", items)
}

// Render writes the bulk lead time results in the requested format.
func (p *BulkPipeline) Render(rc format.RenderContext) error {
	repo := p.Owner + "/" + p.Repo
	switch rc.Format {
	case format.JSON:
		return WriteBulkJSON(rc.Writer, repo, p.Since, p.Until, p.Items, p.Stats, p.SearchURL, p.Warnings, p.Insights)
	case format.Markdown:
		return WriteBulkMarkdown(rc, repo, p.Since, p.Until, p.Items, p.Stats, p.SearchURL, p.Insights)
	default:
		return WriteBulkPretty(rc, repo, p.Since, p.Until, p.Items, p.Stats, p.SearchURL, p.Insights)
	}
}
