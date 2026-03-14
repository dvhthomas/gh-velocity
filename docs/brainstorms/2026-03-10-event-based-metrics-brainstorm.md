---
title: Event-Based Metric Data Model
date: 2026-03-10
status: completed
type: brainstorm
---

# Event-Based Metric Data Model

## What We're Building

A consistent `Event` and `Metric` data model where every metric (lead-time, cycle-time, release-lag) carries explicit start and end events with timestamps, signal names, and human-readable detail. This makes output self-explanatory — users see *why* a metric was calculated, not just the number.

Example output (default):
```
Issue #10131  PAT scopes...
  Lead Time:  10d 13h  (created -> closed)
  Cycle Time: 5h 18m   (pr-created -> closed)
```

With `--verbose`:
```
Issue #10131  PAT scopes...
  Created:    2024-12-23 (issue-created)
  Closed:     2025-01-02 (issue-closed)
  Lead Time:  10d 13h
  Started:    2025-01-02 (pr-created: PR #10164)
  Cycle Time: 5h 18m
```

JSON output:
```json
{
  "lead_time": {
    "start": { "time": "...", "signal": "issue-created" },
    "end":   { "time": "...", "signal": "issue-closed" },
    "duration_seconds": 907200
  },
  "cycle_time": {
    "start": { "time": "...", "signal": "pr-created", "detail": "PR #10164: Add mention..." },
    "end":   { "time": "...", "signal": "issue-closed" },
    "duration_seconds": 19080
  }
}
```

In-progress items have `start` but `end: null` and `duration_seconds: null`.

## Why This Approach

**Problem:** Current output shows "Cycle Time: 5h 18m" with no explanation of where those endpoints came from. The release path completely bypasses the signal hierarchy (only uses first-commit), while the single-issue path has a 5-level hierarchy. Data is computed then thrown away before reaching formatters.

**Solution:** A `Metric` struct that bundles `Start *Event`, `End *Event`, `Duration *time.Duration` into a single cohesive unit. This:
- Makes every metric self-describing in all output formats
- Eliminates ad-hoc local variables in cmd/cycletime.go
- Enables the release path to use the full signal hierarchy
- Lays groundwork for aging reports (start with no end = in progress)

## Key Decisions

1. **Metric struct with embedded events** — Each metric is `Metric{Start, End, Duration}` not flat fields. Cohesive, self-describing, prevents forgetting to populate events alongside durations.

2. **Fix release path signal gap in same work** — The release path currently only uses first-commit for cycle time. Wiring the full signal hierarchy into `BuildReleaseMetrics` is a natural consequence of the Event model.

3. **Break JSON schema** — Pre-1.0, no external consumers. Clean break: `lead_time_seconds` becomes `lead_time.start` + `lead_time.end` + `lead_time.duration_seconds`.

4. **Minimal pretty output by default, compact with --verbose** — Default shows `Lead Time: 10d 13h (created -> closed)`. With `--verbose`, shows separate Start/End lines with dates and signal details.

5. **Aging reports: data model only** — The Event model supports null end events for in-progress items. An `aging` subcommand comes later (YAGNI).

## Core Types

```go
// model/types.go

type Event struct {
    Time   time.Time
    Signal string // "issue-created", "issue-closed", "pr-created", "status-change", etc.
    Detail string // "PR #42: title" or "Backlog -> In progress"
}

type Metric struct {
    Start    *Event
    End      *Event
    Duration *time.Duration
}

type IssueMetrics struct {
    Issue      Issue
    LeadTime   Metric
    CycleTime  Metric
    ReleaseLag Metric
    CommitCount      int
    LeadTimeOutlier  bool
    CycleTimeOutlier bool
}
```

## Signal Vocabulary

Consistent signal names across all metrics:

| Signal | Used By | Meaning |
|--------|---------|---------|
| `issue-created` | lead-time start | Issue opened |
| `issue-closed` | lead-time end, cycle-time end, release-lag start | Issue closed |
| `pr-merged` | cycle-time end (PR mode) | PR merged |
| `status-change` | cycle-time start | Moved out of backlog in Projects v2 |
| `label` | cycle-time start | Active label added |
| `pr-created` | cycle-time start | Linked PR opened |
| `assigned` | cycle-time start | First assignment |
| `commit` | cycle-time start (fallback) | First commit referencing issue |
| `release-published` | release-lag end | Release created |

## What Changes

- `model.Event` and `model.Metric` — new types
- `model.IssueMetrics` — `LeadTime`, `CycleTime`, `ReleaseLag` change from `*time.Duration` to `Metric`
- `github.CycleStart` — replaced by or adapted to `model.Event`
- `metrics.BuildReleaseMetrics` — receives and uses Event-populated IssueMetrics, runs full signal hierarchy
- `cmd/cycletime.go` — populates Events instead of local variables
- `cmd/leadtime.go` — populates Events
- All formatters (JSON, pretty, markdown) — consume Metric struct
- Smoke tests — updated for new output format
- `--verbose` flag on root command

## Resolved Questions

- **Should the release path use the full signal hierarchy?** Yes — fix in same work.
- **How to handle JSON breaking change?** Just break it. Pre-1.0.
- **Default pretty verbosity?** Minimal (signal summary on duration line). `--verbose` for compact with separate event lines.
- **Aging reports scope?** Data model only. Command comes later.

## Open Questions

None — all questions resolved during brainstorm.
