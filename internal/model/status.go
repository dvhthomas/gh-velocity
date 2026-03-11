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

// DaysBetween returns the number of whole days between two times.
func DaysBetween(a, b time.Time) int {
	return int(math.Floor(b.Sub(a).Hours() / 24))
}
