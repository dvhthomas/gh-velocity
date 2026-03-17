// Package cycletime implements the cycle-time metric pipeline.
// Cycle time measures how long an issue or PR was actively worked on.
package cycletime

import (
	"context"
	"fmt"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/format"
	gh "github.com/bitsbyme/gh-velocity/internal/github"
	"github.com/bitsbyme/gh-velocity/internal/metrics"
	"github.com/bitsbyme/gh-velocity/internal/model"
)

// BulkItem holds a single issue's cycle time result for bulk output.
type BulkItem struct {
	Issue  model.Issue
	Metric model.Metric
}

// IssuePipeline implements pipeline.Pipeline for single-issue cycle-time.
type IssuePipeline struct {
	// Constructor params
	Client      *gh.Client
	Owner       string
	Repo        string
	IssueNumber int
	Strategy    metrics.CycleTimeStrategy
	StrategyStr string // model.StrategyIssue or model.StrategyPR

	// GatherData output
	Issue    *model.Issue
	PR       *model.PR // populated for PR strategy
	Warnings []string

	// ProcessData output
	CycleTime model.Metric
}

// GatherData fetches the issue and optionally its closing PR.
func (p *IssuePipeline) GatherData(ctx context.Context) error {
	issue, err := p.Client.GetIssue(ctx, p.IssueNumber)
	if err != nil {
		return err
	}
	p.Issue = issue

	if p.StrategyStr == model.StrategyPR {
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
func (p *IssuePipeline) ProcessData() error {
	input := metrics.CycleTimeInput{Issue: p.Issue, PR: p.PR}
	p.CycleTime = p.Strategy.Compute(context.Background(), input)

	// Warn on negative cycle time (project board timestamp after close).
	if p.CycleTime.Duration != nil && *p.CycleTime.Duration < 0 {
		p.Warnings = append(p.Warnings, "Negative cycle time detected — project board status was updated after issue was closed. The timestamp reflects the last field update, not the original transition. Consider using lifecycle.in-progress.match with label matchers.")
	}

	// Warn when cycle time is truly N/A (no start signal at all).
	if p.CycleTime.Start == nil && p.CycleTime.Duration == nil {
		switch p.StrategyStr {
		case model.StrategyIssue:
			p.Warnings = append(p.Warnings, "No cycle time signal — configure lifecycle.in-progress with project_status or match for issue cycle time")
		case model.StrategyPR:
			if p.PR == nil {
				p.Warnings = append(p.Warnings, "No closing PR found — PR strategy requires PRs that reference issues with 'closes #N'")
			}
		}
	}
	return nil
}

// Render writes the single-issue cycle time in the requested format.
func (p *IssuePipeline) Render(rc format.RenderContext) error {
	repo := p.Owner + "/" + p.Repo
	switch rc.Format {
	case format.JSON:
		return WriteIssueJSON(rc.Writer, repo, p.IssueNumber, p.Issue.Title, p.Issue.State, p.Issue.URL, p.Issue.Labels, p.CycleTime, p.Warnings)
	case format.Markdown:
		return WriteMarkdown(rc, "Issue", p.IssueNumber, p.Issue.Title, p.Issue.URL, p.CycleTime)
	default:
		return WritePretty(rc, "Issue", p.IssueNumber, p.Issue.Title, p.Issue.URL, p.StrategyStr, p.CycleTime)
	}
}

// PRPipeline implements pipeline.Pipeline for single-PR cycle-time.
type PRPipeline struct {
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
func (p *PRPipeline) GatherData(ctx context.Context) error {
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

// ProcessData computes cycle time for the PR (created -> merged).
func (p *PRPipeline) ProcessData() error {
	strat := &metrics.PRStrategy{}
	p.CycleTime = strat.Compute(context.Background(), metrics.CycleTimeInput{PR: p.PR})
	return nil
}

// Render writes the single-PR cycle time in the requested format.
func (p *PRPipeline) Render(rc format.RenderContext) error {
	repo := p.Owner + "/" + p.Repo
	switch rc.Format {
	case format.JSON:
		return WritePRJSON(rc.Writer, repo, p.PRNumber, p.PR.Title, p.PR.State, p.PR.URL, p.PR.Labels, p.CycleTime, p.Warnings)
	case format.Markdown:
		return WriteMarkdown(rc, "PR", p.PRNumber, p.PR.Title, p.PR.URL, p.CycleTime)
	default:
		return WritePretty(rc, "PR", p.PRNumber, p.PR.Title, p.PR.URL, "pr", p.CycleTime)
	}
}

// BulkPipeline implements pipeline.Pipeline for bulk cycle-time queries.
type BulkPipeline struct {
	// Constructor params
	Client      *gh.Client
	Owner       string
	Repo        string
	Since       time.Time
	Until       time.Time
	Strategy    metrics.CycleTimeStrategy
	StrategyStr string
	SearchQuery string
	SearchURL   string
	ClosingPRs  map[int]*model.PR // pre-fetched by cmd layer for PR strategy

	// Project board params (for batch pre-fetch of project statuses).
	// Set by the cmd layer when issue strategy + project board is configured.
	ProjectID     string
	StatusFieldID string
	BacklogStatus []string

	// GatherData output
	issues []model.Issue

	// ProcessData output
	Items    []BulkItem
	Stats    model.Stats
	Warnings []string
	Insights []model.Insight
}

// GatherData fetches issues from GitHub search and pre-fetches project
// statuses in batch (if project board is configured) to avoid N+1 queries.
func (p *BulkPipeline) GatherData(ctx context.Context) error {
	issues, err := p.Client.SearchIssues(ctx, p.SearchQuery)
	if err != nil {
		return err
	}
	p.issues = issues

	// Batch pre-fetch project statuses to warm cache (avoids N+1).
	if p.ProjectID != "" && len(issues) > 0 {
		numbers := make([]int, len(issues))
		for i, issue := range issues {
			numbers[i] = issue.Number
		}
		backlog := ""
		if len(p.BacklogStatus) > 0 {
			backlog = p.BacklogStatus[0]
		}
		p.Client.BatchGetProjectStatuses(ctx, numbers, p.ProjectID, p.StatusFieldID, backlog)
	}

	return nil
}

// ProcessData computes per-issue cycle times and aggregate stats.
func (p *BulkPipeline) ProcessData() error {
	var durations []time.Duration

	for _, issue := range p.issues {
		input := metrics.CycleTimeInput{Issue: &issue}
		if p.ClosingPRs != nil {
			if pr, ok := p.ClosingPRs[issue.Number]; ok {
				input.PR = pr
			}
		}

		m := p.Strategy.Compute(context.Background(), input)
		p.Items = append(p.Items, BulkItem{Issue: issue, Metric: m})
		if m.Duration != nil {
			durations = append(durations, *m.Duration)
		}
	}

	p.Stats = metrics.ComputeStats(durations)

	// Warn when negative durations were filtered (project board timestamp issue).
	if p.Stats.NegativeCount > 0 {
		p.Warnings = append(p.Warnings, fmt.Sprintf(
			"%d issues had negative cycle times (project board timestamp reflects last update, not transition) — excluded from stats. Consider using lifecycle.in-progress.match with label matchers for reliable timestamps.",
			p.Stats.NegativeCount))
	}

	// Warn when all items have no cycle time data.
	if len(durations) == 0 && len(p.Items) > 0 {
		switch p.StrategyStr {
		case model.StrategyIssue:
			p.Warnings = append(p.Warnings, "Cycle time unavailable for all issues — configure lifecycle.in-progress with project_status or match. Run: gh velocity config preflight --write")
		case model.StrategyPR:
			p.Warnings = append(p.Warnings, "Cycle time unavailable — no issues had a closing PR. Ensure PRs reference issues with 'closes #N'")
		}
	}

	p.generateInsights()
	return nil
}

// generateInsights derives human-readable observations from the computed stats.
func (p *BulkPipeline) generateInsights() {
	var statsPtr *model.Stats
	if p.Stats.Count > 0 {
		statsPtr = &p.Stats
	}
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
	p.Insights = metrics.GenerateCycleTimeInsights(statsPtr, p.StrategyStr, items)
}

// Render writes the bulk cycle time results in the requested format.
func (p *BulkPipeline) Render(rc format.RenderContext) error {
	repo := p.Owner + "/" + p.Repo
	switch rc.Format {
	case format.JSON:
		return WriteBulkJSON(rc.Writer, repo, p.Since, p.Until, p.StrategyStr, p.Items, p.Stats, p.SearchURL, p.Warnings, p.Insights)
	case format.Markdown:
		return WriteBulkMarkdown(rc, repo, p.Since, p.Until, p.StrategyStr, p.Items, p.Stats, p.SearchURL, p.Insights)
	default:
		return WriteBulkPretty(rc, repo, p.Since, p.Until, p.StrategyStr, p.Items, p.Stats, p.SearchURL, p.Insights)
	}
}
