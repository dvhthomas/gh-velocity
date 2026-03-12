package metric

import (
	"context"

	"github.com/bitsbyme/gh-velocity/internal/format"
	"github.com/bitsbyme/gh-velocity/internal/metrics"
	"github.com/bitsbyme/gh-velocity/internal/model"
)

// ReleasePipeline implements Pipeline for the release command.
// GatherData is a no-op because release data gathering is complex and
// stays in the cmd layer (gatherReleaseData). The Pipeline receives
// the pre-built ReleaseInput and computes/renders from there.
type ReleasePipeline struct {
	// Constructor params (populated by cmd layer after gatherReleaseData)
	Owner    string
	Repo     string
	Input    metrics.ReleaseInput
	Warnings []string

	// ProcessData output
	Result model.ReleaseMetrics
}

// GatherData is a no-op — release data gathering is handled by the cmd layer.
func (p *ReleasePipeline) GatherData(_ context.Context) error {
	return nil
}

// ProcessData computes release metrics from the pre-built input.
func (p *ReleasePipeline) ProcessData() error {
	rm, metricWarnings, err := metrics.BuildReleaseMetrics(context.Background(), p.Input)
	if err != nil {
		return err
	}
	p.Result = rm
	p.Warnings = append(p.Warnings, metricWarnings...)
	return nil
}

// Render writes the release metrics in the requested format.
func (p *ReleasePipeline) Render(rc format.RenderContext) error {
	repo := p.Owner + "/" + p.Repo
	switch rc.Format {
	case format.JSON:
		return format.WriteReleaseJSON(rc.Writer, repo, p.Result, p.Warnings)
	case format.Markdown:
		return format.WriteReleaseMarkdown(rc, p.Result, p.Warnings)
	default:
		return format.WriteReleasePretty(rc, p.Result, p.Warnings)
	}
}
