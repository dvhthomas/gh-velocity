// Package cycletime implements cycle-time computation strategies.
// Each strategy defines how to measure the start and end of active work
// on an issue or PR.
package cycletime

import (
	"context"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

// Strategy computes cycle time for a single work item.
type Strategy interface {
	// Name returns the strategy identifier ("issue", "pr", "project-board").
	Name() string
	// Compute returns a Metric with start/end events and duration.
	// Returns a zero Metric when the required signal is not available.
	Compute(ctx context.Context, input Input) model.Metric
}

// Input provides the data each strategy needs.
type Input struct {
	Issue   *model.Issue   // nil if PR-only
	PR      *model.PR      // from linking strategies, may be nil
	Commits []model.Commit // from linking strategies, may be empty
}
