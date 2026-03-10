package cycletime

import (
	"context"

	"github.com/bitsbyme/gh-velocity/internal/metrics"
	"github.com/bitsbyme/gh-velocity/internal/model"
)

// IssueStrategy measures cycle time as issue created → issue closed.
// This is the default strategy and requires no additional API calls.
type IssueStrategy struct{}

func (s *IssueStrategy) Name() string { return "issue" }

func (s *IssueStrategy) Compute(_ context.Context, input Input) model.Metric {
	if input.Issue == nil {
		return model.Metric{}
	}
	start := &model.Event{Time: input.Issue.CreatedAt, Signal: model.SignalIssueCreated}
	if input.Issue.ClosedAt == nil {
		return model.Metric{Start: start}
	}
	end := &model.Event{Time: *input.Issue.ClosedAt, Signal: model.SignalIssueClosed}
	return metrics.NewMetric(start, end)
}
