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
	SHA        string
	Message    string
	AuthoredAt time.Time
	URL        string
}

// PR represents a GitHub pull request with fields needed for metrics.
type PR struct {
	Number    int
	Title     string
	State     string
	Labels    []string
	CreatedAt time.Time
	MergedAt  *time.Time
	URL       string
}

// Release represents a GitHub release or git tag.
type Release struct {
	TagName      string
	Name         string
	Body         string // release notes body (used by changelog strategy)
	CreatedAt    time.Time
	PublishedAt  *time.Time
	URL          string
	IsDraft      bool
	IsPrerelease bool
}

// IssueMetrics holds computed metrics for a single issue within a release.
type IssueMetrics struct {
	Issue            Issue
	LeadTime         *time.Duration
	CycleTime        *time.Duration
	ReleaseLag       *time.Duration
	CommitCount      int
	LeadTimeOutlier  bool // flagged by IQR method
	CycleTimeOutlier bool
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

// DiscoveredItem represents an issue or PR found by a linking strategy.
type DiscoveredItem struct {
	Issue    *Issue   // nil if PR-only (no linked issue)
	PR       *PR      // nil if discovered via commit-ref without a PR
	Commits  []Commit // commits associated with this item
	Strategy string   // "pr-link", "commit-ref", or "changelog"
}

// StrategyResult holds items found by a single strategy.
type StrategyResult struct {
	Name  string // "pr-link", "commit-ref", "changelog"
	Items []DiscoveredItem
}

// ScopeResult holds the output of running all strategies for a release.
type ScopeResult struct {
	Tag         string
	PreviousTag string
	Strategies  []StrategyResult
	Merged      []DiscoveredItem // deduplicated union
}

// CategoryConfig defines a user-defined classification category.
type CategoryConfig struct {
	Name     string   // e.g., "bug", "feature", "regression"
	Matchers []string // e.g., ["label:bug", "type:Bug", "title:/fix/i"]
}

// Stats holds aggregate statistics for a set of durations.
type Stats struct {
	Count         int
	Mean          *time.Duration
	Median        *time.Duration
	StdDev        *time.Duration // sample standard deviation; nil if N < 2
	P90           *time.Duration // 90th percentile; nil if N < 2
	P95           *time.Duration // 95th percentile; nil if N < 2
	OutlierCutoff *time.Duration // Q3 + 1.5*IQR; values above are outliers
	OutlierCount  int            // number of values above the cutoff
}
