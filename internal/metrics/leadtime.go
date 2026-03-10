package metrics

import (
	"github.com/bitsbyme/gh-velocity/internal/model"
)

// LeadTime calculates the lead time for an issue: created → closed.
// Returns a Metric with nil Duration if the issue is still open.
func LeadTime(issue model.Issue) model.Metric {
	start := &model.Event{
		Time:   issue.CreatedAt,
		Signal: model.SignalIssueCreated,
	}

	if issue.ClosedAt == nil {
		return model.Metric{Start: start}
	}

	end := &model.Event{
		Time:   *issue.ClosedAt,
		Signal: model.SignalIssueClosed,
	}
	return NewMetric(start, end)
}
