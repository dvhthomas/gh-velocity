---
title: "Cycle Time Strategy Rework: Lifecycle-Aware Issue Strategy"
date: 2026-03-12
status: complete
---

# Cycle Time Strategy Rework

## What We're Building

Fix the cycle-time strategy model so that:

1. **Lead time** = issue created → closed (total time including backlog wait)
2. **Cycle time** = work started → closed (active work time only)

Currently `IssueStrategy` computes created → closed, which is lead time — not cycle time. The three strategies (issue, pr, project-board) collapse into two: **issue** and **pr**. The project-board strategy is absorbed into issue strategy, since project board status is just one way to detect "work started."

This is a global fix affecting all consumers: standalone `cycle-time` command, `report`, and `my-week`.

## Why This Approach

The lifecycle config already defines `backlog → in-progress → in-review → done → released`. Each stage supports both REST query qualifiers and project board status values. So "work started" = transition from backlog to in-progress, and the lifecycle config is the source of truth for what that means.

Having a separate `project-board` strategy was redundant — it's just the issue strategy with a specific "work started" detection mechanism. Collapsing to two strategies (issue + pr) is simpler and maps cleanly to the two fundamental measurement perspectives: issue-centric and PR-centric.

## Key Decisions

1. **Two strategies, not three.** Issue strategy and PR strategy. Drop `project-board` as a separate strategy — it's an implementation detail of how issue strategy detects "in-progress."

2. **Issue strategy uses lifecycle config to detect "work started."** If `lifecycle.in-progress.project_status` is configured, use project board status change events. If lifecycle has label-based qualifiers, use label events. The lifecycle config determines the signal.

3. **Warn and skip when no signal available.** If the issue strategy can't detect "work started" (no project board, no in-progress lifecycle signal), don't show cycle time. Warn: "Configure lifecycle.in-progress for cycle time metrics." Don't fall back to created date (that's lead time, not cycle time).

4. **Always show both lead time and cycle time** in my-week insights (when cycle time is available). They measure different things and both are valuable for 1:1 prep.

5. **Global fix across all consumers.** Standalone cycle-time command, report, and my-week all use the corrected strategy model.

6. **my-week wiring.** `ComputeInsights` needs access to the cycle-time strategy. Compute cycle-time durations in the cmd layer (which has access to config, client, and strategy) and pass pre-computed durations to `ComputeInsights`, keeping it a pure computation function.

## Current State

### What exists today

- `CycleTimeStrategy` interface in `internal/metrics/cycletime.go` with 3 implementations:
  - `IssueStrategy`: created → closed (WRONG — this is lead time)
  - `PRStrategy`: PR created → merged (correct)
  - `ProjectBoardStrategy`: board status change → closed (correct concept, should be absorbed into issue strategy)
- `LifecycleConfig` in `internal/config/config.go` with `backlog`, `in-progress`, `in-review`, `done`, `released` stages
- Each `LifecycleStage` has `Query string` (REST) and `ProjectStatus []string` (GraphQL)
- Config validation: `project-board` strategy requires `project.url`
- `cmd/helpers.go:buildCycleTimeStrategy()` switches on config string

### What needs to change

- `IssueStrategy` → detect "work started" from lifecycle's in-progress stage
- `ProjectBoardStrategy` → merge into `IssueStrategy`
- Config validation: `cycle_time.strategy` accepts `"issue"` or `"pr"` only
- `buildCycleTimeStrategy()` → simplified to two cases
- `ComputeInsights` → receives pre-computed cycle-time durations from cmd layer
- All format functions that display strategy name → update for two-strategy model

## Scope Boundary

**In scope:**
- Fix IssueStrategy to use lifecycle "in-progress" signal
- Merge ProjectBoardStrategy into IssueStrategy
- Wire my-week to use configured strategy
- Update config validation (2 strategies instead of 3)
- Warn when cycle time can't be computed (no lifecycle signal)

**Out of scope:**
- New lifecycle detection mechanisms (first assignment, etc.)
- Changes to how lead time is computed
- Timeline events API integration
- New config fields

## References

- `internal/metrics/cycletime.go` — CycleTimeStrategy interface, IssueStrategy, PRStrategy, ProjectBoardStrategy
- `internal/config/config.go:58-74` — LifecycleStage and LifecycleConfig
- `internal/metrics/insights.go` — ComputeInsights (hardcoded PR cycle time)
- `cmd/helpers.go:9-24` — buildCycleTimeStrategy
- `cmd/myweek.go` — my-week data fetching
- `cmd/report.go` — report pipeline wiring with cycle time
- `docs/solutions/architecture-refactors/pipeline-per-metric-and-preflight-first-config.md` — pipeline architecture context
