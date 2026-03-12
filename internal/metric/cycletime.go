package metric

import (
	"context"
	"fmt"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/cycletime"
	"github.com/bitsbyme/gh-velocity/internal/format"
	gh "github.com/bitsbyme/gh-velocity/internal/github"
	"github.com/bitsbyme/gh-velocity/internal/metrics"
	"github.com/bitsbyme/gh-velocity/internal/model"
)

// CycleTimeIssuePipeline implements Pipeline for single-issue cycle-time.
type CycleTimeIssuePipeline struct {
	// Constructor params
	Client      *gh.Client
	Owner       string
	Repo        string
	IssueNumber int
	Strategy    cycletime.Strategy
	StrategyStr string // "issue", "pr", "project-board" for display

	// GatherData output
	Issue    *model.Issue
	PR       *model.PR // populated for PR strategy
	Warnings []string

	// ProcessData output
	CycleTime model.Metric
}

// GatherData fetches the issue and optionally its closing PR.
func (p *CycleTimeIssuePipeline) GatherData(ctx context.Context) error {
	issue, err := p.Client.GetIssue(ctx, p.IssueNumber)
	if err != nil {
		return err
	}
	p.Issue = issue

	if p.StrategyStr == "pr" {
		pr, prErr := p.Client.GetClosingPR(ctx, p.IssueNumber)
		if prErr != nil {
			p.Warnings = append(p.Warnings, fmt.Sprintf("could not find closing PR: %v", prErr))
		} else if pr == nil {
			p.Warnings = append(p.Warnings, "no closing PR found for this issue")
		} else {
			p.PR = pr
		}
	}
	return nil
}

// ProcessData computes cycle time using the configured strategy.
func (p *CycleTimeIssuePipeline) ProcessData() error {
	input := cycletime.Input{Issue: p.Issue, PR: p.PR}
	p.CycleTime = p.Strategy.Compute(context.Background(), input)
	return nil
}

// Render writes the single-issue cycle time in the requested format.
func (p *CycleTimeIssuePipeline) Render(rc format.RenderContext) error {
	repo := p.Owner + "/" + p.Repo
	switch rc.Format {
	case format.JSON:
		return format.WriteCycleTimeJSON(rc.Writer, repo, p.IssueNumber, p.Issue.Title, p.Issue.State, p.Issue.URL, p.Issue.Labels, p.CycleTime, p.Warnings)
	case format.Markdown:
		return format.WriteCycleTimeMarkdown(rc, "Issue", p.IssueNumber, p.Issue.Title, p.Issue.URL, p.CycleTime)
	default:
		return format.WriteCycleTimePretty(rc, "Issue", p.IssueNumber, p.Issue.Title, p.Issue.URL, p.StrategyStr, p.CycleTime)
	}
}

// CycleTimePRPipeline implements Pipeline for single-PR cycle-time.
type CycleTimePRPipeline struct {
	// Constructor params
	Client   *gh.Client
	Owner    string
	Repo     string
	PRNumber int

	// GatherData output
	PR       *model.PR
	Warnings []string

	// ProcessData output
	CycleTime model.Metric
}

// GatherData fetches the PR from GitHub.
func (p *CycleTimePRPipeline) GatherData(ctx context.Context) error {
	pr, err := p.Client.GetPR(ctx, p.PRNumber)
	if err != nil {
		return err
	}
	p.PR = pr

	if pr.MergedAt == nil {
		if pr.State == "closed" {
			p.Warnings = append(p.Warnings, "PR was closed without merging")
		} else {
			p.Warnings = append(p.Warnings, "PR is still open; cycle time is in progress")
		}
	}
	return nil
}

// ProcessData computes cycle time for the PR (created → merged).
func (p *CycleTimePRPipeline) ProcessData() error {
	strat := &cycletime.PRStrategy{}
	p.CycleTime = strat.Compute(context.Background(), cycletime.Input{PR: p.PR})
	return nil
}

// Render writes the single-PR cycle time in the requested format.
func (p *CycleTimePRPipeline) Render(rc format.RenderContext) error {
	repo := p.Owner + "/" + p.Repo
	switch rc.Format {
	case format.JSON:
		return format.WriteCycleTimePRJSON(rc.Writer, repo, p.PRNumber, p.PR.Title, p.PR.State, p.PR.URL, p.PR.Labels, p.CycleTime, p.Warnings)
	case format.Markdown:
		return format.WriteCycleTimeMarkdown(rc, "PR", p.PRNumber, p.PR.Title, p.PR.URL, p.CycleTime)
	default:
		return format.WriteCycleTimePretty(rc, "PR", p.PRNumber, p.PR.Title, p.PR.URL, "pr", p.CycleTime)
	}
}

// CycleTimeBulkPipeline implements Pipeline for bulk cycle-time queries.
type CycleTimeBulkPipeline struct {
	// Constructor params
	Client      *gh.Client
	Owner       string
	Repo        string
	Since       time.Time
	Until       time.Time
	Strategy    cycletime.Strategy
	StrategyStr string
	SearchQuery string
	SearchURL   string
	ClosingPRs  map[int]*model.PR // pre-fetched by cmd layer for PR strategy

	// GatherData output
	issues []model.Issue

	// ProcessData output
	Items []format.BulkCycleTimeItem
	Stats model.Stats
}

// GatherData fetches issues from GitHub search.
func (p *CycleTimeBulkPipeline) GatherData(ctx context.Context) error {
	issues, err := p.Client.SearchIssues(ctx, p.SearchQuery)
	if err != nil {
		return err
	}
	p.issues = issues
	return nil
}

// ProcessData computes per-issue cycle times and aggregate stats.
func (p *CycleTimeBulkPipeline) ProcessData() error {
	var durations []time.Duration

	for _, issue := range p.issues {
		input := cycletime.Input{Issue: &issue}
		if p.ClosingPRs != nil {
			if pr, ok := p.ClosingPRs[issue.Number]; ok {
				input.PR = pr
			}
		}

		ct := p.Strategy.Compute(context.Background(), input)
		p.Items = append(p.Items, format.BulkCycleTimeItem{Issue: issue, Metric: ct})
		if ct.Duration != nil {
			durations = append(durations, *ct.Duration)
		}
	}

	p.Stats = metrics.ComputeStats(durations)
	return nil
}

// Render writes the bulk cycle time results in the requested format.
func (p *CycleTimeBulkPipeline) Render(rc format.RenderContext) error {
	repo := p.Owner + "/" + p.Repo
	switch rc.Format {
	case format.JSON:
		return format.WriteCycleTimeBulkJSON(rc.Writer, repo, p.Since, p.Until, p.StrategyStr, p.Items, p.Stats, p.SearchURL)
	case format.Markdown:
		return format.WriteCycleTimeBulkMarkdown(rc, repo, p.Since, p.Until, p.StrategyStr, p.Items, p.Stats, p.SearchURL)
	default:
		return format.WriteCycleTimeBulkPretty(rc, repo, p.Since, p.Until, p.StrategyStr, p.Items, p.Stats, p.SearchURL)
	}
}
