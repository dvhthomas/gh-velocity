# Brainstorm: Separating Results Format from Process Output

**Date:** 2026-03-16
**Status:** Draft

## Problem

`--format json` currently serves double duty:
1. Changes the report data format (JSON instead of pretty/markdown)
2. Suppresses stderr (`log.SuppressStderr = true`) because agents need clean stdout

This conflates two orthogonal concerns. An agent might need markdown output (because the human asked for it) AND structured JSON diagnostics (for the agent to reason about the run). A CI pipeline might need both JSON and markdown artifacts from a single data-gathering pass. Today, getting both formats requires running the command twice, doubling all API calls.

## What We're Building

Three orthogonal axes for controlling CLI output:

### 1. `--results <format>[,<format>]` (replaces `--format`)
What format(s) for the report data.
- Values: `pretty`, `markdown` (`md`), `json`
- Default: `pretty`
- Multiple values allowed: `--results md,json`

### 2. `--output text|json`
How the process communicates about what it's doing (encoding, not verbosity).
- `text` (default): human-readable stderr as today (`[debug]`, `warning:`, etc.)
- `json`: structured JSON lines on stderr with the same information content as text mode (queries, timing, warnings, errors, cache stats). Agents are first-class citizens with the same information as humans.

`--debug` remains independent — it controls verbosity (level), not encoding (format). `--output json --debug` gives verbose structured diagnostics. `--output json` without `--debug` gives normal-level structured diagnostics. The two axes are orthogonal: encoding x verbosity.

### 3. `--write-to <dir>`
Where result files are written.
- When set: all `--results` formats are written as files to this directory (e.g., `report.md`, `report.json`). **Nothing goes to stdout** for results.
- When not set and single result format: goes to stdout (today's behavior).
- When not set and multiple result formats: error — must specify `--write-to`.

## Why This Approach

**Format rendering is pure.** Once data is gathered (API calls) and stats are computed, writing to different formatters takes microseconds and never requires additional API work. This is already proven by the `--artifact-dir` implementation on `report`, which writes both JSON and markdown from the same `StatsResult`. This design generalizes that to all commands.

**Agents are first-class.** `--output json` gives agents structured diagnostics on stderr regardless of what report format was requested. No more "json mode suppresses all stderr" hack.

**CI gets everything in one pass.** `--results md,json --write-to ./artifacts` produces all needed files from a single data-gathering + compute pass.

## Key Decisions

1. **Clean break from `--format`.** Rename to `--results` immediately, update all docs/tests/scripts. No deprecation alias — the codebase is young enough for a clean rename.

2. **`--output json` has full parity with `--output text`.** Same information, structured encoding. Not a subset or superset. Agents see everything humans see.

3. **`--write-to` silences stdout for results.** When writing to files, stdout is empty (for results). Process output (`--output`) always goes to stderr regardless.

4. **Rendering is always zero-API.** The render phase reads from in-memory domain types (`StatsResult`, pipeline results). It must never trigger API calls. This is a hard architectural invariant.

## Interaction Matrix

| Scenario | `--results` | `--output` | `--write-to` | stdout | stderr | files |
|----------|-------------|------------|---------------|--------|--------|-------|
| Human default | `pretty` | `text` | (none) | pretty report | debug/warnings | none |
| Agent wants markdown | `md` | `json` | (none) | markdown report | JSON diagnostics | none |
| CI showcase | `md,json` | `text` | `./artifacts` | (empty) | text logs | report.md, report.json |
| Agent + artifacts | `md,json` | `json` | `./out` | (empty) | JSON diagnostics | report.md, report.json |
| Simple JSON pipe | `json` | `text` | (none) | JSON report | text logs | none |

## Implementation Sketch (high-level)

1. **Replace `format.Format` with `[]format.Format`** — parse comma-separated `--results` into a slice.
2. **New `output.Mode` type** — `text` or `json`. Controls how `log.Debug/Warn/Error` encode their output.
3. **Remove `SuppressStderr`** — replaced by `--output` mode. JSON output mode writes structured lines; text mode writes human text. Neither suppresses. `--debug` controls verbosity independently.
4. **Generalize `writeReportArtifacts`** — every command's render phase writes to `--write-to` when set, using all requested formats.
5. **Update Pipeline.Render** — accept `[]format.Format` and a write target (stdout or dir).

## Migration

- Replace `--format`/`-f` with `--results`/`-r` in all persistent flags
- Update all smoke tests, showcase script, workflow files, CLAUDE.md examples
- Update `format.ParseFormat()` to `format.ParseResults()` returning a slice
- Update `handleError()` to use `--output` mode instead of checking `--format json`

## Resolved Questions

1. **Flag naming**: `--results` confirmed. Pairs well with `--output` and `--write-to`.
2. **`--output json` schema**: Informal, evolving. JSON lines with best-effort structure. Agents parse what they need. Schema emerges over time as patterns stabilize.
3. **Per-command file naming**: Full path convention. `flow-lead-time.md`, `flow-cycle-time.md`, `quality-release.md`, `report.md`. Includes parent group for clarity.
4. **Post integration**: `--post` uses the first `--results` format. User controls what gets posted. `--post --results md,json` posts markdown.
