// Package model defines shared domain types used across internal packages.
// These are pure data structs with no API or external dependency.
package model

import "time"

// Issue represents a GitHub issue with the fields needed for metrics.
type Issue struct {
	Number    int
	Title     string
	State     string // "open" or "closed"
	Labels    []string
	CreatedAt time.Time
	ClosedAt  *time.Time
	URL       string
}

// Commit represents a git commit.
type Commit struct {
	SHA       string
	Message   string
	AuthoredAt time.Time
	URL       string
}

// Release represents a GitHub release or git tag.
type Release struct {
	TagName     string
	Name        string
	CreatedAt   time.Time
	PublishedAt *time.Time
	URL         string
	IsDraft     bool
	IsPrerelease bool
}

// IssueMetrics holds computed metrics for a single issue within a release.
type IssueMetrics struct {
	Issue        Issue
	LeadTime     *time.Duration
	CycleTime    *time.Duration
	ReleaseLag   *time.Duration
	CommitCount  int
}

// ReleaseMetrics holds computed metrics for an entire release.
type ReleaseMetrics struct {
	Tag             string
	PreviousTag     string
	Date            time.Time
	Issues          []IssueMetrics
	TotalIssues     int
	BugCount        int
	FeatureCount    int
	OtherCount      int
	BugRatio        float64
	FeatureRatio    float64
	OtherRatio      float64
	Cadence         *time.Duration // time since previous release
	IsHotfix        bool
	LeadTimeStats   Stats
	CycleTimeStats  Stats
	ReleaseLagStats Stats
}

// Stats holds aggregate statistics for a set of durations.
type Stats struct {
	Count  int
	Mean   *time.Duration
	Median *time.Duration
	StdDev *time.Duration // sample standard deviation; nil if N < 2
}
