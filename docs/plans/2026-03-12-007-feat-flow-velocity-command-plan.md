---
title: "feat: flow velocity command"
type: feat
status: active
date: 2026-03-12
origin: docs/brainstorms/2026-03-12-velocity-command-brainstorm.md
---

# feat: flow velocity command

## Overview

Add a `flow velocity` command that measures **effort completed per iteration** (velocity = a number) and **completion rate** (done / committed = a ratio). This is the tool's namesake metric: not just counting items (throughput) but measuring weighted work output aligned to iteration boundaries.

Three effort strategies (count, attribute-based via scope-style matchers, numeric via project board Number field), two period strategies (ProjectV2 Iteration field, fixed calendar periods), configurable work unit (issues or PRs), burnup support via closedAt timestamps.

Also: `config validate --velocity` for live matcher testing, and preflight heuristics that suggest velocity config from repo signals.

(See brainstorm: `docs/brainstorms/2026-03-12-velocity-command-brainstorm.md`)

## Problem Statement / Motivation

`velocity throughput` counts items in a sliding window — it's a proxy, not true velocity. Teams need effort-weighted output per iteration to answer "how much work did we ship this sprint?" and "how predictable are we?" This is the core metric the tool is named after.

## Proposed Solution

### Architecture

New pipeline at `internal/pipeline/velocity/` following the existing GatherData/ProcessData/Render pattern. New config section `velocity:` with effort and iteration strategy configuration. Extensions to preflight and config validate for velocity-specific guidance.

### Design Decisions Carried Forward from Brainstorm

1. **Velocity is a number** (effort completed), completion rate is a separate ratio — both shown, not conflated
2. **Effort strategies**: count (item=1), attribute (scope-style query→value), numeric (ProjectV2 Number field)
3. **Period strategies**: project-field (ProjectV2IterationField) and fixed (length+anchor)
4. **Work unit**: `issues | prs` — configurable
5. **"Done"**: Issues closed with `reason:completed`, PRs merged
6. **"Committed"**: Project-field → board assignment; fixed → open at start OR opened during
7. **Carry-over**: No cap, trust scope to filter stale work
8. **Effort matchers**: First-match wins (config order), unmatched → "not assessed"
9. **Burnup only**: No burndown (requires snapshots). Burnup from closedAt timestamps
10. **Scope**: `--scope` AND'd with iteration boundaries, filters committed + completed uniformly

### Critical Design Clarifications (from SpecFlow analysis)

**Effort matchers reuse `classify.Matcher`** — attribute queries use the existing `label:`, `type:`, and `title:` matcher syntax from `internal/classify/`, NOT full GitHub Search API syntax. This keeps evaluation client-side (iterate items, run matchers), avoids a second parser, and is consistent with quality category matchers. The brainstorm's "scope-style queries" language means the same qualifier syntax that categories use, which is testable in the GitHub Issues UI for label/type filters.

**Carry-over for project-field is informational, not synthetic** — committed = items assigned to that iteration on the board (from `fieldValueByName`). Carry-over is reported as a count of items that appeared in a prior iteration, but does not inflate the committed denominator. This keeps CLI numbers consistent with what the board shows.

**`--since`/`--until` filters iterations** — shows only iterations whose date ranges overlap the specified window. Does not create a virtual single-period. For a sliding-window view, use `flow throughput`.

**Issue-close comments are a future enhancement** — v1 supports discussion posting (bulk summaries). Per-issue comments on close would require webhook/Action integration not yet built. Defer to a follow-up.

**Numeric `0` vs `null`** — `null` (field never set) = not assessed. Explicit `0` = valid zero effort (consistent with attribute `value: 0` for chores). This differs slightly from the brainstorm's initial statement; updated here for correctness.

**Negative numeric values** — treated as "not assessed" with a warning.

**Cross-repo items on project boards** — filtered to `deps.Owner/deps.Repo`. Only items from the target repo count toward velocity.

**Fixed-period naming** — date range strings in output (e.g., "Mar 4 – Mar 17"), not numbered iterations.

**Completion rate can exceed 100%** — if unplanned work is completed, rate reflects that honestly.

**Empty iterations** — shown as rows with zero velocity, not omitted.

**Default iteration count** — 6 when `count` not specified in config.

## Technical Approach

### Implementation Phases

#### Phase 1: Config & Model Foundation

Add velocity config parsing, validation, and model types. Update preflight, config show, config validate, defaultConfigTemplate, and example configs.

**Tasks:**

- [x] Add `VelocityConfig` struct to `internal/config/config.go`
  ```go
  type VelocityConfig struct {
      Unit      string              `yaml:"unit" json:"unit"`           // "issues" or "prs"
      Effort    EffortConfig        `yaml:"effort" json:"effort"`
      Iteration IterationConfig     `yaml:"iteration" json:"iteration"`
  }
  type EffortConfig struct {
      Strategy  string              `yaml:"strategy" json:"strategy"`   // "count", "attribute", "numeric"
      Attribute []EffortMatcher     `yaml:"attribute" json:"attribute"`
      Numeric   NumericEffortConfig `yaml:"numeric" json:"numeric"`
  }
  type EffortMatcher struct {
      Query string  `yaml:"query" json:"query"`   // classify.Matcher syntax
      Value float64 `yaml:"value" json:"value"`
  }
  type NumericEffortConfig struct {
      ProjectField string `yaml:"project_field" json:"project_field"`
  }
  type IterationConfig struct {
      Strategy     string `yaml:"strategy" json:"strategy"`       // "project-field" or "fixed"
      ProjectField string `yaml:"project_field" json:"project_field"`
      Fixed        FixedIterationConfig `yaml:"fixed" json:"fixed"`
      Count        int    `yaml:"count" json:"count"`             // default 6
  }
  type FixedIterationConfig struct {
      Length string `yaml:"length" json:"length"` // e.g., "14d"
      Anchor string `yaml:"anchor" json:"anchor"` // e.g., "2026-01-06"
  }
  ```
- [x] Add `"velocity"` to `knownTopLevelKeys` in `internal/config/config.go`
- [x] Add velocity defaults in `defaults()`: unit="issues", effort.strategy="count", iteration.count=6
- [x] Add velocity validation in `validate()`:
  - `unit` must be "issues" or "prs"
  - `effort.strategy` must be "count", "attribute", or "numeric"
  - If strategy=attribute: at least one matcher required, values must be non-negative, queries must parse via `classify.ParseMatcher`
  - If strategy=numeric: `project_field` required, `project.url` must be set
  - `iteration.strategy` must be "project-field" or "fixed"
  - If iteration=project-field: `project_field` required, `project.url` must be set
  - If iteration=fixed: `length` and `anchor` required, anchor must parse as date, anchor must not be more than one period in the future
  - `count` must be > 0
- [x] Add `VelocityResult` to `internal/model/types.go`
  ```go
  type VelocityResult struct {
      Repository    string
      Unit          string // "issues" or "prs"
      EffortUnit    string // "pts", "items", etc.
      Current       *IterationVelocity // nil if --history
      History       []IterationVelocity // nil if --current
      AvgVelocity   float64
      AvgCompletion float64
      StdDev        float64
  }
  type IterationVelocity struct {
      Name          string // iteration title or date range
      Start         time.Time
      End           time.Time
      Velocity      float64 // effort completed
      Committed     float64 // effort committed
      CompletionPct float64 // velocity/committed * 100
      ItemsDone     int
      ItemsTotal    int
      CarryOver     int
      NotAssessed   int
      NotAssessedItems []int // issue/PR numbers
      Trend         string  // "▲", "▼", "─"
  }
  ```
- [x] Add `StateReason` field to `model.Issue` (needed for `reason:completed` filtering)
- [x] Update `defaultConfigTemplate` in `cmd/config.go` with commented velocity section
- [x] Update `newConfigShowCmd()` in `cmd/config.go` to display velocity config
- [x] Update example configs in `docs/examples/` with velocity sections
- [x] Table-driven tests for config parsing and validation (`internal/config/config_test.go`)

**Files:**
- `internal/config/config.go` (modify)
- `internal/model/types.go` (modify)
- `cmd/config.go` (modify)
- `docs/examples/*.yml` (modify)
- `internal/config/config_test.go` (modify)

#### Phase 2: GraphQL — Iteration Field & Number Field Queries

Extend the GitHub client to fetch iteration field configuration and item-level iteration/number values.

**Tasks:**

- [x] Add `ListIterationField(ctx, projectID, fieldName) (*IterationFieldConfig, error)` to `internal/github/`
  - Query `ProjectV2IterationField.configuration` for `iterations` + `completedIterations`
  - Returns `IterationFieldConfig{Iterations []Iteration, CompletedIterations []Iteration, DefaultDuration int}`
  - Each `Iteration{ID, Title, StartDate, Duration}` — end date computed
  - GraphQL variables only (per AGENTS.md)
- [x] Add `ResolveIterationField(ctx, projectURL, fieldName) (projectID, fieldID string, err error)` or extend `ResolveProject`
  - Similar to how status field is resolved, but for iteration fields
  - Must handle type mismatch (field exists but is not an iteration field)
- [x] Extend `ListProjectItems` or add `ListProjectItemsWithFields(ctx, projectID, iterationFieldName, numberFieldName)` to return iteration assignment and number field value per item
  - GraphQL fragment uses `fieldValueByName(name: "Sprint")` for iteration and `fieldValueByName(name: "Story Points")` for number
  - Returns extended `ProjectItem` or new `VelocityItem` with `IterationID`, `IterationTitle`, `EffortValue *float64`
  - Filter to items matching target repo (`deps.Owner/deps.Repo`)
- [x] Add model types for iteration data
  ```go
  // internal/model/types.go
  type Iteration struct {
      ID        string
      Title     string
      StartDate time.Time
      Duration  int // days
      EndDate   time.Time // computed: StartDate + Duration
  }
  type VelocityItem struct {
      ContentType string // "Issue" or "PullRequest"
      Number      int
      Title       string
      Repo        string // "owner/repo"
      State       string
      StateReason string // "completed", "not_planned", ""
      ClosedAt    *time.Time
      MergedAt    *time.Time
      CreatedAt   time.Time
      Labels      []string
      IssueType   string
      IterationID string
      Effort      *float64 // from Number field, nil if unset
  }
  ```
- [x] Table-driven tests with mock GraphQL responses

**Files:**
- `internal/github/iteration.go` (new)
- `internal/github/iteration_test.go` (new)
- `internal/model/types.go` (modify)

#### Phase 3: Effort Evaluation Engine

Build the effort classification system that maps items to effort values using the configured strategy.

**Tasks:**

- [x] Create `internal/pipeline/velocity/effort.go`
  - `type EffortEvaluator interface { Evaluate(item VelocityItem) (float64, bool) }` — returns (effort, assessed)
  - `CountEvaluator{}` — always returns (1, true)
  - `AttributeEvaluator{matchers []compiledMatcher}` — iterates matchers in config order, first match wins. Uses `classify.Matcher.Match(classify.Input{Labels, IssueType, Title})`. Returns (value, true) on match, (0, false) on no match
  - `NumericEvaluator{}` — returns (*item.Effort, true) if non-nil and >= 0. Returns (0, false) if nil. Warns and returns (0, false) if negative
  - Factory: `NewEffortEvaluator(cfg EffortConfig) (EffortEvaluator, error)` — parses matchers, validates
- [x] Table-driven tests for each evaluator:
  - Count: always returns 1
  - Attribute: first-match wins, no match → not assessed, `value: 0` is valid
  - Numeric: nil → not assessed, 0 → valid, negative → not assessed + warn
  - Attribute with overlapping matchers: verify first-match order

**Files:**
- `internal/pipeline/velocity/effort.go` (new)
- `internal/pipeline/velocity/effort_test.go` (new)

#### Phase 4: Period Computation Engine

Build the iteration boundary computation for both strategies.

**Tasks:**

- [ ] Create `internal/pipeline/velocity/period.go`
  - `type PeriodStrategy interface { Iterations(count int) ([]Iteration, error); Current() (*Iteration, error) }`
  - `ProjectFieldPeriod{iterations []Iteration, completed []Iteration}` — built from GraphQL data
    - `Current()` returns the iteration spanning `now`
    - `Iterations(count)` returns last N from `completedIterations`, ordered newest first
  - `FixedPeriod{length time.Duration, anchor time.Time, now time.Time}` — calendar math
    - Compute iteration boundaries backward from anchor
    - `Current()` returns the period spanning `now`
    - `Iterations(count)` returns last N completed periods
    - Name = date range string ("Mar 4 – Mar 17")
- [ ] Fixed-period edge cases:
  - Anchor in the future: compute backward; current iteration is whichever spans now
  - Partial current iteration: correctly identify in-progress period
- [ ] Table-driven tests for period computation:
  - Fixed: anchor math, boundary alignment, current identification
  - Project-field: iteration ordering, current detection, empty completedIterations

**Files:**
- `internal/pipeline/velocity/period.go` (new)
- `internal/pipeline/velocity/period_test.go` (new)

#### Phase 5: Velocity Pipeline — GatherData, ProcessData, Render

The core pipeline that ties together iteration resolution, data fetching, effort evaluation, and metric computation.

**Tasks:**

- [ ] Create `internal/pipeline/velocity/velocity.go`
  - Pipeline struct:
    ```go
    type Pipeline struct {
        Client          // github client interface (narrow)
        Owner, Repo     string
        Config          config.VelocityConfig
        ProjectConfig   config.ProjectConfig
        Scope           string
        ExcludeUsers    []string
        Now             time.Time
        ShowCurrent     bool // --current flag
        ShowHistory     bool // --history flag
        IterationCount  int  // --iterations N override
        Since, Until    *time.Time // --since/--until overrides
        // unexported
        result          *model.VelocityResult
    }
    ```
  - **GatherData(ctx)**:
    1. Resolve project board if needed (numeric effort or project-field iteration)
    2. Build period strategy (project-field: fetch iteration config via `ListIterationField`; fixed: construct from config)
    3. Get current iteration + last N completed iterations
    4. For project-field: fetch all project items with iteration/number fields via `ListProjectItemsWithFields`, filter by repo
    5. For fixed period + attribute/count effort: use search API (`scope.ClosedIssueQuery` / `scope.MergedPRQuery`) per iteration window, apply scope AND
    6. For fixed period + numeric effort: fetch project items (need board for Number field), but assign to iterations by close date
  - **ProcessData()**:
    1. Build effort evaluator from config
    2. For each iteration:
       - Classify items as done (reason:completed / merged) or not
       - Evaluate effort per item
       - Sum completed effort = velocity
       - Count committed items (project-field: all items assigned to iteration; fixed: open at start OR opened during)
       - Sum committed effort
       - Compute completion rate = velocity / committed * 100 (allow > 100%)
       - Count not-assessed items
       - Detect carry-over (items in this iteration that also appeared in prior iteration)
       - Compute trend vs previous iteration
    3. Compute aggregate stats: avg velocity, avg completion, std dev
    4. If `--since`/`--until`: filter iterations to those overlapping the window
    5. Store `VelocityResult`
  - **Render(rc)**: switch on format, delegate to render functions
- [ ] Narrow client interface at consumer site (per AGENTS.md):
  ```go
  type Client interface {
      SearchIssues(ctx context.Context, query string) ([]model.Issue, error)
      SearchPRs(ctx context.Context, query string) ([]model.PR, error)
      ListIterationField(ctx context.Context, projectID, fieldName string) (*model.IterationFieldConfig, error)
      ListProjectItemsWithFields(ctx context.Context, projectID, iterFieldName, numFieldName string) ([]model.VelocityItem, error)
  }
  ```

**Files:**
- `internal/pipeline/velocity/velocity.go` (new)
- `internal/pipeline/velocity/velocity_test.go` (new)

#### Phase 6: Output Rendering (JSON, Pretty, Markdown)

Three-format output following existing patterns.

**Tasks:**

- [ ] Create `internal/pipeline/velocity/render.go`
  - `WriteJSON(w, result)` — struct-based JSON with nested iterations
    ```go
    type jsonOutput struct {
        Repository string              `json:"repository"`
        Unit       string              `json:"unit"`
        Current    *jsonIteration      `json:"current,omitempty"`
        History    []jsonIteration     `json:"history,omitempty"`
        Summary    jsonSummary         `json:"summary"`
    }
    type jsonIteration struct {
        Name          string   `json:"name"`
        Start         string   `json:"start"`
        End           string   `json:"end"`
        Velocity      float64  `json:"velocity"`
        Committed     float64  `json:"committed"`
        CompletionPct float64  `json:"completion_pct"`
        ItemsDone     int      `json:"items_done"`
        ItemsTotal    int      `json:"items_total"`
        CarryOver     int      `json:"carry_over"`
        NotAssessed   int      `json:"not_assessed"`
        Trend         string   `json:"trend"`
    }
    ```
  - `WritePretty(w, result, width)` — formatted table output matching brainstorm mockup
    - Current section with velocity, committed, completion, carry-over, not-assessed, projected
    - History table using `format.NewTable()`
    - Summary line with avg velocity, avg completion, std dev
  - `WriteMarkdown(w, result)` — embedded template
- [ ] Create `internal/pipeline/velocity/templates/velocity.md.tmpl`
  - Current iteration section
  - History table
  - Summary stats
  - Use shared `format.TemplateFuncMap()` helpers
- [ ] Table-driven render tests: verify JSON structure, pretty output shape, markdown template execution

**Files:**
- `internal/pipeline/velocity/render.go` (new)
- `internal/pipeline/velocity/templates/velocity.md.tmpl` (new)
- `internal/pipeline/velocity/render_test.go` (new)

#### Phase 7: Command Wiring & Flags

Wire the Cobra command, register under `flow`, connect posting.

**Tasks:**

- [ ] Create `cmd/velocity.go`
  - `NewVelocityCmd() *cobra.Command`
  - Flags: `--current`, `--history`, `--iterations N`, `--since`, `--until`, `--verbose`, `--post`
  - RunE: extract deps, build client, build pipeline, run GatherData/ProcessData, handle posting via `postIfEnabled()`, Render
  - Examples in Long/Example showing common invocations
- [ ] Add `cmd.AddCommand(NewVelocityCmd())` to `cmd/flow.go`
- [ ] Update `flow` command Short description to include velocity
- [ ] Posting: discussion target for bulk summaries (same pattern as throughput)
  - Marker context: iteration name or date range
- [ ] Add `--verbose` flag for not-assessed item list (new flag, not `--debug`)

**Files:**
- `cmd/velocity.go` (new)
- `cmd/flow.go` (modify)

#### Phase 8: Config Validate --velocity

Live validation that runs effort matchers against real issues.

**Tasks:**

- [ ] Add `--velocity` flag to `newConfigValidateCmd()` in `cmd/config.go`
- [ ] When `--velocity` is set:
  1. Load config, verify velocity section exists
  2. Build GitHub client
  3. Fetch recent closed issues / merged PRs (last 90 days via search API, respecting scope)
  4. For attribute strategy:
     - Run each matcher against all items, count matches
     - Detect overlaps (items matching multiple matchers), show resolution (first-match)
     - Count unmatched items
     - Show distribution table
  5. For numeric strategy:
     - Fetch project items with number field
     - Count items with value, items without, items with 0, items with negative
     - Show value distribution
  6. For project-field iteration:
     - Verify iteration field exists on board
     - Report iteration count (active + completed)
     - Check if any items have iteration assigned
  7. Pretty output showing match counts, overlaps, gaps, distribution (as in brainstorm mockup)
  8. Respect `--verbose` for full unmatched item list
- [ ] Tests for validation logic (unit tests with fake data, no live API)

**Files:**
- `cmd/config.go` (modify)
- `cmd/config_validate_velocity.go` (new — keeps validation logic separate)
- `cmd/config_validate_velocity_test.go` (new)

#### Phase 9: Preflight Velocity Heuristics

Suggest velocity config when none exists.

**Tasks:**

- [ ] Add velocity suggestion section to `renderPreflightConfig()` in `cmd/preflight.go`
- [ ] Label heuristic detection:
  - Scan labels from preflight's existing label fetch
  - Match patterns: `size/*`, `effort:*`, `points-*`, `estimate-*` prefixes
  - Match t-shirt sizes: standalone XS, S, M, L, XL labels
  - No digit-only labels
  - Map to fibonacci defaults: XS/S=1, S/size-S=2, M/size-M=3, L/size-L=5, XL/size-XL=8
- [ ] Project field heuristic detection:
  - Scan discovered fields (already fetched by preflight)
  - Number fields: rank by name similarity to "points", "story points", "estimate", "effort", "size"
  - Iteration fields: any `ProjectV2IterationField` is a candidate
- [ ] Strategy suggestion logic:
  - Number field found → suggest `numeric` with that field name
  - Else sizing labels found → suggest `attribute` with mapped values
  - Else → suggest `count` with comment about adding effort later
  - Iteration field found → suggest `project-field` with that field name
  - Else → suggest `fixed` with 14-day default
- [ ] Emit YAML block with evidence comments (following existing preflight evidence pattern)
- [ ] Update `verifyConfig()` to round-trip validate generated velocity section
- [ ] Tests for heuristic detection functions (table-driven, no live API)

**Files:**
- `cmd/preflight.go` (modify)
- `cmd/preflight_test.go` (modify)

#### Phase 10: Smoke Tests & Documentation

End-to-end validation and docs updates.

**Tasks:**

- [ ] Add velocity smoke tests to `scripts/smoke-test.sh`:
  - `flow velocity` with count+fixed config
  - `flow velocity --history` / `--current`
  - `flow velocity --format json` schema validation
  - `flow velocity --iterations 3`
  - `config validate --velocity`
  - Preflight with velocity-suggesting repo signals
- [ ] Update README.md with velocity command documentation
- [ ] Update AGENTS.md if any new conventions emerge
- [ ] Verify `task quality` passes (golangci-lint)
- [ ] Verify `task test` passes

**Files:**
- `scripts/smoke-test.sh` (modify)
- `README.md` (modify)

## System-Wide Impact

### Interaction Graph

- `cmd/velocity.go` → `internal/pipeline/velocity/` → `internal/github/` (GraphQL) + `internal/scope/` (search queries)
- `internal/config/config.go` ← velocity config parsing ← `cmd/preflight.go` (suggestions) + `cmd/config.go` (validate/show)
- `internal/classify/` ← reused by effort attribute evaluator (existing matchers, no changes to classify package)
- `internal/posting/` ← reused for discussion posting (existing pattern, no changes)

### Error Propagation

- GraphQL errors (project not found, field not found, permission denied) → `model.AppError` with exit code 4 (not found) or 3 (auth)
- Config validation errors → `model.AppError` with exit code 2 (config)
- Empty iterations → not an error, render with zero values

### State Lifecycle Risks

- No persistent state. All computation is from API queries at invocation time.
- Project board item assignments are read-only. No mutations.

### API Surface Parity

- `flow velocity` joins `flow lead-time`, `flow cycle-time`, `flow throughput` — same flag patterns (`--format`, `--scope`, `--post`, `--since`, `--until`)
- `config validate --velocity` extends existing `config validate` command
- Preflight velocity section follows existing preflight output pattern

## Acceptance Criteria

### Functional Requirements

- [ ] `gh velocity flow velocity` shows current iteration + history table with velocity (number), committed, completion rate, items, trend
- [ ] `--current` shows only current iteration; `--history` shows only history
- [ ] `--iterations N` controls history depth (default 6)
- [ ] `--since`/`--until` filters iterations to overlapping window
- [ ] `--format json` outputs structured JSON; `--format markdown` uses template
- [ ] `--post` posts markdown to configured discussion
- [ ] `--verbose` includes not-assessed item numbers
- [ ] `--scope` AND'd with iteration boundaries
- [ ] Count effort strategy: every item = 1 unit
- [ ] Attribute effort strategy: scope-style matchers, first-match wins, unmatched = not assessed
- [ ] Numeric effort strategy: project Number field, null = not assessed, 0 = valid, negative = not assessed + warn
- [ ] Project-field iteration: reads iteration boundaries from ProjectV2 Iteration field
- [ ] Fixed iteration: calendar math from length + anchor
- [ ] Carry-over reported per iteration (informational count, not synthetic inflation)
- [ ] `config validate --velocity` shows matcher hit counts, overlaps, gaps, distribution
- [ ] Preflight suggests velocity config from labels, Number fields, Iteration fields
- [ ] `reason:completed` filtering for issue "done" definition
- [ ] Cross-repo items on project board filtered to target repo

### Non-Functional Requirements

- [ ] `task build` succeeds
- [ ] `task test` passes
- [ ] `task quality` passes (golangci-lint)
- [ ] Table-driven tests for all computation (effort evaluation, period math, velocity aggregation)
- [ ] No hardcoded GraphQL strings (variables only)
- [ ] errgroup.SetLimit(5) for concurrent API calls

### Quality Gates

- [ ] Config validation covers all invalid states (missing fields, bad strategy names, negative values)
- [ ] defaultConfigTemplate includes velocity section with comments
- [ ] Example configs updated
- [ ] Preflight round-trip validation passes for generated velocity config
- [ ] Smoke tests cover major strategy combinations

## Dependencies & Risks

- **GraphQL iteration field API**: Well-documented (see research doc) but not yet used in codebase. Risk: field structure edge cases.
- **`reason:completed` on issues**: Requires adding `stateReason` to search API responses. The `state_reason` field is available in GitHub REST API but may not be in the current search response parsing.
- **Project board permissions**: Iteration/Number field queries need `project` scope on token. Existing `GH_VELOCITY_TOKEN` pattern handles this.
- **Large boards**: Fetching all items for velocity computation could be slow on boards with thousands of items. Pagination already exists; may need to profile.

## Future Considerations

- **Issue-close comments**: Per-issue posting when an issue is closed (requires webhook/Action trigger). Deferred from v1.
- **Burnup visualization**: v1 computes burnup data (cumulative closedAt within iteration). Future: ASCII chart in pretty output, SVG in markdown.
- **Velocity forecasting**: "At current pace, Sprint 12 will complete X pts" — projection logic.
- **`config validate --cycle-time`, `--quality`**: Extend the validate pattern to other config sections.

## Sources & References

### Origin

- **Brainstorm document:** [docs/brainstorms/2026-03-12-velocity-command-brainstorm.md](../brainstorms/2026-03-12-velocity-command-brainstorm.md)
- **Research document:** [docs/brainstorms/2026-03-12-velocity-burndown-research.md](../brainstorms/2026-03-12-velocity-burndown-research.md)

### Internal References

- Command registration: `cmd/flow.go:1-18`
- Pipeline pattern: `internal/pipeline/pipeline.go:1-46`
- Throughput pipeline (template): `internal/pipeline/throughput/throughput.go`
- Config structs: `internal/config/config.go:33-81`
- Config validation: `internal/config/config.go:218-296`
- Known keys: `internal/config/config.go:174-184`
- Default config template: `cmd/config.go:110-170`
- Preflight config generation: `cmd/preflight.go:725-904`
- Classify matchers: `internal/classify/classify.go:1-175`
- Project items GraphQL: `internal/github/projectitems.go:1-175`
- Discover (iteration field type): `internal/github/discover.go:83-86`
- Project resolution: `internal/github/project.go:1-74`
- Model types: `internal/model/types.go:1-274`
- Posting: `internal/posting/poster.go:1-215`, `cmd/post.go:1-126`
- Format helpers: `internal/format/formatter.go:1-140`
- Scope queries: `internal/scope/scope.go:1-185`

### Institutional Learnings

- Pipeline-per-metric layout: `docs/solutions/architecture-refactors/pipeline-per-metric-and-preflight-first-config.md`
- Cycle time signal hierarchy: `docs/solutions/cycle-time-signal-hierarchy.md`
- Evidence-driven preflight: `docs/solutions/evidence-driven-preflight-config.md`
- Three-state metric status: `docs/solutions/three-state-metric-status-pattern.md`
- Config validation requirement (from MEMORY.md): every phase must verify preflight --write, config show, examples, and defaultConfigTemplate are current
