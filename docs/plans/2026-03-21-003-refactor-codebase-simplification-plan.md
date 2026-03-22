---
title: "refactor: Codebase simplification — simple over smart"
type: refactor
status: active
date: 2026-03-21
origin: docs/brainstorms/2026-03-21-codebase-simplification-requirements.md
deepened: 2026-03-21
---

# refactor: Codebase Simplification — Simple Over Smart

## Enhancement Summary

**Deepened on:** 2026-03-21
**Agents used:** architecture-strategist, code-simplicity-reviewer, pattern-recognition-specialist, best-practices-researcher, solution-docs-analyst

### Key Improvements from Deepening
1. Replaced BulkMetricItem interface (7 methods, over-engineered) with shared concrete `BulkItem` struct + embedded `BulkEnvelope` for JSON — zero new interfaces for render consolidation
2. Dropped custom `MarshalJSON` in favor of embedded envelope struct with per-command item types — simpler, type-safe, no runtime field-name tricks
3. Added Phase 0 (JSON snapshot tests) as a safety prerequisite before any refactoring begins
4. Added report integration test to verify phase parity with RunPipeline
5. Renamed `ClassifyFlags` → `ClassifyDurationFlags` to avoid collision with non-duration metrics
6. Added atomic migration safety protocol for WarningCollector embedding

### New Considerations Discovered
- The pipeline-per-metric solution doc says render logic should stay co-located with each metric — shared render *utilities* are fine, but shared render *functions* must not reverse this decision
- Enricher optional interface has exactly 2 call sites (WIP, report) — borderline per three-instances rule, but matches established `provenanceRenderer` pattern
- Report's cross-pipeline enrichment (leadtime items → quality computation) cannot be captured by a single-pipeline Enricher — remains explicitly outside the pattern
- `WarningCollector` embedding requires atomic migration: remove `Warnings []string` field before embedding to avoid silent method shadowing

---

## Overview

Reduce ~3,000-4,000 LOC of structural duplication and ad-hoc patterns across the pipeline/render/command layers while enforcing correctness through a redesigned Pipeline interface. The goal: a contributor can read one metric end-to-end, understand it, and add a new metric in <200 LOC.

Motivated by a confirmed WIP bug where a pipeline step was skipped — the interface should prevent this class of bug, not just name the phases.

## Problem Statement / Motivation

The codebase has accumulated three kinds of debt (see origin: `docs/brainstorms/2026-03-21-codebase-simplification-requirements.md`):

1. **Bypassed interface**: Every command manually calls `p.GatherData()` → `p.ProcessData()` → `p.Render()` instead of using `RunPipeline()`, because warnings, posting, and enrichment happen between phases. This caused a real bug where WIP forgot a step.
2. **Render duplication**: `cycletime/render.go` (350 LOC) and `leadtime/render.go` (268 LOC) are structurally identical. `classifyFlags`, `flagEmojis`, JSON structs, template loading, and format dispatch are copied across 7+ packages.
3. **Command boilerplate**: Every `run*` function repeats 15-20 lines of deps extraction, client creation, date parsing, and scope assembly.

## Proposed Solution

Five-phase refactoring executed bottom-up (snapshot safety net first, shared utilities, render consolidation, interface redesign, command cleanup) to keep tests green at each step.

### Design Decisions (resolved from origin deferred questions)

**Pipeline interface shape (affects R1):** Keep `GatherData/ProcessData/Render` as separate interface methods — this is necessary because the report runs GatherData concurrently via errgroup and needs phase-by-phase control. The fix is not changing the interface methods, but making `RunPipeline()` comprehensive enough that standalone commands ALWAYS use it. `RunPipeline()` will handle warnings and support pre-ProcessData hooks (for IssueType enrichment). The report is explicitly exempt (R9) and continues to call phases directly — it's the one place where manual orchestration is justified by concurrency and cross-pipeline data injection.

**Warnings design (affects R1, R8):** Standardize on a `Warnings() []string` method on the Pipeline interface via an embedded `WarningCollector` struct. `RunPipeline()` surfaces them. Commands stop manually iterating over `p.Warnings`. The report's mutex-guarded collection stays as-is (it's a legitimate concurrent pattern). **Migration safety**: each pipeline struct must be migrated atomically — remove the `Warnings []string` field before embedding `WarningCollector`, because Go's embedding rules silently shadow promoted methods when a field with the same name exists at the outer level.

**Generics approach (affects R4-R5):** Use generics for data-oriented operations only (e.g., existing `SortBy[T,K]`). Do NOT use `MetricResult[T any]` — this was explicitly rejected because ~40% of metrics don't fit a common shape. Do NOT create a `BulkMetricItem` interface — the two BulkItem types are identical structs, so a shared concrete type is simpler. Use generics only where three or more concrete instances exist.

**JSON wire format preservation (affects R5):** Use an embedded `BulkEnvelope` struct for the shared fields (repository, window, search_url, sort, stats, capped, warnings, insights) plus per-command item types with compile-time JSON tags (`json:"lead_time"` vs `json:"cycle_time"`). This eliminates 80% of envelope duplication while keeping the one field that truly differs in a per-command struct. **No custom `MarshalJSON` needed** — the embedded struct promotes its fields correctly via Go's default marshaling. This matches the codebase's existing pattern in `model.Provenance`.

**Command preamble (affects R8):** Extract focused sub-helpers (`parseDateWindow`, `buildBulkScope`) that each command composes, not a single `runMetric` god-function. Commands remain readable top-to-bottom.

**Release pipeline (affects R2-R3):** Move `gatherReleaseData()` into `release.Pipeline.GatherData()` so it participates in the enforced interface. This requires making `deps` and `gitdata.Source` pipeline fields — a straightforward mechanical change.

**WIP rendering (affects R1):** Bring WIP into the `renderPipeline()` flow for consistency. Currently it's the only standalone command that bypasses it, which means no `--write-to` support. Do NOT add `--post` behavior — just normalize the rendering path.

### What stays untouched (scope boundaries from origin)

- `strategy/`, `classify/`, `effort/` — justified polymorphism
- `github/` client and caching — appropriate and clear
- `config/`, `scope/`, `dateutil/`, `log/`, `model/` — well-scoped
- JSON output wire shapes — existing consumers must not break
- CLI flag names and behavior

### Hard constraints from solution docs

These constraints are documented in `docs/solutions/` and must be preserved:

- **Four-layer output shape** (stats → detail → insights → provenance) — every command's output follows this contract (`docs/solutions/architecture-patterns/command-output-shape.md`)
- **Rendering is zero-API** — the render phase reads only from in-memory domain types, never calls APIs
- **`metrics/` never imports `format/`** — insight messages are markdown strings; the render layer adapts per format (`docs/solutions/architecture-patterns/render-layer-linking-and-insight-quality.md`)
- **Three-state metric status** — duration metrics use independent `startedAt *time.Time` + `duration *time.Duration` pointers. `FormatCycleStatus` is the single source of truth (`docs/solutions/three-state-metric-status-pattern.md`)
- **JSON stderr purity** — when `--results json`, warnings go into JSON `"warnings"` field, never stderr. `deps.WarnUnlessJSON()` is the correct helper (`docs/solutions/architecture-patterns/complete-json-output-for-agents.md`)
- **Render logic co-located with metrics** — per the pipeline-per-metric layout, each metric's render functions live in its own directory. Shared *utilities* in `format/` are fine; shared render *functions* that replace per-metric render.go files must not reverse this decision (`docs/solutions/architecture-refactors/pipeline-per-metric-and-preflight-first-config.md`)

## Technical Approach

### Phase 0: Safety Net (prerequisite, before any refactoring)

Capture current output to detect regressions across all four phases.

#### 0a. JSON snapshot tests

**Files to change:**
- `cmd/snapshot_test.go` (NEW) — capture JSON output for at least: leadtime bulk, cycletime bulk, throughput, velocity, wip, report. Compare against golden files.
- `testdata/snapshots/` (NEW) — golden JSON files

**Why:** The "JSON output byte-identical" acceptance criterion needs a machine-verified gate. Without this, template consolidation (Phase 2) and command migration (Phase 3) can introduce subtle output drift that passes unit tests but breaks consumers.

**Test:** `task test` — snapshots pass trivially on first run (they are the baseline).

#### 0b. Report phase-parity test

**Files to change:**
- `cmd/report_test.go` (EDIT) — add a test that verifies report.go calls the same lifecycle phases as RunPipeline (gather, enrich, process, render) for each section. This converts the "report must replicate RunPipeline phases" dependency from human memory to machine verification.

### Phase 1: Extract Shared Utilities (low risk, high payoff)

Move duplicated code into shared locations. No interface changes. Tests stay green throughout.

#### 1a. Consolidate render utilities into `internal/format/`

**Files to change:**
- `internal/format/durationflags.go` (NEW) — move `classifyFlags()` as `ClassifyDurationFlags()` and `flagEmojis()` as `FlagEmojis()` from `cycletime/render.go` and `leadtime/render.go`. Name is `ClassifyDurationFlags` (not `ClassifyFlags`) because reviews uses different logic (age-based, not duration-based). Also consolidate noise/hotfix threshold constants here — leadtime uses named constants, cycletime uses inline literals.
- `internal/format/json.go` (EDIT) — add `WriteIndentedJSON(w io.Writer, v any) error` helper to replace the 10+ copies of `enc := json.NewEncoder(w); enc.SetIndent("", "  "); return enc.Encode(v)`
- `internal/format/insights.go` (EDIT) — remove redundant `jsonVelocityInsight` and `jsonThroughputInsight` types from velocity and throughput render files; use `format.JSONInsight` everywhere
- `internal/pipeline/cycletime/render.go` (EDIT) — replace local functions with `format.ClassifyDurationFlags`/`format.FlagEmojis`
- `internal/pipeline/leadtime/render.go` (EDIT) — same replacement
- `internal/pipeline/velocity/render.go` (EDIT) — replace local insight type with `format.JSONInsight`
- `internal/pipeline/throughput/render.go` (EDIT) — replace local insight type with `format.JSONInsight`

**Test:** `task test` — all existing tests pass with import path changes only. Snapshot tests from Phase 0 verify output unchanged.

#### 1b. Extract shared test helpers

**Files to change:**
- `internal/pipeline/testutil_test.go` (NEW) — extract `containsStr()` duplicated in cycletime, throughput, and wip test files
- Update test files to use shared helper

**Test:** `task test`

#### 1c. Extract command-layer helpers

**Files to change:**
- `cmd/datewindow.go` (NEW) — extract `parseDateWindow(sinceStr, untilStr string, now time.Time) (since, until time.Time, err error)` from the ~15 lines duplicated in every bulk command (cycletime.go, leadtime.go, throughput.go, velocity.go)
- `cmd/cycletime.go`, `cmd/leadtime.go`, `cmd/throughput.go`, `cmd/velocity.go` (EDIT) — replace inline date parsing with `parseDateWindow()`

**Test:** `task test` + verify existing command tests still pass.

### Phase 2: Collapse Bulk Duration Rendering (medium risk, highest LOC reduction)

Unify cycletime and leadtime bulk rendering using shared concrete types and parameterized helpers. Per the pipeline-per-metric convention, render *utilities* go in `format/` but the per-metric `Render()` method and format dispatch stay in each pipeline's `render.go`.

#### 2a. Create shared bulk types

**Files to change:**
- `internal/pipeline/bulkitem.go` (NEW) — shared concrete struct:

```go
// BulkItem holds a single item's metric result for bulk duration-based commands.
// Used by cycletime and leadtime (identical struct shape).
type BulkItem struct {
    Issue  model.Issue
    Metric model.Metric
}
```

- `internal/format/bulkenvelope.go` (NEW) — shared JSON envelope:

```go
// BulkEnvelope holds the shared JSON fields for bulk metric output.
// Per-command JSON output types embed this to avoid duplicating
// repository, window, sort, stats, warnings, and insights fields.
type BulkEnvelope struct {
    Repository string        `json:"repository"`
    Window     JSONWindow    `json:"window"`
    SearchURL  string        `json:"search_url"`
    Sort       JSONSort      `json:"sort"`
    Stats      JSONStats     `json:"stats"`
    Capped     bool          `json:"capped,omitempty"`
    Warnings   []string      `json:"warnings,omitempty"`
    Insights   []JSONInsight `json:"insights,omitempty"`
}
```

- `internal/format/bulkrender.go` (NEW) — shared rendering utilities:

```go
// WriteBulkPrettyHeader writes the standard header for bulk metric output.
func WriteBulkPrettyHeader(rc RenderContext, title, repo string, since, until time.Time, insights []model.Insight, stats model.Stats) { ... }

// WriteBulkPrettyFooter writes the capped-rows message.
func WriteBulkPrettyFooter(w io.Writer, total, shown int) { ... }

// BuildBulkMarkdownData builds the common template data fields for bulk markdown.
func BuildBulkMarkdownData(repo string, since, until time.Time, insights []model.Insight, stats model.Stats, searchURL string, sortField, sortLabel string, sorted Sorted[BulkItem], totalCount int) BulkMarkdownData { ... }
```

These are render *utilities* (build data, write headers) — not complete render functions. Each metric's `render.go` still owns its `WriteBulkMarkdown`, `WriteBulkJSON`, and `WriteBulkPretty` but delegates the common parts.

- `internal/format/bulkenvelope_test.go` (NEW) — tests for shared envelope marshaling
- `internal/format/bulkrender_test.go` (NEW) — tests for shared render utilities

### Research Insights for Phase 2

**From simplicity review:** The two `BulkItem` types are identical structs. A shared concrete type eliminates the need for any interface. Each metric's render.go keeps its own `jsonBulkItem` with the correct `json:"lead_time"` or `json:"cycle_time"` tag — this is the one field that truly differs and is best expressed as a compile-time JSON tag, not a runtime abstraction.

**From pattern review:** `format.SortBy[T,K]` already works on generic item types. The shared `pipeline.BulkItem` can be used directly with `SortBy` — no adapter needed.

**From best-practices research:** Go struct embedding promotes fields correctly for JSON marshaling. `BulkEnvelope` embedded in a per-command output type will serialize all fields at the top level, exactly matching the current wire format.

#### 2b. Adapt cycletime and leadtime to use shared types

**Files to change:**
- `internal/pipeline/cycletime/render.go` (MAJOR EDIT):
  - Remove local `BulkItem` type, import `pipeline.BulkItem`
  - Remove `jsonBulkOutput` envelope fields that are now in `format.BulkEnvelope`
  - Keep `jsonBulkItem` with its `CycleTime format.JSONMetric` field (compile-time tag)
  - Replace `classifyFlags`/`flagEmojis` calls with `format.ClassifyDurationFlags`/`format.FlagEmojis` (already done in 1a)
  - Delegate common header/footer in `WriteBulkPretty` to `format.WriteBulkPrettyHeader`/`Footer`
- `internal/pipeline/cycletime/cycletime.go` (EDIT) — use `pipeline.BulkItem` instead of local type
- `internal/pipeline/leadtime/render.go` (MAJOR EDIT) — same treatment
- `internal/pipeline/leadtime/leadtime.go` (EDIT) — use `pipeline.BulkItem`

**Markdown templates:**
- `internal/format/templates/bulkmetric.md.tmpl` (NEW) — unified template for the shared sections (header, stats, insights, footer). Per-metric templates include this via `{{ template "bulkmetric-header" . }}` and add their own detail table.
- Keep `cycletime/templates/cycletime-bulk.md.tmpl` and `leadtime/templates/leadtime-bulk.md.tmpl` but refactor them to use the shared template blocks

**Test strategy:** Run snapshot tests from Phase 0 to verify output is identical. `task test` after each file change.

**Estimated reduction:** ~300-400 LOC from render files + ~30 LOC from shared BulkItem.

#### 2c. Evaluate other bulk metrics for shared rendering

After 2a-2b, evaluate whether throughput, reviews, or quality bulk rendering can also use the shared helpers. Based on research:
- **Throughput**: Different shape (counts, not durations) — likely needs its own rendering but can use `WriteIndentedJSON` and `format.BulkEnvelope`.
- **Reviews**: Different shape (age-based, stale flags) — keep separate but can use `BulkEnvelope` for the JSON wrapper.
- **Quality**: Different shape (category breakdown) — keep separate.

Apply the three-instances rule: only extract further if three concrete cases exist. Do not force-fit metrics that don't match the duration-based pattern.

### Phase 3: Pipeline Interface Redesign (highest risk, correctness enforcement)

Redesign `RunPipeline()` to be comprehensive enough that standalone commands always use it. Keep the three-method interface but make bypass unnecessary.

#### 3a. Add Warnings to the Pipeline interface

**Files to change:**
- `internal/pipeline/pipeline.go` (EDIT):

```go
type Pipeline interface {
    GatherData(ctx context.Context) error
    ProcessData() error
    Render(rc format.RenderContext) error
    Warnings() []string
}
```

- Add `WarningCollector` embeddable struct:

```go
// WarningCollector provides the Warnings() method for Pipeline implementations.
// Embed this in pipeline structs instead of manually declaring []string fields.
//
// Migration safety: when embedding WarningCollector in an existing struct,
// FIRST remove any existing `Warnings []string` field. Go silently shadows
// promoted methods when a field with the same name exists at the outer level.
type WarningCollector struct {
    warnings []string
}

func (wc *WarningCollector) AddWarning(msg string)                    { wc.warnings = append(wc.warnings, msg) }
func (wc *WarningCollector) AddWarningf(format string, args ...any)   { wc.warnings = append(wc.warnings, fmt.Sprintf(format, args...)) }
func (wc *WarningCollector) Warnings() []string                       { return wc.warnings }
```

- Update ALL pipeline structs to embed `WarningCollector` and replace `p.Warnings = append(p.Warnings, ...)` with `p.AddWarning(...)`.

**Atomic migration per struct:**
1. Remove `Warnings []string` field
2. Add `pipeline.WarningCollector` embed
3. Replace all `p.Warnings = append(p.Warnings, ...)` with `p.AddWarning(...)`
4. Add compile-time check: `var _ pipeline.Pipeline = (*MyPipeline)(nil)`
5. Run `task test`

Do NOT change multiple structs in a single commit. One struct at a time.

### Research Insights for Phase 3a

**From best-practices research:** Go's embedding promotes methods from inner to outer struct. If the outer struct has a *field* named `Warnings`, it silently shadows the promoted `Warnings()` method — the compiler does not warn. The `go vet` tool also does not catch this. The only protection is the atomic migration protocol above.

**From pattern review:** The zero-value `WarningCollector{}` is safe — `append` on nil creates the slice, no constructor needed. Pointer receivers on `WarningCollector` require pipeline structs to be used as `*T`, which already matches how all pipelines work.

**Test:** `task test` — purely mechanical change, no behavior difference. Snapshot tests verify output unchanged.

#### 3b. Make RunPipeline comprehensive

**Files to change:**
- `internal/pipeline/pipeline.go` (EDIT):

```go
// Optional interfaces checked by RunPipeline:
//   - Enricher: called between GatherData and ProcessData
//
// Optional interfaces checked by renderPipeline in cmd/:
//   - provenanceRenderer: controls provenance footer
//
// Keep this list short. If a third optional interface is needed,
// reconsider whether the Pipeline interface itself should change.

// RunResult holds the outcome of RunPipeline for the caller.
type RunResult struct {
    Warnings []string
}

// RunPipeline executes the three-phase lifecycle in order.
// Standalone commands should ALWAYS use this — never call phases directly.
// The report command is the one exception (see cmd/report.go).
func RunPipeline(ctx context.Context, p Pipeline, rc format.RenderContext) (RunResult, error) {
    if err := p.GatherData(ctx); err != nil {
        return RunResult{}, err
    }
    // Optional enrichment between gather and process.
    if e, ok := p.(Enricher); ok {
        if err := e.Enrich(ctx); err != nil {
            return RunResult{}, err
        }
    }
    if err := p.ProcessData(); err != nil {
        return RunResult{}, err
    }
    if err := p.Render(rc); err != nil {
        return RunResult{}, err
    }
    return RunResult{Warnings: p.Warnings()}, nil
}

// Enricher is an optional interface. If a pipeline implements it,
// RunPipeline calls Enrich between GatherData and ProcessData.
// Used by WIP (IssueType enrichment) and issue detail pipeline.
type Enricher interface {
    Enrich(ctx context.Context) error
}
```

### Research Insights for Phase 3b

**From architecture review:** The Enricher pattern formalizes what is currently ad-hoc code at the cmd layer. It matches the established `provenanceRenderer` optional-interface pattern. The Go stdlib uses this extensively (`http.Flusher`, `http.Hijacker`, `io.WriterTo`). The ceiling comment caps optional interfaces and makes the next person think twice before adding a third.

**From simplicity review:** Enricher has exactly 2 call sites (WIP, issue detail). This is borderline per the three-instances rule. However, the alternative is commands calling phases manually, which is exactly the pattern that caused the WIP bug. The correctness benefit outweighs the concept cost.

**From solution docs:** Report's cross-pipeline enrichment (leadtime items → quality computation) cannot be captured by single-pipeline Enricher — this stays in report.go's manual orchestration, which is already exempted.

#### 3c. Migrate standalone commands to RunPipeline

**Files to change (each command file):**
- `cmd/leadtime.go` — replace manual `GatherData/ProcessData/warn loop` with `renderPipeline()` driving the full lifecycle
- `cmd/cycletime.go` — same
- `cmd/throughput.go` — same
- `cmd/velocity.go` — same
- `cmd/reviews.go` — same
- `cmd/issue.go` — same
- `cmd/pr.go` — same
- `cmd/wip.go` — bring into `renderPipeline()` flow (currently bypasses it); move IssueType enrichment into WIP pipeline's `Enrich()` method
- `cmd/release.go` — move `gatherReleaseData()` into pipeline's `GatherData()`

**Pattern for each command after migration:**
```go
func runLeadTimeBulk(cmd *cobra.Command, sinceStr, untilStr string) error {
    ctx := cmd.Context()
    deps := DepsFromContext(ctx)
    // ... parse flags, build scope (unique per command) ...

    p := &leadtime.BulkPipeline{
        Client: client, Owner: deps.Owner, Repo: deps.Repo,
        Since: since, Until: until,
        SearchQuery: query.Build(), SearchURL: query.URL(),
    }

    return renderPipeline(cmd, deps, p, client, posting.PostOptions{...})
}
```

Where `renderPipeline` calls `pipeline.RunPipeline()` internally (it already calls `p.Render()` — extend it to call all phases).

**Critical: `cmd/report.go` stays as-is.** The report continues to call phases directly because it needs concurrent GatherData, cross-pipeline data injection, and per-section error isolation. This is explicitly allowed by R9. Add a comment listing the RunPipeline phases it must replicate:

```go
// report.go orchestrates pipelines directly (not via RunPipeline) because:
// - GatherData runs concurrently via errgroup
// - Cross-pipeline data injection (throughput → WIP)
// - Per-section error isolation (one failure doesn't abort others)
// - Cross-pipeline enrichment (IssueType on leadtime items for quality)
//
// When RunPipeline phases change, update this orchestration to match.
// Phases: GatherData → [Enrich] → ProcessData → Render → Warnings
```

**Migrate one command at a time.** Run `task test` + snapshot tests after each.

#### 3d. Thin wrapper pipelines

After 3c is complete and all commands go through `RunPipeline`, evaluate thin wrappers:

- `leadtime.SinglePipeline` — 4 methods, ~30 LOC total. **Decision: keep it.** The overhead is small and the benefit of enforced `RunPipeline()` sequencing is proven by the WIP bug.
- `cycletime.PRPipeline` — similarly small. **Same decision: keep it.**

The real LOC savings are in render consolidation (Phase 2), not in eliminating small structs.

### Phase 4: Command Layer Cleanup (low risk, readability improvement)

With phases 1-3 done, commands are already simpler. This phase extracts remaining boilerplate.

#### 4a. Simplify renderPipeline to drive the full lifecycle

**Files to change:**
- `cmd/render.go` (EDIT) — `renderPipeline` currently only calls `p.Render()`. After phase 3, it calls `pipeline.RunPipeline()` to handle the full lifecycle, then handles posting and `--write-to`. Commands pass the pipeline struct and let renderPipeline drive everything.

```go
// renderPipeline runs the full pipeline lifecycle and handles output routing.
func renderPipeline(cmd *cobra.Command, deps *Deps, p pipeline.Pipeline, client *gh.Client, postOpts posting.PostOptions) error {
    rc := deps.RenderCtx(/* writer setup */)
    result, err := pipeline.RunPipeline(cmd.Context(), p, rc)
    if err != nil {
        return err
    }
    for _, w := range result.Warnings {
        deps.Warn("%s", w)
    }
    // ... posting logic, provenance, --write-to ...
}
```

#### 4b. Extract scope query helpers

**Files to change:**
- `cmd/scopehelpers.go` (NEW) — extract `buildBulkIssueScope(deps, since, until)` and `buildBulkPRScope(deps, since, until)` patterns repeated across bulk commands
- Update `cmd/cycletime.go`, `cmd/leadtime.go`, `cmd/throughput.go`, `cmd/velocity.go`

**Test:** `task test`

## System-Wide Impact

- **Interaction graph**: Pipeline interface change affects all 10 pipeline implementations + all command files + report orchestration. `renderPipeline()` becomes the single entry point for standalone commands. The report is explicitly exempt and must manually replicate RunPipeline phases.
- **Error propagation**: No change — errors flow up from pipeline methods through Cobra's RunE. The report's per-section error isolation is untouched.
- **State lifecycle risks**: Low — refactoring is mechanical; no new state introduced. The enrichment hook is the one new concept, and it replaces ad-hoc enrichment that already exists.
- **API surface parity**: JSON output shapes are frozen. CLI flags are frozen. Only internal Go API changes. Verified by Phase 0 snapshot tests.
- **Integration test scenarios**: Smoke tests (`task test:smoke`) validate end-to-end CLI behavior. Run after each phase. Phase 0 snapshots catch output drift.

## Acceptance Criteria

### Functional Requirements

- [ ] All existing tests pass (`task test`)
- [ ] All smoke tests pass (`task test:smoke`)
- [ ] JSON snapshot tests pass (output identical to Phase 0 baseline)
- [ ] CLI flag names and behavior unchanged
- [ ] `--post`, `--write-to`, `--results` work identically for all commands including WIP (which gains `--write-to` support)

### Contributor Experience

- [ ] A new duration-based metric can be added in <200 LOC (validate by counting what leadtime would require after refactoring)
- [ ] Each metric command reads top-to-bottom without jumping to interface definitions
- [ ] Shared render utilities have clear doc comments explaining parameters

### Quality Gates

- [ ] Net LOC reduction of at least 1,000 lines (revised down from 1,500 — simpler approach trades less LOC reduction for fewer new concepts)
- [ ] No new packages created (shared code goes in existing `format/` and `pipeline/` packages)
- [ ] `task quality` passes (lint + staticcheck)
- [ ] New concept count: exactly 3 (WarningCollector, Enricher, BulkEnvelope) — not 6 as originally planned
- [ ] Report phase-parity test passes

## Success Metrics

- **Primary**: contributor can trace the full data flow of any metric command without leaving the pipeline subdirectory + command file
- **Secondary**: net LOC reduction (target: 1,000-2,000 after phases 0-4)
- **Regression signal**: `task test`, `task test:smoke`, and snapshot tests green after every phase

## Dependencies & Risks

**Risk: Phase 2 markdown template consolidation may produce subtly different output.**
Mitigation: Phase 0 snapshot tests detect this automatically. Capture current markdown output for cycletime and leadtime before starting. Diff after.

**Risk: Phase 3c (migrating commands to RunPipeline) breaks edge cases in posting or --write-to.**
Mitigation: One command at a time. Run smoke tests + snapshot tests after each migration.

**Risk: Report command is exempted from RunPipeline enforcement, leaving it vulnerable to the same class of bug.**
Mitigation: Phase 0b adds a report phase-parity test. When RunPipeline phases change, this test fails, forcing report.go to be updated.

**Risk: WarningCollector embedding silently shadows existing Warnings field.**
Mitigation: Atomic migration protocol (Phase 3a). One struct at a time, remove field before embedding.

**Dependency: The WIP bug should be fixed before or during this refactoring.**
The bug that motivated R1 should be identified and fixed. Phase 3's migration will likely fix it naturally when WIP goes through `RunPipeline()`.

## Execution Order

```
Phase 0a (JSON snapshots)       ← safety prerequisite
Phase 0b (report parity test)   ← safety prerequisite
Phase 1a (render utilities)     ← safe, mechanical
Phase 1b (test helpers)         ← safe, mechanical
Phase 1c (command helpers)      ← safe, mechanical
Phase 2a (shared bulk types)    ← new code, needs tests
Phase 2b (cycletime/leadtime)   ← highest LOC reduction
Phase 2c (evaluate others)      ← judgment call, may be no-op
Phase 3a (Warnings interface)   ← mechanical, one struct at a time
Phase 3b (RunPipeline redesign) ← core correctness fix
Phase 3c (migrate commands)     ← one at a time, highest risk
Phase 4a (renderPipeline)       ← depends on 3b
Phase 4b (scope helpers)        ← safe, mechanical
```

Phase 0 must complete before any other work. Phases 1a-1c can run in parallel. Phase 2 depends on 1a. Phase 3 depends on 2 (so render files are already simplified). Phase 4 depends on 3.

## Sources & References

### Origin

- **Origin document:** [docs/brainstorms/2026-03-21-codebase-simplification-requirements.md](docs/brainstorms/2026-03-21-codebase-simplification-requirements.md) — Key decisions carried forward: interfaces for correctness not taxonomy, generics where they collapse duplication, shared helpers over generic frameworks, one big sweep execution.

### Internal References

- Pipeline interface: `internal/pipeline/pipeline.go`
- Render entry point: `cmd/render.go`
- Report orchestration: `cmd/report.go`
- Highest duplication: `internal/pipeline/leadtime/render.go` + `internal/pipeline/cycletime/render.go`
- Pipeline interface brainstorm: `docs/brainstorms/2026-03-12-metric-pipeline-interface-brainstorm.md` — MetricResult[T] explicitly rejected
- Pipeline-per-metric refactor: `docs/solutions/architecture-refactors/pipeline-per-metric-and-preflight-first-config.md`
- Output shape contract: `docs/solutions/architecture-patterns/command-output-shape.md`
- Generics precedent: `docs/solutions/architecture-refactors/lipgloss-table-migration.md` — SortBy[T,K] is the proven pattern
- JSON stderr purity: `docs/solutions/architecture-patterns/complete-json-output-for-agents.md`
- Render-layer boundaries: `docs/solutions/architecture-patterns/render-layer-linking-and-insight-quality.md`
- Three-state metrics: `docs/solutions/three-state-metric-status-pattern.md`

### External References

- Go embedding best practices: [Effective Go - Embedding](https://go.dev/doc/effective_go)
- Optional interface pattern: [The Trouble with Optional Interfaces (Axel Wagner)](https://blog.merovius.de/posts/2017-07-30-the-trouble-with-optional-interfaces/)
- stdlib optional interfaces: `net/http.Flusher`, `io.WriterTo`, `fmt.Stringer`
