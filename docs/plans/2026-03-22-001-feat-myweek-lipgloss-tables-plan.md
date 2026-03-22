# Plan: Add lipgloss tables to my-week pretty output

**Complexity:** Quick
**Risk:** Low — only changes pretty output rendering, no data/API changes

## Context

`gh velocity status my-week` pretty output uses plain `fmt.Fprintf` for item lists while every other command (velocity, lead-time, cycle-time, reviews, wip) uses lipgloss tables via `format.NewTable()`. This makes my-week look inconsistent.

## Approach

Use `format.NewTable()` (the existing wrapper in `internal/format/table.go`) for all item-list sections in `WriteMyWeekPretty`. This wrapper already handles:
- TTY detection (lipgloss tables vs TSV fallback)
- OSC 8 hyperlink sanitization (critical — lipgloss can't handle OSC 8)
- Terminal width constraints
- Bold headers, rounded borders, consistent styling

### Sections to convert to tables

1. **Waiting on — PRs Waiting for Review**: columns `#`, `Title`, `Age`
2. **Waiting on — Stale Issues**: columns `#`, `Title`, `Idle`
3. **What I shipped — Issues Closed**: columns `#`, `Date`, `Title`
4. **What I shipped — PRs Merged**: columns `#`, `Date`, `Title` (with `[ai]` suffix)
5. **What I shipped — PRs Reviewed**: columns `#`, `Title`
6. **What's ahead — Open Issues**: columns `#`, `Title`, `Status`
7. **What's ahead — Open PRs**: columns `#`, `Title`, `Status`
8. **Review queue — Awaiting Your Review**: columns `#`, `Title`, `Author`, `Age`

### What stays as text (not tables)
- Header (login, repo, date range) — single-line metadata
- Insights section — bullet-point prose, not tabular data
- Section headers (`── Insights ──`) — visual separators
- Zero-count sections with verify URLs
- "No activity" / "Nothing planned" messages
- Release list (1-2 items typically, not worth a table)

## Files changed

1. `internal/format/myweek.go` — rewrite item loops in `WriteMyWeekPretty` to use `format.NewTable()`
2. `internal/format/myweek_test.go` — update assertions (existing tests check for substring presence, most should still pass since the content is the same, just formatted differently)

## Test plan

- `task test` — all unit tests pass
- Existing `TestWriteMyWeekPretty` assertions check for content substrings — table formatting preserves the content, just wraps it in borders
- Manual: `gh velocity status my-week` shows lipgloss tables in TTY
