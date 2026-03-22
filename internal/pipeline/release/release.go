// Package release implements the release metric pipeline.
// Release metrics include per-issue lead time, cycle time, release lag, and quality.
package release

import (
	"context"

	"github.com/dvhthomas/gh-velocity/internal/format"
	"github.com/dvhthomas/gh-velocity/internal/metrics"
	"github.com/dvhthomas/gh-velocity/internal/model"
	"github.com/dvhthomas/gh-velocity/internal/pipeline"
)

// Pipeline implements pipeline.Pipeline for the release command.
// GatherData is a no-op because release data gathering is complex and
// stays in the cmd layer (gatherReleaseData). The Pipeline receives
// the pre-built ReleaseInput and computes/renders from there.
type Pipeline struct {
	pipeline.WarningCollector

	// Constructor params (populated by cmd layer after gatherReleaseData)
	Owner string
	Repo  string
	Input metrics.ReleaseInput

	// ProcessData output
	Result model.ReleaseMetrics
}

// GatherData is a no-op — release data gathering is handled by the cmd layer.
func (p *Pipeline) GatherData(_ context.Context) error {
	return nil
}

// ProcessData computes release metrics from the pre-built input.
func (p *Pipeline) ProcessData() error {
	rm, metricWarnings, err := metrics.BuildReleaseMetrics(context.Background(), p.Input)
	if err != nil {
		return err
	}
	p.Result = rm
	for _, w := range metricWarnings {
		p.AddWarning(w)
	}
	return nil
}

// Render writes the release metrics in the requested format.
func (p *Pipeline) Render(rc format.RenderContext) error {
	repo := p.Owner + "/" + p.Repo
	switch rc.Format {
	case format.JSON:
		return WriteJSON(rc.Writer, repo, p.Result, p.Warnings())
	case format.Markdown:
		return WriteMarkdown(rc, p.Result, p.Warnings())
	default:
		return WritePretty(rc, p.Result, p.Warnings())
	}
}
