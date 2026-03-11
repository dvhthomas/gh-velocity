package model

import (
	"math"
	"time"
)

// Status constants for open items.
const (
	StatusNew         = "new"          // created within the lookback window
	StatusNeedsReview = "needs_review" // PR with zero reviews, open 2+ days
	StatusStale       = "stale"        // no update in StaleThresholdDays+
	StatusActive      = "active"       // normal open item
)

// StaleThresholdDays is the number of days without an update before
// an issue is considered stale.
const StaleThresholdDays = 7

// NeedsReviewGraceDays is the minimum age (in days) before a PR
// with no reviews is flagged as needing review.
const NeedsReviewGraceDays = 2

// ItemStatus describes why an open item deserves attention.
type ItemStatus struct {
	Status    string // StatusNew, StatusNeedsReview, StatusStale, or StatusActive
	AgeDays   int    // days since creation
	StaleDays int    // days since last update (0 if not stale)
}

// IssueStatus computes the status of an open issue relative to the
// given lookback window [since, now].
func IssueStatus(iss Issue, since, now time.Time) ItemStatus {
	s := ItemStatus{
		AgeDays: DaysBetween(iss.CreatedAt, now),
		Status:  StatusActive,
	}
	switch {
	case !iss.CreatedAt.Before(since):
		s.Status = StatusNew
	case DaysBetween(iss.UpdatedAt, now) >= StaleThresholdDays:
		s.Status = StatusStale
		s.StaleDays = DaysBetween(iss.UpdatedAt, now)
	}
	return s
}

// PRStatus computes the status of an open PR. Set needsReview to true
// if the PR has received zero reviews (from a review:none search).
func PRStatus(pr PR, needsReview bool, since, now time.Time) ItemStatus {
	s := ItemStatus{
		AgeDays: DaysBetween(pr.CreatedAt, now),
		Status:  StatusActive,
	}
	switch {
	case !pr.CreatedAt.Before(since):
		s.Status = StatusNew
	case needsReview && s.AgeDays >= NeedsReviewGraceDays:
		s.Status = StatusNeedsReview
	}
	return s
}

// IsStale returns true if the issue has had no update in StaleThresholdDays.
func (iss Issue) IsStale(now time.Time) bool {
	return DaysBetween(iss.UpdatedAt, now) >= StaleThresholdDays
}

// PRNeedsReview returns true if the given PR number appears in the set.
func PRNeedsReview(pr PR, needingReview []PR) bool {
	for _, nr := range needingReview {
		if nr.Number == pr.Number {
			return true
		}
	}
	return false
}

// MyWeekInsights captures derived observations for 1:1 talking points.
type MyWeekInsights struct {
	StaleIssues         int // open issues with no recent update
	PRsNeedingReview    int // open PRs waiting for first review
	NewIssues           int // issues opened in the lookback window
	NewPRs              int // PRs opened in the lookback window
	PRsAwaitingMyReview int // PRs from others waiting on my review
	// Velocity metrics computed from lookback data
	LeadTime  *time.Duration // median lead time of closed issues (created → closed)
	CycleTime *time.Duration // median cycle time of merged PRs (created → merged)
	Releases  int            // releases published in the lookback window
}

// ComputeInsights derives talking points from a MyWeekResult.
func ComputeInsights(r MyWeekResult) MyWeekInsights {
	var ins MyWeekInsights
	for _, iss := range r.IssuesOpen {
		s := IssueStatus(iss, r.Since, r.Until)
		switch s.Status {
		case StatusStale:
			ins.StaleIssues++
		case StatusNew:
			ins.NewIssues++
		}
	}
	for _, pr := range r.PRsOpen {
		nr := PRNeedsReview(pr, r.PRsNeedingReview)
		s := PRStatus(pr, nr, r.Since, r.Until)
		switch s.Status {
		case StatusNeedsReview:
			ins.PRsNeedingReview++
		case StatusNew:
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
		if len(durations) > 0 {
			med := medianDuration(durations)
			ins.LeadTime = &med
		}
	}

	// Cycle time: median of created → merged for merged PRs
	if len(r.PRsMerged) > 0 {
		var durations []time.Duration
		for _, pr := range r.PRsMerged {
			if pr.MergedAt != nil {
				d := pr.MergedAt.Sub(pr.CreatedAt)
				if d > 0 {
					durations = append(durations, d)
				}
			}
		}
		if len(durations) > 0 {
			med := medianDuration(durations)
			ins.CycleTime = &med
		}
	}

	return ins
}

// medianDuration returns the median of a slice of durations.
func medianDuration(durations []time.Duration) time.Duration {
	n := len(durations)
	sorted := make([]time.Duration, n)
	copy(sorted, durations)
	// Simple insertion sort — small N
	for i := 1; i < n; i++ {
		key := sorted[i]
		j := i - 1
		for j >= 0 && sorted[j] > key {
			sorted[j+1] = sorted[j]
			j--
		}
		sorted[j+1] = key
	}
	if n%2 == 0 {
		return (sorted[n/2-1] + sorted[n/2]) / 2
	}
	return sorted[n/2]
}

// DaysBetween returns the number of whole days between two times.
// Returns 0 if b is before a (never negative).
func DaysBetween(a, b time.Time) int {
	d := int(math.Floor(b.Sub(a).Hours() / 24))
	if d < 0 {
		return 0
	}
	return d
}
