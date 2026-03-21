---
title: "feat: Cap detail sections at 50 rows with smart filtering and sorting"
type: feat
status: completed
date: 2026-03-21
---

# feat: Cap detail sections at 50 rows with smart filtering and sorting

## Overview

Detail sections in markdown and pretty output can produce unbounded rows, making reports unwieldy for active repos. Each detail section should cap at 50 rows with section-specific filtering, sorting, and an omitted-items count.

## Problem Statement

When a repo has hundreds of closed issues in a reporting window, the Lead Time, Cycle Time, and Quality detail tables render every single item. This makes the report difficult to scan and the most interesting items hard to find. The WIP detail section already implements a 50-row cap with smart sorting — the same pattern needs to be applied to the remaining detail sections.

## Current State Assessment

**Already implemented (WIP detail):**
- `maxWIPDetailItems = 50` constant in `internal/format/wip.go:148`
- Capping + truncation in both markdown (`wip_detail.go:296-330`) and pretty (`wip_detail.go:490-537`)
- Smart sort: STALE first, then AGING, then ACTIVE (needs-attention ordering)
- Omitted items message: "*N more items not shown. Use `--format json` for complete data.*"
- JSON output: uncapped (all items included)

**Not yet implemented:**
- Lead Time detail: renders all items, no cap
- Cycle Time detail: filters N/A items but no cap
- Quality detail: renders all items (all categories), no cap, no bugs-only filter

## Proposed Solution

Add a shared `MaxDetailRows = 50` constant in `internal/format/` and apply capping to lead time, cycle time, and quality detail sections. Each section defines its own filtering and sort heuristic per the issue spec.

## Technical Approach

### Shared constant

Add to `internal/format/flags.go`:

```go
// MaxDetailRows is the maximum number of items shown in detail tables.
// Full data is always available via --format json.
const MaxDetailRows = 50
```

Also update `maxWIPDetailItems` in `wip.go` to reference this constant (or replace with `MaxDetailRows` directly).

### Phase 1: Lead Time detail capping

**Files:** `internal/pipeline/leadtime/render.go`, `internal/pipeline/leadtime/templates/leadtime-bulk.md.tmpl`

Sort: Already sorted by lead time descending (longest first) — matches issue spec. No change needed.

Filter: All items (no filtering) — matches issue spec.

Changes to `WriteBulkMarkdown`:
1. After `SortBy`, compute `total := len(sorted.Items)` and `capped := total > format.MaxDetailRows`
2. If capped, slice `sorted.Items` to `format.MaxDetailRows`
3. Pass `Total`, `Capped`, and `Showing` fields to template data
4. Update template: change `<summary>Details ({{len .Items}} issues)</summary>` to show "showing N of M" when capped, plus omitted-items footnote

Changes to `WriteBulkPretty`:
1. Same cap logic after `SortBy`
2. Print header line "Items (showing 50 of N — longest lead time first):" when capped
3. After table render, print "N more items not shown. Use --format json for complete data."

### Phase 2: Cycle Time detail capping

**Files:** `internal/pipeline/cycletime/render.go`, `internal/pipeline/cycletime/templates/cycletime-bulk.md.tmpl`

Sort: Already sorted by cycle time descending (longest first) — matches issue spec.

Filter: Already filters N/A items. No additional filtering needed.

Changes to `WriteBulkMarkdown`:
1. After building the filtered `data.Items` slice, apply cap
2. Track `total` (filtered count) vs `showing` (capped count)
3. Update template summary line and add omitted footnote when capped

Changes to `WriteBulkPretty`:
1. After filtering N/A items in the loop, apply cap
2. Refactor: collect filtered items into a slice first, then cap, then render table
3. Add header/footer capping messages

### Phase 3: Quality detail capping and filtering

**Files:** `internal/pipeline/quality/render.go`, `internal/pipeline/quality/templates/quality.md.tmpl`

Sort: Already sorted by lead time descending (longest first) — matches issue spec.

**Filter: Bugs only** (this is the key new behavior). The issue spec says "Quality (release) details → Bugs only". This means:
1. Filter `sorted.Items` to only items where `Category == "bug"`
2. Show count of filtered vs total: "Details (showing 12 bugs of 45 issues)"
3. The category breakdown table above already shows all categories — the filter only applies to the per-item detail table

Changes to `WriteMarkdown`:
1. After `SortBy`, filter to bugs only for the detail table
2. Apply 50-row cap on the filtered set
3. Pass total/filtered/capped counts to template
4. Update template summary and add footnote

Changes to `WritePretty`:
1. Same filter-then-cap logic
2. Update header: "Bug details (showing N of M bugs — longest lead time first):"
3. Add footer when capped

### Phase 4: Update report.go detail headers

**File:** `cmd/report.go` (lines 394-460)

The `summary` strings passed to `writeDetail` (e.g., `"Lead Time (N issues)"`) should reflect capping when applicable. However, since the capping happens inside the `WriteBulk*` functions, the outer summary in `report.go` shows total counts which is correct — the detail function handles the "showing X of Y" internally.

No changes needed in `report.go` itself.

### JSON output: unchanged

Per issue spec, JSON is unfiltered. No changes to `WriteBulkJSON` / `WriteJSON` functions. The existing `Capped` field in JSON output refers to API truncation (1000-result limit), not display capping — this is a different concern and should remain as-is.

## Acceptance Criteria

- [ ] Shared `MaxDetailRows = 50` constant in `internal/format/flags.go`
- [ ] Lead Time markdown detail: capped at 50 rows, shows "showing N of M" when capped, omitted footnote
- [ ] Lead Time pretty detail: capped at 50 rows, header/footer messages when capped
- [ ] Cycle Time markdown detail: capped at 50 rows (after N/A filtering), shows "showing N of M" when capped
- [ ] Cycle Time pretty detail: capped at 50 rows (after N/A filtering), header/footer messages when capped
- [ ] Quality markdown detail: filtered to bugs only, capped at 50, shows "showing N bugs of M issues"
- [ ] Quality pretty detail: filtered to bugs only, capped at 50, header/footer messages
- [ ] JSON output: completely unchanged for all sections
- [ ] WIP detail: migrated to use shared `MaxDetailRows` constant (optional cleanup)
- [ ] Unit tests for each section verifying capping behavior with >50 items
- [ ] Unit tests verifying no capping when items <= 50
- [ ] Unit tests verifying quality bugs-only filter
- [ ] `task test` passes
- [ ] `task quality` passes

## Sources & References

- GitHub issue: [#149](https://github.com/dvhthomas/gh-velocity/issues/149)
- WIP capping template: `internal/format/wip_detail.go:296-330` (markdown), `wip_detail.go:490-537` (pretty)
- Lead Time render: `internal/pipeline/leadtime/render.go:145-217`
- Cycle Time render: `internal/pipeline/cycletime/render.go:209-295`
- Quality render: `internal/pipeline/quality/render.go:143-209`
- Report orchestration: `cmd/report.go:391-472`
- Documented learning: `docs/solutions/architecture-patterns/render-layer-linking-and-insight-quality.md` — N/A row filtering done at render layer, not data layer
