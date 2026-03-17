---
title: "feat: insights for all report sections"
type: feat
status: completed
date: 2026-03-17
origin: docs/brainstorms/2026-03-17-insights-for-all-report-sections-brainstorm.md
---

# feat: Insights for All Report Sections

## Enhancement Summary

**Deepened on:** 2026-03-17
**Sections enhanced:** 8
**Research agents used:** architecture-strategist, code-simplicity-reviewer, pattern-recognition-specialist, agent-native-reviewer, performance-oracle, best-practices-researcher, learnings-researcher

### Key Improvements
1. **Add `Type` field to `Insight`** — enables agent filtering/routing without parsing natural language; `Type` is identity (what rule fired), not presentation styling
2. **Defer per-category insights for lead-time/cycle-time** — avoid injecting `classify` into pipelines that don't need it; quality already classifies so category insights live there only
3. **Consolidate shared stats rules** — one `GenerateStatsInsights()` function for outlier/skew/fastest-slowest, reused by lead time and cycle time (DRY)
4. **Collapse to 3 phases** — foundation+pipeline, report rendering, standalone+smoke tests
5. **Fix velocity insights missing from report JSON** — `jsonVelocitySummary` currently drops insights; add them
6. **Classification stays in cmd layer** — pipelines remain classification-unaware; cmd builds classified items before calling insight generators
7. **File naming: `report_insights.go`** — avoids collision with existing MyWeek `insights.go`

### YAGNI Decisions
- `Insight.Level` deferred — presentation concern, not needed yet
- Structured `data` map on Insight deferred — `Type` field is sufficient for agent filtering in v1
- Top-level `insights` array in report JSON deferred — per-section arrays are sufficient; easy to add later
- `--insights=false` suppression flag not needed
- `HotfixMaxHours` config override not built — use constant directly

---

## Overview

Add deterministic heuristic insights to every report section — lead time, cycle time, throughput, and quality (velocity already has them). Insights are the headline of every section; per-item tables are supporting evidence. The report leads with a **Key Findings** section grouping all insights by metric, before the metrics summary table.

(see brainstorm: `docs/brainstorms/2026-03-17-insights-for-all-report-sections-brainstorm.md`)

## Problem Statement / Motivation

The current report output tells you numbers but not what they mean. "median 43d 20h, mean 203d 3h, P90 446d 20h (n=21, 4 outliers)" requires mental effort to interpret. An insight like "4 outliers >447d dominate the mean — heavy right skew from ancient browser bugs" is immediately actionable.

Only the velocity section generates `[]model.Insight` today. Lead time, cycle time, throughput, and quality have no commentary.

## Proposed Solution

### Architecture: Per-Pipeline Generation

Each pipeline generates its own `[]model.Insight` during `ProcessData()`, following the velocity pattern. No central engine. Each pipeline has the richest context about its own data.

The report command collects insights from all pipelines and groups them by section for the Key Findings block. Standalone commands render their pipeline's insights directly.

(see brainstorm: Key Decision #2 — per-pipeline architecture)

### Layout: Insights First

Report-level: Key Findings section → metrics table → detail sections.
Detail sections: Insights → per-item table → summary line.
Standalone commands: Insights → per-item table → summary line.

(see brainstorm: Key Decision #1 — insights first, then metrics table)

### Constraints

**No AI at runtime.** All insight rules are deterministic heuristics — simple conditionals on computed stats. No LLM calls, no ML models. Users who want deeper analysis can export JSON/markdown and feed it to an agent.

(see brainstorm: Design Constraints)

## Technical Approach

### Resolved Design Questions

These were raised by SpecFlow analysis and resolved here. Deepening updated Q2, Q3, and added Q13-Q15.

| # | Question | Resolution |
|---|----------|-----------|
| Q1 | Struct design for insights in StatsResult | Add `LeadTimeInsights`, `CycleTimeInsights`, `ThroughputInsights`, `QualityInsights []Insight` fields to `StatsResult`. Insights are sibling fields, not nested inside Stats/StatsThroughput/StatsQuality. |
| Q2 | Category injection into pipelines | **Revised: classification stays in cmd layer.** Do NOT add `Categories` to pipeline structs. The `cmd` layer classifies items after `ProcessData()` and passes classified data to insight generators. Pipelines remain classification-unaware. Per-category insights for lead-time/cycle-time **deferred** — only quality gets category insights (it already classifies). |
| Q3 | JSON schema | Each section object gains `"insights": [{"type":"...","message":"..."}]` field. Added to `JSONStats`, `jsonThroughput`, `jsonStatsQuality`, **and `jsonVelocitySummary`** (currently missing). |
| Q4 | `--compare-prior` architecture | **Deferred to follow-up.** Ship insights without trends first. Trend rules skipped in this plan. |
| Q5 | Single-item commands | **No insights.** Single-item commands (`lead-time 42`, `cycle-time 42`) have no aggregate data. Insights require bulk/aggregate context. |
| Q6 | Insight position in output | Always before the per-item table, all formats. Pretty: `→` bullets. Markdown: `**Insights:**` section. JSON: `insights` array. |
| Q7 | Sections with zero insights in Key Findings | Omit the section heading. Only sections with ≥1 insight appear in Key Findings. |
| Q8 | Per-category stats exposure | Internal-only for insight generation. Not exposed in JSON or tables. Can add later if needed. |
| Q9 | Numeric thresholds | Defined as constants in `internal/metrics/report_insights.go` (see Thresholds section below). |
| Q10 | Issue references in insights | Use plain `#N` references. No format-specific link generation. Markdown renderers auto-link `#N` in GitHub. |
| Q11 | Fastest/slowest count | Single fastest and single slowest item. |
| Q12 | `--insights=false` flag | Not needed. JSON consumers ignore the field. No suppression flag. |
| Q13 | Conversion function placement | Conversion functions (`leadTimeToInsightItems` etc.) live in `cmd/report.go` for the report path and inline in pipeline `generateInsights()` wrappers for standalone paths. **Never in `internal/metrics/`** — would create circular dependency. |
| Q14 | Velocity insight asymmetry | Velocity insights remain on `VelocityResult.Insights`. Report rendering explicitly extracts them alongside the four sibling fields when building `InsightGroups`. Documented asymmetry, not a refactor target. |
| Q15 | Insights vs warnings boundary | Warnings are about data quality issues (low label coverage, API caps hit). Insights are about metric interpretation. `Type` field on Insight makes this self-documenting for agents. |

### Insight Thresholds (constants)

All in `internal/metrics/report_insights.go`:

```go
const (
    SkewThreshold      = 3.0   // mean/median ratio to trigger skew warning
    DefectRateHigh     = 0.20  // 20% defect rate threshold
    OutlierMinCount    = 2     // min outliers to surface insight
    MismatchRatio      = 3.0   // PR:issue ratio to flag mismatch
    HotfixMaxHours     = 72    // hotfix window in hours
    MinItemsForInsight = 3     // need ≥3 items for statistical insights
)
```

### Model Changes

**`model.Insight`** — add `Type` field for agent-native filtering:

```go
type Insight struct {
    Type    string // rule identifier: "outlier_detection", "skew_warning", etc.
    Message string // human-readable, may contain inline markdown
}
```

The `Type` field exposes the rule name that already exists as a function/comment identifier. It costs nothing to add and enables agents to filter, route, and aggregate insights without parsing natural language. This is identity (what fired), not presentation (how to style). The existing velocity insights will be updated to include `Type`.

#### Research Insight: Agent-Native Reviewer

> The `my-week` command already uses a hybrid pattern: both `lines` (human-readable strings) AND structured fields. For report insights, `Type` alone is sufficient — the structured data is already in the per-section JSON fields (`median_seconds`, `defect_rate`, etc.). Agents can correlate the insight `Type` with the section's numeric fields.

**`model.StatsResult`** — add per-section insight fields:

```go
type StatsResult struct {
    // ... existing fields ...
    LeadTimeInsights    []Insight
    CycleTimeInsights   []Insight
    ThroughputInsights  []Insight
    QualityInsights     []Insight
}
```

Velocity insights remain on `VelocityResult.Insights` (existing field). Report rendering handles the asymmetry explicitly.

### Insight Rules

#### Shared Stats Insights (`internal/metrics/report_insights.go`)

Lead time and cycle time share the same statistical rules. One function serves both:

```go
func GenerateStatsInsights(stats model.Stats, sectionName string, fastest, slowest *ItemRef) []model.Insight
```

Where `ItemRef` is a minimal struct for fastest/slowest callout: `{Number int, Title string, Duration time.Duration}`.

Rules (each fires independently):
1. **Outlier detection** (`outlier_detection`): `stats.OutlierCount >= OutlierMinCount` → "N outliers above Xd threshold — consider investigating long-lived items"
2. **Skew warning** (`skew_warning`): `mean/median > SkewThreshold` → "Mean Xd vs median Yd — heavy right skew from N outliers"
3. **Fastest/slowest callout** (`fastest_slowest`): When `stats.Count >= MinItemsForInsight` and refs provided, name fastest and slowest by title and duration

#### Research Insight: Best Practices

> Message format follows the "[Quantified observation] — [consequence or action]" pattern used by Terraform diagnostics and the existing velocity insights. Always include specific numbers. One sentence, two at most. Must read correctly in JSON arrays, markdown bullets, and CLI output.

#### Cycle Time Extras (`internal/metrics/report_insights.go`)

```go
func GenerateCycleTimeInsights(stats *model.Stats, strategy string, fastest, slowest *ItemRef) []model.Insight
```

Calls `GenerateStatsInsights` when stats is non-nil, then adds:
1. **No data — strategy-specific** (`no_data`): When stats is nil, generate strategy-specific guidance
2. **Strategy callout** (`strategy_callout`): When stats is non-nil and strategy is PR, "PR cycle time median Xd — [fast/moderate/slow] review turnaround"

#### Throughput (`internal/metrics/report_insights.go`)

```go
func GenerateThroughputInsights(issuesClosed, prsMerged int) []model.Insight
```

Rules:
1. **Issue/PR mismatch** (`issue_pr_mismatch`): PRs > 0 AND issues == 0, OR prsMerged/issuesClosed > MismatchRatio → "N PRs merged but M issues closed — PRs may not be linked to issues"
2. **Zero activity** (`zero_activity`): Both counts == 0 → "No issues closed or PRs merged in this window"

Per-category distribution for throughput **deferred** — avoids threading classification into the throughput pipeline. Quality section covers category breakdown.

#### Quality (`internal/metrics/report_insights.go`)

```go
func GenerateQualityInsights(quality model.StatsQuality, bugItems, nonBugItems []ItemRef, hotfixWindowHours int) []model.Insight
```

Rules:
1. **Defect rate threshold** (`defect_rate_high`): `quality.DefectRate > DefectRateHigh` → "X% defect rate — above typical 20% threshold"
2. **Bug fix speed comparison** (`bug_fix_speed`): When both bug and non-bug items exist, compare median durations. "Bug fixes (median Xd) [faster/slower] than other work (median Yd)"
3. **Category distribution** (`category_distribution`): When items have categories, show what's being shipped. "N items: X% bug, Y% feature, Z% other"
4. **Hotfix detection** (`hotfix_count`): Count items where duration < hotfixWindowHours. "N hotfixes (resolved within Xh of creation)"

Quality already classifies items via `computeQuality` in `cmd/report.go`, so per-category data is available with no new plumbing.

### ItemRef (replaces InsightItem for most uses)

#### Research Insight: Simplicity Reviewer

> The original `InsightItem` adapter was over-engineered for the statistical rules. Outlier/skew rules need only `model.Stats`. Fastest/slowest needs just number + title + duration. Per-category insights are deferred for lead-time/cycle-time. Only quality needs classified items.

Minimal struct for fastest/slowest callout:

```go
// In internal/metrics/report_insights.go
type ItemRef struct {
    Number   int
    Title    string
    Duration time.Duration
}
```

For quality insights that need classified items, the `cmd` layer builds `bugItems` and `nonBugItems` slices of `ItemRef` after classification (classification already happens in `computeQuality`).

Conversion functions live in `cmd/report.go` (for report) and in each pipeline's cmd file (for standalone). **Never in `internal/metrics/`** to avoid circular imports.

### Rendering Changes

#### Report Markdown Template (`internal/format/templates/report.md.tmpl`)

Add Key Findings section before the metrics table:

```
{{- if .HasInsights}}

**Key Findings:**
{{- range .InsightGroups}}

*{{.Section}}:*
{{- range .Messages}}
- {{.}}
{{- end}}
{{- end}}
{{- end}}

| Metric | Value |
| --- | --- |
...
```

Template data struct gains:

```go
type insightGroup struct {
    Section  string
    Messages []string
}

// Added to reportTemplateData:
HasInsights   bool
InsightGroups []insightGroup
```

The `renderReportMarkdown()` function builds `InsightGroups` from the five insight sources: `result.LeadTimeInsights`, `result.CycleTimeInsights`, `result.ThroughputInsights`, `result.QualityInsights`, and `result.Velocity.Insights`. Sections with zero insights are omitted.

#### Report Pretty (`internal/format/report.go`)

After the title, before the metric lines:

```go
groups := buildInsightGroups(r) // shared helper, builds from all 5 sources
if len(groups) > 0 {
    fmt.Fprintln(w, "Key Findings:")
    for _, group := range groups {
        fmt.Fprintf(w, "\n  %s:\n", group.Section)
        for _, msg := range group.Messages {
            fmt.Fprintf(w, "  → %s\n", msg)
        }
    }
    fmt.Fprintln(w)
}
```

#### Report JSON (`internal/format/report.go`)

Add `insights` arrays to each section struct. Insights are serialized as objects with `type` and `message`:

```go
type jsonInsight struct {
    Type    string `json:"type"`
    Message string `json:"message"`
}

type JSONStats struct {
    // ... existing fields ...
    Insights []jsonInsight `json:"insights,omitempty"`
}

type jsonThroughput struct {
    // ... existing fields ...
    Insights []jsonInsight `json:"insights,omitempty"`
}

type jsonStatsQuality struct {
    // ... existing fields ...
    Insights []jsonInsight `json:"insights,omitempty"`
}

type jsonVelocitySummary struct {
    // ... existing fields ...
    Insights []jsonInsight `json:"insights,omitempty"` // FIX: currently missing
}
```

#### Research Insight: Agent-Native Reviewer

> Velocity insights are currently dropped from report JSON (`jsonVelocitySummary` has no `Insights` field). This creates a parity gap: velocity insights appear in markdown Key Findings but are absent from report JSON. This plan fixes that by adding the field.

#### Standalone Command Rendering

Each pipeline's `Render()` method (in its `render.go`) adds insight rendering before the per-item table. Each pipeline gets a private `generateInsights()` method that mirrors velocity's pattern:

```go
// In each pipeline's ProcessData():
func (p *BulkPipeline) ProcessData() error {
    // ... existing computation ...
    p.generateInsights() // last step, matching velocity pattern
    return nil
}

func (p *BulkPipeline) generateInsights() {
    fastest, slowest := p.findExtremes()
    p.Insights = metrics.GenerateStatsInsights(p.Stats, "Lead Time", fastest, slowest)
}
```

Three rendering formats:
- **Pretty**: `model.WriteInsightsPretty(w, p.Insights)` — already exists as a helper
- **Markdown**: `**Insights:**` bullet list before the table
- **JSON**: `Insights []jsonInsight` field in the pipeline's JSON output struct

#### Detail Sections in Report

The report command's own output gets the Key Findings section only. Detail sections with per-item tables only appear in standalone commands (rendered separately by the showcase script). This keeps the report template simple and avoids duplicating insight rendering.

## System-Wide Impact

### Interaction Graph

`Pipeline.ProcessData()` → `p.generateInsights()` → calls `metrics.GenerateStatsInsights()` → stores `[]Insight` on pipeline struct → report command reads `.Insights` from each pipeline → populates `StatsResult.*Insights` fields → `format.WriteReport*` reads insight fields → `buildInsightGroups()` assembles from all 5 sources (including `result.Velocity.Insights`) → renders per format.

No callbacks, middleware, or observers involved. Pure data flow.

### Error Propagation

Insight generation cannot fail — it operates on already-computed stats with simple conditionals. If stats are nil (pipeline failed), insights are empty. No new error paths.

### State Lifecycle Risks

None. Insights are ephemeral — computed during ProcessData, rendered during Render, never persisted.

### API Surface Parity

| Interface | Gets Insights | Notes |
|-----------|:---:|-------|
| `report` command | Yes | Key Findings section + per-section in JSON |
| `flow lead-time` (bulk) | Yes | Before per-item table |
| `flow cycle-time` (bulk) | Yes | Before per-item table |
| `flow throughput` | Yes | Before summary table |
| `quality release` | Deferred | Quality insights can be added here later |
| `flow lead-time N` (single) | No | No aggregate data for insights |
| `flow cycle-time N` (single) | No | No aggregate data for insights |
| `flow velocity` | Already done | Update to include `Type` field |
| `status my-week` | Separate pattern | Uses `MyWeekInsights`, not `Insight` |
| `risk bus-factor` | Not applicable | Different metric type |
| `status reviews` | Not applicable | Different metric type |

### Integration Test Scenarios

1. `report --since 30d --format json` → JSON has `lead_time.insights`, `throughput.insights`, `quality.insights`, `velocity.insights` arrays with `type` and `message` fields
2. `report --since 30d` (markdown) → Key Findings section appears before metrics table, grouped by section
3. `flow lead-time --since 30d` → insights bullet list before per-item table
4. `report --since 30d` with no categories configured → quality category insights omitted, other rules still fire
5. `report --since 30d` on repo with zero activity → throughput shows "no activity" insight, lead time section omitted (no data)

## Implementation Phases

### Phase 1: Foundation + Pipeline Integration

**Goal**: Insight generator functions, thresholds, model changes, pipeline wiring, report population. Everything except rendering.

**Files**:
- `internal/metrics/report_insights.go` (new) — threshold constants, `ItemRef`, `GenerateStatsInsights()`, `GenerateCycleTimeInsights()`, `GenerateThroughputInsights()`, `GenerateQualityInsights()` functions
- `internal/metrics/report_insights_test.go` (new) — table-driven tests for every insight rule
- `internal/model/types.go` — add `Type string` to `Insight`, add `LeadTimeInsights`, `CycleTimeInsights`, `ThroughputInsights`, `QualityInsights []Insight` to `StatsResult`
- `internal/pipeline/leadtime/leadtime.go` — add `Insights []model.Insight` field, add private `generateInsights()` method called at end of `ProcessData()`
- `internal/pipeline/cycletime/cycletime.go` — same pattern, plus strategy-specific insights
- `internal/pipeline/throughput/throughput.go` — add `Insights []model.Insight`, add private `generateInsights()` method
- `internal/pipeline/velocity/velocity.go` — update existing `generateInsights()` to set `Type` field on each insight
- `cmd/report.go` — after `ProcessData()` for each pipeline, populate `result.*Insights`. Expand `computeQuality` to also return quality insights using classified items.

#### Research Insight: Testing Best Practices

> Table-driven tests with `t.Run` subtest names matching the rule being tested. Use `wantCount` + `wantSubstr` assertion style (not exact message matching) for resilience to wording changes. Explicit boundary tests at each threshold. Zero/nil/empty cases always produce zero insights, never panic.

**Acceptance**:
- [ ] `GenerateStatsInsights()` shared function with table-driven tests
- [ ] `GenerateCycleTimeInsights()` wraps shared function + strategy-specific rules
- [ ] `GenerateThroughputInsights()` with mismatch and zero-activity rules
- [ ] `GenerateQualityInsights()` with defect rate, bug speed, distribution, hotfix rules
- [ ] Each threshold is a named constant in `report_insights.go`
- [ ] `Insight.Type` field populated for all rules (including existing velocity insights)
- [ ] `StatsResult` has per-section insight fields
- [ ] Pipelines populate `.Insights` during `ProcessData()` via private `generateInsights()` methods
- [ ] Report command populates `StatsResult.*Insights` from pipeline results
- [ ] Quality insights generated alongside `computeQuality` using already-classified items
- [ ] Classification stays in `cmd` layer — no `classify` import in pipeline packages

### Phase 2: Report Rendering

**Goal**: Key Findings section in all three report formats.

**Files**:
- `internal/format/templates/report.md.tmpl` — add Key Findings section before metrics table
- `internal/format/templates.go` — update `reportTemplateData` with `HasInsights`, `InsightGroups`, populate in `renderReportMarkdown()` from all 5 insight sources
- `internal/format/report.go` — add `buildInsightGroups()` helper, update `WriteReportPretty` with Key Findings block, update `WriteReportJSON` with per-section insight arrays (including `jsonVelocitySummary`), add `jsonInsight` struct
- `internal/format/report_test.go` — test all three formats render insights correctly

**Acceptance**:
- [ ] Report markdown shows Key Findings section grouped by section before metrics table
- [ ] Report pretty shows Key Findings with `→` bullets before metric lines
- [ ] Report JSON has `insights` arrays (with `type` and `message`) on all sections including velocity
- [ ] Sections with zero insights omitted from Key Findings
- [ ] Velocity insights (from `VelocityResult.Insights`) appear in Key Findings alongside other sections
- [ ] `buildInsightGroups()` handles the velocity asymmetry (nested in VelocityResult vs sibling fields)

### Phase 3: Standalone Command Rendering + Smoke Tests

**Goal**: Individual commands show insights, end-to-end validation, documentation.

**Files**:
- `internal/pipeline/leadtime/render.go` — add insight rendering (pretty: `WriteInsightsPretty`, markdown: `**Insights:**` list, JSON: `Insights []jsonInsight` field)
- `internal/pipeline/cycletime/render.go` — same
- `internal/pipeline/throughput/render.go` — same
- Each pipeline's markdown template — add insights section before table
- Each pipeline's render tests
- `scripts/smoke-test.sh` — add assertions for insight output in report and standalone commands
- Site docs (`site/content/`) — update report and command reference pages to describe insights

**Acceptance**:
- [ ] `flow lead-time --since 30d` shows insights before the per-item table (all 3 formats)
- [ ] `flow cycle-time --since 30d` shows insights before the per-item table (all 3 formats)
- [ ] `flow throughput --since 30d` shows insights before the summary table (all 3 formats)
- [ ] `flow lead-time 42` (single item) shows NO insights
- [ ] Smoke tests verify insights appear in report output (markdown and JSON)
- [ ] Smoke tests verify standalone commands show insights
- [ ] Hugo site documentation updated

## Alternative Approaches Considered

1. **Central insights engine** — A single `insights.Generate(StatsResult)` function. Rejected: requires the engine to understand every metric type. Per-pipeline is simpler and follows the existing velocity pattern. (see brainstorm: Key Decision #2)

2. **Insights-after-table layout** — Keep metrics table at top. Rejected: user wants insights as the headline, not an afterthought. (see brainstorm: Key Decision #1)

3. **AI-powered insights** — Use an LLM to generate natural language insights. Rejected: no AI budget at runtime. Deterministic heuristics are sufficient and predictable. Users can export to JSON/markdown for agent analysis. (see brainstorm: Design Constraints)

4. **Start with a minimal rule set** — 2-3 rules per section. Rejected: the rules are simple conditionals, the cost of adding each is low, and sparse insights are as important as rich ones. (see brainstorm: Key Decision #5)

5. **Add `Level` field to `Insight`** — The solutions doc suggests `Level: "info"|"warning"|"success"`. Deferred: `Type` serves the agent-native need (identity/routing) without the presentation concern. Can add Level when we need to filter or style by severity.

6. **`InsightItem` universal adapter** — Original plan had a single adapter struct for all pipelines. Simplified: shared stats rules take `model.Stats` + optional `ItemRef` for fastest/slowest. Quality takes pre-classified `[]ItemRef` slices built by the cmd layer. No universal adapter needed. (Research: simplicity reviewer)

7. **Per-category insights in lead-time/cycle-time** — Would require injecting `classify` into pipeline packages. Deferred: quality already classifies, so category insights live there. Lead-time and cycle-time get statistical insights only. (Research: simplicity reviewer, architecture strategist)

8. **Structured `data` map on Insight** — Agent-native reviewer suggested `data: {"outlier_count": 4, "threshold_seconds": 38620800}`. Deferred: the structured data is already available in the per-section JSON fields. Agents can correlate `Type` with section fields. Avoids `map[string]any` in a typed codebase.

## Deferred Work

**`--compare-prior` flag for trend insights**: Requires fetching a prior-period window of equal length, doubling API calls. Opt-in via flag as decided in brainstorm. Deferred to a follow-up issue after this plan ships. The insight rules that require prior-period data (trend vs prior period for lead time, cycle time, throughput) are skipped in this plan. (see brainstorm: Resolved Question #1)

**Per-category insights for lead-time and cycle-time**: Requires injecting `classify` into pipeline packages or threading classified items from the cmd layer. Deferred until users request per-category lead-time breakdown. Quality section covers category distribution. (Research: simplicity reviewer)

**Quality insights in `quality release` command**: The release command has its own quality rendering. Adding insights there follows the same pattern but is a separate scope.

**Top-level `insights` array in report JSON**: An aggregated array with `section` tags for easy agent iteration. Currently per-section arrays are sufficient. Easy to add as a follow-up.

## Acceptance Criteria

### Functional Requirements

- [ ] Lead time insights generated: outliers, skew, fastest/slowest
- [ ] Cycle time insights generated: all lead-time rules + strategy-specific guidance + no-data warning
- [ ] Throughput insights generated: issue/PR mismatch, zero activity
- [ ] Quality insights generated: defect rate threshold, bug fix speed, category distribution, hotfix detection
- [ ] `Insight.Type` populated for all rules (including retrofitted velocity insights)
- [ ] Report Key Findings section leads the report (before metrics table) in markdown and pretty formats
- [ ] Report JSON has per-section `insights` arrays with `type` and `message` fields
- [ ] Velocity insights included in report JSON (`jsonVelocitySummary.Insights`)
- [ ] Standalone commands show insights before per-item tables (all 3 formats)
- [ ] Single-item commands (`lead-time N`, `cycle-time N`) do NOT show insights
- [ ] Sections with zero insights omitted from Key Findings
- [ ] Quality category insights fire only when `quality.categories` configured
- [ ] All threshold values are named constants
- [ ] Classification stays in cmd layer — no `classify` import in pipeline packages

### Testing Requirements

- [ ] Table-driven unit tests for every insight rule in `internal/metrics/report_insights_test.go`
- [ ] Tests use `wantCount` + `wantSubstr` assertions (not exact message matching)
- [ ] Boundary tests at each threshold (at threshold, below threshold)
- [ ] Edge cases tested: zero items, one item, no outliers, no categories, nil stats
- [ ] Pipeline integration tests verify ProcessData populates Insights with Type field
- [ ] Report format tests verify Key Findings rendering (markdown, pretty, JSON)
- [ ] Standalone command format tests verify insight rendering
- [ ] Smoke tests verify end-to-end insight output

### Documentation Requirements

- [ ] Hugo site command reference updated for report and flow commands
- [ ] Insight rules documented (what each heuristic detects and threshold values)
- [ ] Agent integration guide updated with insight JSON shape and Type taxonomy
- [ ] `docs/solutions/architecture-patterns/command-output-shape.md` updated to match actual `Insight` struct

## Dependencies & Risks

| Risk | Mitigation |
|------|-----------|
| Large surface area (4 pipelines × 3 formats) | 3-phase approach: foundation+pipeline first (proves the pattern), then report rendering, then standalone. Vertical slice. |
| Insight messages could be noisy for small datasets | `MinItemsForInsight` threshold (3) prevents statistical insights on tiny samples |
| `--compare-prior` deferred but brainstorm decided it's opt-in | Clean cut: trend-related rules are clearly marked as deferred. No stubs or partial implementation |
| Conversion functions placed in wrong package → circular import | Explicitly pinned: conversion lives in `cmd/` layer, never in `internal/metrics/` |
| Velocity insight asymmetry in StatsResult | Documented; `buildInsightGroups()` handles both access patterns explicitly |

## Success Metrics

- Report output leads with actionable Key Findings — not just numbers
- Every pipeline with aggregate data surfaces at least one insight when data is non-trivial
- Insight generation adds zero API calls (operates on already-fetched data)
- JSON consumers get per-section insights with `type` field for filtering without parsing natural language

## Sources & References

### Origin

- **Brainstorm document:** [docs/brainstorms/2026-03-17-insights-for-all-report-sections-brainstorm.md](docs/brainstorms/2026-03-17-insights-for-all-report-sections-brainstorm.md) — Key decisions: insights-first layout, per-pipeline generation, all commands get insights, comprehensive deterministic rules, no AI at runtime

### Internal References

- Existing insight generation: `internal/pipeline/velocity/velocity.go:338-378`
- `WriteInsightsPretty` helper: `internal/model/provenance.go:38-47`
- Command output shape convention: `docs/solutions/architecture-patterns/command-output-shape.md`
- Complete JSON for agents: `docs/solutions/architecture-patterns/complete-json-output-for-agents.md`
- Multi-category classification: `docs/solutions/architecture-patterns/multi-category-classification.md`
- Pipeline-per-metric layout: `docs/solutions/architecture-refactors/pipeline-per-metric-and-preflight-first-config.md`
- Pipeline interface: `internal/pipeline/pipeline.go:13-17`
- Report assembly: `cmd/report.go:236-293`
- Report rendering: `internal/format/report.go`
- Report markdown template: `internal/format/templates/report.md.tmpl`
- StatsResult: `internal/model/types.go:208-220`
- Stats: `internal/model/types.go:157-167`
- Quality computation: `cmd/report.go:364-387`
- Classifier: `internal/classify/classify.go`
- MyWeek hybrid JSON pattern: `internal/format/myweek.go:339-349`

### External References

- GitHub issue: [#82](https://github.com/dvhthomas/gh-velocity/issues/82)
- Example current output: [Discussion #83](https://github.com/dvhthomas/gh-velocity/discussions/83#discussioncomment-16171967)
- Terraform diagnostics pattern: [HashiCorp Developer docs](https://developer.hashicorp.com/terraform/plugin/framework/diagnostics)
- Go table-driven tests: [Go Wiki](https://go.dev/wiki/TableDrivenTests)
- Data storytelling principles: [Storytelling with Data](https://www.storytellingwithdata.com/blog/from-dashboard-to-story)

### Related Work

- Velocity insights (already implemented): PR series from 2026-03-12
- First-run experience warnings: `2026-03-12-004` plan (completed)
