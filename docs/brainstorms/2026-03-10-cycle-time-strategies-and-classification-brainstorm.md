---
title: Cycle-Time Strategies & Flexible Classification
date: 2026-03-10
status: completed
type: brainstorm
supersedes: docs/brainstorms/2026-03-09-linking-strategies-brainstorm.md
related_issue: https://github.com/dvhthomas/gh-velocity/issues/2
---

# Cycle-Time Strategies & Flexible Classification

## What We're Building

Two complementary changes folded into one body of work:

1. **Cycle-time strategies** — Replace the blended signal hierarchy (`cmd/cycletime.go`'s 50-line cascade of project-board → label → PR → assigned → commit) with explicit, user-chosen strategies behind a common interface. The team picks one strategy and sticks with it.

2. **Flexible classification** — Replace fixed bug/feature/other with user-defined categories via config matchers (`label:`, `type:`, `title:`). This is issue #2.

Both build on the existing linking strategies (pr-link, commit-ref, changelog) which discover *what* shipped. Classification answers *what kind of work*. Cycle-time strategies answer *when work started and ended*.

## Why This Approach

The current blended hierarchy tries to be clever — "I'll try project status, then labels, then PRs, then assignments, then commits." The result:
- Confusing output (users don't know which signal was used without `--verbose`)
- Inconsistent numbers across items in a release (one uses PR-created, another uses commit)
- Lots of API calls for marginal benefit (querying project status + timeline + commits for every issue)
- When a signal source isn't available, you get cryptic warnings instead of a clear N/A

A single declared strategy is simpler, more predictable, and N/A is meaningful feedback — it tells the team their process wasn't followed for that item.

## Key Decisions

### 1. Three Cycle-Time Strategies

Each implements a `CycleTimeStrategy` interface with one job: given an issue (or PR), return a `model.Metric` with start/end events.

| Strategy | Start signal | End signal | When to use |
|----------|-------------|------------|-------------|
| `issue` | issue created | issue closed | Default. Works everywhere, no config needed. |
| `pr` | PR created | PR merged | Teams where PRs are the unit of work. Requires linked PRs. |
| `project-board` | Status change (out of backlog) | issue closed | Teams using GitHub Projects v2 boards. Requires project config. |

### 2. Single Strategy from Config

```yaml
cycle_time:
  strategy: issue    # or "pr" or "project-board"
```

No fallback chains. Pick one, get consistent numbers. If the signal isn't available for an item, result is N/A. Default when no config: `issue`.

### 3. `--pr` Flag Overrides Config

Config sets the default strategy. `--pr` is a one-off override for spot-checking. This preserves the existing CLI behavior while adding config-driven defaults.

```
gh-velocity cycle-time 42          # uses config strategy (default: issue)
gh-velocity cycle-time --pr 42     # overrides to pr strategy for this run
```

### 4. Uniform Strategy in Release Command

All issues in a release use the same cycle-time strategy. If strategy is `pr` and an issue has no linked PR, its cycle time is N/A. Consistent and honest — you can compare numbers across items because they all measure the same thing.

### 5. Flexible Classification (Issue #2)

Replace fixed bug/feature/other with user-defined categories:

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

Matcher types: `label:<name>`, `type:<name>`, `title:<regex>`. First match wins. Unmatched → "other". Backward compatible: auto-generate from `bug_labels`/`feature_labels` when `categories` absent.

### 6. CycleTimeStrategy Interface

Follows the same pattern as the existing linking `Strategy` interface:

```go
type CycleTimeStrategy interface {
    Name() string
    Compute(ctx context.Context, input CycleTimeInput) (model.Metric, error)
}
```

Each implementation is a small focused struct. No hierarchy, no fallbacks inside.

### 7. GetCycleStart Refactored Into Strategy Implementations

The current `github.GetCycleStart` (timeline queries, project status queries) gets split:
- `issue` strategy: no API calls beyond GetIssue — just uses created/closed dates
- `pr` strategy: needs the linked PR (from linking strategies or a lookup)
- `project-board` strategy: uses the existing `GetProjectStatus` call

The 50-line signal cascade in `cmd/cycletime.go` disappears entirely.

## How It Connects to Linking Strategies

Linking strategies discover *what* shipped in a release (pr-link, commit-ref, changelog). The results include both issues and PRs with their metadata. The cycle-time strategy then uses that metadata:

- `issue` strategy: uses `issue.CreatedAt` / `issue.ClosedAt` from the discovered item
- `pr` strategy: uses `pr.CreatedAt` / `pr.MergedAt` from the discovered item's linked PR
- `project-board` strategy: makes an additional API call per issue for board status

The linking strategies are a prerequisite — they provide the data that cycle-time strategies consume.

## Resolved Questions

- **Blended hierarchy or pick-one?** → Pick one strategy. N/A is useful feedback.
- **Config or flag for strategy selection?** → Config sets default, `--pr` flag overrides.
- **Fallback chains?** → No. Single strategy, consistent numbers.
- **Default when no config?** → `issue` (created → closed). Universal, no config needed.
- **Per-item or uniform in releases?** → Uniform. All items use same strategy for comparability.
- **What about the existing `GetCycleStart`?** → Split into per-strategy implementations. The monolithic function goes away.
- **Does `--pr` flag still make sense?** → Yes, as a config override for one-off runs.

## Out of Scope

- Fallback chains (deliberately excluded — keep it simple)
- Custom cycle-time strategies beyond the three built-in ones
- Per-item strategy selection within a release
