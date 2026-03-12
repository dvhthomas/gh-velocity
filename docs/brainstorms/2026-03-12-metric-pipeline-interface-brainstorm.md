---
title: "Metric Pipeline Interface — Compile-Time Guardrails for Adding Metrics"
date: 2026-03-12
status: complete
origin: deepening of docs/plans/2026-03-12-001-feat-metrics-architecture-and-ux-plan.md (Phase 3)
---

# Metric Pipeline Interface

## What We're Building

A single Go interface that every metric command implements, providing:

1. **Compile-time guardrails** — forget to implement `Render()` and it won't compile
2. **Testable separation** — inject fake data into `ProcessData()` without API calls
3. **Report auto-wiring** — explicit registration list, one line per metric
4. **Consistent pipeline** — every metric follows GatherData → ProcessData → Render

## Why This Approach

### The problem today

Adding a new metric (like bus-factor) touches 6-7 files across 4 packages. The pain points are:

- **Discovery**: no template or scaffold — you copy from the most similar metric and adapt
- **Report wiring**: after the metric works standalone, connecting it to `report` is a separate puzzle in `metrics/dashboard.go` and `format/report.go`
- **Output shape inconsistency**: some metrics have Stats (p50/p90), some have counts, some have items — no common result structure

### What we explored and rejected

| Approach | Why rejected |
|----------|-------------|
| **Structured envelope** (`MetricResult[T any]` with Stats, Items, Insights) | Doesn't fit all metrics — bus-factor has risk levels not durations, reviews is a list with no stats, throughput is counts. Forces a shape that ~40% of metrics don't match. |
| **Directory-per-metric** (internal/flow/leadtime/) | Go package rules make shared types awkward. Moves complexity rather than removing it. Overkill for 5 metrics. |
| **Document the recipe only** (no interfaces) | No compile-time safety. User explicitly wants guardrails against forgetting an output format. |
| **ReportSection abstraction** (sections for composition) | Overfit to the report command. User wants the guardrails on *every* command, not just report. |
| **Format dispatch interface only** (Renderer with WritePretty/JSON/Markdown) | Too narrow — user also wants the data-gathering and processing pipeline to be consistent, not just rendering. |

### What we chose

A three-method pipeline interface that covers the full lifecycle. Each phase is independently testable:

```go
// internal/metric/pipeline.go (or wherever it lands)
type Pipeline interface {
    GatherData(ctx context.Context, deps *Deps) error
    ProcessData() error
    Render(format Format, w io.Writer, rc RenderContext) error
}
```

**GatherData** — not "Fetch" because sources include GitHub REST API, GraphQL, and local git repos. This method populates the metric's raw data (issues, PRs, commits, project items). Uses the `Deps` struct which carries client, config, owner/repo, time window.

**ProcessData** — pure computation on gathered data. Computes stats, insights, classifications. No I/O. This is the primary unit test target: inject fake data into the struct's fields, call ProcessData, assert on computed results.

**Render** — pure output. Takes the processed result and writes it in the requested format. No computation — just formatting. The format dispatch (`switch format`) lives in a shared helper, not in each command.

### Registration for report

An explicit slice in one file. No init() magic.

```go
// cmd/report.go
var reportSections = []metric.Pipeline{
    leadtime.New(config),
    cycletime.New(config),
    throughput.New(config),
    busfactor.New(config),
    reviews.New(config),
}
```

Adding a metric to the report = adding one line. Forgetting this line is the only manual step — and a test can verify the list covers all known metrics.

## Key Decisions

1. **Interface, not struct.** The pipeline is an interface so each metric can have its own result fields. No forced common shape. The interface is the guardrail; the data is metric-specific.

2. **GatherData takes Deps, not individual args.** The `Deps` struct already carries everything (client, config, owner, repo, time window). Passing it directly keeps the interface stable as new deps are added.

3. **ProcessData is separate from Render.** This is explicitly for testability. You can populate a metric struct with fake issues/PRs, call `ProcessData()`, and assert on computed stats without touching the formatter. Then test `Render()` separately with known processed results.

4. **Explicit registration, not self-registration.** An explicit list in `cmd/report.go` is grep-able, debuggable, and has no init() ordering surprises. One line per metric.

5. **No directory restructuring.** Keep the current package layout (cmd/, internal/metrics/, internal/format/). The interface provides consistency without moving files. This avoids a large mechanical refactor for a pre-1.0 project with 5 metrics.

6. **Shared format dispatch helper.** A single `WriteOutput(p Pipeline, format, w, rc)` function replaces the per-command switch statement. The command's RunE becomes: create metric → GatherData → ProcessData → WriteOutput.

## How a Cobra command looks after this

```go
func NewLeadTimeCmd() *cobra.Command {
    return &cobra.Command{
        Use: "lead-time",
        RunE: func(cmd *cobra.Command, args []string) error {
            deps := DepsFromContext(cmd.Context())
            w := cmd.OutOrStdout()

            lt := leadtime.New(deps.Config)
            if err := lt.GatherData(cmd.Context(), deps); err != nil {
                return err
            }
            if err := lt.ProcessData(); err != nil {
                return err
            }
            return metric.WriteOutput(lt, deps.Format, w, deps.RenderCtx(w))
        },
    }
}
```

## How testing looks after this

```go
func TestLeadTimeProcessData(t *testing.T) {
    lt := &leadtime.Metric{
        // Inject fake data — no API calls
        Issues: []model.Issue{
            {CreatedAt: day1, ClosedAt: &day5},
            {CreatedAt: day2, ClosedAt: &day4},
        },
    }
    err := lt.ProcessData()
    require.NoError(t, err)
    assert.Equal(t, 2, lt.Stats.Count)
    assert.Equal(t, 3*24*time.Hour, lt.Stats.Median)
}
```

## Open Questions

*None — all resolved during brainstorming.*

## Scope Boundaries

**In scope:**
- Pipeline interface definition
- Shared WriteOutput dispatch helper
- Refactor existing metrics to implement the interface
- Explicit report registration
- Tests for ProcessData with injected data

**Out of scope:**
- Directory restructuring (keep current layout)
- Generics-based MetricResult envelope (rejected)
- Auto-discovery or init() registration
- Changing the format package's internal structure
- Abstract Metric interface with ForItem/ForPerson/ForScope methods

## Relationship to Existing Plan

This brainstorm supersedes Phase 3 of `docs/plans/2026-03-12-001-feat-metrics-architecture-and-ux-plan.md`. The original Phase 3 proposed documenting the recipe without interfaces. This brainstorm adds the Pipeline interface as a compile-time guardrail while keeping the same goals: eliminate duplication, make it easy to add metrics, standardize the pattern.

Phases 1 (config required) and 2 (empty block messaging) of the original plan are unaffected.
