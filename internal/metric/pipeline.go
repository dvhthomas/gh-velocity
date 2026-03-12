// Package metric defines the Pipeline interface that every metric command implements.
// The three-phase lifecycle (GatherData → ProcessData → Render) provides compile-time
// safety: forget to implement Render and it won't compile.
package metric

import (
	"context"

	"github.com/bitsbyme/gh-velocity/internal/format"
)

// Pipeline defines the three-phase lifecycle every metric command follows.
// Command-specific parameters (issue number, --since, etc.) are captured
// by the struct's constructor, NOT passed through interface methods.
type Pipeline interface {
	// GatherData fetches raw data from GitHub API, GraphQL, or local git.
	// For partial failures (e.g., PR lookup fails but issues succeed),
	// store warnings internally and return nil. Only return error for
	// total failures that prevent any useful output.
	GatherData(ctx context.Context) error

	// ProcessData computes metrics from gathered data. No I/O.
	// This is the primary unit test target: inject fake data, assert results.
	ProcessData() error

	// Render writes the processed result in the requested format.
	// No computation — pure output. Uses rc.Format and rc.Writer.
	Render(rc format.RenderContext) error
}

// RunPipeline executes the three-phase lifecycle in order.
// Post logic (--post flag) stays outside Pipeline — the command's RunE
// wraps the writer before calling this.
func RunPipeline(ctx context.Context, p Pipeline, rc format.RenderContext) error {
	if err := p.GatherData(ctx); err != nil {
		return err
	}
	if err := p.ProcessData(); err != nil {
		return err
	}
	return p.Render(rc)
}
