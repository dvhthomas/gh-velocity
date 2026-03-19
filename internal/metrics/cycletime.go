package metrics

import (
	"context"
	"fmt"

	gh "github.com/dvhthomas/gh-velocity/internal/github"
	"github.com/dvhthomas/gh-velocity/internal/model"
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
// "Work started" is detected from label timeline events matching InProgressMatch.
// When InProgressMatch is empty, Compute returns a zero Metric.
type IssueStrategy struct {
	Client          *gh.Client
	InProgressMatch []string // from lifecycle.in-progress.match (classify.Matcher syntax)
}

func (s *IssueStrategy) Name() string { return model.StrategyIssue }

func (s *IssueStrategy) Compute(ctx context.Context, input CycleTimeInput) model.Metric {
	if input.Issue == nil {
		return model.Metric{}
	}

	// Use label timeline events (immutable createdAt timestamps).
	if s.Client != nil && len(s.InProgressMatch) > 0 {
		return s.computeFromLabels(ctx, input)
	}

	// No signal source configured.
	return model.Metric{}
}

func (s *IssueStrategy) computeFromLabels(ctx context.Context, input CycleTimeInput) model.Metric {
	cs, err := s.Client.GetLabelCycleStart(ctx, input.Issue.Number, s.InProgressMatch)
	if err != nil || cs == nil {
		return model.Metric{}
	}
	start := &model.Event{
		Time:   cs.Time,
		Signal: model.SignalLabelAdded,
		Detail: cs.Detail,
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
