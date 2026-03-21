---
title: "Migrate table rendering from go-gh tableprinter to lipgloss v2"
category: "architecture-refactors"
date: "2026-03-20"
tags:
  - lipgloss
  - table-rendering
  - output-format
  - osc8
  - sorting
  - provenance
  - generics
  - go-gh
module: "internal/format, internal/pipeline, cmd"
severity: "moderate"
symptom: "go-gh tableprinter lacks styling control; output columns cluttered; no unified flag/sort system; provenance missing from most commands"
root_cause: "Dependency on go-gh tableprinter limited rendering; lipgloss v2 fluent builder API incompatible with AddField/EndRow pattern; OSC 8 hyperlinks miscompute cell widths in lipgloss"
---

# Migrate Table Rendering from go-gh tableprinter to lipgloss v2

## Problem

The CLI used `go-gh/v2/pkg/tableprinter` for all pretty-format tables. This worked but lacked styling control (no borders, no bold headers, no column alignment). Output was also cluttered: 7-8 columns with Labels, Created, Started columns that added noise without driving action. Flag labels were inconsistent (text "OUTLIER"/"STALE" in some commands, emoji in others). Provenance existed only on the velocity command.

## Root Cause

Three interrelated problems:

1. **API mismatch**: go-gh's `tableprinter` uses an imperative `AddField`/`EndRow` API. lipgloss v2's `table` package uses a fluent builder (`table.New().Headers(...).Rows(...)`). All data must be available before rendering, requiring a buffering adapter.

2. **OSC 8 width corruption**: Terminal hyperlinks (`\x1b]8;;url\x07text\x1b]8;;\x07`) are invisible escape sequences, but lipgloss counts their bytes when computing column widths, producing misaligned tables with extreme wrapping. lipgloss's `ansi.StringWidth()` strips SGR sequences but not OSC 8.

3. **Scattered conventions**: Flag labels, sort logic, and provenance were duplicated across commands with inconsistent implementations.

## Solution

### Buffering Table Wrapper

`internal/format/table.go` — A `Table` struct buffers headers and rows via the familiar `AddField`/`EndRow` API, then on `Render()` either emits tab-separated text (non-TTY) or builds a lipgloss styled table (TTY). This preserves the existing calling convention while swapping the rendering engine.

```go
type Table struct {
    headers []string
    rows    [][]string
    current []string  // row being built
}
func (t *Table) AddField(text string) { t.current = append(t.current, text) }
func (t *Table) EndRow()              { t.rows = append(t.rows, t.current); t.current = nil }
func (t *Table) Render() error        { /* flush partial row, then renderTSV or renderLipgloss */ }
```

### OSC 8 Sanitization

Before passing cells to lipgloss, a regex strips OSC 8 hyperlink sequences while preserving the visible display text. A secondary `stripControlChars` pass removes remaining control characters for security (user-controlled issue titles could inject terminal escapes).

```go
var osc8Re = regexp.MustCompile(
    `\x1b\]8;[^;]*;[^\x07\x1b]*(?:\x07|\x1b\\)(.*?)\x1b\]8;;\x07|\x1b\]8;;\x1b\\`)

func sanitizeForLipgloss(s string) string {
    s = osc8Re.ReplaceAllString(s, "$1")
    return stripControlChars(s)
}
```

Applied in `renderLipgloss()` to all cell values before building the lipgloss table. TSV output is NOT sanitized (preserves raw content for pipe consumers). Clickable hyperlinks still work in markdown output and non-table pretty output.

### Generic Sort with Metadata

`internal/format/flags.go` — `SortBy[T, K]` produces a `Sorted[T]` that carries the sort field name and direction alongside the sorted slice:

```go
sorted := format.SortBy(items, "lead_time", format.Desc,
    func(it BulkItem) *time.Duration { return it.Metric.Duration })

sorted.Items                             // the sorted slice
sorted.Header("lead_time", "Lead Time")  // "Lead Time ↓"
sorted.JSONSort()                        // {"field":"lead_time","direction":"desc"}
```

One call does everything — sort, header arrow, and JSON metadata all derive from the same object. Nil keys sort to the end regardless of direction. The original slice is never mutated (defensive copy).

### Provenance Wiring

`cmd/provenance.go` — Shared `buildProvenance` helper walks Cobra flags to reconstruct the command invocation. `cmd/render.go` — `renderPipeline` appends provenance after rendering unless the pipeline implements `provenanceRenderer` (opt-out interface for velocity, which has its own rich provenance block with effort strategy details).

```go
type provenanceRenderer interface { HasProvenance() bool }
```

Pretty: single-line footer `— gh velocity flow lead-time --since 30d`. Markdown: collapsible `<details>` block. JSON: embedded in the pipeline's own struct.

## Investigation Steps

1. Discovered lipgloss v2 uses `charm.land/lipgloss/v2` import path (not `github.com/charmbracelet/lipgloss/v2`) via Context7 docs
2. Initial `go get` upgraded `charmbracelet/x/ansi` v0.8→v0.11 and `cellbuf` broke — fixed by upgrading `cellbuf` too
3. First render test showed OSC 8 hyperlinks exploding column widths — `]8;;https://...` visible as text in cells
4. Root cause: `stripControlChars` in `AddField` stripped `\x1b` bytes, destroying OSC 8 sequences and leaving visible garbage. Moved sanitization to `renderLipgloss()` with proper regex
5. "Signal" naming collided with `model.SignalIssueCreated` lifecycle constants — renamed to "Flag" matching existing `leadTimeFlag()` and `"flags"` JSON field
6. Initial plan proposed generic `CapAndSort()` with `Signals() []string` interface — simplicity review flagged as YAGNI. Replaced with `SortBy[T]` + inline `sort.Slice` per command
7. Velocity provenance appeared twice (its own + generic footer) — added `provenanceRenderer` opt-out interface

## Prevention Strategies

### Terminal Escape Sequence Testing
- Before adopting any TUI library, write a width-computation test feeding strings with every escape sequence you use (OSC 8, ANSI color, etc.) into the library's width function
- Inventory all escape-sequence-producing code (`\x1b]8`, `\033[`) and treat each as a test case
- Pin a golden-file test capturing raw byte output of a representative table

### Naming Discipline
- `grep -rn 'FlagXxx\|Flag ' internal/` before introducing new exported identifiers — takes 5 seconds, catches semantic collisions
- Constants sharing a prefix must share a domain concept. If a package has `SignalOpen` for lifecycle, new flag constants get a different prefix (`Flag*`)

### Dependency Upgrades
- First step after `go get library@v2`: run `go build ./...` and `go test ./...` before writing any migration code
- Use `go mod graph | grep charmbracelet` to preview the transitive blast radius
- Separate the `go.mod`/`go.sum` commit from migration code commits for clean bisecting

### Generic Abstraction YAGNI
- Apply the "three instances" rule — don't extract until three concrete cases exist
- Start with inline code, refactor to generic only when duplication actually appears
- Measure "concept cost": if the abstraction introduces more concepts than the duplication it removes, inline wins
- `SortBy[T]` works because it is data (field, direction) interpreted by one function. `CapAndSort` failed because it pushed behavior into an interface each type had to implement

### Opt-Out Interfaces for Generic Behavior
- When adding generic behavior to existing code, audit every call site: "Does any implementation already do this?"
- Use opt-out (generic is default, implement interface to skip) not opt-in
- Test for duplication explicitly: count occurrences of the provenance marker, assert exactly one

## Related

- [go-gh tableprinter migration](../go-gh-tableprinter-migration.md) — **STALE**: documents the migration *to* tableprinter which is now replaced. Core patterns (TTY detection in `PersistentPreRunE`, `Deps.IsTTY`/`Deps.TermWidth`) still apply but library-specific details are outdated.
- [Command output shape](../architecture-patterns/command-output-shape.md) — four-layer output shape (stats, detail, insights, provenance). Still accurate.
- [Render-layer linking and insight quality](../architecture-patterns/render-layer-linking-and-insight-quality.md) — format-aware linking pattern. Still accurate.
- [Complete JSON output for agents](../architecture-patterns/complete-json-output-for-agents.md) — JSON warnings and error envelopes. Still accurate.
- [Pipeline-per-metric architecture](pipeline-per-metric-and-preflight-first-config.md) — directory-per-metric layout. Still accurate.
- [lipgloss OSC 8 incompatibility](../../..) (auto memory [claude]) — memory note capturing the OSC 8 finding
- [#82](https://github.com/dvhthomas/gh-velocity/issues/82) — insights for all report sections
- Plan: `docs/plans/2026-03-20-002-feat-action-focused-output-rework-plan.md`
- Requirements: `docs/brainstorms/2026-03-20-action-focused-output-rework-requirements.md`
