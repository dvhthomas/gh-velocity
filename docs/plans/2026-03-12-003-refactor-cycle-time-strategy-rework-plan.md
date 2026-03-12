---
title: "Cycle Time Strategy Rework: Lifecycle-Aware Issue Strategy"
type: refactor
status: completed
date: 2026-03-12
origin: docs/brainstorms/2026-03-12-cycle-time-strategy-rework-brainstorm.md
---

# Cycle Time Strategy Rework: Lifecycle-Aware Issue Strategy

## Overview

The current `IssueStrategy` computes created → closed, which is **lead time**, not cycle time. Cycle time should measure **work started → closed** (active work only). The three cycle-time strategies (issue, pr, project-board) collapse to two: **issue** and **pr**. The project-board strategy is absorbed into the issue strategy, since project board status is just one way to detect "work started."

This is a global fix affecting all consumers: standalone `cycle-time` command, `report`, and `my-week`.

(see brainstorm: `docs/brainstorms/2026-03-12-cycle-time-strategy-rework-brainstorm.md`)

## Problem Statement / Motivation

1. **IssueStrategy is wrong.** It computes `created → closed` which is lead time, not cycle time. Users expecting cycle time get misleading numbers.
2. **ProjectBoardStrategy is redundant.** It's the issue strategy with a specific "work started" detection mechanism (project board status change). Having it as a separate strategy creates config complexity.
3. **ProjectBoardStrategy is non-functional.** `buildCycleTimeStrategy` in `cmd/helpers.go:14-19` creates it with empty `ProjectID`/`StatusFieldID`/`BacklogStatus` (TODO in code).
4. **my-week hardcodes PR cycle time.** `ComputeInsights` in `metrics/insights.go:52-66` always computes cycle time as PR created → merged, ignoring the configured strategy.

## Proposed Solution

### Two strategies, not three

| Strategy | Measures | Start signal | End signal |
|----------|----------|-------------|------------|
| **issue** | Issue cycle time | First departure from backlog (lifecycle `in-progress` signal) | Issue closed |
| **pr** | PR cycle time | PR created | PR merged |

The issue strategy uses lifecycle config to detect "work started":
- If `lifecycle.in-progress.project_status` is configured → use project board status change events via `GetProjectStatus` (already exists in `github/cyclestart.go:145`)
- If no lifecycle signal is available → warn and skip (don't show cycle time, don't fall back to created date)

### Resolve project IDs at strategy build time

`ResolveProject` (`github/project.go:47`) already converts project URL + status field name → `ProjectInfo{ProjectID, StatusFieldID}`. Wire this into `buildCycleTimeStrategy` to populate the issue strategy fields.

### Always show both lead time and cycle time

When cycle time is available, show both metrics. They measure different things and both are valuable.

## Technical Considerations

### Files that change

| File | Change |
|------|--------|
| `internal/metrics/cycletime.go` | Rewrite `IssueStrategy` to use lifecycle signal; delete `ProjectBoardStrategy` |
| `internal/config/config.go` | Validation: accept only `"issue"` and `"pr"` for `cycle_time.strategy` |
| `cmd/helpers.go` | `buildCycleTimeStrategy`: resolve project IDs for issue strategy, remove project-board case |
| `internal/metrics/insights.go` | Accept pre-computed `[]time.Duration` for issue cycle time (cmd layer computes) |
| `cmd/myweek.go` | Compute issue cycle-time durations via strategy, pass to `ComputeInsights` |
| `cmd/report.go` | Verify cycle-time pipeline uses updated strategy |
| `cmd/preflight.go` | Project board presence → strategy stays `"issue"` (not `"project-board"`) |
| `internal/format/myweek.go` | Display both lead time and cycle time when available |

### Architecture: IssueStrategy fields

```go
// IssueStrategy measures cycle time as "work started" → issue closed.
// "Work started" is detected from lifecycle config (project board status change).
type IssueStrategy struct {
    Client        *gh.Client
    ProjectID     string   // resolved from config project.url via ResolveProject
    StatusFieldID string   // resolved from config project.status_field
    BacklogStatus []string // from lifecycle.backlog.project_status
}
```

When `ProjectID` is empty (no project configured), `Compute` returns a zero `Metric` — the signal is unavailable.

### Architecture: ComputeInsights receives durations

`ComputeInsights` stays a pure computation function. The cmd layer computes cycle-time durations using the configured strategy and passes them in:

```go
type MyWeekInsights struct {
    // ... existing fields ...
    LeadTime  *time.Duration // median lead time (issues)
    CycleTime *time.Duration // median cycle time (from strategy)
}

// ComputeInsights accepts optional pre-computed cycle-time durations.
func ComputeInsights(r model.MyWeekResult, cycleTimeDurations []time.Duration) model.MyWeekInsights
```

### Preflight changes

Current preflight (`cmd/preflight.go:265`) sets `result.Strategy = "project-board"` when a project board is found. After this change:
- Project board found → `result.Strategy = "issue"` (project board is used by the issue strategy for "work started" detection)
- More PRs than issues, no project → `result.Strategy = "pr"`
- Default → `result.Strategy = "issue"`

The hint text at line 302 should change from "created → closed" to "Configure lifecycle.in-progress for cycle time metrics."

### Config validation

Remove `"project-board"` as a valid strategy value. Accept only `"issue"` (default) and `"pr"`. The `project.url` requirement moves from strategy validation to lifecycle validation — if `lifecycle.in-progress.project_status` is set, `project.url` must also be set.

### Backward compatibility

Users with `strategy: project-board` in their config need a migration path. Options:
- Config validation warns and treats `"project-board"` as `"issue"` with a deprecation message
- Preflight `--write` generates `strategy: issue` going forward

## Acceptance Criteria

### Phase 1: Fix IssueStrategy + merge ProjectBoardStrategy

- [x] `IssueStrategy` struct gains `Client`, `ProjectID`, `StatusFieldID`, `BacklogStatus` fields (`internal/metrics/cycletime.go`)
- [x] `IssueStrategy.Compute` uses `GetProjectStatus` when ProjectID is set, returns zero Metric when not set (`internal/metrics/cycletime.go`)
- [x] `IssueStrategy.Compute` returns zero Metric (no cycle time) when no lifecycle signal available — never falls back to created date
- [x] `ProjectBoardStrategy` type deleted (`internal/metrics/cycletime.go`)
- [x] `buildCycleTimeStrategy` resolves project IDs via `ResolveProject` and passes to `IssueStrategy` (`cmd/helpers.go`)
- [x] `buildCycleTimeStrategy` has no `"project-board"` case (`cmd/helpers.go`)
- [x] Existing cycle-time tests updated for new IssueStrategy behavior (`internal/metrics/cycletime_test.go`)

### Phase 2: Config + preflight updates

- [x] Config validation accepts only `"issue"` and `"pr"` (`internal/config/config.go`)
- [x] Config validation warns on `"project-board"` (treats as `"issue"` with deprecation hint) (`internal/config/config.go`)
- [x] Preflight sets `strategy: "issue"` when project board found (`cmd/preflight.go:265`)
- [x] Preflight hint text updated: no more "created → closed" for issue strategy (`cmd/preflight.go:302`)
- [x] `defaultConfigTemplate` updated: strategy comments reflect 2 options (`cmd/config.go`)

### Phase 3: Wire my-week + format updates

- [x] `ComputeInsights` accepts pre-computed cycle-time durations (`internal/metrics/insights.go`)
- [x] `cmd/myweek.go` computes issue cycle-time durations via strategy, passes to `ComputeInsights`
- [x] my-week displays both lead time and cycle time when available (`internal/format/myweek.go`)
- [x] my-week insights test updated (`internal/metrics/insights_test.go`)
- [x] `cmd/report.go` verified to use updated strategy correctly

### Cross-cutting

- [x] All existing tests pass
- [ ] Smoke tests pass (`scripts/smoke-test.sh`)
- [x] No strategy named `"project-board"` appears anywhere in code or config templates

## Success Metrics

- `cycle-time` command with issue strategy + lifecycle config shows **work started → closed** (not created → closed)
- `my-week` shows both lead time and cycle time when lifecycle signal is available
- `my-week` shows lead time only (no cycle time) when no lifecycle signal — with a warning
- `preflight --write` generates `strategy: issue` for repos with project boards

## Dependencies & Risks

| Risk | Mitigation |
|------|-----------|
| `GetProjectStatus` GraphQL query may be slow for many issues in my-week | Batch via errgroup with rate limit (existing pattern) |
| Users with `strategy: project-board` in existing configs | Deprecation warning + treat as `"issue"` |
| `updatedAt` on project field gives last status change, not first departure from backlog | Known limitation (see brainstorm). Accept as-is; timeline events API is out of scope |
| `ResolveProject` may fail (permissions, deleted project) | Return error, `buildCycleTimeStrategy` falls back to empty IssueStrategy (no cycle time) |

## Sources & References

- **Origin brainstorm:** [docs/brainstorms/2026-03-12-cycle-time-strategy-rework-brainstorm.md](docs/brainstorms/2026-03-12-cycle-time-strategy-rework-brainstorm.md) — Key decisions: two strategies not three, lifecycle config as signal source, warn-and-skip when no signal
- `internal/metrics/cycletime.go` — CycleTimeStrategy interface, IssueStrategy (line 28), PRStrategy (line 44), ProjectBoardStrategy (line 67)
- `internal/config/config.go:58-81` — LifecycleStage, LifecycleConfig, CycleTimeConfig
- `internal/metrics/insights.go` — ComputeInsights (hardcoded PR cycle time)
- `cmd/helpers.go:9-24` — buildCycleTimeStrategy with TODO
- `cmd/preflight.go:265,296` — strategy auto-detection
- `internal/github/project.go:47` — ResolveProject (URL → node IDs)
- `internal/github/cyclestart.go:145` — GetProjectStatus GraphQL query
- `docs/solutions/architecture-refactors/pipeline-per-metric-and-preflight-first-config.md` — pipeline architecture context
