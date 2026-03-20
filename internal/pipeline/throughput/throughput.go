// Package throughput implements the throughput metric pipeline.
// It counts issues closed and PRs merged in a date window.
package throughput

import (
	"context"
	"fmt"
	"time"

	"github.com/dvhthomas/gh-velocity/internal/format"
	"github.com/dvhthomas/gh-velocity/internal/metrics"
	"github.com/dvhthomas/gh-velocity/internal/model"
)

// searcher is the narrow interface for GitHub search operations.
// *gh.Client satisfies this interface.
type searcher interface {
	SearchIssues(ctx context.Context, query string) ([]model.Issue, error)
	SearchPRs(ctx context.Context, query string) ([]model.PR, error)
}

// Pipeline implements pipeline.Pipeline for the throughput command.
type Pipeline struct {
	// Constructor params
	Client     searcher
	Owner      string
	Repo       string
	Since      time.Time
	Until      time.Time
	IssueQuery string
	PRQuery    string
	SearchURL  string

	// Open-item query configuration (optional, built by caller)
	OpenIssueQueries []string // one per lifecycle label
	OpenPRQueries    []string // one per lifecycle label + one for unlabeled PRs

	// GatherData output — retained items
	ClosedIssues []model.Issue
	MergedPRs    []model.PR
	OpenIssues   []model.Issue
	OpenPRs      []model.PR

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
		p.ClosedIssues = issues
	} else {
		p.Warnings = append(p.Warnings, fmt.Sprintf("issue search failed: %v", issueErr))
	}
	if prErr == nil {
		p.MergedPRs = prs
	} else {
		p.Warnings = append(p.Warnings, fmt.Sprintf("PR search failed: %v", prErr))
	}

	// Partial failure is OK — we show whatever we got
	if issueErr != nil && prErr != nil {
		return issueErr // both failed, return one
	}

	// Fetch open items if queries are configured (optional)
	p.fetchOpenItems(ctx)

	return nil
}

// fetchOpenItems executes open-item queries, deduplicates results, and stores
// them in OpenIssues/OpenPRs. Failures are partial — they produce warnings
// but do not fail the pipeline.
func (p *Pipeline) fetchOpenItems(ctx context.Context) {
	if len(p.OpenIssueQueries) > 0 {
		seen := make(map[int]bool)
		for _, q := range p.OpenIssueQueries {
			issues, err := p.Client.SearchIssues(ctx, q)
			if err != nil {
				p.Warnings = append(p.Warnings, fmt.Sprintf("open issue search failed: %v", err))
				continue
			}
			if len(issues) >= 1000 {
				p.Warnings = append(p.Warnings, "open item query may be truncated (1000 results)")
			}
			for _, issue := range issues {
				if !seen[issue.Number] {
					seen[issue.Number] = true
					p.OpenIssues = append(p.OpenIssues, issue)
				}
			}
		}
	}

	if len(p.OpenPRQueries) > 0 {
		seen := make(map[int]bool)
		for _, q := range p.OpenPRQueries {
			prs, err := p.Client.SearchPRs(ctx, q)
			if err != nil {
				p.Warnings = append(p.Warnings, fmt.Sprintf("open PR search failed: %v", err))
				continue
			}
			if len(prs) >= 1000 {
				p.Warnings = append(p.Warnings, "open item query may be truncated (1000 results)")
			}
			for _, pr := range prs {
				if !seen[pr.Number] {
					seen[pr.Number] = true
					p.OpenPRs = append(p.OpenPRs, pr)
				}
			}
		}
	}
}

// ProcessData builds the throughput result.
func (p *Pipeline) ProcessData() error {
	p.Result = model.ThroughputResult{
		Repository:   p.Owner + "/" + p.Repo,
		Since:        p.Since,
		Until:        p.Until,
		IssuesClosed: len(p.ClosedIssues),
		PRsMerged:    len(p.MergedPRs),
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
