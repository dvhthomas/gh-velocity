---
title: "Three-State Metric Status: In Progress vs N/A vs Completed"
category: architecture-decisions
tags: [state-tracking, started_at, duration, formatting, json, cycle-time, lead-time]
module: internal/format/formatter.go
symptom: "Metrics showed N/A for both 'work not started' and 'work started but not complete'"
root_cause: "Single optional duration field cannot represent three distinct states"
date: 2026-03-09
---

# Three-State Metric Status: In Progress vs N/A vs Completed

## Problem

Cycle time and lead time outputs showed "N/A" for both cases:
- Work hasn't started (no signal found — truly not applicable)
- Work started but hasn't completed (PR open, issue not closed — in progress)

Users couldn't distinguish idle issues from active ones. JSON consumers couldn't filter for in-progress work.

## Solution

Track `startedAt *time.Time` independently from `duration *time.Duration`. The combination of these two fields expresses three states:

| `startedAt` | `duration` | Meaning |
|------------|-----------|---------|
| nil | nil | N/A — no start signal found |
| non-nil | nil | In progress — work started, not complete |
| non-nil | non-nil | Completed — measurable duration |

### Core Formatter

Single source of truth for all output formats:

```go
// internal/format/formatter.go
func FormatCycleStatus(d *time.Duration, started bool) string {
    if d != nil {
        return FormatDuration(*d)
    }
    if started {
        return "in progress"
    }
    return "N/A"
}
```

### JSON Output

JSON includes both fields so consumers can programmatically distinguish states:

```go
// internal/format/json.go
type JSONCycleTimeOutput struct {
    StartedAt        *time.Time `json:"started_at,omitempty"`
    CycleTimeSeconds *int64     `json:"cycle_time_seconds,omitempty"`
    CycleTime        string     `json:"cycle_time"`  // "2d 3h", "in progress", or "N/A"
    // ...
}
```

The string field (`cycle_time`) always reflects the three-state logic. The numeric field (`cycle_time_seconds`) is only present when completed. The timestamp field (`started_at`) is present when work has started.

### Command Usage

Each command computes both values and passes them through:

```go
// cmd/cycletime.go — issue path
var ctDuration *time.Duration
var startedAt *time.Time
// ... signal detection sets startedAt when a signal is found ...
// ... sets ctDuration when issue is closed ...
started := startedAt != nil

// All formatters use the same function:
format.FormatCycleStatus(ctDuration, started)
```

For PRs, `startedAt` is always `&pr.CreatedAt` (PRs always have a creation time). For lead time, `started` is always `true` (issues always have a creation time).

## Key Design Decisions

- **Separate fields, not an enum.** Using `startedAt` and `duration` as independent pointers is simpler than a state enum and naturally maps to the data model (timestamps are real values, not synthetic state).
- **Boolean param in formatter, not pointer.** `FormatCycleStatus` takes `started bool` rather than `startedAt *time.Time` because the formatter doesn't need the timestamp — it only needs to know which string to emit.
- **Consistent across all formats.** JSON, pretty, and markdown all call the same `FormatCycleStatus` function. JSON additionally exposes the raw fields for programmatic access.

## Gotchas

- `*time.Duration(nil)` is different from `time.Duration(0)`. The formatter checks `d != nil`, not whether the duration is zero. A zero duration is valid (work started and ended at the same instant).
- The combination `started=false, d!=nil` is semantically invalid. Don't construct it — the formatter will return the formatted duration, hiding the inconsistency.
- JSON `started_at` serializes as `null` when nil, not as an absent field (unless `omitempty` is used). Current code uses `omitempty` so the field is omitted when nil.

## Files

- `internal/format/formatter.go` — `FormatCycleStatus()` three-state formatter
- `internal/format/json.go` — `WriteLeadTimeJSON`, `WriteCycleTimeJSON`, `WriteCycleTimePRJSON` with `started_at` fields
- `cmd/cycletime.go` — Tracks `startedAt` and `ctDuration` separately in both PR and issue paths
- `cmd/leadtime.go` — Sets `started = true` always (issues always have creation time)
