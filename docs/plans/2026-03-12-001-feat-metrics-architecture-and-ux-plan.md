---
title: "feat: Metrics Architecture and UX — Config Required, Empty Messaging, Metric Patterns"
type: feat
status: superseded
superseded_by: docs/plans/2026-03-12-002-feat-metrics-pipeline-and-ux-plan.md
date: 2026-03-12
origin: docs/brainstorms/2026-03-11-metrics-architecture-and-ux-brainstorm.md
---

# feat: Metrics Architecture and UX — Config Required, Empty Messaging, Metric Patterns

## Overview

Three reinforcing improvements from the brainstorm during my-week development:

1. **Config required** — eliminate implicit fallback scope, require `.gh-velocity.yml`
2. **Empty block messaging** — explain *why* results are empty with verify links
3. **Metric patterns** — standardize how to add a new metric so it's easy to follow (model → compute → format → command → report)

(see brainstorm: `docs/brainstorms/2026-03-11-metrics-architecture-and-ux-brainstorm.md`)

---

## Phase 1: Config Required — No Implicit Fallback

### Problem

The implicit `repo:owner/repo` fallback when no config exists causes confusion. The `-R` flag, config scope, and fallback scope interact in surprising ways. (see brainstorm: section 1)

### Decision

**All commands except `config *` subcommands require `.gh-velocity.yml`.** No implicit fallback.

- `preflight` is the entry point — it generates config
- Commands without config error: `"No config found. Run 'gh velocity config preflight -R owner/repo' to get started."`
- Use exit code `ErrConfigInvalid` (exit 2) for missing config

### Ad-hoc `-R` usage

**The `-R` flag still works, but config must exist.** The `-R` flag determines repo context for API calls. Config provides scope, categories, strategies. These are separate concerns.

For users who want quick one-off queries: `gh velocity config preflight -R cli/cli --write` creates a config in one step. This is the intended workflow — not implicit fallback.

### Implementation

**`cmd/root.go`** — in `PersistentPreRunE`:

```go
// After config loading attempt:
if cfg == nil && !isConfigSubcommand(cmd) {
    return &model.AppError{
        Code:    model.ErrConfigInvalid,
        Message: "no config found. Run 'gh velocity config preflight -R owner/repo' to get started.",
    }
}
```

- [ ] Add `isConfigSubcommand()` helper (checks if cmd or parent is `config`)
- [ ] Remove `config.Defaults()` fallback in root.go (line ~201)
- [ ] Remove implicit `repo:owner/repo` scope injection (root.go lines ~214-217)
- [ ] Ensure `config create`, `config show`, `config validate`, `config preflight`, `config discover` all skip the check
- [ ] Ensure `version` skips the check
- [ ] Update error message to reference `preflight --write` for quick setup
- [ ] Update smoke tests — tests that run without config need a config file or `-R` with config

### Edge cases

- **CI with `GH_REPO` env var**: still needs config file. Document in guide.
- **Subdirectories**: config search walks up to git root (already implemented in `config.Load()`)

### Acceptance Criteria

- [ ] Commands without config fail with clear error mentioning `preflight`
- [ ] `config *` subcommands and `version` work without config
- [ ] `-R` flag still works when config exists
- [ ] Smoke tests updated for new requirement
- [ ] No implicit `repo:owner/repo` scope injection anywhere

---

## Phase 2: Empty Block Messaging with Evidence Links

### Problem

Empty sections show `_None_` or skip content. Users can't tell if it's a scope problem, wrong time window, or genuinely no data. (see brainstorm: section 2)

### Decision

Empty results include: (1) human-readable explanation, (2) clickable GitHub search URL to verify.

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

**JSON output:**
```json
{
  "issues_closed": [],
  "issues_closed_count": 0,
  "issues_closed_search_url": "https://github.com/search?q=..."
}
```

### Implementation

**`internal/scope/scope.go`** — fix `Query.URL()` to use `/search?q=` (currently uses `/issues?q=`):

```go
func (q Query) URL() string {
    query := q.Build()
    if query == "" {
        return ""
    }
    return "https://github.com/search?q=" + url.QueryEscape(query)
}
```

**Formatter changes** — pass search URL to formatters for empty-state rendering:

- [ ] Fix `Query.URL()` to use `github.com/search?q=` not `github.com/issues?q=`
- [ ] Add `SearchURL string` field to format data structs that can be empty (bulk results, my-week sections)
- [ ] Update `WriteLeadTimeBulkPretty/Markdown/JSON` — show verify link when items empty
- [ ] Update `WriteCycleTimeBulkPretty/Markdown/JSON` — same
- [ ] Update `WriteMyWeekPretty/Markdown/JSON` — per-section verify links
- [ ] Update `WriteReviewsPretty/Markdown/JSON` — verify link when queue empty
- [ ] Update `WriteThroughputPretty/Markdown/JSON` — verify link when counts are zero
- [ ] Update markdown templates to include verify links in empty state
- [ ] JSON: include `*_search_url` field only when results array is empty (keeps schema clean)

### Scope

Commands that search GitHub via REST search get verify links. Commands that use GraphQL (wip/project-board) or local git (bus-factor) do NOT — there's no equivalent search URL.

### Edge cases

- **Long URLs**: GitHub search URLs can be long with complex scope. Accept this — they're functional links, not display text.
- **Private repos**: URL requires GitHub auth in browser. Document this caveat, don't try to solve it.

### Acceptance Criteria

- [ ] Empty bulk lead-time/cycle-time shows verify search URL
- [ ] Empty my-week sections show per-section verify URLs
- [ ] Empty reviews shows verify URL
- [ ] Empty throughput shows verify URL
- [ ] JSON includes `*_search_url` only when results are empty
- [ ] `Query.URL()` produces correct `github.com/search?q=` URLs
- [ ] Verify links are functional (manual test against real repo)

---

## Phase 3: Standardized Metric Pattern

### Problem

Adding a new metric requires knowing where to put things across 5+ files. Lead time logic is duplicated between `metrics.LeadTime()` and `model.ComputeInsights()`. There's no documented pattern for "how to add a metric." (see brainstorm: section 3)

### Decision

**Don't build an abstract interface.** Instead, standardize the existing pattern and eliminate duplication so adding a new metric is a matter of following the recipe.

The brainstorm's own open question — "Is the interface worth the abstraction, or is 'each command calls metrics.ComputeStats(durations)' simple enough?" — answers itself: the simple version is enough. The pattern is the abstraction.

### The Metric Pattern (recipe for adding a new metric)

Each metric follows this path:

```
1. model/types.go        — Define result types (pure data)
2. metrics/<name>.go     — Compute function: items → []duration → Stats
3. format/<name>.go      — Pretty/JSON/Markdown formatters
4. format/templates/     — Markdown template
5. cmd/<name>.go         — Command wiring (fetch → compute → format)
6. format/report.go      — Wire into summary report
```

### Concrete tasks

**Eliminate duplication:**

- [ ] Refactor `model.ComputeInsights()` (status.go:117-149) to call `metrics.LeadTime()` and `metrics.CycleTime()` instead of computing inline
- [ ] Remove `model.medianDuration()` — use `metrics.ComputeStats()` instead
- [ ] Ensure `my-week` insights use the same computation path as `flow lead-time` and `flow cycle-time`

**Standardize `metrics.ComputeStats` as the single aggregation point:**

- [ ] All commands that compute stats should call `metrics.ComputeStats(durations)` — no ad-hoc median/mean computation elsewhere
- [ ] Verify `cmd/leadtime.go`, `cmd/cycletime.go`, `cmd/myweek.go`, and `internal/metrics/dashboard.go` all use this path

**Wire metrics into summary report:**

The `report` command (`cmd/report.go`) should demonstrate the full pattern. Currently it computes dashboard metrics via `metrics.ComputeDashboard()`.

- [ ] Ensure the report command's metric computation follows the same pattern as standalone commands
- [ ] Document the wiring: how `ComputeDashboard` delegates to `LeadTime`, `CycleTime`, `ComputeStats`

**Document the pattern:**

- [ ] Add `docs/solutions/adding-a-new-metric.md` — step-by-step recipe with file list, code snippets, and checklist
- [ ] Include example: "If you wanted to add Review Turnaround Time, here's what you'd create"
- [ ] Reference existing metrics (lead-time, cycle-time, bus-factor) as examples at each step

### What this is NOT

- NOT a `ScopedMetric` interface with `ForItem`/`ForPerson`/`ForScope` methods — that's premature abstraction
- NOT a plugin system — metrics are compiled in
- NOT touching the format package's structure — the embedded template pattern already works well

### Acceptance Criteria

- [ ] `model.ComputeInsights()` delegates to `metrics.LeadTime()` / `metrics.CycleTime()`
- [ ] No duplicate median/stats computation outside `metrics.ComputeStats()`
- [ ] `my-week` insights produce identical results to standalone `flow lead-time` for the same data
- [ ] `docs/solutions/adding-a-new-metric.md` exists with step-by-step recipe
- [ ] All existing tests pass (especially my-week and report)
- [ ] A developer can add a new metric by following the documented pattern without reading unrelated code

---

## Priority & Ordering

| Phase | Value | Effort | Dependencies |
|-------|-------|--------|-------------|
| 1. Config required | High — eliminates bug class | Low | None |
| 2. Empty messaging | Medium — better UX | Medium | Phase 1 (scope always explicit) |
| 3. Metric patterns | Medium — developer velocity | Medium | Phase 2 (stable formatter signatures) |

Phase 1 should be done first — it simplifies Phase 2 because scope is always explicit.

---

## Sources & References

### Origin

- **Brainstorm document:** [docs/brainstorms/2026-03-11-metrics-architecture-and-ux-brainstorm.md](docs/brainstorms/2026-03-11-metrics-architecture-and-ux-brainstorm.md) — Key decisions: (1) config required over implicit fallback, (2) search URLs for empty results, (3) pattern over interface for metrics

### Internal References

- Config loading: `cmd/root.go:178-217`
- Implicit scope fallback: `cmd/root.go:214-217`
- `Query.URL()`: `internal/scope/scope.go:31-38`
- Lead time computation: `internal/metrics/leadtime.go`
- Duplicate lead time: `internal/model/status.go:117-131`
- `ComputeStats`: `internal/metrics/stats.go:15-83`
- `ComputeDashboard`: `internal/metrics/dashboard.go`
- Three-state metric pattern: `docs/solutions/three-state-metric-status-pattern.md`
- Evidence-driven preflight: `docs/solutions/evidence-driven-preflight-config.md`

### Related Work

- PRs #42, #43, #44 — actionable output phases (established RenderContext, template patterns, formatter conventions)
