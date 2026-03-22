---
date: 2026-03-21
topic: codebase-simplification
---

# Codebase Simplification: Simple Over Smart

## Problem Frame

The gh-velocity codebase (~23K LOC, ~16K test LOC) has accumulated structural complexity that makes it hard for contributors (including the author) to follow. The only genuinely complex problem is minimizing redundant API queries — everything else is fast client-side processing that should read simply. Today, adding a new metric or updating a report requires copying ~600 LOC of boilerplate across pipeline, render, and command layers, with high risk of subtle divergence.

The target contributor experience: someone can read one metric end-to-end, understand the pattern, and add or modify a metric without needing to understand framework-level abstractions.

## Requirements

### Pipeline Layer

- R1. Redesign the `Pipeline` interface to enforce correctness, not just naming convention. The current interface has the right idea (compile-time guarantee that all phases exist) but two problems: (a) commands bypass `RunPipeline()` and call phases manually, creating bugs when a step is forgotten (confirmed: WIP bug where a pipeline step was skipped), and (b) cross-cutting concerns like warnings, insights, and posting are handled ad-hoc outside the interface. The new design should make it impossible to forget a step — either through a stronger interface that `RunPipeline()` always drives, or through composable building blocks that enforce the sequence. Limited use of generics is acceptable where it reduces duplication without obscuring intent.

- R2. Eliminate per-metric pipeline structs where they are thin wrappers. A "single issue leadtime" pipeline that does `client.GetIssue()` → `metrics.LeadTime()` → `WriteSingleJSON()` should just be three function calls in the command, not a struct with constructor fields, GatherData, ProcessData, and Render methods. The correctness enforcement from R1 applies to bulk/complex pipelines; single-item lookups are simple enough to be inline.

- R3. Keep pipeline structs for metrics that carry meaningful state across phases (e.g., velocity iteration preprocessing, WIP staleness classification, bulk stats + insights generation). These should implement the enforced interface from R1 so that `RunPipeline()` drives them — no manual phase calls in the command layer.

### Render Layer

- R4. Extract shared bulk-output rendering. The `WriteBulkJSON`, `WriteBulkMarkdown`, and `WriteBulkPretty` functions for cycletime and leadtime are structurally identical (sort → build rows → cap → render). Factor into shared helpers parameterized by metric name and field accessor.

- R5. Unify duplicated JSON structs. `jsonSingleOutput`, `jsonBulkOutput`, and `jsonBulkItem` are copied across cycletime and leadtime with only field-name differences (`cycle_time` vs `lead_time`). Use a shared struct with a generic metric field name, or use `map[string]any` for the metric-specific key.

- R6. Consolidate `classifyFlags` and `flagEmojis`. These are duplicated identically in cycletime and leadtime render files. Move to the `format` package as shared utilities.

- R7. Standardize format dispatch. Every pipeline's Render method repeats `switch rc.Format { case JSON/Markdown/Pretty }`. This switch should exist in one place per metric, not be copied. If pipeline structs are removed (R1-R2), the dispatch moves to the command layer or a thin format helper.

### Command Layer

- R8. Extract common command boilerplate. Every `run*` function repeats: extract deps → create client → parse dates → build scope query → instantiate pipeline → gather → process → warn → render → post. Factor the repeated preamble and postamble into helpers, leaving each command responsible only for its unique logic (which fields, which computation).

- R9. Keep the report command's orchestration explicit. The report runs multiple metrics — its wiring should remain readable inline, not hidden behind generic abstractions. Simplify by reducing per-metric wiring overhead (benefits from R1-R8).

### Test Suite

- R10. Preserve all existing test coverage as a regression safety net. The refactoring must not reduce test count or coverage. Run tests after each refactoring step.

- R11. Tests for shared helpers. When extracting shared render/command helpers (R4-R8), add unit tests for the extracted functions. The existing per-metric tests become integration-level confirmation that the wiring is correct.

### Scope Boundaries (what NOT to do)

- Do NOT refactor `strategy/`, `classify/`, or `effort/` packages. These use interfaces with real polymorphism and are well-designed.
- Do NOT refactor the `github/` client or caching layer. It's appropriate and clear.
- Do NOT refactor `config/`, `scope/`, `dateutil/`, `log/`, or `model/`. These are well-scoped.
- Do NOT change JSON output shapes — existing consumers must not break.
- Do NOT change CLI flag names or behavior.
- Do NOT add new abstractions purely for architectural elegance. New abstractions are justified only when they prevent real bugs (like the pipeline step-skipping bug) or collapse real duplication.

## Success Criteria

- A contributor can read one metric command (e.g., leadtime) top-to-bottom and understand the full data flow without jumping to a Pipeline interface definition or tracing render dispatch
- Adding a new duration-based metric (like "time to first review") requires < 200 LOC of new code, not 600+
- All existing tests pass
- No change to JSON output shapes or CLI behavior
- Net LOC reduction of at least 1,500 lines (conservative estimate; likely 2,500-3,500)

## Key Decisions

- **Interfaces for correctness, not taxonomy**: Keep an interface where it prevents bugs (forgetting a pipeline step). Remove interfaces where they only provide naming convention (single-item lookups). The WIP bug proves the value of enforced sequencing.
- **Generics where they collapse duplication**: Limited use of Go generics to parameterize bulk rendering by metric type (e.g., `RenderBulk[T BulkItem]`) is encouraged when it eliminates 500+ LOC of copy-paste. Avoid generics for cleverness.
- **Shared helpers over generic frameworks**: Extract common rendering into parameterized helpers, not into a generic "metric renderer" abstraction. The helpers should be obvious to read without understanding a framework.
- **One big sweep**: Execute as a single coherent effort to avoid half-refactored transitional states.
- **Existing tests as safety net**: Run `task test` after every refactoring move. Red tests mean stop and fix.

## Outstanding Questions

### Deferred to Planning

- [Affects R1][Needs research] What should the enforced pipeline interface look like? Options: (a) current interface but `RunPipeline()` handles warnings/posting so commands never call phases directly, (b) a `Run(ctx) (Result, error)` single-method interface where `Result` carries output + warnings + insights, (c) a generic `Pipeline[T]` where T is the output type. The right answer depends on how much the output shapes vary across metrics.
- [Affects R4-R5][Needs research] How different are the velocity, WIP, throughput, and quality render files from cycletime/leadtime? The shared helpers should accommodate all metric types, not just duration-based ones.
- [Affects R2-R3][Technical] Which pipeline structs carry enough inter-phase state to justify keeping as structs vs. inlining? Velocity and WIP are likely keepers; need to verify release, throughput, quality.
- [Affects R5][Technical] Best approach for unified JSON output without changing wire format: shared struct with `json` tag override, or `map[string]any` for the metric-specific field? Former is type-safe, latter is flexible.
- [Affects R8][Technical] What's the right factoring for command preamble? A `runMetric` helper function, or just extracted sub-helpers (e.g., `parseDateWindow`, `buildScopeQuery`) that each command composes?

## Next Steps

→ `/ce:plan` for structured implementation planning
