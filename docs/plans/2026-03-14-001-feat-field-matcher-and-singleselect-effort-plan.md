---
title: "feat: field: matcher for SingleSelect effort strategy"
type: feat
status: completed
date: 2026-03-14
origin: https://github.com/dvhthomas/gh-velocity/issues/56
---

# feat: `field:` matcher for SingleSelect effort strategy

## Overview

Add a `field:Name/Value` matcher to the classify package so that effort evaluation
can match against ProjectV2 SingleSelect field values (e.g., `field:Size/M`).
Extend the velocity pipeline to fetch and carry those field values, update preflight
to detect SingleSelect sizing fields, and add an upper-bound guard on board item count.

Related issue: #56

## API Cost Context

Real-world board sizes (surveyed 2026-03-14):

| Board | Items | Pages |
|---|---|---|
| dvhthomas/projects/1 | 24 | 1 |
| github/gh-skyline | 94 | 1 |
| grafana/Alloy proposals | 163 | 2 |
| microsoft/eBPF Triage | 397 | 4 |

At ~1-5 GraphQL points per page and 5,000 points/hour, even a 400-item board costs
<0.2% of the budget. Enterprise boards may be much larger though, so we need an
upper-bound guard that informs users when results are capped, with suggestions to
tighten scope, shorten time range, or switch to label-based attribute strategy.

**Caching**: Current in-process cache (10 min TTL) is sufficient. Do NOT add persistent
disk cache unless real-world usage shows API consumption issues.

## Design Decisions

1. **Matcher syntax**: `field:FieldName/Value` (slash separator, not equals, consistent
   with `label:size/M` aesthetic). Case-insensitive on both field name and value.
2. **`needsBoard` trigger**: When any effort matcher uses `field:` prefix, the pipeline
   must use board path (not search path), even with `strategy: attribute`.
3. **Upper bound**: Hard cap at 2000 items. When exceeded, return a structured warning
   with count and suggestions — do NOT silently truncate.
4. **GraphQL extension**: Add optional SingleSelect field fragments to
   `buildVelocityItemsQuery`. Pass field names extracted from `field:` matchers.
5. **Preflight priority**: numeric → SingleSelect → labels → count.
6. **Integration test**: Use Microsoft eBPF triage board (397 items) as a scale test.

## Implementation Phases

### Phase 1: `field:` Matcher in Classify Package

Add `Fields map[string]string` to `classify.Input` and a new `FieldMatcher`.

**Tasks:**

- [ ] Add `Fields map[string]string` to `classify.Input` in `internal/classify/classify.go`
- [ ] Add `FieldMatcher{Field, Value string}` implementing `Matcher` — case-insensitive
      match on `input.Fields[field] == value`
- [ ] Add `"field"` case to `ParseMatcher` — parse `field:Name/Value`, require exactly
      one `/` separator after the `field:` prefix
- [ ] Update error message in `ParseMatcher` to include `field:` in the format hint
- [ ] Table-driven tests: field match, case insensitivity, missing field → no match,
      empty value → no match, parse errors (no slash, empty name, empty value)

**Files:**
- `internal/classify/classify.go` (modify)
- `internal/classify/classify_test.go` (modify)

### Phase 2: VelocityItem Field Storage & GraphQL Extension

Carry arbitrary SingleSelect field values through the pipeline.

**Tasks:**

- [ ] Add `Fields map[string]string` to `model.VelocityItem` in `internal/model/types.go`
- [ ] Add `velocityFieldSingleSelect` struct to `internal/github/iteration.go`
- [ ] Extend `buildVelocityItemsQuery` to accept `singleSelectFields []string` parameter,
      generating a `fieldValueByName` + `ProjectV2ItemFieldSingleSelectValue` fragment
      per field name (use unique aliases like `ss0`, `ss1`, etc.)
- [ ] Extend `velocityItemNode` to carry dynamic SingleSelect fields (use `json.RawMessage`
      or named fields based on alias convention)
- [ ] Update `listProjectItemsWithFieldsUncached` to parse SingleSelect values into
      `item.Fields` map
- [ ] Update `ListProjectItemsWithFields` signature: add `singleSelectFields []string`
      parameter. Update cache key to include these fields.
- [ ] Update the `Client` interface in `internal/pipeline/velocity/velocity.go` to match
- [ ] Table-driven tests with mock GraphQL responses including SingleSelect values

**Files:**
- `internal/model/types.go` (modify)
- `internal/github/iteration.go` (modify)
- `internal/github/iteration_test.go` (modify)
- `internal/pipeline/velocity/velocity.go` (modify)

### Phase 3: Effort Evaluator Wiring

Connect `field:` matchers through the pipeline.

**Tasks:**

- [ ] In `AttributeEvaluator.Evaluate`, populate `classify.Input.Fields` from
      `item.Fields` (in `internal/pipeline/velocity/effort.go`)
- [ ] Add `HasFieldMatchers(cfg config.EffortConfig) bool` helper that scans effort
      attribute queries for `field:` prefix
- [ ] In `Pipeline.GatherData`, update `needsBoard` to also be true when
      `HasFieldMatchers(p.Config.Effort)` is true
- [ ] In `Pipeline.gatherFromBoard`, extract SingleSelect field names from `field:`
      matchers and pass to `ListProjectItemsWithFields`
- [ ] Table-driven tests: attribute evaluator with field matchers, mixed label+field
      matchers, field matcher with no board data → not assessed

**Files:**
- `internal/pipeline/velocity/effort.go` (modify)
- `internal/pipeline/velocity/effort_test.go` (modify)
- `internal/pipeline/velocity/velocity.go` (modify)

### Phase 4: Upper Bound Guard

Warn when board item count exceeds a safe limit.

**Tasks:**

- [ ] Add `const MaxBoardItems = 2000` to `internal/pipeline/velocity/velocity.go`
- [ ] After fetching items in `gatherFromBoard`, if `len(items) > MaxBoardItems`:
      - Truncate to MaxBoardItems
      - Add a structured warning to the result (new `Warnings []string` field on
        `VelocityResult`, or reuse `Insights`)
      - Warning text: "Board has N items (limit: 2000). Results may be incomplete.
        Consider: tighter --scope, shorter time range, or switch to label-based
        attribute strategy."
- [ ] Render warning in pretty/JSON/markdown output
- [ ] Test: verify warning is emitted at boundary

**Files:**
- `internal/pipeline/velocity/velocity.go` (modify)
- `internal/model/types.go` (modify — if adding Warnings)
- `internal/pipeline/velocity/render.go` (modify)

### Phase 5: Preflight SingleSelect Detection

Detect T-shirt sizing SingleSelect fields and suggest `field:` matchers.

**Tasks:**

- [ ] In `detectVelocityHeuristics`, after Number field check, scan `ProjectV2SingleSelectField`
      fields for T-shirt sizing patterns in their options
- [ ] T-shirt detection: options containing a majority of {XS, S, M, L, XL} (case-insensitive),
      or containing numeric-looking values (1, 2, 3, 5, 8, 13)
- [ ] Add `SingleSelectField string` and `SingleSelectMatchers []SizingLabelMatch` to
      `VelocityHeuristic` struct
- [ ] Update strategy priority: numeric → SingleSelect → labels → count
- [ ] Generate `attribute` strategy config with `field:FieldName/Value` matchers and
      fibonacci-ish effort values
- [ ] Update `renderPreflightConfig()` to emit `field:` matchers in velocity section
- [ ] Update `defaultConfigTemplate` with commented `field:` example
- [ ] Table-driven tests for SingleSelect detection heuristics

**Files:**
- `cmd/preflight.go` (modify)
- `cmd/preflight_test.go` (modify)
- `cmd/config.go` (modify — defaultConfigTemplate)

### Phase 6: Config Validation for `field:` Syntax

Ensure config validation accepts `field:` matchers and validates their structure.

**Tasks:**

- [ ] Config validation in `internal/config/config.go` already calls `classify.ParseMatcher`
      for effort attribute queries — `field:` will be accepted once Phase 1 is done
- [ ] Add validation: if any effort matcher uses `field:` prefix, `project.url` must be set
      (same requirement as numeric strategy)
- [ ] Update `config validate --velocity` to report field matcher results when board data available
- [ ] Table-driven config validation tests for `field:` matchers

**Files:**
- `internal/config/config.go` (modify)
- `internal/config/config_test.go` (modify)
- `cmd/config_validate_velocity.go` (modify)

### Phase 7: Integration Test with Microsoft eBPF Board

Add a real-world scale test using a large public project board.

**Tasks:**

- [ ] Add integration test (build tag `integration`) that fetches Microsoft eBPF board
      (org: microsoft, project: 2098, 397 items)
- [ ] Verify: pagination works correctly, all items fetched, no rate limit errors
- [ ] Verify: SingleSelect "Status" field values are populated on items
- [ ] Add to `Taskfile.yaml` as `task test:integration` target
- [ ] Add smoke test in `scripts/smoke-test.sh` for `field:` config parsing

**Files:**
- `internal/github/iteration_integration_test.go` (new)
- `Taskfile.yaml` (modify)
- `scripts/smoke-test.sh` (modify)

## Acceptance Criteria

- [ ] `field:Name/Value` matcher implemented and tested in classify package
- [ ] VelocityItem carries arbitrary SingleSelect field values
- [ ] GraphQL query dynamically includes requested SingleSelect fields
- [ ] Effort evaluation works with `field:` matchers end-to-end
- [ ] `needsBoard` is true when any effort matcher uses `field:` prefix
- [ ] Preflight detects SingleSelect fields with T-shirt sizing patterns
- [ ] Priority order: numeric → SingleSelect → labels → count
- [ ] Upper bound guard warns at 2000 items with actionable suggestions
- [ ] Config validation requires `project.url` when `field:` matchers are used
- [ ] Microsoft eBPF board integration test passes
- [ ] `task build`, `task test`, `task quality` all pass

## Files Summary

| File | Action |
|---|---|
| `internal/classify/classify.go` | Add FieldMatcher, Fields on Input |
| `internal/classify/classify_test.go` | Add field matcher tests |
| `internal/model/types.go` | Add Fields to VelocityItem, Warnings to VelocityResult |
| `internal/github/iteration.go` | Extend query builder, parse SingleSelect values |
| `internal/github/iteration_test.go` | Add SingleSelect mock tests |
| `internal/github/iteration_integration_test.go` | New: Microsoft eBPF scale test |
| `internal/pipeline/velocity/effort.go` | Wire Fields into Input, add HasFieldMatchers |
| `internal/pipeline/velocity/effort_test.go` | Add field matcher evaluator tests |
| `internal/pipeline/velocity/velocity.go` | Update needsBoard, add upper bound guard |
| `internal/pipeline/velocity/render.go` | Render warnings |
| `cmd/preflight.go` | Add SingleSelect detection |
| `cmd/preflight_test.go` | Add detection tests |
| `cmd/config.go` | Update defaultConfigTemplate |
| `cmd/config_validate_velocity.go` | Report field matcher validation |
| `internal/config/config.go` | Validate field: requires project.url |
| `internal/config/config_test.go` | Add validation tests |
| `Taskfile.yaml` | Add test:integration target |
| `scripts/smoke-test.sh` | Add field: smoke tests |
