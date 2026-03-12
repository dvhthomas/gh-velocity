package metric

import (
	"context"
	"fmt"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/format"
	gh "github.com/bitsbyme/gh-velocity/internal/github"
	"github.com/bitsbyme/gh-velocity/internal/metrics"
	"github.com/bitsbyme/gh-velocity/internal/model"
)

// LeadTimeSinglePipeline implements Pipeline for single-issue lead-time.
type LeadTimeSinglePipeline struct {
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
func (p *LeadTimeSinglePipeline) GatherData(ctx context.Context) error {
	issue, err := p.Client.GetIssue(ctx, p.IssueNumber)
	if err != nil {
		return err
	}
	p.Issue = issue
	return nil
}

// ProcessData computes lead time from the fetched issue.
func (p *LeadTimeSinglePipeline) ProcessData() error {
	p.LeadTime = metrics.LeadTime(*p.Issue)
	return nil
}

// Render writes the single-issue lead time in the requested format.
func (p *LeadTimeSinglePipeline) Render(rc format.RenderContext) error {
	repo := p.Owner + "/" + p.Repo
	switch rc.Format {
	case format.JSON:
		return format.WriteLeadTimeJSON(rc.Writer, repo, p.IssueNumber, p.Issue.Title, p.Issue.State, p.Issue.URL, p.Issue.Labels, p.LeadTime, nil)
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

// LeadTimeBulkPipeline implements Pipeline for bulk lead-time queries.
type LeadTimeBulkPipeline struct {
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
	Items []format.BulkLeadTimeItem
	Stats model.Stats
}

// GatherData fetches issues from GitHub search.
func (p *LeadTimeBulkPipeline) GatherData(ctx context.Context) error {
	issues, err := p.Client.SearchIssues(ctx, p.SearchQuery)
	if err != nil {
		return err
	}
	p.issues = issues
	return nil
}

// ProcessData computes per-issue lead times and aggregate stats.
func (p *LeadTimeBulkPipeline) ProcessData() error {
	var durations []time.Duration

	for _, issue := range p.issues {
		lt := metrics.LeadTime(issue)
		p.Items = append(p.Items, format.BulkLeadTimeItem{Issue: issue, Metric: lt})
		if lt.Duration != nil {
			durations = append(durations, *lt.Duration)
		}
	}

	p.Stats = metrics.ComputeStats(durations)
	return nil
}

// Render writes the bulk lead time results in the requested format.
func (p *LeadTimeBulkPipeline) Render(rc format.RenderContext) error {
	repo := p.Owner + "/" + p.Repo
	switch rc.Format {
	case format.JSON:
		return format.WriteLeadTimeBulkJSON(rc.Writer, repo, p.Since, p.Until, p.Items, p.Stats, p.SearchURL)
	case format.Markdown:
		return format.WriteLeadTimeBulkMarkdown(rc, repo, p.Since, p.Until, p.Items, p.Stats, p.SearchURL)
	default:
		return format.WriteLeadTimeBulkPretty(rc, repo, p.Since, p.Until, p.Items, p.Stats, p.SearchURL)
	}
}
