---
title: "fix: deprecate project board as cycle-start signal, add negative-duration guard"
type: fix
status: completed
date: 2026-03-13
origin: docs/brainstorms/2026-03-12-cycle-time-strategy-rework-brainstorm.md
---

# fix: deprecate project board as cycle-start signal, add negative-duration guard

## Overview

GitHub Projects v2 exposes only a mutable `updatedAt` timestamp on status field values — not a transition history. When a card is moved after an issue is closed (e.g., moved to "Done"), the `updatedAt` reflects the *post-close* move, producing `start > end` and negative cycle times. Label events have immutable `createdAt` timestamps and are the more reliable signal.

This change: (1) adds a negative-duration guard, (2) reverses signal priority so labels are preferred over project board for cycle start, (3) adds a deprecation warning when project board is the only cycle-start signal, and (4) updates preflight to recommend label-based lifecycle alongside project board status.

Project board status remains fully supported for WIP detection and backlog filtering — only its use as a **cycle-start timestamp** is deprecated.

## Problem Statement / Motivation

Running `gh velocity report` against a repo with project board lifecycle config produced:

```
Cycle Time:  median -1s, mean -16m, P90 -1s (n=22)
```

Negative cycle times are nonsensical. They occur because `GetProjectStatus()` returns the status field's `updatedAt` as cycle start — but `updatedAt` reflects the last field modification, not the original "In Progress" transition. The GitHub Projects v2 API has no field change history; the REST timeline API would require per-issue calls.

The label-based lifecycle (`lifecycle.in-progress.match`) shipped recently and uses `LABELED_EVENT.createdAt` — an immutable timestamp that correctly captures when work started.

## Proposed Solution

### Design Decisions

1. **Guard negatives in `ComputeStats`**, not `NewMetric`. Negative durations are a data quality issue specific to aggregate statistics. `NewMetric` should faithfully record what it's given; the stats layer filters and reports. Single-issue output should show the negative value with a warning so users understand the data problem. (See brainstorm: docs/brainstorms/2026-03-12-cycle-time-strategy-rework-brainstorm.md — "warn and skip when no signal available" principle.)

2. **Reverse signal priority**: labels first, project board second. When both `lifecycle.in-progress.match` and `lifecycle.in-progress.project_status` are configured, prefer labels. Project board remains as fallback (still useful when a label hasn't been applied but the card was moved).

3. **Keep project board for backlog detection and WIP**. The `InBacklog` check in `computeFromProject` is a different concern from cycle-start timing — it reads current status, not historical timestamps. WIP command reads `project_status` for current-state filtering. Both are unaffected.

4. **Deprecation is "recommend, not require"**. Warn when `project_status` is used for cycle start without `match` as a companion. No plans to hard-remove — teams with disciplined workflows (never move cards after close) get correct results.

5. **Once-per-run deprecation warning**, not per-issue. Include the count of affected issues (negative durations filtered).

## Technical Considerations

### Negative Duration Guard Location

`ComputeStats` is the right place because:
- It's the aggregate computation kernel — all bulk pipelines use it
- Single-issue mode should display the raw negative value + warning (helps users diagnose)
- Lead time and release lag should never be negative but get the same guard defensively

The guard filters negative durations before statistical computation and returns a count via a new `NegativeCount int` field on `model.Stats`.

### Signal Priority Reversal

In `buildCycleTimeStrategy()` (`cmd/helpers.go`), swap the `if` conditions so `InProgressMatch` is checked before `ProjectStatus`. The `IssueStrategy.Compute()` method (`internal/metrics/cycletime.go`) also needs reordering: try `computeFromLabels` first, then `computeFromProject`.

This affects all callers: `cmd/cycletime.go`, `cmd/report.go`, `cmd/myweek.go`, `cmd/release.go`. All share `buildCycleTimeStrategy`, so the change propagates automatically.

### Preflight Changes

When a project board is present, `writeLifecycleMapping` currently emits only `project_status`. It should also emit `match` entries when matching labels exist in the repo (e.g., `label:in-progress`). If no matching labels exist, emit a comment recommending the user create labels matching their board columns.

### JSON Output

Add `"negative_durations_filtered"` count to warnings array in bulk cycle time JSON. No new stats fields needed — the warning string is sufficient and backward-compatible.

## Implementation

### Phase 1: Negative Duration Guard

#### 1.1 Filter negatives in `ComputeStats`

`internal/metrics/stats.go`:

- [x] Filter `durations` to exclude `d < 0` before statistical computation
- [x] Count filtered negatives
- [x] Add `NegativeCount int` field to `model.Stats`
- [x] Populate count from filtered items

#### 1.2 Surface negative count in bulk pipeline

`internal/pipeline/cycletime/cycletime.go` — `BulkPipeline.ProcessData()`:

- [x] After `ComputeStats`, if `Stats.NegativeCount > 0`, append warning: "N issues had negative cycle times (project board timestamp reflects last update, not transition) — excluded from stats"
- [x] Warning flows through existing `Warnings []string` → JSON/pretty/markdown output

#### 1.3 Warn on single-issue negative duration

`internal/pipeline/cycletime/cycletime.go` — `IssuePipeline.ProcessData()`:

- [x] If `CycleTime.Duration != nil && *CycleTime.Duration < 0`, append warning explaining the timestamp issue

#### 1.4 Add `NegativeCount` to `model.Stats`

`internal/model/types.go`:

- [x] Add `NegativeCount int` field to `Stats` struct

#### 1.5 Tests

`internal/metrics/stats_test.go`:

- [x] Test `ComputeStats` with mixed positive/negative durations — negatives excluded from mean/median/P90
- [x] Test `ComputeStats` with all negative durations — returns count=0 stats with NegativeCount populated
- [x] Test `ComputeStats` with no negative durations — NegativeCount=0, stats unchanged

### Phase 2: Signal Priority Reversal

#### 2.1 Reverse priority in `IssueStrategy.Compute()`

`internal/metrics/cycletime.go`:

- [x] Try `computeFromLabels` first (when `InProgressMatch` is set)
- [x] Fall back to `computeFromProject` (when `ProjectID` is set)
- [x] Update comment from "Try project board first (higher fidelity signal)" to "Try labels first (immutable timestamps), fall back to project board"

#### 2.2 Reverse priority in `buildCycleTimeStrategy()`

`cmd/helpers.go`:

- [x] Check `InProgressMatch` first, `ProjectStatus` second
- [x] When both are available, populate both fields on `IssueStrategy` (labels get tried first in `Compute()`)
- [x] When only `ProjectStatus` is configured (no `Match`), emit deprecation warning via `deps.WarnUnlessJSON`

#### 2.3 Tests

`internal/metrics/cycletime_test.go`:

- [x] Test `IssueStrategy` with both signals configured — labels win
- [x] Test `IssueStrategy` with labels only — labels used
- [x] Test `IssueStrategy` with project board only — project board used (with deprecation at cmd layer)
- [x] Test `IssueStrategy` where labels find nothing, project board has data — fallback works

### Phase 3: Deprecation Warning

#### 3.1 Add deprecation warning in `buildCycleTimeStrategy`

`cmd/helpers.go`:

- [x] When `ProjectStatus` is set and `InProgressMatch` is empty, emit: `"cycle time: project board timestamps can be unreliable (reflects last update, not transition). Recommend adding lifecycle.in-progress.match with label matchers. Run: gh velocity config preflight --write"`
- [x] Single warning per run (already natural — `buildCycleTimeStrategy` is called once per command)

#### 3.2 Test

`cmd/helpers_test.go` (or integration test):

- [x] Verify warning is emitted when project_status only
- [x] Verify no warning when match is configured (alone or with project_status)

### Phase 4: Preflight Update

#### 4.1 Emit `match` alongside `project_status`

`cmd/preflight.go` — `writeLifecycleMapping`:

- [x] Accept repo labels as parameter
- [x] For the `in-progress` stage, check if any repo labels match active patterns (e.g., `in-progress`, `wip`, `doing`, `active`)
- [x] If matching labels found, emit `match: ["label:<name>"]` alongside `project_status`
- [x] If no matching labels found, emit a comment: `# Tip: add a label like "in-progress" for more reliable cycle time timestamps`

#### 4.2 Update `renderPreflightConfig`

`cmd/preflight.go`:

- [x] Pass detected repo labels to `writeLifecycleMapping`
- [x] Ensure the label detection that currently only runs in the no-board branch also runs in the board branch

#### 4.3 Update `defaultConfigTemplate`

`cmd/config.go`:

- [x] Add a comment recommending `match` alongside `project_status` for cycle time reliability
- [x] Show `match` as the primary recommendation, `project_status` as "for WIP and backlog detection"

#### 4.4 Tests

`cmd/preflight_test.go`:

- [x] Test `writeLifecycleMapping` with repo labels present — emits both `project_status` and `match`
- [x] Test `writeLifecycleMapping` without matching labels — emits comment recommendation
- [x] Verify `config show` and `config preflight` outputs are consistent

### Phase 5: Documentation

#### 5.1 Update existing solution doc

`docs/solutions/architecture-patterns/label-based-lifecycle-for-cycle-time.md`:

- [x] Add section explaining why labels are now preferred over project board for cycle start
- [x] Note that project board is still used for WIP and backlog detection

## Acceptance Criteria

- [ ] `ComputeStats` filters negative durations and reports count via `Stats.NegativeCount`
- [ ] Bulk cycle time output includes warning when negative durations are filtered
- [ ] Single-issue cycle time warns on negative duration with explanation
- [ ] When both `match` and `project_status` are configured, labels take priority for cycle start
- [ ] When only `project_status` is configured, deprecation warning is emitted once per run
- [ ] WIP command is unaffected — still uses `project_status` for current-state filtering
- [ ] Backlog detection from project board still works (suppresses cycle time for backlog items)
- [ ] Preflight with project board emits `match` recommendations alongside `project_status`
- [ ] `go test ./...` passes
- [ ] Existing report/cycle-time tests continue to pass

## Success Metrics

- No more negative cycle times in aggregate stats
- Users with project board config get clear guidance toward label-based lifecycle
- Zero regression in WIP or backlog detection

## Dependencies & Risks

- **Low risk**: Signal priority reversal may change cycle time values for users who have both signals configured. Likely an improvement since labels are more accurate.
- **Config migration**: Users with only `project_status` get a warning, not an error. No action required unless they want more reliable cycle times.
- **Preflight label detection**: Depends on repo having relevant labels. If no matching labels exist, preflight recommends creating them — no silent failure.

## Sources & References

- **Origin brainstorm:** [docs/brainstorms/2026-03-12-cycle-time-strategy-rework-brainstorm.md](docs/brainstorms/2026-03-12-cycle-time-strategy-rework-brainstorm.md) — key decisions: two strategies not three, lifecycle config as source of truth
- **Solution doc:** [docs/solutions/architecture-patterns/label-based-lifecycle-for-cycle-time.md](docs/solutions/architecture-patterns/label-based-lifecycle-for-cycle-time.md) — label signal implementation details
- **Signal hierarchy:** [docs/solutions/cycle-time-signal-hierarchy.md](docs/solutions/cycle-time-signal-hierarchy.md) — priority-based signal selection, backlog suppression
- Existing deprecation pattern: `internal/config/config.go:304-307` (`"project-board"` → `StrategyIssue`)
- Bug evidence: report output showing `median -1s, mean -16m` from project board `updatedAt`
- Key files: `internal/metrics/stats.go`, `internal/metrics/cycletime.go`, `cmd/helpers.go`, `cmd/preflight.go`, `internal/github/cyclestart.go`
