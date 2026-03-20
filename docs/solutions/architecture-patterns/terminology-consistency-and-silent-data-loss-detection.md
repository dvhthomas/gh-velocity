---
title: "Terminology consistency and silent data loss detection for off-board items"
category: "architecture-patterns"
date: "2026-03-20"
module:
  - "internal/pipeline/velocity"
  - "internal/config"
  - "internal/model"
  - "site/content/concepts"
tags:
  - "terminology"
  - "documentation"
  - "silent-data-loss"
  - "effort"
  - "project-board"
  - "insights"
severity: "medium"
root_cause_type: "implicit-assumption"
---

# Terminology Consistency and Silent Data Loss Detection

## Problem

Two related issues:

1. **Terminology drift**: Key terms (scope, lifecycle, iteration, effort) were defined locally in multiple doc pages and godoc strings, leading to inconsistency. "Effort" vs "estimate", "iteration" vs "sprint" appeared interchangeably.

2. **Silent data loss**: When the effort strategy depended on a project board (`numeric` or `field:` matchers), `gatherFromBoard` only returned items ON the board via `ListProjectItemsWithFields`. Items in scope but not on the board silently vanished from velocity output — no insight, no item numbers, nothing.

## Root Cause

**Terminology**: No canonical glossary existed. Each doc page and godoc comment defined terms independently, and they diverged over time.

**Silent data loss**: `gatherFromBoard` treats the board as the complete scope. It never cross-references against a search-based scope query, so items not on the board are invisible to the pipeline. This is an implicit assumption — the four measurement axes (scope, lifecycle, iteration, effort) are orthogonal by design, but scope and effort were conflated when the board was the sole data source.

## Solution

### 1. Canonical Definitions page

Created `site/content/concepts/terminology.md` as the single source of truth for all key terms. Other pages link here instead of re-explaining. The page defines:

- **Measurement axes**: Scope, Lifecycle, Iteration, Effort — explicitly stated as orthogonal
- **Metrics**: Velocity, Throughput, Lead Time, Cycle Time, Quality — one-liner definitions linking to reference pages
- **Other terms**: Insight, Linking Strategy, Matcher

### 2. Godoc alignment

Fixed terminology in pipeline code comments:

- "effort estimates" → "effort values"
- "sprint commitments" → "iteration commitments"
- "(no estimate on board)" → "(no effort value on board)"
- `Iteration` struct godoc: "project iteration (sprint) period" → "bounded time period used to bucket velocity results"

No identifier renames — only comments and godoc strings.

### 3. Off-board item detection

Added `detectOffBoardItems()` to the velocity pipeline:

```go
// When effort depends on the board, also search for in-scope items
// and diff against board items to find the missing ones.
effortNeedsBoard := p.Config.Effort.Strategy == "numeric" || HasFieldMatchers(p.Config.Effort)
if effortNeedsBoard && p.Scope != "" {
    p.detectOffBoardItems(ctx)
}
```

The function:
1. Determines time range from resolved iterations
2. Runs a search query for in-scope closed/merged items (same query `gatherFromSearch` would use)
3. Builds a set of board item numbers
4. Computes the delta: items in search results but not on board

### 4. Insight generation with suppression rules

Added `field_effort_off_board` insight in `generateInsights()` with sorted issue numbers:

```go
if len(p.offBoardItems) > 0 {
    sort.Ints(p.offBoardItems)
    // Generate: "N items are not on the project board and have no effort assigned: #5, #12, #18"
    r.Insights = append(r.Insights, model.Insight{
        Type:    "field_effort_off_board",
        Message: fmt.Sprintf("%d items are not on the project board...", ...),
    })
    r.OffBoardItems = p.offBoardItems
}
```

Suppression rules are explicit:

| Strategy | Insight generated? | Rationale |
|---|---|---|
| `count` | Never | Board not used for effort |
| `attribute` (label-only) | Never | Labels come from the issue, not the board |
| `attribute` (field matchers) | Yes, for items not assessed by labels | Board needed for field values |
| `numeric` | Yes, for all off-board items | Effort comes exclusively from the board |

### 5. JSON output extension

Added `off_board_items` structured array to JSON output (and `OffBoardItems []int` to `VelocityResult`) so agent consumers can act on specific issue numbers without parsing insight message text.

## Prevention

### Preventing terminology drift

- **Define once, link everywhere**: The Definitions page is the single source of truth. Other pages link to it with Hugo `relref` anchors (e.g., `#iteration`, `#effort`). These anchors are effectively public API — changing them requires searching for references.
- **Doc heading anchors as contract**: If terminology.md headings become link targets, treat them as stable. Search for references before renaming.
- **Review check**: When adding or modifying docs, verify that new text links to definitions rather than re-explaining terms.

### Preventing silent data loss

- **Never drop silently**: When joining two data sources (search + board), the pipeline must account for items in one source but not the other. The `detectOffBoardItems` pattern computes the delta explicitly.
- **Insight, not warning**: Missing-board items surface as an informational insight (the user may have intentionally not added items to the board), not a warning (which implies something is wrong).
- **Include specific identifiers**: The insight lists issue numbers because GitHub search cannot filter "not on project board X" — this is the only way users can discover which items are missing.

### Testing edge cases

Six pipeline-level tests cover the insight:

| Test | Verifies |
|---|---|
| `TestProcessData_FieldEffortOffBoard` | Off-board items produce insight with correct sorted issue numbers |
| `TestProcessData_FieldEffortAllOnBoard` | No insight when all items are on board |
| `TestProcessData_CountStrategyNoOffBoardInsight` | Count strategy never generates insight |
| `TestProcessData_AttributeLabelOnlyNoOffBoardInsight` | Label-only attribute never generates insight |
| `TestProcessData_MixedMatchersPartialBoard` | Mixed label+field matchers: only unassessed off-board items in insight |
| `TestWriteJSON_OffBoardItems` | JSON output includes `off_board_items` array |

## Related

- [Render-layer linking and insight quality](render-layer-linking-and-insight-quality.md) — insight message quality standards
- [Command output shape](command-output-shape.md) — four-layer output architecture (stats, detail, insights, provenance)
- [Label-based lifecycle for cycle time](label-based-lifecycle-for-cycle-time.md) — project board vs label signal patterns
- [Pipeline-per-metric architecture](../architecture-refactors/pipeline-per-metric-and-preflight-first-config.md) — velocity pipeline directory layout
