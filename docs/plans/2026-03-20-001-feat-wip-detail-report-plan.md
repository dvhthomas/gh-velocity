---
title: "feat: WIP detail report with effort-weighted limits and report integration"
type: feat
status: completed
date: 2026-03-20
origin: docs/brainstorms/2026-03-20-wip-detail-report-requirements.md
---

# WIP Detail Report

## Overview

Transform the incomplete `gh velocity status wip` command into a full WIP detail report that answers two fundamental questions: **how many people are working on things** (assigned capacity) and **how many things are in progress** (work volume by stage and health). Integrate WIP into the composite `gh velocity report` dashboard, promote effort config to a shared top-level concept, and add effort-weighted WIP limit warnings.

## Problem Statement / Motivation

The existing WIP command fetches only issues (not PRs), makes its own API calls (no data sharing with the report), produces a flat item table without summary analytics, and has no concept of assignee load, effort weighting, or WIP limits. The report dashboard has a `TODO` placeholder at `report.go:304` where WIP should appear. Teams running the report get no visibility into current work-in-progress.

(see origin: `docs/brainstorms/2026-03-20-wip-detail-report-requirements.md`)

## Proposed Solution

Five-phase implementation:
1. **Foundation** — Model additions, config restructure, Search API parsing
2. **Data Layer** — Throughput pipeline retains items + fetches open items
3. **WIP Pipeline** — New `internal/pipeline/wip/` with classify, analyze, compute
4. **Output** — JSON/Markdown/Pretty renderers for enriched WIP output
5. **Integration** — Report dashboard integration + standalone command refactor

## Technical Approach

### Architecture

WIP follows the established pipeline-per-metric pattern (`GatherData` / `ProcessData` / `Render`) under `internal/pipeline/wip/`. In report context, WIP receives open items from the throughput pipeline's retained data rather than making its own API calls. In standalone context, the WIP pipeline fetches its own data using the same query-building logic.

Effort evaluation is extracted from the velocity pipeline into a shared function that accepts a generic input (labels, issue type, optional effort value) and returns an effort weight. Both velocity and WIP consume this shared evaluator.

```
┌─────────────────────────────────────────────────────────┐
│  gh velocity report                                     │
│                                                         │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐    │
│  │ Lead Time   │  │ Cycle Time  │  │ Throughput   │    │
│  │ Pipeline    │  │ Pipeline    │  │ Pipeline     │    │
│  └─────────────┘  └─────────────┘  │ (expanded)   │    │
│                                     │              │    │
│                                     │ ClosedIssues │    │
│                                     │ MergedPRs    │    │
│                                     │ OpenIssues ──┼──┐ │
│                                     │ OpenPRs ─────┼──┤ │
│                                     └──────────────┘  │ │
│                                                       │ │
│                                     ┌─────────────┐  │ │
│                                     │ WIP Pipeline │◄─┘ │
│                                     │ (consumer)   │    │
│                                     │              │    │
│                                     │ Classify     │    │
│                                     │ Stage Counts │    │
│                                     │ Assignees    │    │
│                                     │ Staleness    │    │
│                                     │ Effort Eval  │    │
│                                     │ Insights     │    │
│                                     └──────────────┘    │
│                                                         │
│  ┌─────────────┐  ┌─────────────┐                      │
│  │ Velocity    │  │ Quality     │                      │
│  │ Pipeline    │  │ (from lead) │                      │
│  └─────────────┘  └─────────────┘                      │
└─────────────────────────────────────────────────────────┘
```

### Planning Question Resolutions

**How open items are bounded (highest-priority question from origin):**

Use **label-filtered queries** — one Search API query per lifecycle label, same approach as the current `wip` command. This:
- Naturally limits result sets to WIP-qualifying items
- Avoids hitting the 1000-result cap for most repos
- Scales with number of lifecycle labels (typically 2-5), not total open items
- Non-label matchers (`type:`, `title:`, `field:`) are applied **client-side** after fetching label-matching items

For throughput's open-item queries: run one query per lifecycle label with `is:open label:"<name>"`, deduplicate results client-side (matching the existing `seen` map pattern in `cmd/wip.go`). If a query returns 1000 results (the API cap), emit a warning that results may be truncated.

To support PRs: run the same label queries but with `is:pr` instead of `is:issue`. For PRs without lifecycle labels (native signal fallback), add one additional query: `is:pr is:open -label:<all lifecycle labels>` to catch unlabeled open PRs, then classify them by draft status client-side.

**Config conflict (both top-level `effort` and `velocity.effort` present):**

Top-level wins. Emit deprecation warning: "ignoring velocity.effort because top-level effort is configured; remove velocity.effort to silence this warning." This follows the standard "prefer new, warn on old" migration pattern.

**Numeric effort for open items:**

Fall back to count-based effort with a warning: "WIP limits using item count instead of numeric effort because project board data is not available for open items." This avoids violating the "no additional API calls" goal. Document that `numeric` effort strategy is fully supported for velocity but partially supported for WIP (count-based fallback).

**Standalone WIP vs report WIP shared logic:**

Create `internal/pipeline/wip/` with a `Pipeline` struct. In report context, call `pipeline.SetItems(openIssues, openPRs)` then `ProcessData()`. In standalone context, call `GatherData()` which fetches its own items, then `ProcessData()`. `ProcessData` and `Render` are identical in both paths.

**Top N assignees:** Hardcode to 10. YAGNI for configurability.

**Backlog exclusion:** Items are WIP only if they match lifecycle `in-progress` or `in-review` matchers. Items with no lifecycle match are excluded. Items matching backlog matchers are excluded even if they also match an in-progress matcher — backlog takes precedence (per institutional learning from `docs/solutions/cycle-time-signal-hierarchy.md`).

### Implementation Phases

#### Phase 1: Foundation — Models, Config, Search API

**Goal:** All prerequisite data structures and config changes in place.

##### 1a. Model additions

- `internal/model/types.go`:
  - Add `Assignees []string` to `Issue` struct (after `URL` field)
  - Add `Assignees []string` and `Draft bool` and `UpdatedAt time.Time` to `PR` struct
  - Add `MatchedMatcher string` field to `WIPItem` (which specific matcher classified this item)
  - Add `Assignees []string` and `EffortValue float64` fields to `WIPItem`
  - Create `WIPResult` struct to replace `WIPCount *int` on `StatsResult`:

```go
// WIPResult holds the complete WIP analysis for output.
type WIPResult struct {
    Repository string
    Items      []WIPItem
    StageCounts []WIPStageCount  // per-stage with matcher sub-counts
    Assignees   []WIPAssignee    // top N by item count
    Staleness   WIPStaleness     // aggregate ACTIVE/AGING/STALE counts
    TotalEffort float64          // sum of effort across all WIP items
    TeamLimit   *float64         // configured team limit (nil if not set)
    PersonLimit *float64         // configured person limit (nil if not set)
    Warnings    []string
    Insights    []Insight
}

type WIPStageCount struct {
    Stage        string           // "In Progress" or "In Review"
    Count        int
    MatcherCounts []WIPMatcherCount // per-matcher breakdown within stage
}

type WIPMatcherCount struct {
    Matcher string // the matcher string, e.g. "label:design"
    Label   string // human-readable display name, e.g. "Design"
    Count   int
}

type WIPAssignee struct {
    Login         string
    ItemCount     int
    TotalEffort   float64
    ByStage       map[string]int  // stage -> count
    OverLimit     bool            // true if exceeds person_limit
}

type WIPStaleness struct {
    Active int
    Aging  int
    Stale  int
}
```

  - Replace `WIPCount *int` with `WIP *WIPResult` on `StatsResult`

- **Tests:** Update any tests that reference `StatsResult.WIPCount` to use `StatsResult.WIP`

##### 1b. Search API parsing

- `internal/github/pullrequests.go` — update `searchIssueResponse`:
  - Add `Assignees []struct{ Login string }` field (JSON: `"assignees"`)
  - Add `Draft bool` field (JSON: `"draft"`)
- `internal/github/search.go`:
  - `searchItemToIssue()`: populate `issue.Assignees` from response assignees
  - `searchItemToPR()`: populate `pr.Assignees` and `pr.Draft` from response, populate `pr.UpdatedAt` from response `UpdatedAt`
- **Tests:** Add test cases for assignee/draft parsing in search response conversion

##### 1c. Config restructure — effort promotion

- `internal/config/config.go`:
  - Add `Effort EffortConfig` field to `Config` struct
  - Add `WIP WIPConfig` field to `Config` struct:
    ```go
    type WIPConfig struct {
        TeamLimit   *float64 `yaml:"team_limit" json:"team_limit"`
        PersonLimit *float64 `yaml:"person_limit" json:"person_limit"`
    }
    ```
  - Add `"effort"` and `"wip"` to `knownTopLevelKeys`
  - In `defaults()`: set `Effort.Strategy` to `"count"` as default
  - In `Parse()` or post-parse migration step:
    - If top-level `Effort.Strategy` is empty and `Velocity.Effort.Strategy` is non-empty: copy `Velocity.Effort` to top-level `Effort`, emit deprecation warning
    - If both are non-empty: use top-level, emit "ignoring velocity.effort" warning
    - Always set `Velocity.Effort = Effort` so velocity pipeline reads from the canonical location
  - Extract effort validation from `validateVelocity()` into `validateEffort(e *EffortConfig, projectURL string) error`
  - Call `validateEffort` from the top-level validate, not from `validateVelocity`

- `cmd/config.go`:
  - Update `defaultConfigTemplate` to show top-level `effort:` section with all three strategy examples
  - Add commented-out `wip:` section with `team_limit` and `person_limit`
  - Update `config show` output to include top-level `effort` and `wip` sections

- `cmd/preflight.go`:
  - Update preflight to generate top-level `effort:` (not under velocity) when writing config
  - Add WIP limit stubs when lifecycle labels are detected

- **Tests:**
  - Config parsing: top-level effort, velocity.effort fallback, both present
  - Validation: effort validation works from top-level
  - Preflight: generates top-level effort section
  - Config show: reflects new sections

##### 1d. Effort evaluator extraction

- `internal/effort/effort.go` (new package):
  - Move `EffortEvaluator` interface and implementations (`CountEvaluator`, `AttributeEvaluator`, `NumericEvaluator`) from `internal/pipeline/velocity/effort.go`
  - Define generic input type:
    ```go
    type Item struct {
        Labels    []string
        IssueType string
        Title     string
        Fields    map[string]string
        Effort    *float64 // nil when no numeric data available
    }
    ```
  - `NewEvaluator(cfg config.EffortConfig) (Evaluator, error)` factory
  - `Evaluate(item Item) (effort float64, assessed bool)` method
  - For `NumericEvaluator`: when `item.Effort` is nil, return `(0, false)` — caller decides fallback behavior
  - Export `HasFieldMatchers()` and `ExtractFieldMatcherNames()` helpers

- `internal/pipeline/velocity/effort.go`:
  - Remove moved types, import from `internal/effort` instead
  - Adapt `VelocityItem` → `effort.Item` conversion at call sites

- **Tests:** Move existing effort tests from velocity to the new package. Add test for nil effort fallback on NumericEvaluator.

**Acceptance Criteria Phase 1:**
- [ ] `model.Issue` has `Assignees []string`, populated from Search API
- [ ] `model.PR` has `Assignees []string`, `Draft bool`, `UpdatedAt time.Time`, populated from Search API
- [ ] `model.WIPResult` struct exists with all sub-types
- [ ] `model.StatsResult.WIP` is `*WIPResult` (not `*int`)
- [ ] Top-level `effort` config parsed, validated, and used by velocity pipeline
- [ ] `velocity.effort` read as deprecated alias with warning
- [ ] Both-present config conflict handled (top-level wins, warning emitted)
- [ ] `defaultConfigTemplate` has top-level `effort` and `wip` sections
- [ ] `internal/effort` package with shared evaluator, tests passing
- [ ] `task test` passes, `task quality` passes

---

#### Phase 2: Data Layer — Throughput Expansion

**Goal:** Throughput pipeline retains full items and fetches open items.

##### 2a. Throughput retains items

- `internal/pipeline/throughput/throughput.go`:
  - Add exported fields for retained items:
    ```go
    ClosedIssues []model.Issue
    MergedPRs    []model.PR
    OpenIssues   []model.Issue
    OpenPRs      []model.PR
    ```
  - In `GatherData`: store the full slices instead of just counts
  - Update `issueCount`/`prCount` to use `len(p.ClosedIssues)`/`len(p.MergedPRs)`
  - No change to `ProcessData` or `Render` — throughput's own output remains counts

##### 2b. Throughput fetches open items

- `internal/pipeline/throughput/throughput.go`:
  - Add fields for open-item query configuration:
    ```go
    OpenIssueQueries []string // one per lifecycle label, built by caller
    OpenPRQueries    []string // one per lifecycle label + one for unlabeled PRs
    ```
  - In `GatherData`: after existing closed/merged fetches, loop through `OpenIssueQueries` and `OpenPRQueries`, search each, deduplicate by number, store in `OpenIssues`/`OpenPRs`
  - If any open query returns 1000 results, add a warning
  - Open-item fetch failures are partial — add to `Warnings`, don't fail pipeline

- `cmd/report.go`:
  - Build open-item queries from lifecycle config labels (same approach as current `cmd/wip.go`)
  - Pass queries to throughput pipeline before `GatherData`
  - After throughput's `GatherData`/`ProcessData`: pass `throughputPipeline.OpenIssues`/`OpenPRs` to WIP pipeline

- `internal/scope/scope.go`:
  - Add helper `OpenIssueByLabelQuery(scopeStr, label string) Query` — builds `is:open is:issue label:"<label>"` query
  - Add helper `OpenPRByLabelQuery(scopeStr, label string) Query` — builds `is:open is:pr label:"<label>"` query
  - Add helper `OpenUnlabeledPRQuery(scopeStr string, excludeLabels []string) Query` — for PR native signal fallback

- **Tests:**
  - Throughput retains items (not just counts) after GatherData
  - Open-item queries built correctly from lifecycle labels
  - Deduplication works when same item matches multiple labels
  - Partial failure (open fetch fails, closed succeeds) produces warning but doesn't fail
  - 1000-result cap warning emitted

**Acceptance Criteria Phase 2:**
- [ ] `throughputPipeline.ClosedIssues` / `.MergedPRs` populated after `GatherData`
- [ ] `throughputPipeline.OpenIssues` / `.OpenPRs` populated from lifecycle label queries
- [ ] Throughput's own `Result.IssuesClosed` / `.PRsMerged` counts unchanged
- [ ] Deduplication across multiple label queries works correctly
- [ ] 1000-result truncation warning emitted
- [ ] Open-item fetch failure is a warning, not an error
- [ ] `task test` passes

---

#### Phase 3: WIP Pipeline

**Goal:** Full WIP computation: classify, analyze, produce `WIPResult`.

##### 3a. Pipeline structure

- `internal/pipeline/wip/wip.go`:
  - `Pipeline` struct with the standard three-method interface:
    ```go
    type Pipeline struct {
        // Config
        Client            *gh.Client
        Owner, Repo       string
        LifecycleConfig   config.LifecycleConfig
        EffortConfig      config.EffortConfig
        WIPConfig         config.WIPConfig
        Scope             string
        Now               time.Time
        Debug             bool

        // Data injection (report context)
        InjectedIssues []model.Issue
        InjectedPRs    []model.PR

        // Internal
        openIssues []model.Issue
        openPRs    []model.PR

        // Output
        Result   model.WIPResult
        Warnings []string
    }
    ```
  - `GatherData(ctx)`:
    - If `InjectedIssues`/`InjectedPRs` are set, use them (report context)
    - Otherwise, build label-filtered queries and fetch (standalone context)
    - Store in `openIssues`/`openPRs`
  - `ProcessData()`:
    - Classify all items into lifecycle stages (see 3b)
    - Compute stage counts with matcher granularity
    - Compute assignee table
    - Compute staleness breakdown
    - Evaluate effort per item
    - Check WIP limits, generate warnings
    - Generate insights
    - Assemble `Result`
  - `Render(rc)`: dispatch to JSON/Markdown/Pretty

##### 3b. Classification logic

- `internal/pipeline/wip/classify.go`:
  - `classifyItem(item classifyInput, inProgressMatchers, inReviewMatchers []string) (stage string, matchedMatcher string)`:
    - Build `classify.Input` from item labels/type/title/fields
    - Check in-review matchers first (more specific), then in-progress
    - Return stage name and the specific matcher that matched
    - For PRs with no matcher match: use native signal (draft → "In Progress", non-draft → "In Review"), matcher = "draft" or "open-pr"
  - Backlog exclusion: if configured `lifecycle.backlog.match` matchers match, exclude item entirely (backlog overrides, per `docs/solutions/cycle-time-signal-hierarchy.md`)

##### 3c. Computation functions

- `internal/metrics/wip.go`:
  - `ComputeWIPStageCounts(items []model.WIPItem, inProgressMatchers, inReviewMatchers []string) []model.WIPStageCount`
  - `ComputeWIPAssignees(items []model.WIPItem, limit int) []model.WIPAssignee`
  - `ComputeWIPStaleness(items []model.WIPItem) model.WIPStaleness`
  - `ComputeWIPEffort(items []model.WIPItem, evaluator effort.Evaluator) (totalEffort float64, perItem map[int]float64)`
  - `GenerateWIPInsights(result model.WIPResult) []model.Insight`

- Insight types (for `Insight.Type` field):
  - `"wip_assignee_load"` — highest-loaded assignee
  - `"wip_staleness"` — staleness percentage
  - `"wip_capacity"` — items in progress across N people
  - `"wip_stage_health"` — stage-specific observations
  - `"wip_team_limit_exceeded"` — team WIP limit warning
  - `"wip_person_limit_exceeded"` — individual WIP limit warning

- **Tests:** Table-driven tests for each computation function (per AGENTS.md convention):
  - Stage counts with multiple matchers per stage
  - Assignee ranking with ties
  - Staleness thresholds at exact boundaries (3d, 7d)
  - Effort evaluation with count/attribute strategies
  - WIP limit warnings (team + individual, both configured and not)
  - Insight generation for each insight type
  - Empty input (no WIP items)
  - Items with no assignees

**Acceptance Criteria Phase 3:**
- [ ] WIP pipeline classifies issues and PRs into lifecycle stages
- [ ] PR native signal fallback works (draft → In Progress, non-draft → In Review)
- [ ] Label matchers take precedence over native signals for PRs
- [ ] Backlog items excluded even if they match in-progress matchers
- [ ] Stage counts show per-matcher sub-counts
- [ ] Top 10 assignees ranked by item count with per-stage breakdown
- [ ] Staleness breakdown computed with existing 3d/7d thresholds
- [ ] Effort evaluation uses shared `internal/effort` evaluator
- [ ] Numeric effort falls back to count with warning for open items
- [ ] WIP limit warnings generated when limits exceeded
- [ ] Insights generated and pass "would a PM understand this?" test
- [ ] Table-driven tests for all computation functions
- [ ] `task test` passes

---

#### Phase 4: Output / Format

**Goal:** Rich WIP output in all three formats.

##### 4a. JSON output

- `internal/pipeline/wip/render.go` (or `internal/format/wip.go` — extend existing):
  - `WriteWIPJSON(w io.Writer, result model.WIPResult) error`
  - JSON structure:
    ```json
    {
      "repository": "owner/repo",
      "total_items": 14,
      "total_effort": 42.0,
      "stage_counts": [
        {"stage": "In Progress", "count": 8, "matchers": [
          {"matcher": "label:design", "label": "Design", "count": 3},
          {"matcher": "label:in-progress", "label": "In Progress", "count": 5}
        ]},
        {"stage": "In Review", "count": 6, "matchers": [...]}
      ],
      "assignees": [
        {"login": "alice", "item_count": 5, "total_effort": 15.0, "by_stage": {"In Progress": 3, "In Review": 2}, "over_limit": true}
      ],
      "staleness": {"active": 8, "aging": 3, "stale": 3},
      "team_limit": 50.0,
      "person_limit": 8.0,
      "items": [...],
      "insights": [...],
      "warnings": [...]
    }
    ```
  - Follows complete JSON output contract (per `docs/solutions/architecture-patterns/complete-json-output-for-agents.md`)

##### 4b. Markdown output

- `internal/format/templates/wip.md.tmpl` — expand template:
  - Summary line: "**14 items** in progress (3 stale)"
  - Stage counts table with matcher sub-counts
  - Top assignees table with over-limit flag
  - Staleness breakdown
  - WIP limit status (if configured)
  - Insights section
  - Per-item table (existing, enhanced with assignee column)

##### 4c. Pretty output

- Extend `WriteWIPPretty` in `internal/format/wip.go`:
  - Summary stats line
  - Stage counts with color-coded matcher sub-counts
  - Assignee table using `format.NewTable()` (per `docs/solutions/go-gh-tableprinter-migration.md`)
  - Staleness breakdown with color coding (ACTIVE=green, AGING=yellow, STALE=red)
  - WIP limit warnings prominently displayed
  - Per-item table (existing, enhanced)

##### 4d. Report format integration

- `internal/format/report.go`:
  - Replace `WIPCount *int` rendering with rich `WIP *WIPResult` rendering
  - Summary row: "WIP: 14 items (3 stale)" in the dashboard table
  - JSON report: embed full WIP result in the `"wip"` key (breaking change from `{"wip": {"count": N}}`)

- `cmd/report.go`:
  - Add WIP detail section via `writeDetail()` helper (like existing lead time, cycle time, etc.)
  - Add WIP artifact section for `--write-to` mode

- **Tests:**
  - JSON output parses correctly, includes all required fields
  - Markdown renders valid markdown with correct collapsible sections
  - Pretty output uses tableprinter
  - Report summary row shows WIP count + stale count
  - Report JSON includes full WIP result
  - `--summary-only` includes summary row but omits detail section
  - `--write-to` writes `status-wip.json` and `status-wip.md` artifacts

**Acceptance Criteria Phase 4:**
- [ ] JSON output is self-contained with all WIP data, warnings, insights
- [ ] Markdown output includes stage counts, assignees, staleness, insights
- [ ] Pretty output uses `format.NewTable()` with color-coded staleness
- [ ] Report summary row shows "WIP: N items (M stale)"
- [ ] Report detail section is collapsible in markdown
- [ ] Report JSON includes full WIP result (not just count)
- [ ] `--write-to` produces `status-wip.json` and `status-wip.md`
- [ ] `task test` passes

---

#### Phase 5: Integration — Standalone Command + Report Wiring

**Goal:** Everything wired together end-to-end.

##### 5a. Report integration

- `cmd/report.go`:
  - Build open-item queries from lifecycle config:
    ```go
    var openIssueQueries, openPRQueries []string
    for _, m := range cfg.Lifecycle.InProgress.Match {
        if strings.HasPrefix(m, "label:") {
            label := m[len("label:"):]
            openIssueQueries = append(openIssueQueries, scope.OpenIssueByLabelQuery(deps.Scope, label).Build())
            openPRQueries = append(openPRQueries, scope.OpenPRByLabelQuery(deps.Scope, label).Build())
        }
    }
    // same for InReview
    ```
  - Pass queries to throughput pipeline
  - After throughput GatherData: create WIP pipeline with injected items:
    ```go
    wipPipeline := &wip.Pipeline{
        Owner: deps.Owner, Repo: deps.Repo,
        LifecycleConfig: cfg.Lifecycle,
        EffortConfig:    cfg.Effort,
        WIPConfig:       cfg.WIP,
        Now:             now,
        InjectedIssues:  throughputPipeline.OpenIssues,
        InjectedPRs:     throughputPipeline.OpenPRs,
    }
    ```
  - Run `wipPipeline.ProcessData()` (no GatherData needed — items injected)
  - Add `result.WIP = &wipPipeline.Result` to StatsResult
  - Add WIP detail section in render closure
  - Remove `// TODO(PR C): WIP from project board or active_labels config` at line 304
  - Add `wipOK` boolean flag following existing pattern

- Graceful omission: if no lifecycle matchers configured, skip WIP entirely (no error, no section)

##### 5b. Standalone command refactor

- `cmd/wip.go`:
  - Replace manual fetch + classify with WIP pipeline:
    ```go
    p := &wip.Pipeline{
        Client:          client,
        Owner:           deps.Owner,
        Repo:            deps.Repo,
        LifecycleConfig: cfg.Lifecycle,
        EffortConfig:    cfg.Effort,
        WIPConfig:       cfg.WIP,
        Scope:           deps.Scope,
        Now:             deps.Now(),
        Debug:           deps.Debug,
    }
    if err := p.GatherData(ctx); err != nil { return err }
    if err := p.ProcessData(); err != nil { return err }
    return p.Render(deps.RenderCtx(os.Stdout))
    ```
  - Remove all manual fetch, classify, and format code from `cmd/wip.go`
  - The `classifyWIPStage` and `toWIPItemFromIssue` helper functions move into the pipeline

- **Tests:**
  - Smoke test: `gh velocity status wip` produces enriched output
  - Smoke test: `gh velocity status wip -r json` produces valid JSON with all fields
  - Smoke test: `gh velocity report` includes WIP section
  - Smoke test: `gh velocity report --summary-only` includes WIP summary row
  - Integration test: WIP in report matches standalone WIP data (same items, same counts)

**Acceptance Criteria Phase 5:**
- [ ] `gh velocity report` includes WIP section (summary row + collapsible detail)
- [ ] `gh velocity status wip` produces enriched output with assignees, stages, staleness, insights
- [ ] Report WIP reuses throughput's open items (no additional API calls)
- [ ] Standalone WIP fetches its own data via the pipeline's GatherData
- [ ] No lifecycle config → WIP section gracefully omitted from report
- [ ] `cmd/wip.go` TODO at report.go:304 removed
- [ ] All smoke tests passing
- [ ] `task test` passes, `task quality` passes

## System-Wide Impact

- **Interaction graph:** Throughput pipeline → WIP pipeline (data injection). Config parser → effort evaluator (shared by velocity + WIP). Report renderer → WIP renderer (format dispatch).
- **Error propagation:** WIP pipeline failures in report context produce warnings, not errors (consistent with existing section-failure pattern). Standalone WIP failures are errors.
- **State lifecycle risks:** No persistent state. All computation is per-invocation from fresh API data + in-memory processing.
- **API surface parity:** JSON output schema changes for report (`wip` key goes from `{count: N}` to full `WIPResult`). This is a breaking change for CI consumers.
- **Integration test scenarios:** (1) Report with WIP + all other sections, (2) Standalone WIP with effort-weighted limits, (3) Config migration from `velocity.effort` to top-level `effort`.

## Acceptance Criteria

### Functional Requirements

- [ ] (R1) Throughput retains full items with assignees
- [ ] (R2) Throughput fetches open items via lifecycle label queries
- [ ] (R3) Top-level `effort` config parsed, `velocity.effort` deprecated alias works
- [ ] (R4) WIP includes both issues and PRs with lifecycle classification
- [ ] (R5) Stage counts show per-matcher sub-counts
- [ ] (R6) Top 10 assignees with per-stage breakdown
- [ ] (R7) Staleness breakdown (ACTIVE/AGING/STALE)
- [ ] (R8) Team WIP warning when limit exceeded (effort-weighted)
- [ ] (R9) Individual WIP warning when limit exceeded
- [ ] (R10) Auto-generated insights following Insight pattern
- [ ] (R11) Report summary row: "WIP: N items (M stale)"
- [ ] (R12) Report detail section with full WIP data
- [ ] (R13) Standalone `gh velocity status wip` enriched
- [ ] (R14) JSON, Markdown, Pretty output for all WIP content

### Quality Gates

- [ ] Table-driven tests for all metric computation functions
- [ ] Config change checklist complete: struct, defaults, validate, preflight, template, show
- [ ] No `internal/metrics/` → `internal/format/` import violations
- [ ] All insights pass "would a PM understand this?" test
- [ ] `task test` and `task quality` pass after each phase
- [ ] JSON output is self-contained (no stderr warnings when `-r json`)

## Dependencies & Risks

- **Breaking JSON schema:** Report JSON `wip` key changes shape. CI consumers need updating. Mitigate by documenting the change.
- **API cost:** Throughput adds 2-5 open-item queries per lifecycle label, per item type. With 3 labels × 2 types = ~6-8 additional queries. Acceptable given search throttle and cache.
- **Numeric effort fallback:** WIP limits are less precise when effort strategy is `numeric` (falls back to count). Document this limitation.
- **1000-result cap:** Large repos with many lifecycle-labeled open items could hit the cap. Warning is emitted, but data is truncated. Future work could add pagination or scope narrowing.

## Sources & References

### Origin

- **Origin document:** [docs/brainstorms/2026-03-20-wip-detail-report-requirements.md](docs/brainstorms/2026-03-20-wip-detail-report-requirements.md) — Key decisions: throughput as data layer, effort promoted to top-level config, WIP limits use effort weighting, warnings not hard blocks, matcher granularity in stage counts.

### Internal References

- Pipeline pattern: `internal/pipeline/pipeline.go` (Pipeline interface)
- Throughput pipeline: `internal/pipeline/throughput/throughput.go`
- Effort evaluator: `internal/pipeline/velocity/effort.go`
- WIP command: `cmd/wip.go`
- Report composition: `cmd/report.go:192-304`
- Config struct: `internal/config/config.go:38-80`
- Config template: `cmd/config.go:165-273`
- Search API parsing: `internal/github/pullrequests.go:15-33`, `internal/github/search.go`
- Format layer: `internal/format/wip.go`, `internal/format/templates/wip.md.tmpl`
- Model types: `internal/model/types.go:54-252`
- Scope helpers: `internal/scope/scope.go`

### Institutional Learnings

- `docs/solutions/architecture-refactors/pipeline-per-metric-and-preflight-first-config.md` — pipeline pattern and config change checklist
- `docs/solutions/architecture-patterns/command-output-shape.md` — stats + detail + insights + provenance
- `docs/solutions/architecture-patterns/render-layer-linking-and-insight-quality.md` — insight quality gates
- `docs/solutions/cycle-time-signal-hierarchy.md` — backlog overrides everything for WIP detection
- `docs/solutions/architecture-patterns/complete-json-output-for-agents.md` — JSON output contract
- `docs/solutions/go-gh-tableprinter-migration.md` — `format.NewTable()` for pretty output
