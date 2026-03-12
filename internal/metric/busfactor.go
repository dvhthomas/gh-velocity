package metric

import (
	"context"
	"time"

	"github.com/bitsbyme/gh-velocity/internal/format"
	"github.com/bitsbyme/gh-velocity/internal/git"
	"github.com/bitsbyme/gh-velocity/internal/metrics"
)

// BusFactorPipeline implements Pipeline for the bus-factor command.
type BusFactorPipeline struct {
	// Constructor params
	Repository string
	WorkDir    string
	Since      time.Time
	Depth      int
	MinCommits int
	Format     format.Format

	// GatherData output
	paths []git.PathContributors

	// ProcessData output
	Result metrics.BusFactorResult
}

// GatherData fetches contributor data from local git history.
func (p *BusFactorPipeline) GatherData(ctx context.Context) error {
	runner := git.NewRunner(p.WorkDir)
	paths, err := runner.ContributorsByPath(ctx, p.Since, p.Depth, p.MinCommits)
	if err != nil {
		return err
	}
	p.paths = paths
	return nil
}

// ProcessData computes bus factor risk from gathered contributor data.
func (p *BusFactorPipeline) ProcessData() error {
	p.Result = metrics.ComputeBusFactor(p.paths, p.Since, p.Depth, p.MinCommits)
	p.Result.Repository = p.Repository
	return nil
}

// Render writes the bus factor result in the requested format.
func (p *BusFactorPipeline) Render(rc format.RenderContext) error {
	switch rc.Format {
	case format.JSON:
		return format.WriteBusFactorJSON(rc.Writer, p.Result)
	case format.Markdown:
		return format.WriteBusFactorMarkdown(rc, p.Result)
	default:
		return format.WriteBusFactorPretty(rc, p.Result)
	}
}
