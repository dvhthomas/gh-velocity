package metrics

import (
	"context"
	"fmt"

	gh "github.com/bitsbyme/gh-velocity/internal/github"
	"github.com/bitsbyme/gh-velocity/internal/model"
)

// CycleTimeStrategy computes cycle time for a single work item.
type CycleTimeStrategy interface {
	// Name returns the strategy identifier ("issue", "pr", "project-board").
	Name() string
	// Compute returns a Metric with start/end events and duration.
	// Returns a zero Metric when the required signal is not available.
	Compute(ctx context.Context, input CycleTimeInput) model.Metric
}

// CycleTimeInput provides the data each strategy needs.
type CycleTimeInput struct {
	Issue   *model.Issue   // nil if PR-only
	PR      *model.PR      // from linking strategies, may be nil
	Commits []model.Commit // from linking strategies, may be empty
}

// IssueStrategy measures cycle time as issue created → issue closed.
type IssueStrategy struct{}

func (s *IssueStrategy) Name() string { return "issue" }

func (s *IssueStrategy) Compute(_ context.Context, input CycleTimeInput) model.Metric {
	if input.Issue == nil {
		return model.Metric{}
	}
	start := &model.Event{Time: input.Issue.CreatedAt, Signal: model.SignalIssueCreated}
	if input.Issue.ClosedAt == nil {
		return model.Metric{Start: start}
	}
	end := &model.Event{Time: *input.Issue.ClosedAt, Signal: model.SignalIssueClosed}
	return model.NewMetric(start, end)
}

// PRStrategy measures cycle time as PR created → PR merged.
type PRStrategy struct{}

func (s *PRStrategy) Name() string { return "pr" }

func (s *PRStrategy) Compute(_ context.Context, input CycleTimeInput) model.Metric {
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

// ProjectBoardStrategy measures cycle time as project status change
// (out of backlog) → issue closed. Requires GitHub Projects v2 config.
type ProjectBoardStrategy struct {
	Client        *gh.Client
	ProjectID     string
	StatusFieldID string
	BacklogStatus string
}

func (s *ProjectBoardStrategy) Name() string { return "project-board" }

func (s *ProjectBoardStrategy) Compute(ctx context.Context, input CycleTimeInput) model.Metric {
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
