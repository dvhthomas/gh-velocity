---
title: "Open Issues Rollup — Stability-First Execution"
type: fix
status: active
date: 2026-03-17
origin: docs/brainstorms/2026-03-17-open-issues-rollup-requirements.md
---

# Open Issues Rollup — Stability-First Execution

## Overview

Address all 8 open issues in dependency order: foundation cleanup → bug fixes → config enhancement → planned features → preflight intelligence. Each phase produces mergeable PRs with all tests passing.

## Problem Statement

gh-velocity has 8 open issues spanning bugs (#108, #97), features (#85, #86, #105, #109), and chores (#78, #84). The module path is wrong, report output has a rendering bug, scope conflicts are silent, and several features are queued. Without deliberate sequencing, feature work risks building on a shaky foundation and creating merge conflicts (see origin: `docs/brainstorms/2026-03-17-open-issues-rollup-requirements.md`).

## Proposed Solution

Five phases, strictly ordered, with Phase 4 parallelizable. Each phase gates on `task quality` passing.

---

## Implementation Phases

### Phase 1: Module Path Rename (#78)

**Goal:** Fix `github.com/bitsbyme/gh-velocity` → `github.com/dvhthomas/gh-velocity` across the entire codebase.

**Scope:** 110 Go files (252 occurrences), `go.mod`, `.golangci.yml`, 1 plan doc reference.

**Tasks:**

- [ ] Replace `github.com/bitsbyme/gh-velocity` with `github.com/dvhthomas/gh-velocity` in:
  - `go.mod` (line 1)
  - All `.go` files (imports)
  - `.golangci.yml`
- [ ] Run `go mod tidy` to regenerate `go.sum`
- [ ] Run `task quality` — all tests, lint, staticcheck must pass
- [ ] Verify binary builds: `task build && ./gh-velocity --version`

**Files:** `go.mod`, `.golangci.yml`, all `**/*.go` files (mechanical replace)

**Risks:**
- Build tags or `go:generate` directives referencing the old path — mitigated by grepping for `bitsbyme` after the replace
- CI Go module cache may reference old path — first CI run may be slow but not broken

**Acceptance criteria:**
- [ ] Zero occurrences of `bitsbyme` in codebase (verified by `grep -r bitsbyme`)
- [ ] `task quality` passes
- [ ] Binary builds and runs

**PR:** Single PR, closes #78.

---

### Phase 2: Bug Fixes + Verification (#108, #97, #84)

#### Phase 2a: Fix Throughput Detail Section (#108)

**Goal:** Throughput `<details>` block renders in markdown output when throughput data exists.

**Root cause investigation:** The spec-flow analysis identified three possible causes:
1. `throughputOK` is false (GatherData error swallowed)
2. `throughputPipeline.Result` has data but detail section is skipped
3. **Most likely:** `qualDetail.Categories` is nil because quality categories are not configured or `leadOK` is false, causing the category breakdown to be empty — the throughput detail section may render but with zero content, or the gate skips it

**Investigation approach:**
- [ ] Read `cmd/report.go:354-369` to trace exact data flow from `throughputPipeline.Result` to detail rendering
- [ ] Check if `qualDetail.Categories` is required for throughput detail to render, or only for the category breakdown sub-table
- [ ] Reproduce with showcase config for kubernetes/kubernetes
- [ ] Write a failing test that exercises the throughput detail gate with data present

**Design decision:** Throughput detail should render whenever `throughputPipeline.Result.IssuesClosed + PRsMerged > 0`, regardless of whether quality categories are configured. The category breakdown sub-table is optional within the detail section.

**Files:**
- `cmd/report.go` — fix gate condition and/or decouple from `qualDetail`
- `cmd/report_test.go` — add test for throughput detail rendering
- `internal/pipeline/throughput/render.go` — if rendering logic needs adjustment

**Acceptance criteria:**
- [ ] Throughput detail section appears in markdown output when throughput data > 0
- [ ] Category breakdown renders when quality categories are configured
- [ ] Category breakdown is omitted (not errored) when quality categories are absent
- [ ] `task quality` passes

**PR:** Single PR, closes #108.

#### Phase 2b: Warn on Scope/Repo Conflict (#97)

**Goal:** Emit a warning when config scope contains `repo:X/Y` and `-R A/B` targets a different repo.

**Conflict scenarios (from spec-flow analysis):**

| Config scope | `-R` flag | Action |
|---|---|---|
| `repo:cli/cli` | `-R cli/cli` | No conflict — same repo, no warning |
| `repo:cli/cli` | `-R other/repo` | **Conflict** — warn user |
| `org:myorg` | `-R myorg/repo` | Compatible — no warning |
| `repo:cli/cli label:bug` | `-R other/repo` | **Conflict** on `repo:` — warn; other qualifiers are the user's problem |

**Tasks:**

- [ ] Add `DetectRepoConflict(scopeQuery, repoFlag string) (configRepo string, conflict bool)` to `internal/scope/scope.go`
  - Parse `repo:` qualifier from scope query string using regex `repo:([^\s]+)`
  - Compare against `-R` resolved value (normalized `owner/repo`)
  - Return false if no `repo:` qualifier found, or if repos match
- [ ] Add table-driven tests in `internal/scope/scope_test.go` covering all 4 scenarios above
- [ ] Wire detection in `cmd/root.go` `PersistentPreRunE`, after line 239 where `resolvedScope` is computed
- [ ] Emit warning using `log.Warn()` (not `WarnUnlessJSON` — anticipating #85 which removes that pattern)
  - Message: `config scope includes "repo:X/Y" but -R targets A/B — config scope overrides -R flag`

**Files:**
- `internal/scope/scope.go` — add `DetectRepoConflict`
- `internal/scope/scope_test.go` — table-driven tests
- `cmd/root.go` — wire after `MergeScope` call (~line 239)

**Acceptance criteria:**
- [ ] Warning emitted when `repo:` in scope differs from `-R`
- [ ] No warning when repos match or no `repo:` in scope
- [ ] `task quality` passes

**PR:** Single PR, closes #97.

#### Phase 2c: Verify First-Run Experience (#84)

**Goal:** Complete 3 manual verification checks from plan `2026-03-12-004`.

**Checks:**
- [ ] `preflight --write` on a repo without project board → `report --since 30d` produces cycle time data (not N/A)
- [ ] `config show` displays correct fields (no stale references)
- [ ] No regressions in example configs (`docs/examples/`)

**Scope boundary:** If verification reveals issues, fix only config template bugs or small rendering errors. Strategy changes or new features become separate issues.

**PR:** Single PR if fixes needed, otherwise just close #84.

---

### Phase 3: Configurable Defect Rate Threshold (#105)

**Goal:** Add `quality.defect_rate_threshold` to config, wired through to insight generation.

**Design decisions:**
- Config value is a float in range `(0.0, 1.0)` — matches internal representation (`DefectRate: 0.40`)
- `DefectRateSuspicious` (0.60) remains hardcoded — it signals a data quality issue, not a team baseline
- Validation: reject values ≤ 0, ≥ 1.0, and ≥ `DefectRateSuspicious` (0.60) with a clear error message
- Default: 0.20 (current behavior when not specified)

**Two-threshold interaction (from spec-flow analysis):**
The `GenerateQualityInsights` switch checks `DefectRateSuspicious` first (line 267). If the user configures a threshold above 0.60, the "review matchers" insight fires instead of their configured insight. Solution: validate that configured threshold < `DefectRateSuspicious` and reject with error: `defect_rate_threshold must be less than 0.60 (the suspicious threshold that indicates a data quality issue)`.

**Config change checklist** (per project feedback requirement):

- [ ] Add `DefectRateThreshold *float64` to `QualityConfig` in `internal/config/config.go`
- [ ] Set default in `defaults()` — 0.20
- [ ] Add validation in `validate()` — range check, < suspicious threshold
- [ ] Update `GenerateQualityInsights()` in `internal/metrics/report_insights.go` to accept threshold parameter
- [ ] Update insight message: `"X% of closed issues are bugs (above configured Y% threshold)"`
- [ ] Pass config value from `cmd/report.go` to insight generator
- [ ] Update `defaultConfigTemplate` in `cmd/config.go` with commented example
- [ ] Update `config show` output in `cmd/config.go`
- [ ] Update preflight `--write` output in `cmd/preflight.go` — include threshold if defect matchers detected
- [ ] Verify `docs/examples/` configs remain valid

**Tests:**
- [ ] `internal/config/config_test.go` — default value, valid range, rejection above suspicious
- [ ] `internal/metrics/report_insights_test.go` — custom threshold triggers at configured level, suspicious threshold still fires above 0.60

**Files:**
- `internal/config/config.go` — struct, defaults, validation
- `internal/config/config_test.go`
- `internal/metrics/report_insights.go` — parameterize threshold
- `internal/metrics/report_insights_test.go`
- `cmd/report.go` — pass threshold
- `cmd/config.go` — template, show
- `cmd/preflight.go` — generated config

**Acceptance criteria:**
- [ ] `defect_rate_threshold: 0.15` in config → insight fires at 15%
- [ ] Omitting the key → default 0.20 behavior unchanged
- [ ] Setting threshold ≥ 0.60 → config validation error
- [ ] `config show`, `preflight --write`, and `defaultConfigTemplate` reflect the new field
- [ ] `task quality` passes

**PR:** Single PR, closes #105.

---

### Phase 4: Planned Features (#85, #86) — Parallelizable with Caveats

Both features have existing plans and touch mostly independent subsystems. However, both modify `cmd/preflight.go` (see origin: `docs/brainstorms/2026-03-17-open-issues-rollup-requirements.md`, Dependencies).

**Merge order:** #86 merges first, then #85. Rationale:
- #86 adds new code to preflight (SingleSelect detection) — additive change
- #85 does a sweep of existing code (flag rename, template updates) — change that's easier to do second since it can incorporate #86's additions

#### Phase 4a: Field Matcher for SingleSelect Effort (#86)

**Plan:** `docs/plans/2026-03-14-001-feat-field-matcher-and-singleselect-effort-plan.md`

**Status:** Phase 1 complete (FieldMatcher and `field:` parsing implemented in `internal/classify/classify.go`). Phases 2-7 remain.

**Remaining work:**
- Phase 2: VelocityItem field storage + GraphQL extension (`internal/github/iteration.go`)
- Phase 3: Effort evaluator wiring (`internal/pipeline/velocity/effort.go`, `velocity.go`)
- Phase 4: Upper bound guard (2000 board items)
- Phase 5: Preflight SingleSelect detection (`cmd/preflight.go`)
- Phase 6: Config validation — `field:` requires `project.url`
- Phase 7: Integration test

**Acceptance criteria:** Per existing plan.

**PR(s):** 1-2 PRs per the plan's internal phasing. Closes #86.

#### Phase 4b: Separate --results, --output, --write-to Flags (#85)

**Plan:** `docs/plans/2026-03-16-001-feat-results-output-writeto-flag-separation-plan.md`

**Key risk (from spec-flow analysis):** Phase 1 of #85 removes `SuppressStderr` and replaces 15 `WarnUnlessJSON` call sites. This must not create a regression where JSON stdout is corrupted by stderr. Solution: Phase 1 must also implement conditional stderr suppression in the render orchestration layer (suppress when `--results json` is the sole stdout format). Do not remove `SuppressStderr` without its replacement.

**Internal phasing:**
- Phase 1+2: Rename `--format` → `--results` with deprecation bridge + add `--write-to` + replace `SuppressStderr` with render-layer conditional suppression → **1 PR**
- Phase 3: `--output json` for agent diagnostics → **DEFERRED** (no consumer yet)
- Phase 4: Migration sweep (smoke tests, docs, showcase scripts) → **1 PR**

**Note on #97 interaction:** Phase 2b (#97) adds a new warning using `log.Warn()`. When #85 lands, this warning will naturally flow through the new render orchestration layer without needing migration.

**Acceptance criteria:** Per existing plan.

**PR(s):** 2 PRs (core + migration sweep). Closes #85.

---

### Phase 5: Preflight Intelligence (#109)

**Goal:** Improve lifecycle label detection with fuzzy matching, pipeline discovery, and strategy fallback.

**Tasks:**

#### 5.1 Label Normalization

- [ ] Add `normalizeLabel(s string) string` to `cmd/preflight.go` (or extract to `internal/lifecycle/` if it grows):
  - Lowercase
  - Trim trailing punctuation (`!?:.`)
  - Normalize separators (`-`, `_`, ` `) to single space
  - Trim whitespace
- [ ] Table-driven tests for normalization edge cases

#### 5.2 Expanded Lifecycle Stage Mapping

- [ ] Expand `statusPatterns` map to cover:

| Keywords | Stage | Purpose |
|---|---|---|
| `progress`, `working`, `active`, `wip`, `doing` | in-progress | Cycle time start |
| `review`, `reviewing` | in-review | Active work |
| `confirmed`, `accepted`, `triaged`, `ready`, `approved` | ready | Triage-to-start |
| `design`, `designing`, `planning` | in-design | Broader cycle start |
| `blocked`, `waiting`, `on hold`, `stalled` | blocked | Exclude from cycle time |
| `new`, `triage`, `needs-triage`, `intake` | backlog | Backlog stage |
| `shipped`, `released`, `done`, `resolved` | done | End signal |
| `preview`, `beta`, `alpha`, `canary` | preview | Pre-release (in-progress) |

- [ ] Use `normalizeLabel()` before matching against keyword patterns
- [ ] Keyword matching uses `strings.Contains` on normalized label, not exact match

#### 5.3 Pipeline Detection

- [ ] When multiple lifecycle-stage labels are found, detect ordered progression
- [ ] Suggest all intermediate stages as `in-progress` in generated config
- [ ] Include match evidence comments showing which labels mapped to which stages

#### 5.4 Strategy Fallback

- [ ] When `strategy: issue` is selected but no lifecycle labels detected:
  - Check if recent merged PRs exist that reference issues
  - If yes, suggest `strategy: pr` with explanatory comment
  - Message: `No lifecycle labels found. PR strategy recommended — cycle time measures from first PR to issue close.`

#### 5.5 Testing

- [ ] Unit tests use **synthetic label fixtures**, not external repo snapshots (labels change over time)
- [ ] Test cases for: hashicorp/terraform-style labels (`in progress` with space), github/roadmap-style pipeline (`in design` → `preview` → `shipped`), grafana/k6-style no-label fallback to PR strategy
- [ ] Test normalization: `review me!` → `review me` → matches `review` keyword
- [ ] Test false positive prevention: `do-not-merge/needs-review` should NOT match `review` stage

**Gotcha from learnings:** Labels starting with `event/`, `do-not-merge`, `needs-` are already filtered as noise in current preflight (see `docs/solutions/evidence-driven-preflight-config.md`). Fuzzy matching must respect these filters to avoid false positives.

**Files:**
- `cmd/preflight.go` — normalization, expanded patterns, pipeline detection, strategy fallback
- `cmd/preflight_test.go` — comprehensive table-driven tests

**Acceptance criteria:**
- [ ] `preflight` on a repo with `in progress` (space) label detects it as in-progress
- [ ] `preflight` on a repo with `in design` → `preview` → `shipped` labels suggests lifecycle pipeline
- [ ] `preflight` on a repo with no lifecycle labels but active PRs suggests `strategy: pr`
- [ ] No false positives from `do-not-merge/*` or `needs-*` labels
- [ ] `task quality` passes

**PR:** Single PR, closes #109.

---

## Cross-Phase Dependencies

```
Phase 1 (#78 module rename)
    │
    ▼
Phase 2 (#108 bug, #97 warning, #84 verify)
    │
    ▼
Phase 3 (#105 config threshold)
    │
    ├──────────────────┐
    ▼                  ▼
Phase 4a (#86)    Phase 4b (#85)
    │                  │
    │   merge #86 first│
    ├──────────────────┘
    ▼
Phase 5 (#109 preflight intelligence)
```

**Hidden dependencies surfaced by analysis:**
- Phase 2a (#108) changes report rendering logic that Phase 4b (#85) will restructure → fix the bug first, let #85 adapt
- Phase 2b (#97) adds a `log.Warn()` call → naturally compatible with #85's removal of `WarnUnlessJSON`
- Phase 4a (#86) and Phase 4b (#85) both touch `cmd/preflight.go` → merge #86 first (additive), then #85 (sweep)
- Phase 4a (#86) adds field access infrastructure that Phase 5 (#109) could reuse for project board field inspection

## Quality Gates

Each phase must pass before the next begins:
- `task quality` (lint + staticcheck + integration tests)
- `grep -r bitsbyme` returns zero matches (Phase 1 only)
- Smoke tests pass (76+ tests in `scripts/smoke-test.sh`)
- No `SuppressStderr` regressions (Phase 4b specifically)

## Success Metrics

- All 8 issues closed with merged PRs
- Showcase workflow produces correct output (throughput detail present, lifecycle detection improved)
- No regressions across phases
- Phase 4 features match their existing plan acceptance criteria

## Sources & References

### Origin

- **Origin document:** [docs/brainstorms/2026-03-17-open-issues-rollup-requirements.md](docs/brainstorms/2026-03-17-open-issues-rollup-requirements.md) — Key decisions: module rename first, stability before features, Phase 4 parallelizable, Phase 5 last

### Internal References

- Phase 4a plan: `docs/plans/2026-03-14-001-feat-field-matcher-and-singleselect-effort-plan.md`
- Phase 4b plan: `docs/plans/2026-03-16-001-feat-results-output-writeto-flag-separation-plan.md`
- First-run experience plan: `docs/plans/2026-03-12-004-fix-first-run-experience-and-strategy-completeness-plan.md`
- Scope implementation: `internal/scope/scope.go:170-184`
- Throughput gate: `cmd/report.go:354-369`
- Defect rate constants: `internal/metrics/report_insights.go:16-17`
- Label scanning: `cmd/preflight.go:686-689` (statusPatterns), `cmd/preflight.go:757` (matchesWord)

### Institutional Learnings Applied

- Config change checklist: `docs/solutions/architecture-refactors/pipeline-per-metric-and-preflight-first-config.md`
- Evidence-driven preflight: `docs/solutions/evidence-driven-preflight-config.md`
- Render-layer linking: `docs/solutions/architecture-patterns/render-layer-linking-and-insight-quality.md`
- Multi-category classification: `docs/solutions/architecture-patterns/multi-category-classification.md`
- Label lifecycle: `docs/solutions/architecture-patterns/label-based-lifecycle-for-cycle-time.md`
- Command output shape: `docs/solutions/architecture-patterns/command-output-shape.md`

### Issues

- #78, #84, #85, #86, #97, #105, #108, #109
