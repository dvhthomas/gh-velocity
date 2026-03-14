// Package model defines shared domain types used across internal packages.
// These are pure data structs with no API or external dependency.
package model

import "time"

// Signal name constants for consistent use across metrics.
const (
	SignalIssueCreated     = "issue-created"
	SignalIssueClosed      = "issue-closed"
	SignalStatusChange     = "status-change"
	SignalLabel            = "label"
	SignalLabelAdded       = "label-added"
	SignalPRCreated        = "pr-created"
	SignalPRMerged         = "pr-merged"
	SignalAssigned         = "assigned"
	SignalCommit           = "commit"
	SignalReleasePublished = "release-published"
)

// Cycle time strategy identifiers.
const (
	StrategyIssue = "issue"
	StrategyPR    = "pr"
)

// Event represents a point in time during an issue or PR's lifecycle.
type Event struct {
	Time   time.Time
	Signal string // one of the Signal* constants
	Detail string // e.g., "PR #42: title" or "Backlog -> In progress"
}

// Metric represents a measured duration between two events.
// Start and End may be nil for in-progress or unmeasured metrics.
type Metric struct {
	Start    *Event
	End      *Event
	Duration *time.Duration
}

// NewMetric creates a Metric from start and end events, computing Duration
// when both are present.
func NewMetric(start, end *Event) Metric {
	m := Metric{Start: start, End: end}
	if start != nil && end != nil {
		d := end.Time.Sub(start.Time)
		m.Duration = &d
	}
	return m
}

// Issue represents a GitHub issue with the fields needed for metrics.
type Issue struct {
	Number      int
	Title       string
	State       string // "open" or "closed"
	StateReason string // "completed", "not_planned", or "" (for open issues)
	Labels      []string
	IssueType   string // GitHub Issue Type (from GraphQL); empty for REST-sourced issues
	CreatedAt   time.Time
	ClosedAt    *time.Time
	UpdatedAt   time.Time // last activity timestamp from GitHub
	URL         string
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
	Author    string // GitHub login of the PR author
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
	Category         string // assigned category from classifier
	LeadTime         Metric
	CycleTime        Metric
	ReleaseLag       Metric
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
	CategoryNames   []string           // ordered category names (e.g., ["bug", "feature", "other"])
	CategoryCounts  map[string]int     // category name -> count
	CategoryRatios  map[string]float64 // category name -> ratio
	Cadence         *time.Duration     // time since previous release
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
	Name     string   `yaml:"name" json:"name"`   // e.g., "bug", "feature", "regression"
	Matchers []string `yaml:"match" json:"match"` // e.g., ["label:bug", "type:Bug", "title:/fix/i"]
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
	NegativeCount int            // durations < 0 filtered before computation
}

// ProjectItem represents an item on a GitHub Projects v2 board.
type ProjectItem struct {
	ContentType string // "Issue", "PullRequest", or "DraftIssue"
	Number      int    // 0 for drafts
	Title       string
	Repo        string     // "owner/repo" from content.repository.nameWithOwner
	URL         string     // content URL (issue or PR URL)
	Status      string     // current board status
	StatusAt    *time.Time // when status was last set (updatedAt on field value)
	CreatedAt   time.Time
	UpdatedAt   time.Time // last activity timestamp from GitHub
	Labels      []string
	IssueType   string // GitHub Issue Type; empty for PRs and DraftIssues
}

// StalenessLevel classifies how recently an item had activity.
type StalenessLevel string

const (
	StalenessActive StalenessLevel = "ACTIVE" // activity within 3 days
	StalenessAging  StalenessLevel = "AGING"  // 3-7 days since activity
	StalenessStale  StalenessLevel = "STALE"  // >7 days since activity
)

// WIPItem represents an in-progress work item for display.
type WIPItem struct {
	Number    int
	Title     string
	Status    string
	Age       time.Duration
	Repo      string // populated for cross-repo board views
	Kind      string // "Issue", "PullRequest", "DraftIssue"
	URL       string
	Labels    []string
	UpdatedAt time.Time      // last activity timestamp, for staleness detection
	Staleness StalenessLevel // computed from UpdatedAt
}

// StatsResult holds all dashboard sections for output.
type StatsResult struct {
	Repository        string
	Since             time.Time
	Until             time.Time
	LeadTime          *Stats
	CycleTime         *Stats
	CycleTimeStrategy string // StrategyIssue or StrategyPR
	Throughput        *StatsThroughput
	WIPCount          *int
	Quality           *StatsQuality
	Warnings          []string
}

// StatsThroughput holds throughput counts.
type StatsThroughput struct {
	IssuesClosed int
	PRsMerged    int
}

// ThroughputResult holds standalone throughput output for the flow throughput command.
type ThroughputResult struct {
	Repository   string
	Since        time.Time
	Until        time.Time
	IssuesClosed int
	PRsMerged    int
}

// MyWeekResult holds the "my week" summary for one user.
type MyWeekResult struct {
	Login        string
	Repo         string
	Since        time.Time
	Until        time.Time
	// Lookback: what happened
	IssuesClosed []Issue
	PRsMerged    []PR
	PRsReviewed  []PR
	// Lookahead: what's in progress
	IssuesOpen      []Issue // open issues assigned to me
	PRsOpen         []PR    // open PRs I authored
	PRsNeedingReview []PR   // open PRs with zero reviews (subset of PRsOpen)
	// Review pressure: PRs from others waiting on me
	PRsAwaitingMyReview []PR // open PRs where I'm a requested reviewer
	// Releases published in the lookback period
	Releases []Release
}

// ReviewPressureResult holds the review queue for a repository.
type ReviewPressureResult struct {
	Repository     string
	AwaitingReview []PRAwaitingReview
}

// PRAwaitingReview represents a PR waiting for review.
type PRAwaitingReview struct {
	Number  int
	Title   string
	URL     string
	Age     time.Duration // since PR was opened
	IsStale bool          // >48h without review
}

// StatsQuality holds defect rate metrics.
type StatsQuality struct {
	BugCount    int
	TotalIssues int
	DefectRate  float64
}

// Insight is a human-readable observation derived from the data.
// Message may contain inline markdown (links, bold, code).
type Insight struct {
	Message string
}

// Provenance captures how a result was generated so consumers can
// interpret the data and reproduce the run.
type Provenance struct {
	Command string            // CLI invocation that produced this result
	Config  map[string]string // key config values relevant to interpretation
}

// VelocityResult holds the output of the velocity pipeline.
type VelocityResult struct {
	Repository    string
	Unit          string  // "issues" or "prs"
	EffortUnit    string  // "pts", "items", etc.
	EffortDetail  EffortDetail
	Provenance    Provenance
	Insights      []Insight
	Warnings      []string // user-facing warnings (e.g., board item cap exceeded)
	Current       *IterationVelocity
	History       []IterationVelocity
	AvgVelocity   float64
	AvgCompletion float64
	StdDev        float64
}

// EffortDetail describes the effort strategy used for velocity measurement.
type EffortDetail struct {
	Strategy     string         // "count", "attribute", "numeric"
	Matchers     []EffortMatch  // for attribute strategy
	NumericField string         // for numeric strategy
}

// EffortMatch is a display-friendly effort matcher entry.
type EffortMatch struct {
	Query string
	Value float64
}

// IterationVelocity holds velocity metrics for a single iteration.
type IterationVelocity struct {
	Name             string
	Start            time.Time
	End              time.Time
	Velocity         float64 // effort completed
	Committed        float64 // effort committed
	CompletionPct    float64 // velocity/committed * 100
	ItemsDone        int
	ItemsTotal       int
	CarryOver        int
	NotAssessed      int
	NotAssessedItems []int   // issue/PR numbers
	Trend            string  // "▲", "▼", "─"
	DayOfCycle       int     // days elapsed since iteration start (0 if not current)
	TotalDays        int     // total iteration length in days (0 if not current)
}

// Iteration represents a project iteration (sprint) period.
type Iteration struct {
	ID        string
	Title     string
	StartDate time.Time
	Duration  int       // days
	EndDate   time.Time // computed: StartDate + Duration days
}

// VelocityItem represents a work item with iteration and effort data
// from a project board, used by the velocity pipeline.
type VelocityItem struct {
	ContentType string     // "Issue" or "PullRequest"
	Number      int
	Title       string
	Repo        string     // "owner/repo"
	State       string
	StateReason string     // "completed", "not_planned", ""
	ClosedAt    *time.Time
	MergedAt    *time.Time
	CreatedAt   time.Time
	Labels      []string
	IssueType   string
	IterationID string
	Effort      *float64          // from Number field, nil if unset
	Fields      map[string]string // project board field values (e.g., SingleSelect)
}

// IterationFieldConfig holds the configuration of a ProjectV2 Iteration field.
type IterationFieldConfig struct {
	Iterations          []Iteration
	CompletedIterations []Iteration
}
