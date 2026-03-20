---
title: "Cycle Time"
weight: 2
---

# Cycle Time

Cycle time measures how long active work took on an issue. Unlike [lead time]({{< relref "lead-time" >}}), which includes all waiting time from creation, cycle time starts when work begins.

Two strategies detect when work starts.

## What it tells you

Cycle time reveals how long active work takes, stripped of backlog wait time. A low, consistent cycle time means the team delivers quickly once work begins. High variability suggests inconsistent scope or frequent context-switching.

Comparing cycle time to lead time shows how much elapsed time is active work versus waiting. If lead time is 30 days but cycle time is 3 days, 90% of the time is spent in backlog -- a signal to improve prioritization, not development speed.

## Strategies

### Issue strategy (`cycle_time.strategy: issue`)

```
cycle_time = issue.closed_at - work_started
```

Detects "work started" from labels. When an issue receives a label matching `lifecycle.in-progress.match`, the label's `createdAt` timestamp becomes the cycle start. Label event timestamps are **immutable** -- they never change once applied.

**Start signal**: `label-added` (label match)

**End signal**: `issue-closed`

When no matching label is found for a given issue, cycle time is returned as N/A.

#### Configuration

```yaml
lifecycle:
  in-progress:
    match: ["label:in-progress", "label:wip"]
```

The `match` field uses [matcher syntax]({{< relref "../config#matcher-syntax" >}}): `label:<name>` for exact label matches.

### PR strategy (`cycle_time.strategy: pr`)

```
cycle_time = pr.merged_at - pr.created_at
```

Uses the closing PR's lifecycle as a proxy for active work time. Requires no extra configuration -- just link PRs to issues with "Closes #N" or "Fixes #N" in the PR description.

**Start signal**: `pr-created` -- the timestamp when the closing PR was opened.

**End signal**: `pr-merged` -- the timestamp when the PR was merged.

If no linked PR is found for an issue, or the PR has not been merged, cycle time is N/A.

#### Configuration

```yaml
cycle_time:
  strategy: pr
```

No other configuration needed. PR-to-issue links are discovered through GitHub's `closingIssuesReferences`.

## Choosing a strategy

| Workflow | Recommended strategy | Why |
|----------|---------------------|-----|
| Solo developer / OSS | `pr` | PRs are your primary unit of work; no labels needed |
| Team with lifecycle labels | `issue` | Labels give immutable timestamps for accurate cycle time |
| Team without labels | `pr` | PR creation date is a reliable, zero-config proxy |

## Signals used

| Signal | Strategy | Source | Description |
|--------|----------|--------|-------------|
| `label-added` | issue | Label timeline event `createdAt` | Label matching `lifecycle.in-progress.match` was applied |
| `issue-closed` | issue | `issue.closed_at` | Issue was closed |
| `pr-created` | pr | `pr.created_at` | Closing PR was opened |
| `pr-merged` | pr | `pr.merged_at` | Closing PR was merged |

## How cycle start signals are resolved {#signal-hierarchy}

With the issue strategy, the cycle start signal is resolved using a priority hierarchy. The first available signal wins:

| Priority | Signal | Source | Config required |
|----------|--------|--------|-----------------|
| 1 (highest) | In-progress label | `LABELED_EVENT.createdAt` (immutable) | `lifecycle.in-progress.match` |
| 2 | PR created | `PullRequest.createdAt` (including drafts) | None -- uses GitHub cross-references |
| 3 | First assigned | Issue timeline `AssignedEvent.createdAt` | None -- automatic |
| 4 (lowest) | First commit mentioning issue | Commit date from local git history | Local clone required |

**Backlog suppression:** If an issue is currently in backlog (matches backlog labels), cycle time is N/A regardless of other signals. This prevents deprioritized issues from showing misleading cycle times.

> [!TIP]
> If cycle time shows N/A for an issue despite having a PR, check whether the issue is in a backlog state. Backlog suppression intentionally overrides all other signals.

## Statistical aggregation

Cycle time uses the same aggregation as lead time: mean, median, std dev, P90, P95, and IQR-based outlier detection.

## Example output

### Issue strategy (label-based)

```json
{
  "cycle_time": {
    "start": {
      "time": "2026-01-20T09:00:00Z",
      "signal": "label-added",
      "detail": "in-progress"
    },
    "end": {
      "time": "2026-01-22T16:30:00Z",
      "signal": "issue-closed"
    },
    "duration_seconds": 198600
  }
}
```

### PR strategy

```json
{
  "cycle_time": {
    "start": {
      "time": "2026-01-20T11:00:00Z",
      "signal": "pr-created",
      "detail": "PR #87"
    },
    "end": {
      "time": "2026-01-21T15:45:00Z",
      "signal": "pr-merged"
    },
    "duration_seconds": 103500
  }
}
```

## Commands that report cycle time

- `gh velocity flow cycle-time <issue>` -- single-issue cycle time
- `gh velocity quality release <tag>` -- per-issue and aggregate cycle time within a release
- `gh velocity report` -- aggregate cycle time across a time window

## Configuration reference

| Config field | Effect |
|---|---|
| `cycle_time.strategy` | `"issue"` (default) or `"pr"` |
| `lifecycle.in-progress.match` | Label matchers for issue strategy cycle start |

## Insights

Cycle time shares the same insights as [lead time]({{< relref "/reference/metrics/lead-time" >}}#insights) (noise detection, outliers, predictability, skew, fastest/slowest, category comparison), plus:

| Insight | When it fires | What it means |
|---------|--------------|---------------|
| **No data** | No cycle time measurements available | Strategy-specific guidance: issue strategy needs `lifecycle.in-progress` config; PR strategy needs linked closing PRs. |
| **Strategy callout** | PR strategy with data | States what the metric measures ("first PR to issue close") so readers know the methodology. |

## See also

- [Cycle Time Setup]({{< relref "/guides/cycle-time-setup" >}}) -- choosing and configuring a strategy
- [Labels as Lifecycle Signal]({{< relref "/concepts/labels-vs-board" >}}) -- why labels are the sole lifecycle signal
- [Lead Time]({{< relref "/reference/metrics/lead-time" >}}) -- the full elapsed duration (superset of cycle time)
- [Configuration Reference: cycle_time]({{< relref "/reference/config" >}}#cycle_time) -- all config fields
