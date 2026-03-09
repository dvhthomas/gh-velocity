package metrics

import (
	"time"

	"github.com/bitsbyme/gh-velocity/internal/model"
)

// LeadTime calculates the lead time for an issue: created → closed.
// Returns nil if the issue is still open.
func LeadTime(issue model.Issue) *time.Duration {
	if issue.ClosedAt == nil {
		return nil
	}
	d := issue.ClosedAt.Sub(issue.CreatedAt)
	return &d
}
