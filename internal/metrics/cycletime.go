package metrics

import (
	"context"
	"fmt"

	gh "github.com/bitsbyme/gh-velocity/internal/github"
	"github.com/bitsbyme/gh-velocity/internal/model"
)

// CycleTimeStrategy computes cycle time for a single work item.
type CycleTimeStrategy interface {
	// Name returns the strategy identifier ("issue" or "pr").
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

// IssueStrategy measures cycle time as "work started" → issue closed.
// "Work started" is detected from lifecycle config (project board status change).
// When no project board is configured (ProjectID is empty), Compute returns a
// zero Metric — the signal is unavailable.
type IssueStrategy struct {
	Client        *gh.Client
	ProjectID     string   // resolved from config project.url via ResolveProject
	StatusFieldID string   // resolved from config project.status_field
	BacklogStatus []string // from lifecycle.backlog.project_status
}

func (s *IssueStrategy) Name() string { return model.StrategyIssue }

func (s *IssueStrategy) Compute(ctx context.Context, input CycleTimeInput) model.Metric {
	if input.Issue == nil {
		return model.Metric{}
	}
	// No project configured — cycle time signal unavailable.
	if s.Client == nil || s.ProjectID == "" {
		return model.Metric{}
	}
	// Use the first backlog status for the GetProjectStatus call.
	backlog := ""
	if len(s.BacklogStatus) > 0 {
		backlog = s.BacklogStatus[0]
	}
	ps, err := s.Client.GetProjectStatus(ctx, input.Issue.Number, s.ProjectID, s.StatusFieldID, backlog)
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

// PRStrategy measures cycle time as PR created → PR merged.
type PRStrategy struct{}

func (s *PRStrategy) Name() string { return model.StrategyPR }

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
