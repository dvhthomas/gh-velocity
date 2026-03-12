package metric

import (
	"context"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/format"
	gh "github.com/bitsbyme/gh-velocity/internal/github"
	"github.com/bitsbyme/gh-velocity/internal/model"
)

// ThroughputPipeline implements Pipeline for the throughput command.
type ThroughputPipeline struct {
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
	Result model.ThroughputResult
}

// GatherData fetches issue and PR counts from GitHub search.
func (p *ThroughputPipeline) GatherData(ctx context.Context) error {
	issues, issueErr := p.Client.SearchIssues(ctx, p.IssueQuery)
	prs, prErr := p.Client.SearchPRs(ctx, p.PRQuery)

	if issueErr == nil {
		p.issueCount = len(issues)
	}
	if prErr == nil {
		p.prCount = len(prs)
	}

	// Partial failure is OK — we show whatever we got
	if issueErr != nil && prErr != nil {
		return issueErr // both failed, return one
	}
	return nil
}

// ProcessData builds the throughput result.
func (p *ThroughputPipeline) ProcessData() error {
	p.Result = model.ThroughputResult{
		Repository:   p.Owner + "/" + p.Repo,
		Since:        p.Since,
		Until:        p.Until,
		IssuesClosed: p.issueCount,
		PRsMerged:    p.prCount,
	}
	return nil
}

// Render writes the throughput result in the requested format.
func (p *ThroughputPipeline) Render(rc format.RenderContext) error {
	switch rc.Format {
	case format.JSON:
		return format.WriteThroughputJSON(rc.Writer, p.Result, p.SearchURL)
	case format.Markdown:
		return format.WriteThroughputMarkdown(rc.Writer, p.Result, p.SearchURL)
	default:
		return format.WriteThroughputPretty(rc.Writer, p.Result, p.SearchURL)
	}
}
