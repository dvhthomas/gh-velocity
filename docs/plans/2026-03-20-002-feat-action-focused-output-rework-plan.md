---
title: "feat: Action-focused output rework across all commands"
type: feat
status: active
date: 2026-03-20
origin: docs/brainstorms/2026-03-20-action-focused-output-rework-requirements.md
---

# Action-Focused Output Rework

## Enhancement Summary

**Deepened on:** 2026-03-20
**Sections enhanced:** 8
**Research agents used:** architecture-strategist, performance-oracle, security-sentinel, code-simplicity-reviewer, pattern-recognition-specialist, agent-native-reviewer, best-practices-researcher, framework-docs-researcher, Context7

### Key Improvements
1. **lipgloss v2 API discovery** — lipgloss table uses a fluent builder API (`table.New().Headers().Rows()`), NOT the `AddHeader/AddField/EndRow` pattern. The wrapper must buffer rows internally. Also requires upgrading from lipgloss v1 (indirect) to v2 (direct).
2. **Naming collision fix** — "Signal" already means lifecycle events (`SignalIssueCreated`, `SignalIssueClosed`) in `model/types.go`. Renamed to "Flag" to match existing codebase convention (`leadTimeFlag()`, `"flags"` JSON field).
3. **YAGNI simplifications** — Dropped generic `CapAndSort()` interface, `SignalTier()`, `FormatSignalColumn()`, and `provenance_footer.go`. Each render function does its own `sort.Slice` + cap. Only `WriteCapNote()` and `FlagEmoji()` are shared.
4. **JSON simplification** — Ship `"flags"` array only (matching existing field name), drop backward-compat dual fields. This is a CLI extension, not a public API.

### New Considerations Discovered
- lipgloss tables don't auto-size to terminal width — need explicit width management via `StyleFunc` with per-column widths
- `scope.go` passes `tableprinter.TablePrinter` as a parameter type — this file also needs migration
- `FormatSignalSummary()` already exists in `formatter.go` — avoid naming collision
- Performance is a non-issue: 25-item cap makes rendering trivially fast; sorting 1000 items < 0.05ms

## Overview

Rework every command's output (json, pretty, markdown) so that content is focused, consistent, and drives action. WIP was intended as the gold standard but has not yet been migrated to lipgloss — the entire migration is ahead of us.

This is a cross-cutting sweep touching all 12 commands across 3 formats. The work is mostly mechanical once the shared infrastructure is built.

## Problem Statement / Motivation

Report output is inconsistent and data-heavy. Detail tables dump up to 1000 unsorted items. Signal flags vary between emoji and text per command. Only velocity has provenance. Quality has no JSON output. Doc links point to unvalidated anchors. Readers scan past walls of data instead of acting on what matters. (see origin: `docs/brainstorms/2026-03-20-action-focused-output-rework-requirements.md`)

## Proposed Solution

Build shared infrastructure first (lipgloss table wrapper, flag constants, provenance extraction), then migrate each command mechanically. Four phases: infrastructure → bulk pipelines → special commands → polish.

## Technical Considerations

### Architecture

- **Package boundaries preserved**: `internal/metrics/` never imports `internal/format/`. Insight messages carry markdown; render layer adapts per format. (per learnings: render-layer-linking-and-insight-quality.md)
- **Format rendering is pure**: Zero-API. Gather once, compute once, render N formats from in-memory data. (per AGENTS.md)
- **Pipeline ownership**: Each `internal/pipeline/<metric>/render.go` owns its output. Shared helpers in `internal/format/`, not centralized rendering.
- **Four-layer output shape**: Stats + detail + insights + provenance for every command. (per learnings: command-output-shape.md)

### Key design decisions

**D1. Non-TTY (piped) output with lipgloss tables.**
Keep dual-mode in `format.NewTable()`: lipgloss for TTY, plain tab-separated for pipe. This preserves backward compatibility for automation consumers who parse piped pretty output. The `isTTY` parameter already flows through `RenderContext` from `Deps`. This is justified: the dual mode already exists in `NewTable()` today — we are replacing one library's dual-mode with another's.

**D2. Flag tier sort order.**
Two tiers: has-any-flag sorts above no-flag. Within each tier, sort by duration descending. This is simple, predictable, and matches the WIP mental model. Each render function owns its sort with `sort.Slice` — no generic helper needed (YAGNI: the sort is 3 lines of code).

**D3. JSON flags field — no dual fields.**
Standardize on `"flags": ["outlier", "noise"]` array for all JSON item structs. This matches the existing field name in leadtime/cycletime JSON. For commands that currently use different field names (`is_stale`, `lead_time_outlier`), migrate to `"flags"` directly. This is a CLI extension, not a public API — no external consumers to break. Simplicity over premature backward compatibility.

**D4. Provenance rendering in pretty format.**
Single footer line for all commands: `— gh velocity flow lead-time --since 30d`. Velocity keeps its current multi-line block (it includes effort strategy explanation). Report gets one composite provenance block for the report invocation. Add `WriteFooter()` method to existing `model.Provenance` — no new file needed.

**D5. Flag-per-command matrix.**

| Flag | leadtime | cycletime | quality | release | reviews | wip |
|------|----------|-----------|---------|---------|---------|-----|
| 🚩 outlier | ✓ | ✓ | | ✓ | | |
| 🤖 noise | ✓ | ✓ | | | | |
| ⚡ hotfix | ✓ | ✓ | | | | |
| 🐛 bug | | | ✓ | | | |
| ⏳ stale | | | | | ✓ | ✓ |
| 🟡 aging | | | | | ✓ | ✓ |

Commands with no detail table (throughput) or single-item views (issue, pr) have no flag column. Velocity iteration rows have no flags (summary rows, not items). my-week annotations are inline status badges, not part of the flag system.

**D6. Velocity iterations NOT capped.**
Iteration history rows are summary data, not per-item detail. Teams rarely have >25 iterations in view. Cap applies only to issue/PR detail tables.

**D7. Quality JSON schema.**
Follow the leadtime/cycletime pattern: `{ categories: [...], items: [...], bug_ratio: float, insights: [...], warnings: [...] }`.

### Performance implications

None. All changes are render-layer only — no additional API calls. Confirmed by performance analysis:
- Sorting 1000 items with `sort.Slice`: ~0.05ms
- 25-item cap makes rendering trivially fast regardless of table library
- lipgloss per-cell overhead (ANSI styling, runewidth) is sub-millisecond for 25 rows
- Terminal width detection is already done once in `PersistentPreRunE` — lipgloss uses the provided width
- `Flags().Visit()` for provenance is a pure local operation (iterates in-memory pflag.FlagSet)
- Important: make defensive copy before sorting (as `sortWIPByAgeDesc` already does) to avoid mutating caller's slice

### Security considerations

- lipgloss table rendering inherits the existing `SanitizeMarkdown()` and `stripControlChars()` protections for user-controlled content (issue titles, labels).
- Doc link anchors constructed from constants only — no user input in URLs.
- lipgloss does NOT perform its own sanitization — verify that user-controlled strings (issue titles with ANSI escapes or control chars) don't break table rendering. Existing `stripControlChars()` should be applied to titles before passing to lipgloss.

## Implementation Phases

### Phase 1: Shared Infrastructure

Build the foundation that all commands will use. No command output changes yet.

#### 1a. lipgloss table wrapper

Replace `format.NewTable()` internals with lipgloss/table for TTY, keeping plain TSV for non-TTY (decision D1).

**Critical: lipgloss v2 API is a fluent builder, not AddHeader/AddField/EndRow.**

The lipgloss table API is:
```go
import "charm.land/lipgloss/v2/table"

t := table.New().
    Border(lipgloss.RoundedBorder()).
    BorderStyle(lipgloss.NewStyle().Foreground(purple)).
    StyleFunc(func(row, col int) lipgloss.Style {
        if row == table.HeaderRow {
            return headerStyle
        }
        return cellStyle
    }).
    Headers("Signal", "#", "Title", "Duration").
    Rows(rows...)
```

The `Table` wrapper must **buffer rows internally** and build the lipgloss table on `Render()`. The existing caller API (`AddHeader`, `AddField`, `EndRow`, `Render`) can be preserved as the adapter:

- [ ] `internal/format/table.go` — Replace internals. `NewTable()` returns `*Table` (custom struct, not `tableprinter.TablePrinter`). `AddHeader`/`AddField`/`EndRow` buffer data. `Render()` builds lipgloss table from buffered data (TTY) or writes TSV (non-TTY).
- [ ] `internal/format/scope.go` — Update: currently accepts `tableprinter.TablePrinter` as parameter type in `addScopeItemRow`. Change to accept `*Table`.
- [ ] `internal/pipeline/release/render.go` — Update: currently imports `tableprinter` directly for `writePrettyStatsRow`. Change to use `format.NewTable()`.
- [ ] `go.mod` — Upgrade to `charm.land/lipgloss/v2` (currently v1 as indirect). Add `charm.land/lipgloss/v2/table`.
- [ ] Style: rounded borders (`lipgloss.RoundedBorder()`), bold headers via `StyleFunc`, right-aligned numeric columns via `lipgloss.NewStyle().Align(lipgloss.Right)`.
- [ ] Width management: Set per-column widths via `StyleFunc`. Fixed-width columns (Signal: 4, #: 8, Duration: 10, etc.) get explicit widths. Title column gets `termWidth - sum(fixedWidths) - borderChars`.
- [ ] Test: verify TTY output has borders/styling, piped output is clean TSV. Use `bytes.Buffer` and `got`/`want` pattern per existing test conventions.

```go
// internal/format/table.go — target implementation sketch
type Table struct {
    w       io.Writer
    isTTY   bool
    width   int
    headers []string
    rows    [][]string
    current []string
    // per-column options (alignment, width)
    colOpts []ColumnOption
}

func NewTable(w io.Writer, isTTY bool, width int) *Table
func (t *Table) AddHeader(cols ...string)
func (t *Table) AddField(text string, opts ...FieldOption)
func (t *Table) EndRow()
func (t *Table) Render() error  // builds lipgloss table or TSV
```

#### 1b. Unified flag constants

**Naming: "Flag" not "Signal"** — `Signal` already means lifecycle events in `model/types.go` (`SignalIssueCreated`, `SignalIssueClosed`) and is rendered by `FormatSignalSummary()` in `formatter.go`. The existing codebase calls these "flags" consistently (`leadTimeFlag()`, `"flags"` JSON field).

- [ ] `internal/format/flags.go` (new file) — Define flag constants and emoji mapping. Keep it minimal (constants + one lookup function):

```go
// internal/format/flags.go
package format

// Flag constants for item annotations in detail tables.
const (
    FlagOutlier = "outlier"  // 🚩
    FlagNoise   = "noise"    // 🤖
    FlagHotfix  = "hotfix"   // ⚡
    FlagBug     = "bug"      // 🐛
    FlagStale   = "stale"    // ⏳
    FlagAging   = "aging"    // 🟡
)

// FlagEmoji returns the emoji for a flag constant.
func FlagEmoji(flag string) string {
    // map lookup, return "" for unknown
}

// WriteCapNote writes "Showing N of M items..." when items were capped.
// Omits output when total <= shown or total == 0.
func WriteCapNote(w io.Writer, shown, total int) {
    if total <= shown || total == 0 {
        return
    }
    fmt.Fprintf(w, "\nShowing %d of %d items (sorted by flag). Use --results json for full data.\n", shown, total)
}
```

- [ ] `internal/format/flags_test.go` — Table-driven tests for emoji mapping. Test `WriteCapNote` for edge cases: total=0, total=shown, total>shown.

**Dropped (YAGNI):** `SignalTier()` (use `len(flags) > 0` inline), `FormatSignalColumn()` (callers concatenate emoji directly — it's one line).

#### 1c. Extract provenance to all pipelines

- [ ] Add `Provenance model.Provenance` field to every pipeline result struct that lacks it (leadtime, cycletime, throughput, quality, release, reviews, WIP, issue, pr, my-week).
- [ ] In each pipeline's data-gathering phase, populate `Provenance` from `cmd.Flags().Visit()` (same pattern as velocity: `internal/pipeline/velocity/render.go`).
- [ ] `internal/model/provenance.go` — Add `WriteFooter(w io.Writer)` method that renders single-line footer: `— gh velocity flow lead-time --since 30d` (decision D4). No new file.
- [ ] Existing `RenderProvenanceMarkdown()` and `Provenance.WritePretty()` reused for markdown and velocity pretty (no change to velocity's multi-line rendering).

### Phase 2: Migrate Bulk Pipeline Commands

Apply the shared infrastructure to each pipeline with detail tables. Each command is an independent unit of work. Each pipeline's render function does its own `sort.Slice` + cap — no shared generic helper (YAGNI: the sort/cap is 5 lines per command).

**Sort pattern per command:**
```go
// Copy, sort (flags first, then duration desc), cap
sorted := make([]BulkItem, len(items))
copy(sorted, items)
sort.Slice(sorted, func(i, j int) bool {
    fi, fj := len(flagsFor(sorted[i])) > 0, len(flagsFor(sorted[j])) > 0
    if fi != fj { return fi }
    return durationOf(sorted[i]) > durationOf(sorted[j])
})
total := len(sorted)
if len(sorted) > 25 { sorted = sorted[:25] }
```

#### 2a. lead-time pipeline

- [ ] `internal/pipeline/leadtime/render.go` — Replace `leadTimeFlag()` with inline flag computation using `format.FlagEmoji()` constants. Sort items flag-first via `sort.Slice`. Cap at 25 for pretty/markdown. Move flag column first (R4). Add provenance rendering (footer in pretty, `<details>` in markdown, field in JSON). Standardize JSON items to use `"flags": [...]` array (already uses this field name).
- [ ] Update `internal/pipeline/leadtime/templates/leadtime-bulk.md.tmpl` — Flag column first, add cap note via `WriteCapNote`.
- [ ] Test: verify 26+ items shows cap note, flagged items sort first, JSON uncapped.

#### 2b. cycle-time pipeline

- [ ] `internal/pipeline/cycletime/render.go` — Same changes as lead-time. Replace `cycleTimeFlag()` with shared flag constants. Already uses `"flags"` in JSON — no schema change needed.
- [ ] Update `internal/pipeline/cycletime/templates/cycletime-bulk.md.tmpl`.
- [ ] Test: same as lead-time.

#### 2c. quality pipeline

- [ ] `internal/pipeline/quality/render.go` — Add `WriteJSON()` (R10, decision D7). Replace inline bug flag with `format.FlagEmoji(format.FlagBug)`. Sort flag-first, cap at 25. Add provenance. Remove hardcoded `IsTTY=false` (use actual TTY state from `RenderContext`).
- [ ] JSON schema: `{ repository, window, categories: [{name, count, pct}], items: [{number, title, url, category, lead_time_seconds, lead_time, flags}], bug_ratio, insights, provenance, warnings }`.
- [ ] Test: JSON output round-trips, cap note appears, flag column first.

#### 2d. release pipeline

- [ ] `internal/pipeline/release/render.go` — Replace text "OUTLIER" with 🚩 via `format.FlagEmoji(format.FlagOutlier)`. Sort flag-first, cap at 25. Replace direct `tableprinter` import with `format.NewTable()`. Add provenance. Change JSON from `"lead_time_outlier": true` to `"flags": ["outlier"]`.
- [ ] Test: verify emoji replaces text, provenance appears.

#### 2e. reviews pipeline

- [ ] `internal/pipeline/reviews/render.go` — Replace text "STALE" with ⏳/🟡 via shared flag constants (stale >48h, aging >24h). Sort flag-first (already sorts by age). Cap at 25. Add provenance. Change JSON from `"is_stale": true` to `"flags": ["stale"]` or `"flags": ["aging"]`. Add at least one insight (R13): e.g., "N PRs waiting >48h for review — review bottleneck may slow delivery."
- [ ] Test: verify flag emoji, insight appears when stale PRs exist.

#### 2f. throughput pipeline

- [ ] `internal/pipeline/throughput/render.go` — No detail table, so R1-R4 don't apply. Add provenance only. Audit existing insights for R12 compliance.
- [ ] Test: provenance appears in all three formats.

### Phase 3: Special Commands

Commands with unique rendering that don't follow the standard pipeline pattern.

#### 3a. WIP command

- [ ] `internal/format/wip.go` — Tables now use lipgloss via `format.NewTable()` (automatic from Phase 1a). Replace `StalenessLevel` string display with emoji via shared flags (STALE→⏳, AGING→🟡, ACTIVE stays as-is or gets no emoji). Flag column first. Already sorts by age (flag-first sort adds: stale items above aging above active). Cap at 25. Add provenance. Add at least one insight (R13): e.g., "N items stale >7 days — consider triaging or closing."
- [ ] Add `"flags"` array to WIP JSON items alongside existing `"staleness"` field.
- [ ] Test: flag emoji, cap note, insight.

#### 3b. my-week command

- [ ] `internal/format/myweek.go` — No standard detail table (sections are narrative, not tabular). Add provenance. Audit existing insights for R12. Keep inline status annotations (new, stale, needs review) as-is — they are not part of the flag system. Verify JSON output exists and is complete.
- [ ] Test: provenance in all formats.

#### 3c. issue detail / pr detail

- [ ] `internal/pipeline/issue/render.go` — Single-item view, no detail table. Add provenance only. (Dropped: insights for single-item views — they don't have stats context to compare against without an additional API call, violating the "rendering is pure" constraint.)
- [ ] `internal/pipeline/pr/render.go` — Same: provenance only.
- [ ] Test: provenance appears.

#### 3d. velocity pipeline

- [ ] `internal/pipeline/velocity/render.go` — Already has provenance (keep multi-line style, D4). Iteration table migrates to lipgloss automatically via `format.NewTable()` (Phase 1a). No cap on iterations (D6). No flag column. Audit insights for R12 compliance.
- [ ] Test: lipgloss table renders for iteration history.

#### 3e. report command

- [ ] `cmd/report.go` + `internal/format/report.go` — Summary table migrates to lipgloss via `format.NewTable()` (Phase 1a). Add one composite provenance block (D4). Per-section detail tables in markdown `<details>` blocks now sort flag-first and cap at 25. Audit Key Findings insights for R12.
- [ ] Test: provenance in all formats, detail sections capped.

### Phase 4: Polish

#### 4a. Doc link validation

- [ ] Collect all `DocLink()` anchors and `DocPath*` constants from Go source.
- [ ] Cross-reference against actual headings in doc site source (Hugo content directory).
- [ ] Fix any broken anchors (R7).
- [ ] Consider a lightweight Go test for anchor validation — defer if doc site structure makes it impractical.

#### 4b. Insight audit

- [ ] Review all existing insight messages for R12 compliance (judgment/comparison, not data restatement).
- [ ] Fix any that merely restate table data.
- [ ] Verify `LinkStatTerms()` string replacements still match after any insight message rewording.

#### 4c. Cleanup

- [ ] Remove `go-gh/v2/pkg/tableprinter` import from all files. go-gh is still used for auth/API — only the tableprinter sub-package import is removed.
- [ ] Remove per-command flag functions (`leadTimeFlag()`, `cycleTimeFlag()`) replaced by shared flag constants.
- [ ] Run `task quality` to verify everything passes.

## System-Wide Impact

- **Interaction graph**: `format.NewTable()` is called by 7 render files + `wip.go` + `scope.go`. Replacing its internals changes visual output for every caller simultaneously after Phase 1a. `scope.go` also needs its `addScopeItemRow` parameter type updated from `tableprinter.TablePrinter` to `*Table`.
- **Error propagation**: No new error paths. Rendering failures already propagate via `error` returns from `Render()`.
- **State lifecycle risks**: None — all changes are stateless rendering.
- **API surface parity**: JSON changes: `"flags"` array standardized across all commands (was already the field name in leadtime/cycletime), `"provenance"` added to all outputs. Fields `"is_stale"` and `"lead_time_outlier"` migrated to `"flags"` array. Agents consuming JSON get the same information as humans reading pretty output — flags, insights, and provenance are all present in JSON.
- **Integration test scenarios**: (1) Run `gh velocity report --results json 2>&1` — only valid JSON on stdout. (2) Pipe `gh velocity flow lead-time --results pretty | head -5` — verify plain text, no lipgloss borders. (3) Run with 100+ items — verify cap note and only 25 rows in pretty/markdown.

## Acceptance Criteria

### Functional Requirements

- [ ] All pretty-format tables render with lipgloss (rounded borders, bold headers, right-aligned numbers) when TTY
- [ ] Piped pretty output remains plain tab-separated text (no borders)
- [ ] All detail tables sort flag-first, then duration descending
- [ ] All detail tables cap at 25 items in pretty/markdown with cap note
- [ ] JSON output is always uncapped
- [ ] Flag column is first in all detail tables
- [ ] Emoji flags are consistent: 🚩🤖⚡🐛⏳🟡 across all commands
- [ ] All commands include provenance in all three formats
- [ ] Quality pipeline has JSON output
- [ ] All doc link anchors resolve to real headings
- [ ] Every insight contains judgment/comparison, not data restatement
- [ ] Commands previously missing insights (reviews, WIP) have at least one

### Non-Functional Requirements

- [ ] No additional API calls (rendering is pure)
- [ ] `task quality` passes
- [ ] JSON `"flags"` array is the standard field across all commands

### Quality Gates

- [ ] Table-driven tests for flag constants, cap note edge cases
- [ ] Render tests for at least one command per phase verifying pretty/markdown/json output
- [ ] `task test` passes
- [ ] `task quality` passes
- [ ] Smoke test against a real repo (e.g., cli/cli)

## Dependencies & Risks

- **lipgloss v2 upgrade**: Current go.mod has lipgloss v1 as indirect. Need to upgrade to v2 (`charm.land/lipgloss/v2`) which is a module path change, not just a version bump. Verify no breaking changes in `charmbracelet/x/ansi` usage (OSC 8 hyperlinks).
- **lipgloss table width management**: lipgloss tables don't auto-truncate columns like go-gh tableprinter. Width must be managed explicitly via `StyleFunc` with per-column `Width()` styles. The title column gets remaining space after fixed-width columns. Test on narrow terminals (60 columns) to verify graceful degradation.
- **Unicode emoji width**: Emoji in flag column may render as 1 or 2 terminal columns depending on terminal emulator. lipgloss uses `mattn/go-runewidth` for width calculation. Test in iTerm2, Terminal.app, VS Code integrated terminal.
- **go-gh tableprinter removal**: go-gh is still used for auth/API. Only the `tableprinter` sub-package import is removed.
- **scope.go migration**: `addScopeItemRow` takes `tableprinter.TablePrinter` as a parameter type — must change to `*Table` as part of Phase 1a.

## Sources & References

### Origin

- **Origin document:** [docs/brainstorms/2026-03-20-action-focused-output-rework-requirements.md](docs/brainstorms/2026-03-20-action-focused-output-rework-requirements.md) — Key decisions: 25-item cap, signal-first sort, lipgloss everywhere, provenance on all commands, unified emoji flags, keep doc links but validate.

### Internal References

- Architecture: `docs/solutions/architecture-patterns/command-output-shape.md` — four-layer output shape
- Rendering patterns: `docs/solutions/architecture-patterns/render-layer-linking-and-insight-quality.md` — format-aware linking, insight quality
- JSON contract: `docs/solutions/architecture-patterns/complete-json-output-for-agents.md` — JSON warnings, error envelopes
- Tableprinter migration: `docs/solutions/go-gh-tableprinter-migration.md` — current table architecture gotchas
- Three-state pattern: `docs/solutions/three-state-metric-status-pattern.md` — N/A vs in-progress vs completed display
- Flag format: `internal/format/links.go` — current doc link constants
- Provenance reference: `internal/pipeline/velocity/render.go` — working provenance implementation
- Table wrapper: `internal/format/table.go` — current `NewTable()` to replace
- WIP rendering: `internal/format/wip.go` — target for flag/cap migration
- Existing flag functions: `internal/pipeline/leadtime/render.go:232` (`leadTimeFlag()`), `internal/pipeline/cycletime/render.go:318` (`cycleTimeFlag()`)

### External References

- lipgloss v2 table API: `charm.land/lipgloss/v2/table` — fluent builder with `table.New().Border().StyleFunc().Headers().Rows()`
- lipgloss table docs: Context7 `/charmbracelet/lipgloss` — `StyleFunc` for per-row/column styling, `table.HeaderRow` constant, `lipgloss.RoundedBorder()` for rounded corners
- lipgloss width: Column widths set via `lipgloss.NewStyle().Width(N)` in `StyleFunc`, NOT on the table itself

### Related Work

- Prior brainstorm: `docs/brainstorms/2026-03-17-reporting-quality-drive-brainstorm.md` — noise detection, link infrastructure, insight quality
- Prior brainstorm: `docs/brainstorms/2026-03-10-actionable-output-brainstorm.md` — original vision for actionable output
- Prior brainstorm: `docs/brainstorms/2026-03-16-output-results-separation-brainstorm.md` — `--results` flag design
