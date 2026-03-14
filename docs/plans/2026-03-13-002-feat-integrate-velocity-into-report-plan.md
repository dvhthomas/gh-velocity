---
title: "feat: integrate velocity into the report command"
type: feat
status: completed
date: 2026-03-13
origin: docs/brainstorms/2026-03-12-velocity-command-brainstorm.md
---

# feat: integrate velocity into the report command

## Overview

The `report` command aggregates lead time, cycle time, throughput, and quality into a single dashboard. It should also include a **velocity** section so users get one cohesive output from `gh velocity report`. The velocity pipeline already exists and works as a standalone `flow velocity` command — this is a wiring/composition task.

## Problem Statement / Motivation

Users currently need to run `gh velocity report` AND `gh velocity flow velocity` separately to get a full picture of team performance. The report is intended to be the primary user-facing command; individual `flow` subcommands are for drilling down. Without velocity in the report, users miss iteration-level effort data in their dashboard.

From PR #54 review: "the report summary should correctly aggregate all sub-outputs into a cohesive single report."

## Proposed Solution

Wire the existing `velocity.Pipeline` into `cmd/report.go` alongside the other pipelines, following the established pattern. Gracefully omit the velocity section when `velocity.iteration.strategy` is not configured.

## Technical Considerations

### Time Semantics

Report uses `--since`/`--until` sliding windows. Velocity uses iteration boundaries. The velocity pipeline already accepts optional `Since`/`Until` fields to filter iterations that overlap a date range. Map report's window directly:

```go
p := &velocity.Pipeline{
    Since: &since,
    Until: &until,
    // ...
}
```

This gives users iterations that overlap the report window — semantically correct.

### Concurrency

The velocity pipeline may call the ProjectV2 GraphQL API (slow, rate-limited). It MUST run concurrently with other pipelines inside the existing `errgroup`. The current `SetLimit(5)` provides enough headroom (4 existing pipelines + velocity = 5).

### Graceful Degradation

Unlike the standalone `flow velocity` command which errors when `iteration.strategy` is empty, the report should silently omit velocity. Check before launching the pipeline:

```go
if cfg.Velocity.Iteration.Strategy != "" {
    g.Go(func() error {
        // velocity pipeline
    })
}
```

### Report Summary vs Full Velocity Output

The standalone `flow velocity` command renders a detailed current iteration + history table. The report should show a **summary line** consistent with the other sections (lead time, cycle time, throughput are all single-line summaries):

- **Pretty**: `Velocity:  28.7 pts/sprint avg, 80.7% completion (n=6 sprints)`
- **Markdown**: Single table row like other metrics
- **JSON**: Embed a `velocity` object with summary stats + current iteration name

The full iteration history table belongs in the standalone command, not the report.

### Provenance

Issue #55 mentions "Share Provenance across the entire report." Currently only velocity builds provenance. For the report context, velocity provenance is not needed — the report itself is the provenance context. Skip provenance in report-embedded velocity.

## Implementation

### Phase 1: Model + Report Orchestration

#### 1.1 Add velocity to StatsResult

`internal/model/types.go`:

- [x] Add `Velocity *VelocityResult` field to `StatsResult`

#### 1.2 Wire velocity pipeline in report command

`cmd/report.go`:

- [x] Import velocity pipeline package
- [x] Check `cfg.Velocity.Iteration.Strategy != ""` before launching
- [x] Create `velocity.Pipeline` with report's `since`/`until` mapped to `Since`/`Until`
- [x] Leave `ShowHistory`/`ShowCurrent` at defaults (both false = show everything), set `IterationCount` from config (default 6)
- [x] Run `GatherData` in existing errgroup, capture errors as warnings
- [x] Run `ProcessData` after gather, assign `result.Velocity = &p.Result`
- [x] Skip provenance (not needed in report context)

### Phase 2: Renderers

#### 2.1 Pretty output

`internal/format/report.go` — `WriteReportPretty`:

- [x] Add velocity summary line after throughput: `Velocity: {avg} {unit}/sprint avg, {completion}% completion (n={count} sprints)`
- [x] Handle edge case: velocity configured but no iterations found in window → show "no iterations in window"

#### 2.2 Markdown output

`internal/format/templates/report.md.tmpl`:

- [x] Add conditional `{{if .Velocity}}` row to the metrics table
- [x] Format as: `| Velocity | {avg} {unit}/sprint avg, {completion}% completion (n={count}) |`

`internal/format/report.go` — `renderReportMarkdown`:

- [x] Pass velocity summary string to template data

#### 2.3 JSON output

`internal/format/report.go` — `WriteReportJSON`:

- [x] Add `Velocity *jsonVelocitySummary` field to `jsonStatsOutput`
- [x] Create `jsonVelocitySummary` struct with summary stats only: `avg_velocity`, `avg_completion_pct`, `std_dev`, `effort_unit`, `iteration_count`, `current_iteration` (name only, if in-progress)
- [x] No per-iteration history in report JSON — use `flow velocity -f json` for full detail
- [x] Populate from `VelocityResult` when present

### Phase 3: Tests

- [x] Test report with velocity config present — velocity section appears in all three formats
- [x] Test report with NO velocity config — velocity section omitted, no errors
- [x] Test report with velocity gather failure — warning added, other sections render (same pattern as lead/cycle/throughput)
- [x] Test JSON output includes velocity object with correct structure
- [x] Test markdown template renders velocity row conditionally
- [x] Test pretty output shows summary line

## Acceptance Criteria

- [ ] `gh velocity report` includes velocity when `velocity.iteration.strategy` is configured
- [ ] `gh velocity report` gracefully omits velocity when iteration strategy is not configured (no error, no warning)
- [ ] Velocity pipeline runs concurrently with other pipelines in report
- [ ] Velocity pipeline failure produces a warning, does not block other sections
- [ ] All three output formats (pretty, markdown, JSON) include velocity data
- [ ] Report's `--since`/`--until` correctly filters velocity iterations
- [ ] JSON output includes `velocity` field with summary stats
- [ ] Existing report tests continue to pass
- [ ] `go test ./...` passes

## Success Metrics

- Single `gh velocity report` command gives a complete team performance dashboard
- No regressions in report rendering for users without velocity config
- Velocity section in report matches semantics of standalone `flow velocity`

## Dependencies & Risks

- **Config gating**: Velocity is only useful when `velocity.iteration.strategy` is set. Most users who run `preflight --write` will have this auto-configured, but some won't.
- **API pressure**: Velocity with `project-field` iteration strategy makes GraphQL calls. Combined with other report pipelines, this could hit secondary rate limits. Mitigated by existing `SetLimit(5)` and API throttle.
- **Time window mapping**: Report's sliding window may not align cleanly with iteration boundaries. The velocity pipeline handles this by showing iterations that *overlap* the window — no special handling needed.

## Sources & References

- **Origin brainstorm:** [docs/brainstorms/2026-03-12-velocity-command-brainstorm.md](docs/brainstorms/2026-03-12-velocity-command-brainstorm.md) — velocity design decisions, config shape, output format
- Issue: #55
- Velocity pipeline: `internal/pipeline/velocity/velocity.go`
- Velocity command wiring: `cmd/velocity.go`
- Report command: `cmd/report.go`
- Report renderers: `internal/format/report.go`, `internal/format/templates/report.md.tmpl`
- StatsResult model: `internal/model/types.go:207`
- VelocityResult model: `internal/model/types.go:291`
