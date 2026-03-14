---
title: "Cycle Time"
weight: 2
---

# Cycle Time

Cycle time measures how long active work took on an issue. Unlike [lead time]({{< relref "lead-time" >}}), which includes all waiting time from creation, cycle time starts when someone begins working on the issue.

gh-velocity supports two strategies for detecting when work starts. Choose the one that fits your workflow.

## Strategies

### Issue strategy (`cycle_time.strategy: issue`)

```
cycle_time = issue.closed_at - work_started
```

The issue strategy detects "work started" from two signal sources, tried in priority order:

1. **Labels (preferred)**: When an issue receives a label matching `lifecycle.in-progress.match`, the label's `createdAt` timestamp becomes the cycle start. Label event timestamps are **immutable** -- they never change once the label is applied, making them the most reliable signal.

2. **Project board (fallback)**: If no matching label is found and a project board is configured, the tool checks when the issue's status field was last updated. This is the `updatedAt` timestamp from the project board field value.

**Start signal**: `label-added` (label match) or `status-change` (project board)

**End signal**: `issue-closed`

When neither signal source is configured or no signal is found for a given issue, cycle time is returned as N/A.

#### Why labels over project board

The GitHub Projects v2 API only exposes `updatedAt` on status field values -- the timestamp of the **last** status change, not the original transition to "In Progress." If someone moves a card after the issue is closed, `updatedAt` reflects that post-closure move, producing negative cycle times (`start > end`). The tool filters negative durations from aggregate statistics and warns you, but the root cause cannot be fixed at the API level.

Label timestamps (`LABELED_EVENT.createdAt`) are immutable and record the exact moment the label was applied. This is why labels are the recommended cycle time signal.

#### Configuration

```yaml
# Recommended: labels for cycle time
lifecycle:
  in-progress:
    match: ["label:in-progress", "label:wip"]

# Optional: project board for WIP/backlog detection
# Labels still take priority for cycle time
project:
  url: "https://github.com/users/yourname/projects/1"
  status_field: "Status"

lifecycle:
  backlog:
    project_status: ["Backlog", "Triage"]
  in-progress:
    project_status: ["In progress"]
    match: ["label:in-progress"]
```

The `match` field uses [matcher syntax]({{< relref "../config#matcher-syntax" >}}): `label:<name>` for exact label matches.

### PR strategy (`cycle_time.strategy: pr`)

```
cycle_time = pr.merged_at - pr.created_at
```

The PR strategy uses the closing PR's lifecycle as a proxy for active work time. It requires no extra configuration -- just link PRs to issues with "Closes #N" or "Fixes #N" in the PR description.

**Start signal**: `pr-created` -- the timestamp when the closing PR was opened.

**End signal**: `pr-merged` -- the timestamp when the PR was merged.

If no linked PR is found for an issue, or the PR has not been merged, cycle time is N/A.

#### Configuration

```yaml
cycle_time:
  strategy: pr
```

No other configuration is needed. The tool discovers PR-to-issue links through GitHub's timeline events (`closingIssuesReferences`).

## Choosing a strategy

| Workflow | Recommended strategy | Why |
|----------|---------------------|-----|
| Solo developer / OSS | `pr` | PRs are your primary unit of work; no labels or boards needed |
| Team with project board | `issue` + labels | Labels give immutable timestamps; board gives WIP visibility |
| Team without project board | `pr` | PR creation date is a reliable, zero-config proxy |

## Signals used

| Signal | Strategy | Source | Description |
|--------|----------|--------|-------------|
| `label-added` | issue | Label timeline event `createdAt` | Label matching `lifecycle.in-progress.match` was applied |
| `status-change` | issue (fallback) | Project field value `updatedAt` | Status field was last changed (may be unreliable) |
| `issue-closed` | issue | `issue.closed_at` | Issue was closed |
| `pr-created` | pr | `pr.created_at` | Closing PR was opened |
| `pr-merged` | pr | `pr.merged_at` | Closing PR was merged |

## Deprecated: `project-board` strategy

The `project-board` strategy value is deprecated. If set, it is silently treated as `issue`. Use `cycle_time.strategy: issue` with `lifecycle.in-progress.match` for reliable cycle time, and add `project_status` fields for WIP and backlog detection.

## Statistical aggregation

Cycle time uses the same aggregation as lead time: mean, median, std dev, P90, P95, and IQR-based outlier detection. Negative durations (possible with project board fallback) are filtered from statistics and counted in `negative_count`.

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
| `lifecycle.in-progress.project_status` | Board column names for issue strategy fallback |
| `lifecycle.backlog.project_status` | Board column names excluded from cycle start |
| `project.url` | Project board URL (required for board fallback) |
| `project.status_field` | Status field name on the board (required for board fallback) |
