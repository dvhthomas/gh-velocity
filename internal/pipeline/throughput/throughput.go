// Package throughput implements the throughput metric pipeline.
// It counts issues closed and PRs merged in a date window.
package throughput

import (
	"context"
	"fmt"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/format"
	gh "github.com/bitsbyme/gh-velocity/internal/github"
	"github.com/bitsbyme/gh-velocity/internal/metrics"
	"github.com/bitsbyme/gh-velocity/internal/model"
)

// Pipeline implements pipeline.Pipeline for the throughput command.
type Pipeline struct {
	// Constructor params
	Client     *gh.Client
	Owner      string
	Repo       string
	Since      time.Time
	Until      time.Time
	IssueQuery string
	PRQuery    string
	SearchURL  string

	// GatherData output
	issueCount int
	prCount    int

	// ProcessData output
	Result   model.ThroughputResult
	Warnings []string
	Insights []model.Insight
}

// GatherData fetches issue and PR counts from GitHub search.
func (p *Pipeline) GatherData(ctx context.Context) error {
	issues, issueErr := p.Client.SearchIssues(ctx, p.IssueQuery)
	prs, prErr := p.Client.SearchPRs(ctx, p.PRQuery)

	if issueErr == nil {
		p.issueCount = len(issues)
	} else {
		p.Warnings = append(p.Warnings, fmt.Sprintf("issue search failed: %v", issueErr))
	}
	if prErr == nil {
		p.prCount = len(prs)
	} else {
		p.Warnings = append(p.Warnings, fmt.Sprintf("PR search failed: %v", prErr))
	}

	// Partial failure is OK — we show whatever we got
	if issueErr != nil && prErr != nil {
		return issueErr // both failed, return one
	}
	return nil
}

// ProcessData builds the throughput result.
func (p *Pipeline) ProcessData() error {
	p.Result = model.ThroughputResult{
		Repository:   p.Owner + "/" + p.Repo,
		Since:        p.Since,
		Until:        p.Until,
		IssuesClosed: p.issueCount,
		PRsMerged:    p.prCount,
	}
	p.generateInsights()
	return nil
}

// generateInsights derives human-readable observations from throughput counts.
func (p *Pipeline) generateInsights() {
	p.Insights = metrics.GenerateThroughputInsights(p.Result.IssuesClosed, p.Result.PRsMerged, nil)
}

// Render writes the throughput result in the requested format.
func (p *Pipeline) Render(rc format.RenderContext) error {
	switch rc.Format {
	case format.JSON:
		return WriteJSON(rc.Writer, p.Result, p.SearchURL, p.Warnings, p.Insights)
	case format.Markdown:
		return WriteMarkdown(rc.Writer, p.Result, p.SearchURL, p.Insights)
	default:
		return WritePretty(rc.Writer, p.Result, p.SearchURL, p.Insights)
	}
}
