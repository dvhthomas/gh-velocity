package cycletime

import (
	"context"
	"fmt"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

// PRStrategy measures cycle time as PR created → PR merged.
// Returns a zero Metric when no linked PR is available.
type PRStrategy struct{}

func (s *PRStrategy) Name() string { return "pr" }

func (s *PRStrategy) Compute(_ context.Context, input Input) model.Metric {
	if input.PR == nil {
		return model.Metric{}
	}
	start := &model.Event{
		Time:   input.PR.CreatedAt,
		Signal: model.SignalPRCreated,
		Detail: fmt.Sprintf("PR #%d", input.PR.Number),
	}
	if input.PR.MergedAt == nil {
		return model.Metric{Start: start}
	}
	end := &model.Event{Time: *input.PR.MergedAt, Signal: model.SignalPRMerged}
	return model.NewMetric(start, end)
}
