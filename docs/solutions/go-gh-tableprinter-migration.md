---
title: "CLI table rendering: go-gh tableprinter → lipgloss v2"
category: architecture-decisions
tags: [lipgloss, go-gh, tableprinter, terminal-ui, TTY-detection, formatting, smoke-tests, bash, osc8]
module: internal/format/table.go
symptom: "Pretty output had fixed-width columns, no styled borders, inconsistent flag labels across commands"
root_cause: "go-gh tableprinter lacked styling control; replaced with lipgloss v2 buffering wrapper that sanitizes OSC 8 sequences"
date: 2026-03-20
supersedes: "Original 2026-03-09 version documented migration to go-gh tableprinter, which has since been replaced"
---

# CLI Table Rendering: go-gh tableprinter → lipgloss v2

## History

1. **2026-03-09**: Migrated from manual `fmt.Sprintf` to `go-gh/v2/pkg/tableprinter` for auto-truncation and TSV piped output.
2. **2026-03-20**: Replaced tableprinter with `charm.land/lipgloss/v2/table` for styled output (rounded borders, bold headers). See [lipgloss table migration](architecture-refactors/lipgloss-table-migration.md) for the full problem/solution/prevention writeup.

## Current Architecture

### Detect Once, Use Everywhere (unchanged)

Terminal capabilities are still detected once in `PersistentPreRunE` and stored in `Deps`:

```go
// cmd/root.go — PersistentPreRunE
t := term.FromEnv()
isTTY := t.IsTerminalOutput()
termWidth := 80
if w, _, err := t.Size(); err == nil && w > 0 {
    termWidth = w
}

deps := &Deps{
    IsTTY:     isTTY,
    TermWidth: termWidth,
}
```

### Buffering Table Wrapper (new)

`format.NewTable()` now returns a `*Table` (custom struct) instead of `tableprinter.TablePrinter`. The caller API is preserved — `AddHeader`, `AddField`, `EndRow`, `Render` — but internally it buffers rows and renders via lipgloss (TTY) or plain TSV (pipe).

```go
// internal/format/table.go
type Table struct {
    w       io.Writer
    isTTY   bool
    width   int
    headers []string
    rows    [][]string
    current []string
}

func NewTable(w io.Writer, isTTY bool, width int) *Table
func (t *Table) AddField(text string)
func (t *Table) EndRow()
func (t *Table) Render() error  // lipgloss for TTY, TSV for pipe
```

**Key difference from tableprinter**: lipgloss does not auto-truncate columns to fit terminal width the way tableprinter did. Width is managed via lipgloss's `Width()` method on the table, and `Wrap(false)` prevents multi-line cells. Long titles are truncated with `…` by lipgloss.

### OSC 8 Sanitization

lipgloss cannot handle OSC 8 hyperlink escape sequences — it miscomputes their width. Before passing cell values to lipgloss, `sanitizeForLipgloss()` strips OSC 8 sequences (preserving visible text) and removes control characters for security. TSV output is NOT sanitized.

### Usage Pattern (unchanged API, new rendering)

```go
tp := format.NewTable(rc.Writer, rc.IsTTY, rc.Width)
tp.AddHeader([]string{"", "#", "Title", "Closed", sorted.Header("lead_time", "Lead Time")})
for _, item := range sorted.Items {
    tp.AddField(leadTimeFlag(item, stats))
    tp.AddField(format.FormatItemLink(item.Issue.Number, item.Issue.URL, rc))
    tp.AddField(item.Issue.Title)
    tp.AddField(closedStr)
    tp.AddField(format.FormatMetricDuration(item.Metric))
    tp.EndRow()
}
tp.Render()
```

Output (TTY):
```
╭────┬──────┬───────────────────────────────────────┬────────────┬─────────────╮
│    │ #    │ Title                                 │ Closed     │ Lead Time ↓ │
├────┼──────┼───────────────────────────────────────┼────────────┼─────────────┤
│ ⚡ │ #66  │ feat: Include clickable GitHub searc… │ 2026-03-18 │ 2d 8h       │
╰────┴──────┴───────────────────────────────────────┴────────────┴─────────────╯
```

Output (piped): plain tab-separated text, no borders, no ANSI.

## Bash Arithmetic Bug in Smoke Tests (still relevant)

When `FAIL=0`, the expression `((FAIL++))` returns exit code 1 under `set -e` because bash arithmetic returns the *pre-increment* value (0 = false).

```bash
# BAD — exits with code 1 when FAIL=0 under set -e
((FAIL++))

# GOOD — always succeeds
FAIL=$((FAIL + 1))
```

## Key Design Decisions

- **Single detection point.** TTY and width detected once in `PersistentPreRunE`, not per-formatter.
- **Safe default width.** Falls back to 80 columns if detection fails (POSIX standard).
- **Config subcommands skip detection.** Commands under `config` skip `PersistentPreRunE` entirely.
- **Buffering wrapper, not direct lipgloss usage.** `NewTable()` adapts the lipgloss fluent builder to the existing `AddField`/`EndRow` caller pattern. Callers do not import lipgloss.
- **Dual-mode rendering.** TTY gets lipgloss styled table; pipe gets plain TSV. Same code path, branched in `Render()`.
- **Sanitize in render, not in AddField.** OSC 8 stripping happens in `renderLipgloss()` only — TSV output preserves raw content.

## Gotchas

- **lipgloss does not auto-truncate.** Unlike tableprinter, lipgloss requires explicit `Width()` and `Wrap(false)` for single-line rows. Without these, long content wraps into multi-line cells.
- **OSC 8 sequences break lipgloss width.** `ansi.StringWidth()` strips SGR but not OSC 8. Always sanitize before passing to lipgloss.
- **Dependency cascade.** Upgrading lipgloss v1→v2 also pulls new versions of `charmbracelet/x/ansi` and `cellbuf`. Run `go build ./...` immediately after `go get` before writing any migration code.
- **`scope.go` parameter type.** The `addScopeItemRow` function previously took `tableprinter.TablePrinter` as a parameter — must be changed to `*format.Table`.
- `term.Size()` can return an error on non-TTY. Always check and fall back.

## Files

- `cmd/root.go` — `Deps.IsTTY`, `Deps.TermWidth`, terminal detection in `PersistentPreRunE`
- `internal/format/table.go` — `Table` struct, `NewTable()`, `renderLipgloss()`, `renderTSV()`, `sanitizeForLipgloss()`
- `internal/format/flags.go` — `FlagEmoji()`, `SortBy[T]`/`Sorted[T]`, `WriteCapNote()`
- `internal/format/scope.go` — Scope pretty formatter using `*Table`
- `internal/pipeline/*/render.go` — Per-pipeline rendering using `format.NewTable()`
- `scripts/smoke-test.sh` — Bash arithmetic fix, output assertions
- `docs/solutions/architecture-refactors/lipgloss-table-migration.md` — Full migration writeup

## Related

- [lipgloss table migration](architecture-refactors/lipgloss-table-migration.md) — full problem/solution/prevention documentation
- [Command output shape](architecture-patterns/command-output-shape.md) — four-layer output shape (stats, detail, insights, provenance)
