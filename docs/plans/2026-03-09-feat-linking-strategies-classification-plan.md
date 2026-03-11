---
title: "feat: Linking Strategies & Flexible Classification"
type: feat
status: completed
date: 2026-03-09
brainstorm: docs/brainstorms/2026-03-09-linking-strategies-brainstorm.md
---

# Linking Strategies & Flexible Classification

## Overview

Replace the single commit-message regex approach for discovering release contents with a strategy-based system that runs multiple linking strategies (pr-link, commit-ref, changelog) and merges results. Add a flexible classification system that matches on labels, issue types, and title patterns with user-defined categories. Add a `scope` validation command.

## Problem Statement

The current `linking.LinkCommitsToIssues()` uses a greedy `#N` regex that:
- Pulls in ancient issues (a commit mentioning `#1` links to the first issue ever created)
- Misses repos that link PRs to issues via GitHub's sidebar (no commit message reference)
- Returns 0 issues for PR-centric repos that don't reference issues in commits
- Can't discover PRs as work units

The current `quality.ClassifyIssues()` only checks exact label names (`bug`, `enhancement`) and:
- Can't match GitHub Issue Types (`type:Bug`)
- Can't match title patterns (`fix:`, `bug:`)
- Is fixed to three categories (bug, feature, other) with no customization

## Proposed Solution

### Strategy Pattern for Issue/PR Discovery

Three strategies run by default and results are merged (union):

1. **pr-link** — Search API finds merged PRs in date range, GraphQL `closingIssuesReferences` gets linked issues. Most accurate for teams using GitHub's PR-to-issue linking.
2. **commit-ref** — Regex on commit messages for closing keywords (`fixes #N`, `closes #N`, `resolves #N`). Default is strict (closing keywords only); bare `#N` matching is opt-in.
3. **changelog** — Parse the GitHub Release body for `#N` references.

Priority merge: when the same issue is found by multiple strategies, pr-link wins (PR closing reference is a stronger signal than commit mention). PR timestamps (`merged_at`) are used for cycle time when available.

### Flexible Classification

Replace fixed bug/feature/other with user-defined categories via config:

```yaml
quality:
  categories:
    bug:
      - label:bug
      - label:defect
      - type:Bug
    feature:
      - label:enhancement
      - type:Feature
    regression:
      - label:regression
      - title:/^regression:/i
```

Matcher types: `label:<name>` (case-insensitive), `type:<name>` (GitHub Issue Type), `title:<regex>`.
First match wins (evaluated in config order). Unmatched items → "other".

### `scope` Command

Validation command showing what each strategy discovers:

```
gh-velocity scope v1.5.0 --since v1.4.0 --repo owner/repo
```

Output grouped by strategy. Supports all output formats (pretty, json, markdown).

## Technical Approach

### New Types

#### `internal/model/types.go` — Add PR type and strategy result types

```go
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

// DiscoveredItem represents an issue or PR found by a linking strategy.
type DiscoveredItem struct {
    Issue       *Issue    // nil if PR-only (no linked issue)
    PR          *PR       // nil if discovered via commit-ref without a PR
    Commits     []Commit  // commits associated with this item
    Strategy    string    // "pr-link", "commit-ref", or "changelog"
}

// ScopeResult holds the output of running all strategies for a release.
type ScopeResult struct {
    Tag         string
    PreviousTag string
    Strategies  []StrategyResult
    Merged      []DiscoveredItem // deduplicated union
}

// StrategyResult holds items found by a single strategy.
type StrategyResult struct {
    Name  string           // "pr-link", "commit-ref", "changelog"
    Items []DiscoveredItem
}
```

#### `internal/model/types.go` — Add category types

```go
// CategoryConfig defines a user-defined classification category.
type CategoryConfig struct {
    Name     string   // e.g., "bug", "feature", "regression"
    Matchers []string // e.g., ["label:bug", "type:Bug", "title:/fix/i"]
}
```

### New Package: `internal/strategy/`

```
internal/strategy/
├── strategy.go      # Strategy interface + Runner (executes all, merges)
├── strategy_test.go
├── prlink.go        # pr-link strategy implementation
├── prlink_test.go
├── commitref.go     # commit-ref strategy (refactored from linking/)
├── commitref_test.go
├── changelog.go     # changelog strategy
├── changelog_test.go
└── merge.go         # Priority merge logic
└── merge_test.go
```

#### Strategy Interface

```go
// Strategy discovers issues and PRs belonging to a release.
type Strategy interface {
    Name() string
    Discover(ctx context.Context, input DiscoverInput) ([]model.DiscoveredItem, error)
}

// DiscoverInput provides the data each strategy needs.
type DiscoverInput struct {
    Owner       string
    Repo        string
    Tag         string
    PreviousTag string
    Commits     []model.Commit        // commits between tags
    Release     *model.Release        // GitHub release (for changelog body)
    Client      *github.Client        // for API calls
    CommitRefPatterns []string         // ["closes"] or ["closes", "refs"]
}

// Runner executes all strategies and merges results.
type Runner struct {
    strategies []Strategy
}

func NewRunner(strategies ...Strategy) *Runner { ... }

func (r *Runner) Run(ctx context.Context, input DiscoverInput) (*model.ScopeResult, error) {
    // Run all strategies, collect results
    // Call Merge() to deduplicate
    // Return ScopeResult with per-strategy and merged results
}
```

#### Priority Merge Logic (`merge.go`)

```go
// Merge deduplicates items across strategies.
// Priority: pr-link > commit-ref > changelog.
// When the same issue appears from multiple strategies, the higher-priority
// strategy's data wins (PR timestamps, linked PR info).
func Merge(strategyResults []model.StrategyResult) []model.DiscoveredItem {
    // 1. Build map of issue number → best DiscoveredItem
    // 2. pr-link items always win
    // 3. commit-ref only contributes items not already found by pr-link
    // 4. changelog only contributes items not already found by either
    // 5. Return merged slice
}
```

### GraphQL Client Addition

#### `internal/github/client.go` — Add GraphQL client

```go
type Client struct {
    rest  *ghapi.RESTClient
    gql   *ghapi.GraphQLClient  // new
    owner string
    repo  string
}

func NewClient(owner, repo string) (*Client, error) {
    rest, err := ghapi.DefaultRESTClient()
    if err != nil {
        return nil, fmt.Errorf("github: create REST client: %w", err)
    }
    gql, err := ghapi.DefaultGraphQLClient()
    if err != nil {
        return nil, fmt.Errorf("github: create GraphQL client: %w", err)
    }
    return &Client{rest: rest, gql: gql, owner: owner, repo: repo}, nil
}
```

#### `internal/github/pullrequests.go` — New file for PR operations

```go
// SearchMergedPRs finds all PRs merged in the given date range using the search API.
// Uses: GET /search/issues?q=repo:{owner}/{repo}+is:pr+is:merged+merged:{start}..{end}
func (c *Client) SearchMergedPRs(ctx context.Context, start, end time.Time) ([]model.PR, error) { ... }

// FetchPRLinkedIssues fetches linked issues for multiple PRs in a single
// batched GraphQL query using aliases.
// Query: { repository(owner, name) { pr101: pullRequest(101) { closingIssuesReferences { ... } } ... } }
func (c *Client) FetchPRLinkedIssues(ctx context.Context, prNumbers []int) (map[int][]model.Issue, error) { ... }
```

**GraphQL batch query for linked issues:**

```graphql
{
  repository(owner: "...", name: "...") {
    pr101: pullRequest(number: 101) {
      title
      mergedAt
      closingIssuesReferences(first: 10) {
        nodes {
          number
          title
          state
          createdAt
          closedAt
          labels(first: 10) { nodes { name } }
        }
      }
    }
    pr102: pullRequest(number: 102) {
      ...same fragment...
    }
  }
}
```

Batch up to 20 PRs per GraphQL query to stay well under complexity limits.

### Classification System

#### `internal/classify/classify.go` — New package

```go
// Classifier evaluates items against ordered category rules.
type Classifier struct {
    categories []model.CategoryConfig
}

func NewClassifier(categories []model.CategoryConfig) *Classifier { ... }

// Classify returns the category name for an issue/PR. Returns "other" if no match.
func (c *Classifier) Classify(item ClassifyInput) string {
    // Evaluate categories in order; first match wins
}

type ClassifyInput struct {
    Labels    []string
    IssueType string // GitHub Issue Type (from GraphQL)
    Title     string
}

// ParseMatcher parses a matcher string like "label:bug", "type:Bug", "title:/regex/i".
func ParseMatcher(s string) (Matcher, error) { ... }

// Matcher interface for different match types.
type Matcher interface {
    Matches(input ClassifyInput) bool
}
```

Three matcher implementations:
- `LabelMatcher` — case-insensitive label name match
- `TypeMatcher` — exact match on GitHub Issue Type field
- `TitleMatcher` — regex match on title

### Config Changes

#### `internal/config/config.go` — Extend QualityConfig

```go
type QualityConfig struct {
    // Existing (kept for backward compatibility)
    BugLabels         []string `yaml:"bug_labels" json:"bug_labels"`
    FeatureLabels     []string `yaml:"feature_labels" json:"feature_labels"`
    HotfixWindowHours float64  `yaml:"hotfix_window_hours" json:"hotfix_window_hours"`

    // New: user-defined categories (takes precedence over bug_labels/feature_labels when set)
    Categories []CategoryEntry `yaml:"categories" json:"categories,omitempty"`
}

// CategoryEntry preserves YAML ordering. Parsed from map syntax in YAML.
type CategoryEntry struct {
    Name     string   `yaml:"name" json:"name"`
    Matchers []string `yaml:"matchers" json:"matchers"`
}

// CommitRefConfig controls the commit-ref strategy behavior.
type CommitRefConfig struct {
    Patterns []string `yaml:"patterns" json:"patterns"` // ["closes"] or ["closes", "refs"]
}
```

**Backward compatibility:** When `categories` is empty but `bug_labels`/`feature_labels` are set, auto-generate categories from them:
- `bug` category with `label:<name>` for each bug label
- `feature` category with `label:<name>` for each feature label

When `categories` is set, it takes full precedence — `bug_labels`/`feature_labels` are ignored with a warning if both are set.

**Config YAML syntax for categories** (ordered list, not map, for deterministic evaluation):

```yaml
quality:
  categories:
    - name: bug
      matchers:
        - label:bug
        - label:defect
        - type:Bug
    - name: feature
      matchers:
        - label:enhancement
        - type:Feature
    - name: regression
      matchers:
        - label:regression
```

### Time Window Guardrails

```go
const (
    DefaultMaxWindowDays = 31  // 1 month
    HardMaxWindowDays    = 90  // 3 months
)
```

Enforced in strategy Runner before executing strategies. The time window is calculated from the creation dates of the two tags. If the window exceeds the configured max, return an error with a clear message suggesting narrowing the range.

Config:
```yaml
max_window_days: 60  # configurable up to 90
```

### Inline Flag Overrides

```
--bug-match "label:bug,type:Bug"        # replaces bug category matchers
--feature-match "label:enhancement"      # replaces feature category matchers
--category "regression=label:regression" # adds/replaces a category
```

Same syntax as config values. Flags **replace** (not merge with) config values for that run.

### `scope` Command

#### `cmd/scope.go`

```go
func NewScopeCmd() *cobra.Command {
    var sinceFlag string
    cmd := &cobra.Command{
        Use:   "scope <tag>",
        Short: "Show what a release contains before computing metrics",
        Long:  "Validate which issues and PRs each strategy discovers for a release.",
        Args:  cobra.ExactArgs(1),
        RunE: func(cmd *cobra.Command, args []string) error {
            // 1. Build strategy Runner with all three strategies
            // 2. Run all strategies
            // 3. Output ScopeResult grouped by strategy
        },
    }
    cmd.Flags().StringVar(&sinceFlag, "since", "", "Previous tag for commit range")
    return cmd
}
```

**Pretty output example:**
```
Scope: v1.5.0 (since v1.4.0)
═══════════════════════════

Strategy: pr-link (12 items)
  #45  Add user dashboard        (PR #67, merged 2026-03-01)
  #38  Fix login timeout         (PR #64, merged 2026-02-28)
  ...

Strategy: commit-ref (3 items)
  #45  Add user dashboard        (also found by pr-link)
  #12  Update README             (unique to commit-ref)
  ...

Strategy: changelog (1 item)
  #45  Add user dashboard        (also found by pr-link)

Merged: 13 unique items (12 issues, 8 PRs)
```

### Integration with Existing Release Command

The `cmd/release.go` `gatherReleaseData()` function currently calls `linking.LinkCommitsToIssues()`. This will be replaced with the strategy Runner:

```go
// Before:
issueCommits := linking.LinkCommitsToIssues(commits)

// After:
runner := strategy.NewRunner(
    strategy.NewPRLink(client),
    strategy.NewCommitRef(deps.Config.CommitRef.Patterns),
    strategy.NewChangelog(),
)
scopeResult, err := runner.Run(ctx, strategy.DiscoverInput{...})
// Convert scopeResult.Merged to the existing metrics input format
```

The existing `linking/` package is preserved but becomes an internal detail of the `commitref` strategy.

### API Cost Analysis

**Per release run (~22 calls):**
1. Compare tags (1 REST call)
2. Search merged PRs in date range (1-2 REST calls, paginated)
3. Batch GraphQL for `closingIssuesReferences` (1-2 GraphQL calls, 20 PRs per batch)
4. Fetch individual issues not covered by PR links (variable, typically 5-15 REST calls)
5. Get release info (1 REST call)
6. Get previous release info (1 REST call)

At 5,000 REST calls/hour: supports ~225 runs/hour.

**Search API 1000-result cap:** The GitHub search API returns at most 1,000 results. For releases with >1,000 merged PRs in the date range (very unlikely for a typical release window of 1-3 months), warn on stderr and proceed with the first 1,000.

## Implementation Phases

### Phase 1: Strategy Interface + PR-Link Strategy

**Goal:** Working strategy pattern with pr-link as the first strategy. The release command uses the new system.

- [x] Add `PR` type to `internal/model/types.go`
- [x] Add `DiscoveredItem`, `ScopeResult`, `StrategyResult` to `internal/model/types.go`
- [x] Add `CategoryConfig` and `CategoryEntry` types to `internal/model/types.go`
- [x] Add GraphQL client to `internal/github/client.go` (`api.DefaultGraphQLClient()`)
- [x] Create `internal/github/pullrequests.go` — `SearchMergedPRs()`, `FetchPRLinkedIssues()`
- [ ] Write `internal/github/pullrequests_test.go` — table-driven tests with httptest mocks
- [x] Create `internal/strategy/strategy.go` — `Strategy` interface, `Runner`, `DiscoverInput`
- [x] Create `internal/strategy/prlink.go` — pr-link strategy using search API + GraphQL batch
- [ ] Write `internal/strategy/prlink_test.go`
- [x] Create `internal/strategy/merge.go` — priority merge logic
- [x] Write `internal/strategy/merge_test.go` — test dedup, priority ordering, edge cases
- [x] Add time window constants and enforcement to strategy Runner
- [x] Add `max_window_days` to config
- [x] Update `cmd/release.go` to use strategy Runner instead of `linking.LinkCommitsToIssues()`
- [x] Run `task test` and `task quality`

**Success criteria:**
- `gh-velocity release v2.65.0 --repo cli/cli --since v2.64.0` uses pr-link strategy and discovers issues via PR links
- Strategy Runner enforces time window guardrails
- All existing tests pass (backward compatible)

### Phase 2: Commit-Ref + Changelog Strategies

**Goal:** All three strategies implemented and running as union.

- [x] Create `internal/strategy/commitref.go` — wraps existing `linking.ExtractIssueNumbers()`, adds closing-keyword-only default
- [x] Write `internal/strategy/commitref_test.go`
- [x] Add `commit_ref.patterns` to config (`CommitRefConfig` struct)
- [x] Create `internal/strategy/changelog.go` — parse release body for `#N` references
- [x] Write `internal/strategy/changelog_test.go`
- [x] Update Runner to instantiate all three strategies
- [x] Verify union merge with all three contributing
- [x] Run `task test` and `task quality`

**Success criteria:**
- All three strategies contribute to the merged result
- Commit-ref defaults to closing keywords only (bare `#N` is opt-in)
- Duplicate issues are correctly deduplicated with priority merge

### Phase 3: Flexible Classification

**Goal:** User-defined classification categories replace fixed bug/feature/other.

- [x] Create `internal/classify/classify.go` — `Classifier`, `ParseMatcher()`, matcher types
- [x] Write `internal/classify/classify_test.go` — table-driven tests for each matcher type
- [x] Add `Categories` field to `QualityConfig` in config
- [x] Implement backward compatibility: auto-generate categories from `bug_labels`/`feature_labels`
- [x] ~~Add `--bug-match`, `--feature-match`, `--category` inline flags to root command~~ (skipped — config-only)
- [x] Update `metrics.BuildReleaseMetrics()` to use `Classifier` instead of label arrays
- [x] Update release and scope output to show user-defined categories (not just bug/feature/other)
- [x] Run `task test` and `task quality`

**Success criteria:**
- Custom categories in config are evaluated correctly
- `bug_labels`/`feature_labels` still work when `categories` is not set
- Inline flags override config categories
- Release output shows custom category breakdown

### Phase 4: Scope Command

**Goal:** Working `scope` command for validating release contents.

- [x] Create `cmd/scope.go` — `scope <tag>` with `--since` flag
- [x] Implement pretty output grouped by strategy
- [x] Implement JSON output (`ScopeResult` struct)
- [x] Implement markdown output
- [x] Register in `cmd/root.go`
- [x] Add scope smoke tests to `scripts/smoke-test.sh`
- [x] Run `task test` and `task quality`

**Success criteria:**
- `gh-velocity scope v2.65.0 --since v2.64.0 --repo cli/cli` shows what each strategy found
- Output clearly marks duplicates across strategies
- All three formats work (pretty, json, markdown)

### Phase 5: Polish & Documentation

- [x] Update smoke tests for new strategy behavior
- [ ] Add config documentation for new fields (`categories`, `commit_ref`, `max_window_days`)
- [x] Update CLAUDE.md with new package descriptions
- [ ] Deprecation warning for `bug_labels`/`feature_labels` when `categories` is also set
- [ ] Run full `task quality`

## Acceptance Criteria

### Functional Requirements

- [x] Three linking strategies (pr-link, commit-ref, changelog) run by default
- [x] Union merge produces deduplicated results with priority ordering
- [x] PR is a first-class metric target (lead time = PR opened → merged when no issue)
- [ ] User-defined classification categories work via config and inline flags
- [x] `scope` command shows per-strategy discovery results in all formats
- [x] Time window guardrails prevent excessive API usage (default 31 days, max 90)
- [x] Backward compatible: existing `bug_labels`/`feature_labels` config still works
- [x] Commit-ref defaults to closing keywords only; bare `#N` is opt-in

### Non-Functional Requirements

- [ ] All strategy implementations have table-driven tests
- [ ] GraphQL queries use variables only (no string interpolation)
- [ ] API cost per release run stays at ~22 calls
- [ ] Classification matchers are independently testable
- [ ] `context.Context` propagated through all strategy operations

### Quality Gates

- [ ] `task test` passes
- [ ] `task quality` passes (lint, vet, staticcheck, vulncheck, smoke)
- [ ] Test coverage on `internal/strategy/` and `internal/classify/` packages

## Key Files

| File | Purpose |
|------|---------|
| `internal/model/types.go` | Add PR, DiscoveredItem, ScopeResult, CategoryConfig |
| `internal/github/client.go` | Add GraphQL client field |
| `internal/github/pullrequests.go` | SearchMergedPRs, FetchPRLinkedIssues |
| `internal/strategy/strategy.go` | Strategy interface, Runner |
| `internal/strategy/prlink.go` | PR-link strategy |
| `internal/strategy/commitref.go` | Commit-ref strategy |
| `internal/strategy/changelog.go` | Changelog strategy |
| `internal/strategy/merge.go` | Priority merge deduplication |
| `internal/classify/classify.go` | Classifier, matchers |
| `internal/config/config.go` | Categories, CommitRefConfig, max_window_days |
| `cmd/scope.go` | Scope validation command |
| `cmd/release.go` | Updated to use strategy Runner |

## Risk Analysis

| Risk | Impact | Mitigation |
|------|--------|------------|
| Search API 1000-result cap | Medium — very large releases could miss PRs | Warn on stderr, document limitation. Time window guardrails reduce likelihood. |
| GraphQL rate limit (different from REST) | Medium — GraphQL has separate 5,000 point budget | Batch queries aggressively (20 PRs per call). Monitor cost field in response. |
| Issue Type field not available on all plans | Low — GitHub Free doesn't have Issue Types | `type:` matcher silently returns no match. Other matchers still work. |
| Category config ordering in YAML | Medium — must be deterministic | Use `[]CategoryEntry` slice (not map) to preserve order |
| go-gh GraphQL client API surface | Low — `DoWithContext` is stable | Already verified: `DoWithContext(ctx, query, variables, response)` works |

## References

- Brainstorm: `docs/brainstorms/2026-03-09-linking-strategies-brainstorm.md`
- Existing linking: `internal/linking/linker.go`
- go-gh GraphQL client: `github.com/cli/go-gh/v2/pkg/api.DefaultGraphQLClient()`
- GitHub search API: `GET /search/issues?q=repo:{owner}/{repo}+is:pr+is:merged+merged:{start}..{end}`
- GraphQL `closingIssuesReferences`: `repository.pullRequest.closingIssuesReferences(first: 10)`
