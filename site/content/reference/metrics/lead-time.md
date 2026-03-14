---
title: "Lead Time"
weight: 1
---

# Lead Time

Lead time is the total elapsed time from when an issue is created to when it is closed. It measures how long a piece of work existed in your system, including time spent in backlog, waiting for prioritization, blocked by dependencies, in active development, in review, and waiting for release.

## Definition

```
lead_time = issue.closed_at - issue.created_at
```

**Start signal**: `issue-created` -- the timestamp when the GitHub issue was opened.

**End signal**: `issue-closed` -- the timestamp when the issue was closed (by any means: a merged PR, manual close, or bot).

Lead time is always measured on issues, regardless of which `cycle_time.strategy` you use. The strategy choice does not affect lead time.

## What it tells you

A long lead time does not necessarily mean slow development. It often means slow prioritization or long backlog queues. An issue that sits in "Backlog" for 60 days and takes 2 days to implement has a 62-day lead time but only a 2-day cycle time.

Comparing lead time to cycle time reveals how much of the total elapsed time is active work versus waiting. A large gap between the two indicates process bottlenecks outside of development.

## Signals used

| Signal | Source | Description |
|--------|--------|-------------|
| `issue-created` | `issue.created_at` | GitHub issue creation timestamp |
| `issue-closed` | `issue.closed_at` | GitHub issue closure timestamp |

Both timestamps come from the GitHub REST Search API and are precise to the second.

## Open issues

If an issue is still open (`closed_at` is null), the lead time metric is returned with a start event but no end event and no duration. In JSON output, `lead_time_seconds` will be `null`. In pretty output, it displays as "N/A".

## Statistical aggregation

When lead time is computed across multiple issues (e.g., in a release report), the tool calculates:

- **Count**: Number of issues with a valid lead time
- **Mean**: Average lead time
- **Median**: Middle value (less sensitive to outliers than mean)
- **Std Dev**: Sample standard deviation (requires 2+ data points)
- **P90 / P95**: 90th and 95th percentile (requires 5+ data points)
- **Outlier cutoff**: Q3 + 1.5 * IQR (requires 4+ data points)
- **Outlier count**: Issues exceeding the cutoff

Negative durations (which should not occur for lead time but are guarded against) are filtered from statistics and counted separately.

## Configuration that affects lead time

Lead time itself requires no configuration -- it uses only the issue's creation and closure timestamps. However, other configuration affects which issues are included in lead time calculations:

| Config field | Effect |
|---|---|
| `scope.query` | Filters which issues are analyzed |
| `quality.categories` | Classifies issues but does not change lead time values |
| `exclude_users` | Excludes issues authored by specified users |
| `lifecycle.done.query` | Determines which issues qualify as "done" for bulk queries |

## Example output

### Pretty format

```
Lead Time
  Start:    issue-created  2026-01-15T10:30:00Z
  End:      issue-closed   2026-02-02T14:15:00Z
  Duration: 18d 3h 45m
```

### JSON format

```json
{
  "lead_time": {
    "start": {
      "time": "2026-01-15T10:30:00Z",
      "signal": "issue-created"
    },
    "end": {
      "time": "2026-02-02T14:15:00Z",
      "signal": "issue-closed"
    },
    "duration_seconds": 1567500
  }
}
```

### Aggregate statistics (release report)

```json
{
  "aggregates": {
    "lead_time": {
      "count": 17,
      "mean_seconds": 24271200,
      "median_seconds": 5248800,
      "stddev_seconds": 43981056,
      "p90_seconds": 134236800,
      "p95_seconds": 138499200,
      "outlier_cutoff_seconds": 119448000,
      "outlier_count": 2
    }
  }
}
```

## Commands that report lead time

- `gh velocity flow lead-time <issue>` -- single-issue lead time
- `gh velocity quality release <tag>` -- per-issue and aggregate lead time within a release
- `gh velocity report` -- aggregate lead time across a time window

## See also

- [Interpreting Results]({{< relref "/guides/interpreting-results" >}}) -- what healthy lead time looks like and common patterns
- [Understanding Statistics]({{< relref "/concepts/statistics" >}}) -- median, percentiles, and outlier detection explained
- [Cycle Time]({{< relref "/reference/metrics/cycle-time" >}}) -- the active-work subset of lead time
- [Configuration Reference]({{< relref "/reference/config" >}}) -- config fields that affect which issues are included
