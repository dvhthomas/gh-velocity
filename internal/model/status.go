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
	LeadTime    *time.Duration // median lead time of closed issues (created → closed)
	LeadTimeP90 *time.Duration // p90 lead time (slowest 10% threshold)
	CycleTime   *time.Duration // median cycle time of merged PRs (created → merged)
	Releases    int            // releases published in the lookback window
	AIAssisted  int            // PRs merged with AI assistance
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
