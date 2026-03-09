package metrics

import (
	"time"
)

// CycleTime calculates cycle time from first commit to end time.
// In pr mode, end is the PR merge time. In local mode, end is the last commit time.
// Returns nil if firstCommit or end is zero.
func CycleTime(firstCommit, end time.Time) *time.Duration {
	if firstCommit.IsZero() || end.IsZero() {
		return nil
	}
	d := end.Sub(firstCommit)
	return &d
}
