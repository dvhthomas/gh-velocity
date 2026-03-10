---
title: "Migrating CLI Output to go-gh tableprinter"
category: architecture-decisions
tags: [go-gh, tableprinter, terminal-ui, TTY-detection, formatting, smoke-tests, bash]
module: internal/format/table.go
symptom: "Pretty output had fixed-width columns that broke on narrow terminals and no TSV output when piped"
root_cause: "Manual string formatting with hardcoded spacing cannot adapt to terminal width or detect piped output"
date: 2026-03-09
---

# Migrating CLI Output to go-gh tableprinter

## Problem

Pretty-printed output used `fmt.Sprintf` with fixed-width padding. This caused:
- Truncated or wrapped output on narrow terminals
- No machine-readable output when piped (no TSV fallback)
- Inconsistent formatting across commands

## Solution

Migrated all pretty output to `github.com/cli/go-gh/v2/pkg/tableprinter`, which auto-truncates columns to terminal width and outputs TSV when piped.

### Architecture: Detect Once, Use Everywhere

Terminal capabilities are detected once in `PersistentPreRunE` and stored in the `Deps` struct:

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
    // ...
}
```

A thin wrapper creates tables consistently:

```go
// internal/format/table.go
func NewTable(w io.Writer, isTTY bool, width int) tableprinter.TablePrinter {
    if width <= 0 {
        width = 80
    }
    return tableprinter.New(w, isTTY, width)
}
```

### Usage Pattern

Commands use key-value rows for single-entity output (lead-time, cycle-time) and header+rows for multi-entity output (release, scope):

```go
// Key-value pattern (single entity)
tp := format.NewTable(w, deps.IsTTY, deps.TermWidth)
tp.AddField("Issue #42")
tp.AddField("Fix the widget")
tp.EndRow()
tp.AddField("Cycle Time")
tp.AddField("2d 3h 15m")
tp.EndRow()
tp.Render()

// Table pattern (multiple entities)
tp.AddHeader([]string{"#", "Title", "Lead Time", "Cycle Time"})
for _, item := range items {
    tp.AddField(...)
    tp.EndRow()
}
tp.Render()
```

## Bash Arithmetic Bug in Smoke Tests

After migration, smoke tests broke for an unrelated reason: `((FAIL++))` under `set -e`.

When `FAIL=0`, the expression `((FAIL++))` returns exit code 1 because bash arithmetic expressions return the *pre-increment* value as the exit code. Pre-increment of 0 is 0, which bash treats as false/failure.

**Fix:** Use assignment syntax instead:

```bash
# BAD — exits with code 1 when FAIL=0 under set -e
((FAIL++))

# GOOD — always succeeds
FAIL=$((FAIL + 1))
```

The smoke test now uses helper functions that encapsulate this:

```bash
pass() { PASS=$((PASS + 1)); echo "  ✓ $1"; }
fail() { FAIL=$((FAIL + 1)); ERRORS+="  ✗ $1\n"; echo "  ✗ $1" >&2; }
```

## Smoke Test Output Format Changes

After tableprinter migration, output changed from colon-separated to tab-separated. Smoke test assertions needed updating:

```bash
# Before (manual formatting): "Started: 2024-01-01T00:00:00Z"
[[ "$out" == *"Started:"* ]]

# After (tableprinter): "Started\t2024-01-01T00:00:00Z"
[[ "$out" == *"Started"* ]]
```

## Key Design Decisions

- **Single detection point.** TTY and width detected once in `PersistentPreRunE`, not per-formatter. Avoids repeated `term.Size()` calls and keeps formatters pure.
- **Safe default width.** Falls back to 80 columns if detection fails. This is the POSIX standard terminal width.
- **Config subcommands skip detection.** Commands under `config` skip `PersistentPreRunE` entirely. `config discover` handles its own output without tableprinter (simple `fmt.Fprintf`).
- **Wrapper function, not interface.** `NewTable()` is a simple factory, not an abstraction layer. All callers use `tableprinter.TablePrinter` directly.

## Gotchas

- `tableprinter.New()` behavior changes based on `isTTY`: TTY mode truncates columns to fit; piped mode outputs untruncated TSV. Tests must account for both.
- `term.Size()` can return an error on non-TTY outputs. Always check the error and fall back.
- `tableprinter.AddField()` accepts `...fieldOption` functional options, not `...interface{}`. Pass color functions, not raw values.

## Files

- `cmd/root.go` — `Deps.IsTTY`, `Deps.TermWidth`, terminal detection in `PersistentPreRunE`
- `internal/format/table.go` — `NewTable()` wrapper
- `internal/format/pretty.go` — Release pretty formatter using tableprinter
- `internal/format/scope.go` — Scope pretty formatter using tableprinter
- `cmd/cycletime.go` — Key-value tableprinter for PR and issue cycle-time
- `cmd/leadtime.go` — Key-value tableprinter for lead-time
- `scripts/smoke-test.sh` — Bash arithmetic fix, updated output assertions
