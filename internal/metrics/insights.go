package metrics

import (
	"time"

	"github.com/dvhthomas/gh-velocity/internal/model"
)

// ComputeInsights derives talking points from a MyWeekResult.
// Lead time is computed from closed issues (created → closed).
// Cycle time uses pre-computed durations from the configured strategy
// (computed in the cmd layer which has access to config and API client).
// Pass nil cycleTimeDurations when cycle time is unavailable.
func ComputeInsights(r model.MyWeekResult, cycleTimeDurations []time.Duration) model.MyWeekInsights {
	var ins model.MyWeekInsights
	for _, iss := range r.IssuesOpen {
		s := model.IssueStatus(iss, r.Since, r.Until)
		switch s.Status {
		case model.StatusStale:
			ins.StaleIssues++
		case model.StatusNew:
			ins.NewIssues++
		}
	}
	for _, pr := range r.PRsOpen {
		nr := model.PRNeedsReview(pr, r.PRsNeedingReview)
		s := model.PRStatus(pr, nr, r.Since, r.Until)
		switch s.Status {
		case model.StatusNeedsReview:
			ins.PRsNeedingReview++
		case model.StatusNew:
			ins.NewPRs++
		}
	}
	ins.PRsAwaitingMyReview = len(r.PRsAwaitingMyReview)
	ins.Releases = len(r.Releases)

	// Lead time: median of created → closed for closed issues
	if len(r.IssuesClosed) > 0 {
		var durations []time.Duration
		for _, iss := range r.IssuesClosed {
			if iss.ClosedAt != nil {
				d := iss.ClosedAt.Sub(iss.CreatedAt)
				if d > 0 {
					durations = append(durations, d)
				}
			}
		}
		if stats := ComputeStats(durations); stats.Median != nil {
			ins.LeadTime = stats.Median
		}
	}

	// Cycle time: from pre-computed durations (strategy-aware)
	if len(cycleTimeDurations) > 0 {
		if stats := ComputeStats(cycleTimeDurations); stats.Median != nil {
			ins.CycleTime = stats.Median
		}
	}

	return ins
}
