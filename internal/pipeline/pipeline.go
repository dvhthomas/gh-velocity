// Package pipeline defines the Pipeline interface that every metric command implements.
// The three-phase lifecycle (GatherData → ProcessData → Render) provides compile-time
// safety: forget to implement Render and it won't compile.
//
// Each subcommand has its own subdirectory (e.g., pipeline/leadtime/) containing
// its Pipeline implementation, computation logic, and format functions.
// To add a new metric: create a new subdirectory, implement Pipeline, wire in cmd/.
package pipeline

import (
	"context"
	"fmt"

	"github.com/dvhthomas/gh-velocity/internal/format"
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

	// Warnings returns warnings accumulated during GatherData and ProcessData.
	Warnings() []string
}

// Optional interfaces checked by RunPipeline:
//   - Enricher: called between GatherData and ProcessData
//
// Optional interfaces checked by renderPipeline in cmd/:
//   - provenanceRenderer: controls provenance footer
//
// Keep this list short. If a third optional interface is needed,
// reconsider whether the Pipeline interface itself should change.

// Enricher is an optional interface. If a pipeline implements it,
// RunPipeline calls Enrich between GatherData and ProcessData.
// Used by WIP (IssueType enrichment) and issue detail pipeline.
type Enricher interface {
	Enrich(ctx context.Context) error
}

// RunResult holds the outcome of RunPipeline for the caller.
type RunResult struct {
	Warnings []string
}

// RunPipeline executes the three-phase lifecycle in order.
// Standalone commands should ALWAYS use this — never call phases directly.
// The report command is the one exception (see cmd/report.go).
func RunPipeline(ctx context.Context, p Pipeline, rc format.RenderContext) (RunResult, error) {
	if err := p.GatherData(ctx); err != nil {
		return RunResult{}, err
	}
	// Optional enrichment between gather and process.
	if e, ok := p.(Enricher); ok {
		if err := e.Enrich(ctx); err != nil {
			return RunResult{}, err
		}
	}
	if err := p.ProcessData(); err != nil {
		return RunResult{}, err
	}
	if err := p.Render(rc); err != nil {
		return RunResult{}, err
	}
	return RunResult{Warnings: p.Warnings()}, nil
}

// WarningCollector provides the Warnings() method for Pipeline implementations.
// Embed this in pipeline structs instead of manually declaring []string fields.
//
// Migration safety: when embedding WarningCollector in an existing struct,
// FIRST remove any existing `Warnings []string` field. Go silently shadows
// promoted methods when a field with the same name exists at the outer level.
type WarningCollector struct {
	warnings []string
}

// AddWarning appends a warning message.
func (wc *WarningCollector) AddWarning(msg string) {
	wc.warnings = append(wc.warnings, msg)
}

// AddWarningf appends a formatted warning message.
func (wc *WarningCollector) AddWarningf(format string, args ...any) {
	wc.warnings = append(wc.warnings, fmt.Sprintf(format, args...))
}

// Warnings returns accumulated warnings.
func (wc *WarningCollector) Warnings() []string {
	return wc.warnings
}
