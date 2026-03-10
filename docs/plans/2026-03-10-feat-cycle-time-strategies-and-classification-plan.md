---
title: "feat: Cycle-Time Strategies & Flexible Classification"
type: feat
status: active
date: 2026-03-10
brainstorm: docs/brainstorms/2026-03-10-cycle-time-strategies-and-classification-brainstorm.md
related_issue: https://github.com/dvhthomas/gh-velocity/issues/2
supersedes: docs/plans/2026-03-09-feat-linking-strategies-classification-plan.md
---

# Cycle-Time Strategies & Flexible Classification

## Overview

Two changes folded into one body of work:

1. **Cycle-time strategies** — Replace the blended 5-signal hierarchy in `cmd/cycletime.go` with explicit, config-driven strategies behind a `CycleTimeStrategy` interface. The team picks one strategy and gets consistent, comparable numbers.

2. **Flexible classification** — Replace fixed bug/feature/other with user-defined categories via config matchers. Issue #2.

Both build on the existing linking strategies (pr-link, commit-ref, changelog) which are already implemented in `internal/strategy/`.

## Problem Statement

**Cycle time:** The current `runCycleTimeIssue()` cascades through 5 signals (project-board → label → PR → assigned → commit) in a ~50-line hierarchy. This produces confusing output (which signal was used?), inconsistent numbers across release items, excessive API calls, and cryptic warnings when signals aren't available.

**Classification:** `BuildReleaseMetrics()` uses hard-coded `BugLabels`/`FeatureLabels` string arrays. Can't match GitHub Issue Types, title patterns, or user-defined categories. `ReleaseMetrics` has fixed `BugCount`/`FeatureCount`/`OtherCount` fields that can't represent arbitrary categories.

## Proposed Solution

### Cycle-Time Strategies

A `CycleTimeStrategy` interface with three implementations. Config selects one strategy; `--pr` flag overrides.

```yaml
# .gh-velocity.yml
cycle_time:
  strategy: issue    # "issue" | "pr" | "project-board"
```

| Strategy | Start | End | API calls | Config needed |
|----------|-------|-----|-----------|---------------|
| `issue` (default) | issue created | issue closed | 0 (data already on Issue) | None |
| `pr` | PR created | PR merged | Needs linked PR from linking strategies | None |
| `project-board` | Status change out of backlog | issue closed | 1 GraphQL per issue | `project.*`, `statuses.backlog` |

When the signal isn't available for an item → `N/A`. This is meaningful feedback, not an error.

### Flexible Classification

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

Matcher types: `label:<name>` (case-insensitive), `type:<name>`, `title:<regex>`. First match wins. Unmatched → "other".

## What Changes

### Phase 1: CycleTimeStrategy Interface & Implementations

- [x] Create `internal/cycletime/` package
- [x] Define interface:

```go
// internal/cycletime/strategy.go

// Strategy computes cycle time for a single work item.
type Strategy interface {
    Name() string
    Compute(ctx context.Context, input Input) model.Metric
}

// Input provides data each strategy needs.
type Input struct {
    Issue   *model.Issue
    PR      *model.PR       // from linking strategies, may be nil
    Commits []model.Commit  // from linking strategies, may be empty
}
```

- [x] Implement `IssueStrategy` — returns `NewMetric(issue-created, issue-closed)`. No API calls.

```go
// internal/cycletime/issue.go
func (s *IssueStrategy) Compute(ctx context.Context, input Input) model.Metric {
    if input.Issue == nil {
        return model.Metric{} // N/A
    }
    start := &model.Event{Time: input.Issue.CreatedAt, Signal: model.SignalIssueCreated}
    if input.Issue.ClosedAt == nil {
        return model.Metric{Start: start} // in progress
    }
    end := &model.Event{Time: *input.Issue.ClosedAt, Signal: model.SignalIssueClosed}
    return metrics.NewMetric(start, end)
}
```

Note: This is the same calculation as `metrics.LeadTime()` but intentionally duplicated — calling `LeadTime()` for cycle time would be semantically confusing.

- [x] Implement `PRStrategy` — returns `NewMetric(pr-created, pr-merged)`. Needs linked PR.

```go
// internal/cycletime/pr.go
func (s *PRStrategy) Compute(ctx context.Context, input Input) model.Metric {
    if input.PR == nil {
        return model.Metric{} // N/A — no linked PR
    }
    start := &model.Event{Time: input.PR.CreatedAt, Signal: model.SignalPRCreated}
    if input.PR.MergedAt == nil {
        return model.Metric{Start: start} // in progress
    }
    end := &model.Event{Time: *input.PR.MergedAt, Signal: model.SignalPRMerged}
    return metrics.NewMetric(start, end)
}
```

- [x] Implement `ProjectBoardStrategy` — calls `GetProjectStatus`, returns `NewMetric(status-change, issue-closed)`.

```go
// internal/cycletime/projectboard.go
type ProjectBoardStrategy struct {
    Client        *gh.Client
    ProjectID     string
    StatusFieldID string
    BacklogStatus string
}

func (s *ProjectBoardStrategy) Compute(ctx context.Context, input Input) model.Metric {
    if input.Issue == nil {
        return model.Metric{}
    }
    ps, err := s.Client.GetProjectStatus(ctx, input.Issue.Number, s.ProjectID, s.StatusFieldID, s.BacklogStatus)
    if err != nil || ps.CycleStart == nil {
        return model.Metric{} // N/A
    }
    start := &model.Event{Time: ps.CycleStart.Time, Signal: model.SignalStatusChange, Detail: ps.CycleStart.Detail}
    if input.Issue.ClosedAt == nil {
        return model.Metric{Start: start}
    }
    end := &model.Event{Time: *input.Issue.ClosedAt, Signal: model.SignalIssueClosed}
    return metrics.NewMetric(start, end)
}
```

- [x] Add table-driven tests for each strategy (nil issue, nil PR, open issue, closed issue, merged PR, etc.)

**Similar patterns:** `internal/strategy/strategy.go:16-21` (linking Strategy interface)

### Phase 2: Config — `cycle_time.strategy`

- [x] Add `CycleTimeConfig` to `internal/config/config.go`:

```go
type CycleTimeConfig struct {
    Strategy string `yaml:"strategy" json:"strategy"` // "issue", "pr", "project-board"
}
```

- [x] Add field to `Config`: `CycleTime CycleTimeConfig \`yaml:"cycle_time" json:"cycle_time"\``
- [x] Default to `"issue"` in `defaults()`
- [x] Validate: must be one of `issue`, `pr`, `project-board`
- [x] If `project-board` selected but no `project.id` or `project.status_field_id` configured → config error (exit code 2)
- [x] Add to `config show` and `config validate` output

**Similar patterns:** `config.go:44-47` (CommitRefConfig)

### Phase 3: Wire `cmd/cycletime.go` to Use Strategy

- [x] Replace the 50-line signal hierarchy in `runCycleTimeIssue()` with:

```go
strat := selectCycleTimeStrategy(deps)
ct := strat.Compute(ctx, cycletime.Input{Issue: issue, PR: linkedPR, Commits: commits})
```

- [x] `selectCycleTimeStrategy()` reads config, respects `--pr` flag override
- [x] `--pr` flag behavior:
  - If `--pr N` is given (with a PR number), use PR strategy on that specific PR (existing behavior)
  - If issue number given and config says `pr`, need to find the linked PR for that issue. Add `Client.GetClosingPR(ctx, issueNumber) (*model.PR, error)` to `internal/github/` — extracts the ClosedEvent→PullRequest logic from the current `GetCycleStart` before removing it. This is a simple GraphQL query for `timelineItems(itemTypes: [CLOSED_EVENT])` filtering for PR closers.
- [x] Remove the signal hierarchy code entirely — each strategy is self-contained
- [x] Remove `GetCycleStart` calls from `cmd/cycletime.go` — only `ProjectBoardStrategy` uses `GetProjectStatus` internally

### Phase 4: Wire Release Command to Use Strategy

- [ ] `BuildReleaseMetrics()` receives the strategy name (or the Strategy itself) as part of `ReleaseInput`
- [ ] For each issue in the release, compute cycle time using the uniform strategy:

```go
for _, item := range input.Items {
    ct := strategy.Compute(ctx, cycletime.Input{
        Issue:   item.Issue,
        PR:      item.PR,
        Commits: item.Commits,
    })
    im.CycleTime = ct
}
```

- [ ] All items use the same strategy — N/A when signal isn't available

### Phase 5: Fix Merge Commit-Loss Bug

The spec-flow analysis identified that `strategy.Merge()` discards commits from commit-ref when pr-link also found the same issue. This breaks cycle time for the `commit` fallback.

- [x] Update `Merge()` in `internal/strategy/merge.go` to union commit lists when deduplicating
- [x] When pr-link and commit-ref find the same issue: keep pr-link's PR data, union both commit lists
- [x] Add test case: same issue from both strategies, verify commits are merged

**File:** `internal/strategy/merge.go`, `Merge()` function

### Phase 6: Flexible Classification — `internal/classify/`

- [ ] Create `internal/classify/` package
- [ ] `Classifier` struct holds parsed matchers per category
- [ ] `ParseMatcher(s string) (Matcher, error)` — parses `label:bug`, `type:Bug`, `title:/regex/i`
- [ ] Matcher interface:

```go
type Matcher interface {
    Matches(issue model.Issue) bool
}
```

- [ ] `LabelMatcher` — case-insensitive label check
- [ ] `TypeMatcher` — matches GitHub Issue Type (requires Issue.Type field — see Phase 7). Note: Issue Types are a newer GitHub feature. REST API support may be limited; may need GraphQL `issueType { name }` field. If REST doesn't expose it, defer `type:` matcher to a follow-up and document the limitation.
- [ ] `TitleMatcher` — compiled regex on issue title
- [ ] `Classifier.Classify(issue model.Issue) string` — returns first matching category name, or "other"
- [ ] Validate matchers at config load time (invalid regex → exit code 2)
- [ ] Table-driven tests for each matcher type

**Similar patterns:** `internal/strategy/commitref.go` (regex-based matching on commit messages)

### Phase 7: Config — Categories

- [ ] Add `Categories` field to `QualityConfig`:

```go
type QualityConfig struct {
    BugLabels     []string            `yaml:"bug_labels" json:"bug_labels"`
    FeatureLabels []string            `yaml:"feature_labels" json:"feature_labels"`
    Categories    map[string][]string `yaml:"categories" json:"categories,omitempty"`
    // ...
}
```

- [ ] Backward compatibility: when `categories` is absent, auto-generate from `bug_labels`/`feature_labels`:

```go
if len(cfg.Quality.Categories) == 0 {
    cfg.Quality.Categories = map[string][]string{
        "bug":     toLabelMatchers(cfg.Quality.BugLabels),
        "feature": toLabelMatchers(cfg.Quality.FeatureLabels),
    }
}
```

- [ ] Add `Type` field to `model.Issue` for `type:` matcher. GitHub Issue Types may require GraphQL (`issueType { name }`). Check REST API first — if `type` field is not available, either add a GraphQL call to `GetIssue` or defer the `type:` matcher. `label:` and `title:` matchers work without this.

### Phase 8: Wire Classification into Release Metrics

- [ ] Replace `BugCount`/`FeatureCount`/`OtherCount`/`BugRatio`/`FeatureRatio`/`OtherRatio` in `ReleaseMetrics` with dynamic categories:

```go
type ReleaseMetrics struct {
    // ...
    CategoryCounts map[string]int     `json:"category_counts"`
    CategoryRatios map[string]float64 `json:"category_ratios"`
    // Remove: BugCount, FeatureCount, OtherCount, BugRatio, FeatureRatio, OtherRatio
}
```

- [ ] `BuildReleaseMetrics()` takes a `*classify.Classifier` instead of `BugLabels`/`FeatureLabels`
- [ ] For each issue: `category := classifier.Classify(issue)` then increment `CategoryCounts[category]`
- [ ] **Breaking JSON change**: `bug_count` → `category_counts.bug`, etc.

### Phase 9: Update Formatters

- [ ] **JSON**: Use `CategoryCounts`/`CategoryRatios` maps instead of fixed fields
- [ ] **Pretty**: Show all categories in release summary (not just bug/feature/other)
- [ ] **Markdown**: Dynamic category columns in release table

### Phase 10: Cleanup

- [ ] Remove `github.GetCycleStart` function (replaced by strategies)
- [ ] Remove `github.CycleStart` struct (replaced by `model.Event`)
- [ ] Remove signal hierarchy from `cmd/cycletime.go`
- [ ] Remove `BugLabels`/`FeatureLabels` from `ReleaseInput` (replaced by Classifier)
- [ ] Keep `BugLabels`/`FeatureLabels` in config for backward compat (they feed into auto-generated categories)

### Phase 11: Update Tests & Smoke Tests

- [ ] Unit tests for each `CycleTimeStrategy` implementation
- [ ] Unit tests for `classify.Classifier` and each matcher type
- [ ] Update `release_test.go` for dynamic categories
- [ ] Update smoke tests:
  - cycle-time with default (issue) strategy
  - cycle-time with `--pr` override
  - release JSON: `category_counts` instead of `bug_count`
- [ ] `task test` passes
- [ ] `task quality` passes

## Acceptance Criteria

- [ ] `CycleTimeStrategy` interface with three implementations (issue, pr, project-board)
- [ ] `cycle_time.strategy` config field with validation
- [ ] `--pr` flag overrides config strategy
- [ ] Uniform strategy across all items in release command
- [ ] N/A when signal isn't available (not an error)
- [ ] `internal/classify/` package with `Classifier`, `ParseMatcher()`, matchers (label, type, title)
- [ ] `categories` config field with backward compat from `bug_labels`/`feature_labels`
- [ ] Dynamic `CategoryCounts`/`CategoryRatios` in `ReleaseMetrics` (breaking JSON change)
- [ ] Merge commit-loss bug fixed in `strategy.Merge()`
- [ ] `GetCycleStart` removed — replaced by strategy implementations
- [ ] `task quality` passes
- [ ] All smoke tests pass

## Implementation Order

Execute in dependency order:

1. **Phase 1** — CycleTimeStrategy interface + implementations (pure, testable)
2. **Phase 2** — Config field (no callers yet, just parsing)
3. **Phase 5** — Fix merge bug (independent, correctness fix)
4. **Phase 3** — Wire cycle-time command (replaces hierarchy)
5. **Phase 4** — Wire release command (uniform strategy)
6. **Phase 6** — Classify package (pure, testable)
7. **Phase 7** — Config categories + backward compat
8. **Phase 8** — Wire classification into release metrics
9. **Phase 9** — Update formatters (JSON schema break)
10. **Phase 10** — Cleanup dead code
11. **Phase 11** — Tests and smoke tests throughout

## References

- Brainstorm: `docs/brainstorms/2026-03-10-cycle-time-strategies-and-classification-brainstorm.md`
- Issue #2: https://github.com/dvhthomas/gh-velocity/issues/2
- Existing strategy interface: `internal/strategy/strategy.go:16-21`
- Current signal hierarchy: `cmd/cycletime.go:140-233`
- Current classification: `internal/metrics/release.go` (BugLabels/FeatureLabels)
- Config: `internal/config/config.go`
- GetCycleStart (to be replaced): `internal/github/cyclestart.go`
- Event/Metric types: `internal/model/types.go`
- Merge function (bug): `internal/strategy/merge.go`
- Documented learnings: `docs/solutions/cycle-time-signal-hierarchy.md`, `docs/solutions/three-state-metric-status-pattern.md`
