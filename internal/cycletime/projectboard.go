package cycletime

import (
	"context"

	gh "github.com/bitsbyme/gh-velocity/internal/github"
	"github.com/bitsbyme/gh-velocity/internal/model"
)

// ProjectBoardStrategy measures cycle time as project status change
// (out of backlog) → issue closed. Requires GitHub Projects v2 config.
type ProjectBoardStrategy struct {
	Client        *gh.Client
	ProjectID     string
	StatusFieldID string
	BacklogStatus string
}

func (s *ProjectBoardStrategy) Name() string { return "project-board" }

func (s *ProjectBoardStrategy) Compute(ctx context.Context, input Input) model.Metric {
	if input.Issue == nil {
		return model.Metric{}
	}
	ps, err := s.Client.GetProjectStatus(ctx, input.Issue.Number, s.ProjectID, s.StatusFieldID, s.BacklogStatus)
	if err != nil || ps.CycleStart == nil {
		return model.Metric{}
	}
	if ps.InBacklog {
		return model.Metric{}
	}
	start := &model.Event{
		Time:   ps.CycleStart.Time,
		Signal: model.SignalStatusChange,
		Detail: ps.CycleStart.Detail,
	}
	if input.Issue.ClosedAt == nil {
		return model.Metric{Start: start}
	}
	end := &model.Event{Time: *input.Issue.ClosedAt, Signal: model.SignalIssueClosed}
	return model.NewMetric(start, end)
}
