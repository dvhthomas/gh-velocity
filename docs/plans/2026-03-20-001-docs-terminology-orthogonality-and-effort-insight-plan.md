---
title: "docs: Terminology orthogonality and effort board-presence insight"
type: feat
status: completed
date: 2026-03-20
origin: docs/brainstorms/2026-03-20-terminology-and-orthogonality-requirements.md
---

# Terminology Orthogonality and Effort Board-Presence Insight

## Overview

gh-velocity has three independent measurement axes — **Scope/Lifecycle**, **Iteration**, and **Effort** — but the docs don't name them as orthogonal, and some pages re-explain concepts instead of linking to a single definition. Additionally, when effort strategy requires a project board (`numeric` or `field:` matchers) and some in-scope items aren't on the board, those items silently vanish from velocity — the user gets no signal about which items are missing.

This plan addresses both: define the terms once, link everywhere (making docs shorter), and surface a new insight for board-absent items.

## Problem Statement

1. **Terminology drift**: "iteration" vs "sprint" vs "cycle", "effort" vs "estimate" vs "points" appear inconsistently across docs and godoc strings.
2. **Re-explanation**: `velocity-setup.md` and `labels-vs-board.md` both explain what iteration and effort mean instead of linking to one definition.
3. **Silent data loss**: When `gatherFromBoard` fetches items, only board items are returned. In-scope issues not on the board are invisible — no insight, no item numbers, nothing.
4. **CI-only framing**: Some docs imply gh-velocity is primarily a CI tool. It's a general-purpose CLI that happens to work well in CI/Actions. The terminology page should establish this framing.

## Proposed Solution

### Phase 1: Terminology Foundation (R1)

Create `site/content/concepts/terminology.md` with weight 1 (appears first in Concepts nav). Define four canonical terms:

| Term | Definition |
|------|-----------|
| **Scope** | Which items to measure. Configured via `scope.query` and `--scope` flag. Determines the universe of issues/PRs that all metrics operate on. |
| **Lifecycle** | Where an item is in its workflow journey (backlog → in-progress → in-review → done). Determined by labels — the sole lifecycle signal. Used for cycle time measurement and stage grouping. |
| **Iteration** | A bounded time period used to bucket velocity results. Either read from a GitHub Projects Iteration field (`project-field`) or computed from calendar math (`fixed`). Answers "what sprint/phase/period are we measuring?" |
| **Effort** | How much work an item represents. Either every item counts as 1 (`count`), labels map to numeric values (`attribute`), or a project board Number field provides the value (`numeric`). Answers "how do we weight work output?" |

Include a one-paragraph framing: these four axes are independent. Scope/lifecycle filter *what* to measure. Iteration/effort control *how* velocity is computed. A project board is an optional data source for iteration and effort — it is not a lifecycle mechanism.

Also establish: gh-velocity is a CLI tool for measuring development velocity from GitHub data. It works standalone, in CI, in GitHub Actions, or any automation platform.

Link to: `labels-vs-board.md` (lifecycle detail), `velocity-setup.md` (iteration/effort configuration), `cycle-time-setup.md` (lifecycle in practice).

**Files**: `site/content/concepts/terminology.md` (new), `site/content/concepts/_index.md` (no change needed — `{{< children >}}` auto-discovers)

### Phase 2: Doc Trimming (R2, R3)

**`site/content/guides/velocity-setup.md`** (R2):
- Add a sentence after the H1 intro linking to the terminology page: "For definitions of iteration, effort, and how they relate to scope and lifecycle, see [Key Concepts](relref to terminology)."
- The existing strategy tables are good — they explain *when to choose* each strategy, not *what the terms mean*. Keep them.
- No major rewrite needed. The guide is already well-structured.

**`site/content/concepts/labels-vs-board.md`** (R3):
- In "What project boards do" (lines 29-32), replace the inline explanation of iteration tracking and effort fields with links to the terminology page definitions.
- Change from: "1. **Iteration tracking** -- `velocity.iteration.strategy: project-field` reads an Iteration field from the board to group issues into sprints."
- Change to: "1. **[Iteration](relref to terminology#iteration) tracking** -- reads the board's Iteration field to define time periods."
- Similar for effort. Net effect: shorter text, single source of truth.

**Files**: `site/content/guides/velocity-setup.md`, `site/content/concepts/labels-vs-board.md`

### Phase 3: Code Comments Audit (R4)

Audit godoc strings in these files for consistent use of canonical terms:

| File | What to check |
|------|--------------|
| `internal/pipeline/velocity/effort.go` | "effort" not "estimate" or "points" in godoc |
| `internal/pipeline/velocity/period.go` | "iteration" not "sprint" in godoc; note: `PeriodStrategy` interface name stays (no identifier renames), but godoc should say "resolves iteration boundaries" |
| `internal/pipeline/velocity/velocity.go` | Package doc and `generateInsights` terminology |
| `internal/pipeline/velocity/render.go` | `notAssessedHint` text uses user-facing terms correctly |
| `internal/config/config.go` | `ScopeConfig`, `LifecycleConfig`, `VelocityConfig` godoc expanded to match terminology page |
| `internal/model/types.go` | `EffortDetail`, `IterationVelocity` struct godoc |

**Constraint**: No identifier renames. Only comments and godoc strings.

**Ordering**: Phase 1 must complete first so R4 has canonical terms to align against.

### Phase 4: Effort Board-Presence Insight (R5)

**The architectural problem**: `gatherFromBoard` (velocity.go line ~119) fetches items FROM the project board via `ListProjectItemsWithFields`. Items not on the board never enter `p.items`. To detect missing items, the pipeline must also know which items *should* be there based on scope.

**Approach**: When `needsBoard` is true (effort uses `numeric` or `field:` matchers), add a parallel search query for in-scope closed items. Compare search results against board items by issue/PR number. The delta is the "not on board" set.

**Implementation sketch**:

1. In `GatherData()`, when `needsBoard` is true:
   - Fetch board items (existing behavior)
   - Also run a search query for in-scope closed/merged items (same query `gatherFromSearch` would use)
   - Compute `offBoardNumbers = searchNumbers - boardNumbers`
   - Store `offBoardNumbers` on the `Pipeline` struct

2. In `generateInsights()`, if `len(p.offBoardNumbers) > 0`:
   - Generate insight: `"N issues are not on the project board and have no effort assigned: #5, #12, #18"`
   - Type: `"field_effort_off_board"`
   - Level: `"info"` (it's an insight, not a warning)

3. **Suppression rules** (explicit):
   - `count` strategy: never generate (board not used for effort)
   - `attribute` with label-only matchers: never generate (board not used for effort)
   - `attribute` with `field:` matchers: generate for items not on board AND not assessed by label matchers
   - `numeric`: generate for all items not on board

4. **Output parity**:
   - Pretty: appears in insights section as `-> N issues are not on the project board...`
   - JSON: appears in `insights[]` array. Also add `off_board_items` array (issue numbers) to the velocity result JSON for structured consumption.
   - Markdown: appears in insights bullet list

**API cost**: One additional search API call per velocity run when board is in use. This is acceptable — it's the same call `gatherFromSearch` makes, and it runs in parallel with the board fetch.

**Files**: `internal/pipeline/velocity/velocity.go`, `internal/pipeline/velocity/render.go`, templates

### Phase 5: Tests (R6)

**Pipeline-level tests** (not evaluator-level — evaluators already handle nil correctly):

| Test | What it verifies |
|------|-----------------|
| `TestProcessData_FieldEffortOffBoard` | Items in search but not on board produce the `field_effort_off_board` insight with correct issue numbers |
| `TestProcessData_FieldEffortAllOnBoard` | When all items are on the board, no `field_effort_off_board` insight is generated |
| `TestProcessData_CountStrategyNoOffBoardInsight` | `count` strategy never generates the insight even if items are missing from board |
| `TestProcessData_AttributeLabelOnlyNoOffBoardInsight` | `attribute` with label-only matchers never generates the insight |
| `TestProcessData_MixedMatchersPartialBoard` | `attribute` with label+field matchers: item not on board but assessed via label → no insight for that item; item not on board and not assessed → insight includes it |

**Files**: `internal/pipeline/velocity/velocity_test.go`

## Acceptance Criteria

- [ ] `site/content/concepts/terminology.md` exists with definitions of Scope, Lifecycle, Iteration, Effort
- [ ] Terminology page frames gh-velocity as a general-purpose CLI (not CI-only)
- [ ] `velocity-setup.md` links to terminology page, does not re-define terms
- [ ] `labels-vs-board.md` "What project boards do" links to terminology instead of explaining inline
- [ ] Godoc strings in listed files use canonical terms consistently
- [ ] `PeriodStrategy` godoc says "iteration boundaries" (identifier unchanged)
- [ ] Running velocity with `numeric` effort on mixed scope produces insight with issue numbers
- [ ] Insight suppressed for `count` and label-only `attribute` strategies
- [ ] JSON output includes `off_board_items` structured array
- [ ] All 5 new pipeline tests pass
- [ ] `hugo serve` builds with no broken cross-references
- [ ] `task test` passes
- [ ] `task quality` passes

## Implementation Order

```
Phase 1 (R1) → Phase 2 (R2, R3) → Phase 3 (R4) → Phase 4 (R5) → Phase 5 (R6)
```

Phase 1 must complete first (canonical terms needed by all other phases). Phases 2 and 3 could technically run in parallel after Phase 1. Phase 5 should be written alongside Phase 4 (red-green discipline per project feedback).

## Dependencies & Risks

- **R5 adds one search API call** when board is in use. Acceptable cost, but verify it doesn't trigger rate limiting in showcase runs.
- **R5 JSON schema addition** (`off_board_items`): new field, not breaking, but downstream consumers should be aware.
- **Doc heading anchors**: If terminology.md headings become link targets (e.g., `#iteration`), they are effectively public API. Choose stable anchor names.

## Sources & References

- **Origin document:** [docs/brainstorms/2026-03-20-terminology-and-orthogonality-requirements.md](docs/brainstorms/2026-03-20-terminology-and-orthogonality-requirements.md) — key decisions: three orthogonal axes, define-once-link-everywhere, insight (not warning) with issue numbers, docs+code comments scope
- Insight pattern: `internal/pipeline/velocity/velocity.go:337-377` (`generateInsights`)
- Insight model: `internal/model/types.go:314` (`Insight` struct with `Level` + `Message`)
- Board fetch: `internal/pipeline/velocity/velocity.go:119` (`gatherFromBoard`)
- Existing effort tests: `internal/pipeline/velocity/effort_test.go`
- Render-layer linking learnings: `docs/solutions/architecture-patterns/render-layer-linking-and-insight-quality.md`
- Output shape contract: `docs/solutions/architecture-patterns/command-output-shape.md`
