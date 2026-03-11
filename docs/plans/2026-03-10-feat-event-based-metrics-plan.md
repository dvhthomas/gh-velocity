---
title: "feat: Event-Based Metric Data Model"
type: feat
status: completed
date: 2026-03-10
---

# feat: Event-Based Metric Data Model

## Overview

Introduce `Event` and `Metric` types so every computed metric (lead-time, cycle-time, release-lag) carries explicit start/end events with timestamps, signal names, and human-readable detail. This makes output self-explanatory and enables future aging reports.

**Brainstorm:** `docs/brainstorms/2026-03-10-event-based-metrics-brainstorm.md`

## Problem Statement

Current output shows "Cycle Time: 5h 18m" with no explanation of where the endpoints came from. The `cmd/cycletime.go` computes events into ad-hoc local variables (`startedAt`, `signal`, `ctDuration`) that are thrown away before reaching formatters. The release path (`metrics/release.go:59-71`) only uses first-commit for cycle time, bypassing the full signal hierarchy. Data is computed then discarded.

## Proposed Solution

A `Metric` struct that bundles `Start *Event`, `End *Event`, `Duration *time.Duration` into a single cohesive unit, used across all metrics.

## What Changes

### Phase 1: Core Types (`model/types.go`)

- [x] Add `Event` struct: `Time time.Time`, `Signal string`, `Detail string`
- [x] Add `Metric` struct: `Start *Event`, `End *Event`, `Duration *time.Duration`
- [x] Change `IssueMetrics.LeadTime` from `*time.Duration` to `Metric`
- [x] Change `IssueMetrics.CycleTime` from `*time.Duration` to `Metric`
- [x] Change `IssueMetrics.ReleaseLag` from `*time.Duration` to `Metric`

```go
// model/types.go

type Event struct {
    Time   time.Time
    Signal string // "issue-created", "issue-closed", "pr-created", etc.
    Detail string // "PR #42: title" or "Backlog -> In progress"
}

type Metric struct {
    Start    *Event
    End      *Event
    Duration *time.Duration
}

type IssueMetrics struct {
    Issue            Issue
    LeadTime         Metric
    CycleTime        Metric
    ReleaseLag       Metric
    CommitCount      int
    LeadTimeOutlier  bool
    CycleTimeOutlier bool
}
```

**Design decision — Duration as stored field:** Duration is a stored `*time.Duration`, not a computed method. Rationale: some metrics may have a start but no end (in-progress), and some edge cases (backlog override) have neither. A nil Duration is unambiguous. The builder computes it once from `End.Time - Start.Time` when both exist.

**Backlog override representation:** When an issue is in backlog, the Metric is a zero-value `Metric{}` (all nil fields). The backlog state is communicated via warnings, not embedded in the Metric struct. This keeps the Metric type clean — it represents a measurement, not an override state.

### Phase 2: Signal Vocabulary (Constants)

- [x] Add signal name constants in `model/types.go`

```go
// Signal name constants for consistent use across metrics.
const (
    SignalIssueCreated     = "issue-created"
    SignalIssueClosed      = "issue-closed"
    SignalStatusChange     = "status-change"
    SignalLabel            = "label"
    SignalPRCreated        = "pr-created"
    SignalPRMerged         = "pr-merged"
    SignalAssigned         = "assigned"
    SignalCommit           = "commit"
    SignalReleasePublished = "release-published"
)
```

| Signal | Used By | Meaning |
|--------|---------|---------|
| `issue-created` | lead-time start | Issue opened |
| `issue-closed` | lead-time end, cycle-time end, release-lag start | Issue closed |
| `pr-merged` | cycle-time end (PR mode) | PR merged |
| `status-change` | cycle-time start | Moved out of backlog |
| `label` | cycle-time start | Active label added |
| `pr-created` | cycle-time start | Linked PR opened |
| `assigned` | cycle-time start | First assignment |
| `commit` | cycle-time start (fallback) | First commit referencing issue |
| `release-published` | release-lag end | Release created |

### Phase 3: Metric Builders (`internal/metrics/`)

- [x] Add `NewMetric(start, end *Event) Metric` helper that computes Duration when both events present
- [x] Update `LeadTime()` to return `Metric` instead of `*time.Duration`
- [x] Update `CycleTime()` to return `Metric` instead of `*time.Duration`
- [x] Add `ReleaseLag()` function returning `Metric`
- [x] Update `ComputeStats` to extract durations from `[]Metric`
- [x] Update `IsOutlier` to accept `Metric`

```go
// internal/metrics/metric.go

func NewMetric(start, end *model.Event) model.Metric {
    m := model.Metric{Start: start, End: end}
    if start != nil && end != nil {
        d := end.Time.Sub(start.Time)
        m.Duration = &d
    }
    return m
}
```

### Phase 4: Wire `cmd/leadtime.go`

- [x] Build `Metric` with `Start: Event{issue.CreatedAt, "issue-created", ""}` and `End: Event{*issue.ClosedAt, "issue-closed", ""}` (or nil End if open)
- [x] Pass `Metric` to formatters instead of separate variables
- [x] Remove ad-hoc `lt` and `started` variables

### Phase 5: Wire `cmd/cycletime.go`

- [x] **Issue path:** Replace ad-hoc `startedAt`, `signal`, `ctDuration` locals with a single `Metric`
- [x] Build start Event from whichever signal wins (status-change, label, pr-created, assigned, commit)
- [x] Build end Event from `issue.ClosedAt` with signal `"issue-closed"`
- [x] **PR path:** Build start Event from `pr.CreatedAt` with signal `"pr-created"`, end from `pr.MergedAt` with signal `"pr-merged"`
- [x] Pass `Metric` to formatters

### Phase 6: Wire `metrics/release.go` — Fix Signal Gap

- [x] Replace the first-commit-only cycle time logic (lines 59-71) with full signal hierarchy
- [x] For each issue in the release, compute cycle time using the same priority as the single-issue path:
  1. Status change (if project config available)
  2. Label (if active_labels configured)
  3. PR created (from commit→PR linking already available)
  4. Assigned
  5. First commit (current behavior, becomes fallback)
- [x] Build `Metric` structs for lead-time, cycle-time, and release-lag per issue
- [x] **API cost consideration:** The release path currently avoids per-issue timeline queries. To support signals #1-4 without exploding API calls, introduce a `CycleStartBatch` method that batches timeline queries, or accept the per-issue cost for releases with <50 issues and fall back to commit-only for larger releases.
- [x] Update `ReleaseInput` to carry config needed for signal hierarchy (project config, active labels, etc.)

```go
// Sketch: release.go cycle time with signal hierarchy

// For issues discovered via PR-link strategy, we already have the PR.
// Use PR.CreatedAt as "pr-created" signal without extra API calls.
if len(item.Commits) > 0 {
    // Fallback: first commit
    startEvent = &model.Event{
        Time: item.Commits[len(item.Commits)-1].AuthoredAt,
        Signal: model.SignalCommit,
        Detail: item.Commits[len(item.Commits)-1].SHA[:7],
    }
}
if item.PR != nil {
    // PR-created is higher priority than commit
    startEvent = &model.Event{
        Time: item.PR.CreatedAt,
        Signal: model.SignalPRCreated,
        Detail: fmt.Sprintf("PR #%d: %s", item.PR.Number, item.PR.Title),
    }
}
```

### Phase 7: Update All Formatters

- [x] **JSON (`format/json.go`):** Break schema — replace `lead_time_seconds` with nested `lead_time.start`, `lead_time.end`, `lead_time.duration_seconds`

```go
// New JSON output for single-issue metrics
type JSONMetric struct {
    Start           *JSONEvent `json:"start"`
    End             *JSONEvent `json:"end"`
    DurationSeconds *int64     `json:"duration_seconds"`
    Duration        string     `json:"duration"` // human-readable
}

type JSONEvent struct {
    Time   time.Time `json:"time"`
    Signal string    `json:"signal"`
    Detail string    `json:"detail,omitempty"`
}
```

- [x] **Pretty (`cmd/leadtime.go`, `cmd/cycletime.go`):** Default shows signal summary on duration line

```
Issue #10131  PAT scopes...
  Lead Time:  10d 13h  (created -> closed)
  Cycle Time: 5h 18m   (pr-created -> closed)
```

- [ ] **Pretty with `--verbose`:** Shows separate event lines

```
Issue #10131  PAT scopes...
  Created:    2024-12-23 (issue-created)
  Closed:     2025-01-02 (issue-closed)
  Lead Time:  10d 13h
  Started:    2025-01-02 (pr-created: PR #10164)
  Cycle Time: 5h 18m
```

- [x] **Markdown (`format/markdown.go`):** Add signal columns
- [x] **Release pretty table:** No change to table layout (too dense for events), but `--verbose` on release shows per-issue event details after the table
- [x] **Release JSON:** Each issue in the `issues` array gets `lead_time`, `cycle_time`, `release_lag` as `JSONMetric` objects

### Phase 8: Add `--verbose` Flag

- [ ] Add `--verbose` persistent flag on root command
- [ ] Thread through `Deps` struct
- [ ] Pretty formatters check verbose for expanded event output

### Phase 9: Adapt `github.CycleStart` → `model.Event`

- [ ] The existing `github.CycleStart` struct (`internal/github/cyclestart.go:10-14`) is nearly identical to `model.Event`
- [ ] Option A: Replace `CycleStart` with `model.Event` directly
- [ ] Option B: Keep `CycleStart` as internal to the github package, convert to `model.Event` at the boundary
- [ ] **Recommended: Option B** — keeps the github package's internal types separate from the domain model. Add a `ToEvent() model.Event` method on `CycleStart`.

### Phase 10: Update Tests

- [x] Update `internal/metrics/release_test.go` — assertions change from `im.LeadTime` (duration) to `im.LeadTime.Duration`
- [x] Update `internal/metrics/metrics_test.go` — `LeadTime()` and `CycleTime()` return `Metric` not `*time.Duration`
- [x] Update `internal/format/formatter_test.go` — construct `Metric` structs in test data
- [x] Add new tests for `NewMetric()` helper (nil start, nil end, both present, both nil)
- [x] Add new tests for signal summary formatting (e.g., `"created -> closed"`)

### Phase 11: Update Smoke Tests

- [x] `scripts/smoke-test.sh` — Update assertions for new output format:
  - lead-time pretty: check for `(created -> closed)` or similar signal summary
  - lead-time JSON: check for `.lead_time.duration_seconds` instead of `.lead_time_seconds`
  - cycle-time pretty: check for signal summary
  - cycle-time JSON: check for `.cycle_time.start.signal`
  - release JSON: check for nested metric objects

## Acceptance Criteria

- [x] Every `Metric` in output has `Start` and `End` events (or nil for in-progress)
- [x] Default pretty output shows signal summary: `Lead Time: 10d 13h (created -> closed)`
- [ ] `--verbose` shows separate event lines with timestamps and signal details
- [x] JSON output uses nested `{start, end, duration_seconds}` structure
- [x] Release path uses PR-created signal when available (not just first-commit)
- [x] `task test` passes
- [x] `task quality` passes
- [x] Smoke tests pass with updated assertions
- [x] In-progress items show `start` with `end: null` and `duration_seconds: null`

## Implementation Order

The phases above are listed in dependency order. Execute sequentially:

1. **Types + constants** (Phase 1-2) — no callers yet, tests compile with zero-value Metrics
2. **Builders** (Phase 3) — pure functions, easy to test in isolation
3. **Single-issue commands** (Phase 4-5) — wire events, update pretty output
4. **Release path** (Phase 6) — fix signal gap, biggest behavioral change
5. **Formatters** (Phase 7) — JSON schema break, all output formats
6. **Verbose flag** (Phase 8) — small addition, wires through existing infrastructure
7. **CycleStart migration** (Phase 9) — internal refactor, no output change
8. **Tests + smoke** (Phase 10-11) — validate everything

## References

- Brainstorm: `docs/brainstorms/2026-03-10-event-based-metrics-brainstorm.md`
- Current model: `internal/model/types.go:49-58` (IssueMetrics with bare durations)
- Current CycleStart: `internal/github/cyclestart.go:10-14`
- Release cycle time gap: `internal/metrics/release.go:59-71`
- Signal hierarchy: `cmd/cycletime.go:166-233`
- JSON formatters: `internal/format/json.go`
- Pretty formatters: `internal/format/pretty.go`, `cmd/leadtime.go`, `cmd/cycletime.go`
- Smoke tests: `scripts/smoke-test.sh`
