---
title: "feat: Metrics Pipeline Interface, Config Required, and Empty Messaging"
type: feat
status: active
date: 2026-03-12
origin: docs/brainstorms/2026-03-12-metric-pipeline-interface-brainstorm.md
supersedes: docs/plans/2026-03-12-001-feat-metrics-architecture-and-ux-plan.md
---

# feat: Metrics Pipeline Interface, Config Required, and Empty Messaging

## Overview

Three reinforcing improvements to gh-velocity's architecture and UX:

1. **Config required** — eliminate implicit fallback scope, require `.gh-velocity.yml`
2. **Empty block messaging** — explain *why* results are empty with verify links
3. **Pipeline interface** — `GatherData` / `ProcessData` / `Render` interface for all metrics with compile-time guardrails and report auto-wiring

Phase 3 supersedes the original plan's "document the recipe" approach. The brainstorm concluded that a Go interface provides compile-time safety without premature abstraction. (see brainstorm: `docs/brainstorms/2026-03-12-metric-pipeline-interface-brainstorm.md`)

---

## Phase 1: Config Required — No Implicit Fallback

### Problem

The implicit `repo:owner/repo` fallback when no config exists causes confusion. Users get results scoped to the auto-detected repo without understanding why other repos in their org are excluded. The `-R` flag, config scope, and fallback scope interact in surprising ways. (see brainstorm: `docs/brainstorms/2026-03-11-metrics-architecture-and-ux-brainstorm.md` section 1)

### Decision

**All commands except `config *` subcommands and `version` require `.gh-velocity.yml`.** No implicit fallback.

- `preflight` is the entry point — it generates config
- Commands without config error with a clear message mentioning `preflight --write`
- Use exit code `ErrConfigInvalid` (exit 2) for missing config

### Implementation

**`cmd/root.go`** — collapse lines 183-202 into a config-required check:

```go
// After resolving configPath:
cfg, err = config.Load(configPath)
if err != nil {
    return &model.AppError{
        Code: model.ErrConfigInvalid,
        Message: fmt.Sprintf("no valid config found (%v)\n\n  Quick start:  gh velocity config preflight -R owner/repo --write\n  Or manually:  gh velocity config create", err),
    }
}
```

**Remove implicit scope injection** at lines 214-217:

```go
// REMOVE this block:
if resolvedScope == "" {
    resolvedScope = fmt.Sprintf("repo:%s/%s", owner, repo)
}
```

After removing the implicit fallback, add a validation check:

```go
resolvedScope := scope.MergeScope(cfg.Scope.Query, scopeFlag)
if resolvedScope == "" {
    return &model.AppError{
        Code:    model.ErrConfigInvalid,
        Message: "no scope configured. Add scope.query to .gh-velocity.yml or use --scope flag.",
    }
}
```

### Tasks

- [x] Remove `cfg = config.Defaults()` fallback branch in `cmd/root.go:199-201`
- [x] Collapse the two error branches (lines 186-201) into a single error return
- [x] Remove implicit `repo:owner/repo` scope injection (`cmd/root.go:215-217`)
- [ ] Add empty-scope validation with helpful error message after `scope.MergeScope()` — skipped: would break bus-factor and single-issue commands that don't use scope
- [x] Keep `config.Defaults()` exported for test fixtures; add comment "for tests and config subcommands only"
- [x] Do NOT add `isConfigSubcommand()` helper — keep the existing inline switch (lines 141-148) as-is
- [x] Update smoke tests — tests that run without config need a config file or `--config` with a test fixture
- [x] Verify `config create`, `config show`, `config validate`, `config preflight`, `config discover` all still skip `PersistentPreRunE` via `cmd.Parent().Name() == "config"` check
- [x] Verify `version` still skips via `cmd.Name() == "version"` check

### Edge Cases

- **CI with `GH_REPO` env var**: still needs config file. Document in guide.
- **Subdirectory**: `config.Load()` uses `os.Stat` on a relative path — does NOT walk up to git root. This is a known limitation. Users must run from the repo root or use `--config`.
- **`config show` on missing file**: currently loads defaults. After this change it should error with "no config found" and suggest `config create`. Config show/validate call `config.Load()` directly in their `RunE` — they need their own error handling for missing config.
- **Empty scope after config exists**: a config created by `config create` has `scope.query` commented out. Must validate and error if scope is empty.

### Acceptance Criteria

- [x] Commands without config fail with clear error mentioning `preflight --write`
- [ ] Commands with config but empty scope fail with clear error mentioning scope — deferred: not all commands use scope
- [x] `config *` subcommands and `version` work without config
- [x] `-R` flag still works when config exists
- [x] Smoke tests updated for new requirement
- [ ] No implicit `repo:owner/repo` scope injection anywhere

---

## Phase 2: Empty Block Messaging with Evidence Links

### Problem

Empty sections show `_None_` or skip content. Users can't tell if it's a scope problem, wrong time window, or genuinely no data. (see brainstorm: `docs/brainstorms/2026-03-11-metrics-architecture-and-ux-brainstorm.md` section 2)

### Decision

Empty results include: (1) human-readable explanation, (2) clickable GitHub search URL to verify. The `search_url` field is **always included** in JSON output regardless of whether results are empty — a stable schema is better for machine consumers. (Agent finding: conditional JSON fields are an anti-pattern per pattern-recognition and simplicity reviewers.)

### Design

**Pretty output:**
```
Issues Closed: 0
  No issues closed in this period.
  Verify: https://github.com/search?q=repo:owner/repo+is:issue+is:closed+closed:2026-03-01..2026-03-08
```

**Markdown output:**
```markdown
**Issues Closed (0)** — _No issues closed in this period. [Verify search](https://github.com/search?q=...)_
```

**JSON output — search_url always present:**
```json
{
  "items": [],
  "search_url": "https://github.com/search?q=...",
  "stats": { "count": 0 }
}
```

### Implementation

**Fix `Query.URL()`** — `internal/scope/scope.go:37`:

```go
// Change /issues?q= to /search?q=
return "https://github.com/search?q=" + url.QueryEscape(query)
```

**Pass `searchURL string` as a parameter to Write* functions** — do NOT add `SearchURL` fields to format data structs. The URL is a formatting concern computed in the command layer. (Agent finding: keeps format functions as pure renderers.)

Example signature change:
```go
// Before:
func WriteLeadTimeBulkPretty(rc RenderContext, items []BulkLeadTimeItem, stats model.Stats) error

// After:
func WriteLeadTimeBulkPretty(rc RenderContext, items []BulkLeadTimeItem, stats model.Stats, searchURL string) error
```

For JSON, add `SearchURL` to the JSON output structs (not the domain structs):
```go
type jsonBulkLeadTimeOutput struct {
    Repository string             `json:"repository"`
    Window     jsonWindow         `json:"window"`
    SearchURL  string             `json:"search_url"`
    Items      []jsonBulkLeadItem `json:"items"`
    Stats      JSONStats          `json:"stats"`
}
```

### Tasks

- [x] Fix `Query.URL()` to use `github.com/search?q=` not `github.com/issues?q=` (`internal/scope/scope.go:37`)
- [x] Add `searchURL string` parameter to all Write* functions that can be empty
- [x] Update `WriteLeadTimeBulkPretty/Markdown/JSON` — show verify link, always include `search_url` in JSON
- [x] Update `WriteCycleTimeBulkPretty/Markdown/JSON` — same
- [x] Update `WriteMyWeekPretty/Markdown/JSON` — per-section verify links (lookback sections only: issues closed, PRs merged, PRs reviewed)
- [x] Update `WriteReviewsPretty/Markdown/JSON` — verify link
- [x] Update `WriteThroughputPretty/Markdown/JSON` — verify link
- [x] Update markdown templates to include verify links in empty state
- [x] Add `SearchURL` field to JSON output structs (always present, not conditional)
- [x] Compute searchURL in command layer using `q.URL()` and pass to formatters
- [ ] Add empty-state smoke tests

### Scope

Commands that search GitHub via REST search get verify links. Commands that use GraphQL (wip/project-board) or local git (bus-factor) do NOT — there's no equivalent search URL.

### Edge Cases

- **Long URLs**: GitHub search URLs can be long. Accept this — they're functional links.
- **Private repos**: URL requires GitHub auth in browser. Document this caveat.
- **GitHub search query limit**: 256 chars max for query text. Typical scope queries are ~90 chars. Not a concern for single-repo queries.
- **my-week sections**: Only lookback sections (issues closed, PRs merged, PRs reviewed) get verify URLs. Lookahead sections show current state, not search results.
- **`--post` output**: Verify URLs should be included in posted output — they help readers of the Discussion verify the scope.

### Acceptance Criteria

- [x] Empty bulk lead-time/cycle-time shows verify search URL
- [x] Empty my-week lookback sections show per-section verify URLs
- [x] Empty reviews shows verify URL
- [x] Empty throughput shows verify URL
- [x] JSON always includes `search_url` field (not conditional on empty)
- [x] `Query.URL()` produces correct `github.com/search?q=` URLs
- [ ] Verify links are functional (manual test against real repo)

---

## Phase 3: Pipeline Interface

### Problem

Adding a new metric requires knowing where to put things across 6-7 files. There's no compile-time safety — forget a formatter and it silently fails. `model.ComputeInsights()` duplicates `metrics.ComputeStats()` logic and ignores the configured cycle-time strategy. The report command has manual wiring that's easy to forget. (see brainstorm: `docs/brainstorms/2026-03-12-metric-pipeline-interface-brainstorm.md`)

### Decision

**A three-method `Pipeline` interface** that every metric command implements. The compiler enforces completeness. No directory restructuring, no generics envelope, no abstract MetricResult.

### Alternatives Considered

| Approach | Why rejected |
|----------|-------------|
| Structured envelope `MetricResult[T any]` | Doesn't fit all metrics — bus-factor has risk levels not durations, reviews is a list with no stats, throughput is counts |
| Directory-per-metric | Go package rules make shared types awkward. Moves complexity, doesn't remove it. Overkill for 5 metrics |
| Document the recipe only | No compile-time safety. User explicitly wants guardrails against forgetting an output format |
| ReportSection abstraction | Overfit to report command. User wants guardrails on every command |
| Format Renderer interface only | Too narrow — doesn't cover data-gathering and processing pipeline |

(see brainstorm for full exploration)

### The Pipeline Interface

```go
// internal/metric/pipeline.go
package metric

import (
    "context"
    "io"

    "github.com/bitsbyme/gh-velocity/internal/format"
)

// Pipeline defines the three-phase lifecycle every metric command follows.
// Implementing this interface guarantees compile-time completeness:
// forget Render() and it won't compile.
type Pipeline interface {
    // GatherData fetches raw data from GitHub API, GraphQL, or local git.
    // For partial failures (e.g., PR lookup fails but issues succeed),
    // store warnings internally and return nil. Only return error for
    // total failures that prevent any useful output.
    GatherData(ctx context.Context, deps *Deps) error

    // ProcessData computes metrics from gathered data. No I/O.
    // This is the primary unit test target: inject fake data, call ProcessData, assert results.
    ProcessData() error

    // Render writes the processed result in the requested format.
    // No computation — pure output. Uses rc.Format and rc.Writer.
    Render(rc format.RenderContext) error
}

// Command-specific parameters (issue number, tag, --since, --pr flag)
// are captured by the Pipeline struct's constructor, NOT passed through
// the interface methods. Example: leadtime.NewSingle(issueNum int) and
// leadtime.NewBulk(since, until time.Time).
//
// --post logic stays OUTSIDE Pipeline. The command's RunE calls
// postIfEnabled() which wraps the writer with a tee, then calls
// Render, then the returned postFn() posts the buffer. Pipeline
// is data-in, text-out — no side effects beyond writing to w.
```

**Note on `Deps`**: The Pipeline takes a `Deps` pointer (or a subset interface) for GatherData. The exact type depends on whether we pass `cmd.Deps` directly or define a narrower interface. The simplest approach: pass `*cmd.Deps` and accept the coupling for now. If needed, extract an interface later.

### Shared Format Dispatch

```go
// internal/metric/dispatch.go
func WriteOutput(p Pipeline, rc format.RenderContext) error {
    return p.Render(rc)
}
```

This replaces the per-command `switch deps.Format` boilerplate. Each command's `RunE` becomes:

```go
func runBusFactor(cmd *cobra.Command, sinceStr string) error {
    deps := DepsFromContext(cmd.Context())
    rc := deps.RenderCtx(cmd.OutOrStdout())

    bf := busfactor.New(deps, sinceStr)
    if err := bf.GatherData(cmd.Context(), deps); err != nil {
        return err
    }
    if err := bf.ProcessData(); err != nil {
        return err
    }
    return bf.Render(rc)
}
```

### Testing Pattern

```go
func TestBusFactorProcessData(t *testing.T) {
    bf := &busfactor.Metric{
        // Inject fake data — no git or API calls
        Paths: []git.PathContributors{
            {Path: "cmd/", Contributors: []git.Contributor{{Name: "alice", Commits: 50}}},
        },
    }
    err := bf.ProcessData()
    require.NoError(t, err)
    assert.Equal(t, metrics.RiskHigh, bf.Result.Paths[0].Risk)
}
```

### Report Registration

Explicit list in `cmd/report.go`. One line per metric. No init() magic.

```go
// cmd/report.go
func buildReportSections(deps *Deps) []metric.Pipeline {
    return []metric.Pipeline{
        leadtime.NewBulk(deps),
        cycletime.NewBulk(deps),
        throughput.New(deps),
        // bus-factor, reviews, etc. as desired
    }
}

func runReport(cmd *cobra.Command, args []string) error {
    deps := DepsFromContext(cmd.Context())
    rc := deps.RenderCtx(cmd.OutOrStdout())
    sections := buildReportSections(deps)

    // GatherData concurrently — matches current errgroup pattern in ComputeDashboard.
    // Partial failures are non-fatal: log warning, skip the section in render.
    g, ctx := errgroup.WithContext(cmd.Context())
    g.SetLimit(5)
    for _, s := range sections {
        s := s
        g.Go(func() error { return s.GatherData(ctx, deps) })
    }
    if err := g.Wait(); err != nil {
        return err // total failure
    }

    for _, s := range sections {
        if err := s.ProcessData(); err != nil {
            log.Warn("skipping section: %v", err)
            continue
        }
    }
    // Render all sections (report has its own composite format)
    // ...
}
```

### Handling Single vs Bulk Mode

Commands like `lead-time` have two modes: single-issue (`lead-time 42`) and bulk (`lead-time --since 30d`). Two approaches:

**Option A: Two implementations of Pipeline.** `leadtime.NewSingle(issueNum)` and `leadtime.NewBulk(since, until)`. The command's `RunE` picks which to create based on args.

**Option B: One implementation with mode flag.** The metric struct has a `mode` field (single/bulk) that affects all three methods.

**Recommendation: Option A.** Two small, focused structs are clearer than one struct with mode branching. The command decides which to create, and both satisfy `Pipeline`. This also fixes the existing inconsistency where single-mode uses inline `fmt.Fprintf` — both modes would go through `Render()`.

### Concrete Refactoring Tasks

**Create the interface (`internal/metric/pipeline.go`):**

- [ ] Create `internal/metric/` package with `Pipeline` interface
- [ ] Define `Pipeline` with `GatherData`, `ProcessData`, `Render` methods
- [ ] Add `WriteOutput` dispatch helper

**Migrate bus-factor (simplest, most recent — good first candidate):**

- [ ] Create `busfactor.Metric` struct with raw data fields and result fields
- [ ] Move `runBusFactor` data-fetching logic into `GatherData`
- [ ] Move `metrics.ComputeBusFactor` call into `ProcessData`
- [ ] Move format switch into `Render` (delegating to existing `format.WriteBusFactor*` functions)
- [ ] Update `cmd/busfactor.go` to use the three-step pipeline
- [ ] Add `ProcessData` unit test with injected fake path data
- [ ] Verify smoke tests pass

**Migrate lead-time:**

- [ ] Create `leadtime.SingleMetric` and `leadtime.BulkMetric` structs
- [ ] Move single-issue inline formatting into proper `format.WriteLeadTimeSinglePretty/JSON/Markdown` functions (fixes existing inconsistency)
- [ ] Both structs implement `Pipeline`
- [ ] Add `ProcessData` unit tests for both modes

**Migrate cycle-time:**

- [ ] Create `cycletime.SingleMetric` and `cycletime.BulkMetric` structs
- [ ] Both implement `Pipeline`
- [ ] `ProcessData` uses the configured cycle-time strategy (fixes the duplication where `model.ComputeInsights` ignores strategy)

**Migrate remaining metrics:**

- [ ] Throughput — implements `Pipeline`
- [ ] Reviews — implements `Pipeline`
- [ ] Release — implements `Pipeline`

**Eliminate duplication:**

- [ ] Move `model.ComputeInsights()` out of `model/status.go` — it performs computation that belongs in `metrics/` (model can't import metrics due to dependency direction)
- [ ] Delete `model.medianDuration()` — use `metrics.ComputeStats().Median` instead
- [ ] Delete deprecated `metrics.NewMetric()` wrapper (`internal/metrics/metric.go:12-14`) — callers use `model.NewMetric()` directly
- [ ] Delete hollow `metrics.CycleTime()` wrapper (`internal/metrics/cycletime.go:9-11`) — it's `return NewMetric(start, end)` with no added logic
- [ ] Consolidate duplicate `buildClosingPRMap` — exists in both `cmd/helpers.go:32-59` and `metrics/dashboard.go:225-251`
- [ ] Fix double `ComputeInsights` call in `WriteMyWeekJSON` — compute once and pass result

**Wire into report:**

- [ ] Add `buildReportSections()` in `cmd/report.go` with explicit list
- [ ] Replace `metrics.ComputeDashboard()` with pipeline-based section gathering
- [ ] Each section's `ProcessData` uses `metrics.ComputeStats` — single aggregation path

**Composite commands (my-week):**

- [ ] my-week is a composite — it creates multiple Pipelines internally and renders a combined view
- [ ] my-week's insight computation delegates to the same `metrics.ComputeStats` path as standalone commands
- [ ] my-week insights use the configured cycle-time strategy (currently hardcoded to PR-based)

### Where Metric Structs Live

Keep the current package layout. Each metric struct lives alongside its computation:

| Metric | Struct location | Format functions |
|--------|----------------|-----------------|
| bus-factor | `internal/metrics/busfactor.go` (already there) | `internal/format/busfactor.go` |
| lead-time | `internal/metrics/leadtime.go` | `internal/format/bulk.go` |
| cycle-time | `internal/metrics/cycletime.go` | `internal/format/cycletime.go` |
| throughput | `internal/metrics/throughput.go` (new) | `internal/format/throughput.go` |
| reviews | `internal/metrics/reviews.go` (new) | `internal/format/reviews.go` |
| release | `internal/metrics/release.go` | `internal/format/pretty.go` + `scope.go` |

The Pipeline struct holds both raw gathered data and processed results. `GatherData` populates raw fields. `ProcessData` computes result fields. `Render` reads result fields.

### Escape Hatches

Some commands have modes that don't fit the Pipeline lifecycle:

- **`release --discover`**: Short-circuits after GatherData to render a diagnostic scope view. The command checks the flag before constructing a Pipeline — discovery bypasses Pipeline entirely.
- **Bus-factor's git.Runner**: Bus-factor uses local git (`exec.Command`), not the GitHub API client. Its Pipeline struct constructs its own `git.Runner` internally from the working directory. `Deps` is NOT expanded to carry a git runner — only bus-factor needs it.
- **Single-item mode posting**: Single-item commands post to issue/PR comments. The post target (issue vs PR vs discussion) is determined by the command, not the Pipeline. Post logic stays in the command's RunE, outside Pipeline.

### What This Is NOT

- NOT a directory restructuring — keep `cmd/`, `internal/metrics/`, `internal/format/`
- NOT a generics-based `MetricResult[T]` envelope — metrics have different shapes
- NOT a plugin system — metrics are compiled in
- NOT auto-registration via `init()` — explicit list is simpler and grep-able

### Acceptance Criteria

- [ ] `Pipeline` interface exists in `internal/metric/pipeline.go`
- [ ] All metric commands use the three-step pipeline (GatherData → ProcessData → Render)
- [ ] Forgetting to implement `Render()` on a new metric is a compile error
- [ ] `ProcessData` is testable with injected fake data (no API/git calls)
- [ ] Report command uses explicit `[]Pipeline` list
- [ ] `model.ComputeInsights()` moved out of `model/` — no computation in the types package
- [ ] No duplicate median/stats computation outside `metrics.ComputeStats()`
- [ ] `my-week` insights use configured cycle-time strategy (not hardcoded PR-based)
- [ ] Deprecated wrappers deleted (`metrics.NewMetric`, `metrics.CycleTime`)
- [ ] Duplicate `buildClosingPRMap` consolidated
- [ ] All existing tests pass
- [ ] Single-issue mode uses proper format functions (not inline `fmt.Fprintf`)

---

## Priority & Ordering

| Phase | Value | Effort | Dependencies |
|-------|-------|--------|-------------|
| 1. Config required | High — eliminates bug class | Low | None |
| 2. Empty messaging | Medium — better UX | Medium | Phase 1 (scope always explicit) |
| 3. Pipeline interface | High — developer velocity + quality | High | Phase 2 (stable formatter signatures) |

Phase 1 should be done first — it simplifies Phase 2 because scope is always explicit.
Phase 2 should be done before Phase 3 because it changes formatter signatures (adding `searchURL` parameter). Phase 3 then migrates to the new signatures.

### Recommended implementation order within Phase 3

1. Create `Pipeline` interface and `WriteOutput` dispatch helper
2. Migrate bus-factor first (simplest, most self-contained, recent)
3. Delete deprecated wrappers (`metrics.NewMetric`, `metrics.CycleTime`)
4. Migrate lead-time (single + bulk)
5. Migrate cycle-time (single + bulk)
6. Migrate throughput, reviews, release
7. Move `ComputeInsights` out of `model/`, consolidate `buildClosingPRMap`
8. Wire report command to use `[]Pipeline`
9. Update my-week to use Pipeline-based insight computation

---

## Sources & References

### Origin

- **Pipeline brainstorm:** [docs/brainstorms/2026-03-12-metric-pipeline-interface-brainstorm.md](docs/brainstorms/2026-03-12-metric-pipeline-interface-brainstorm.md) — Key decisions: (1) GatherData/ProcessData/Render interface, (2) compile-time guardrails over documentation, (3) explicit report registration, (4) no directory restructuring
- **Original brainstorm:** [docs/brainstorms/2026-03-11-metrics-architecture-and-ux-brainstorm.md](docs/brainstorms/2026-03-11-metrics-architecture-and-ux-brainstorm.md) — Key decisions: (1) config required over implicit fallback, (2) search URLs for empty results

### Superseded Plan

- [docs/plans/2026-03-12-001-feat-metrics-architecture-and-ux-plan.md](docs/plans/2026-03-12-001-feat-metrics-architecture-and-ux-plan.md) — Phases 1 and 2 carried forward with agent-recommended fixes. Phase 3 replaced by Pipeline interface.

### Agent Review Findings (from `/deepen-plan`)

- **Architecture strategist**: Move `ComputeInsights` out of `model/` into `metrics/`. Compute SearchURL in command layer, pass as parameter. Keep `config.Load()` lenient, check existence in `PersistentPreRunE`.
- **Pattern recognition**: Conditional JSON schema is an anti-pattern — always include `search_url`. Duplicated `buildClosingPRMap` in `cmd/helpers.go` and `metrics/dashboard.go`. Single-item rendering bypasses format functions.
- **Simplicity reviewer**: Drop `isConfigSubcommand()` helper (keep inline switch). Delete deprecated `metrics.NewMetric` and hollow `metrics.CycleTime` (~30 LOC). Pass `searchURL` as parameter not struct field.
- **Performance oracle**: No concerns. Fix double `ComputeInsights` call in `WriteMyWeekJSON`.
- **SpecFlow analyzer (round 1)**: `config.Load()` does NOT walk up to git root (plan assumption was wrong). Empty scope after removing fallback is critical gap. `ComputeInsights` cycle time ignores configured strategy.
- **SpecFlow analyzer (round 2)**: 21 gaps identified. Key adjustments: (1) Render signature simplified to `Render(rc)` — RenderContext already has Format+Writer, (2) report GatherData must be concurrent via errgroup (not serial loop), (3) --post stays outside Pipeline, (4) --discover bypasses Pipeline, (5) GatherData partial failures return nil with internal warnings, (6) bus-factor constructs its own git.Runner, (7) constructor captures command-specific params.
- **Learnings researcher**: 4 relevant solution docs confirm Pipeline aligns with existing patterns. Key: Deps struct is the established context-threading mechanism. Domain logic stays in `internal/metrics/` and `internal/github/`, not in Pipeline implementations.
- **Best practices researcher**: Always-present JSON fields preferred (kubectl, gh patterns). `url.QueryEscape` is correct for browser URLs. GitHub search query limit is 256 chars (not a concern for single-repo).

### Internal References

- Config loading: `cmd/root.go:138-266`
- Implicit scope fallback: `cmd/root.go:214-217`
- `Query.URL()`: `internal/scope/scope.go:31-38`
- Lead time computation: `internal/metrics/leadtime.go`
- Duplicate lead time: `internal/model/status.go:117-131`
- Duplicate `medianDuration()`: `internal/model/status.go:155-173`
- `ComputeStats`: `internal/metrics/stats.go:15-83`
- `ComputeDashboard`: `internal/metrics/dashboard.go:42-222`
- Deprecated `NewMetric`: `internal/metrics/metric.go:12-14`
- Hollow `CycleTime`: `internal/metrics/cycletime.go:9-11`
- Duplicate `buildClosingPRMap`: `cmd/helpers.go:32-59` and `metrics/dashboard.go:225-251`
- Bus-factor command (pipeline migration template): `cmd/busfactor.go`
- Three-state metric pattern: `docs/solutions/three-state-metric-status-pattern.md`
- Evidence-driven preflight: `docs/solutions/evidence-driven-preflight-config.md`
- Cobra command hierarchy: `docs/solutions/architecture-refactors/cobra-command-hierarchy-thematic-grouping.md`
- Tableprinter migration: `docs/solutions/go-gh-tableprinter-migration.md`

### Related Work

- PRs #42, #43, #44 — actionable output phases (established RenderContext, template patterns, formatter conventions)
