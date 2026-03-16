---
title: "feat: Separate --results, --output, and --write-to flags"
type: feat
status: active
date: 2026-03-16
origin: docs/brainstorms/2026-03-16-output-results-separation-brainstorm.md
---

# feat: Separate --results, --output, and --write-to flags

## Enhancement Summary

**Deepened on:** 2026-03-16
**Sections enhanced:** 8
**Research agents used:** architecture-strategist, agent-native-reviewer, code-simplicity-reviewer, best-practices-researcher, 3 learnings agents

### Key Improvements
1. **Pipeline.Render stays unchanged** — loop externally, no interface change needed
2. **YAGNI checkpoint** — Phase 3 (--output json) deferred until a concrete agent consumer exists; SuppressStderr cleanup moved to Phase 1
3. **Agent-native enhancements** — optional `code` field in diagnostic lines, `WRITE_COMPLETE` receipt, schema version field
4. **Migration approach** — use Cobra's `MarkDeprecated` for `--format` bridge instead of hard break
5. **--post prefers markdown** — select markdown from results list if present, not blind "first value"

### YAGNI Decision

The simplicity reviewer raised a strong objection: `--output json` (Phase 3) has zero concrete consumers today. The brainstorm's "agents are first-class" goal is real, but building a structured logging framework for hypothetical agents violates YAGNI. **Decision: Phase 3 is deferred.** Phases 1-2 deliver the concrete value (multi-format output, SuppressStderr cleanup). Phase 3 ships when an agent integration needs it.

---

## Overview

Replace the overloaded `--format` flag with orthogonal flags that cleanly separate what the CLI produces and where output goes. This makes agents, humans, and CI pipelines first-class consumers without special-casing.

## Problem Statement / Motivation

`--format json` currently serves double duty: it changes the report data format AND suppresses stderr (`log.SuppressStderr = true`). This conflates data rendering with diagnostic encoding. A CI pipeline needs both JSON and markdown artifacts from a single data-gathering pass. Today, getting both formats requires running the command twice, doubling all API calls.

(see brainstorm: `docs/brainstorms/2026-03-16-output-results-separation-brainstorm.md`)

## Proposed Solution

### `--results` / `-r` (replaces `--format` / `-f`)

What format(s) for the report data.

- Values: `pretty`, `markdown` (alias `md`), `json`
- Default: `pretty`
- Multiple values: `--results md,json` (comma-separated)
- Short flag: `-r`

#### Research Insights: Flag Parsing

**Best practice:** Use pflag's `StringSliceVar` which automatically splits on commas. `--results md,json` and `--results md --results json` both produce `[]string{"md", "json"}`. Format values (`json`, `md`, `pretty`) are simple enums with no commas, so the known comma-splitting edge case ([pflag#57](https://github.com/spf13/pflag/issues/57)) is irrelevant.

**Alternative:** Custom `pflag.Value` type (20 lines) for parse-time validation that rejects `--results bogus` immediately. This is the pattern Helm and kubectl use for enum-style flags.

### `--write-to` (new, replaces `--artifact-dir`)

Where result files are written.

- When set: all `--results` formats written as files. **Nothing to stdout** for results.
- When not set + single format: goes to stdout (today's behavior).
- When not set + multiple formats: **error** — must specify `--write-to`. Error message: `"multiple --results formats require --write-to <dir>"`.
- Creates directory with `MkdirAll` if it doesn't exist.
- `pretty` format is **disallowed** with `--write-to` (terminal-only format). Returns `AppError{Code: ErrConfigInvalid}`.
- File naming convention: full command path below root, joined by hyphens — `report.md`, `flow-lead-time.json`, `quality-release.md`, `risk-bus-factor.json`, `status-reviews.md`, `status-my-week.json`.
- Overwrites existing files silently (CI-friendly).

#### Research Insights: Write-to

**WRITE_COMPLETE receipt (agent-native):** When `--write-to` is active, emit a debug-level log line listing files created: `artifacts written to ./out (report.json, report.md)`. This is already done by the existing `writeReportArtifacts` function. Agents parsing `--output json` (future Phase 3) would get this as a structured receipt.

**Atomicity:** Write to temp files and rename for atomic updates. Matches the existing `diskcache.go` pattern.

### `--output text|json` (deferred to Phase 3)

How the process communicates about what it's doing (encoding, not verbosity). **Deferred until a concrete agent consumer exists.** See YAGNI Decision above.

When eventually implemented:
- `text` (default): human-readable stderr as today
- `json`: structured JSON lines on stderr with `{"v":1, "level":"warn", "code":"RATE_LIMITED", "message":"...", "ts":"..."}`
- `--debug` remains independent (verbosity level)
- `--output json` overrides GitHub Actions `::warning::` formatting

## Technical Considerations

### Interaction Matrix (Phases 1-2)

| Scenario | `--results` | `--write-to` | `--debug` | stdout | stderr | files |
|----------|-------------|--------------|-----------|--------|--------|-------|
| Human default | `pretty` | — | off | pretty report | (quiet) | — |
| Human verbose | `pretty` | — | on | pretty report | debug lines | — |
| Agent wants JSON | `json` | — | off | JSON report | (quiet) | — |
| CI showcase | `md,json` | `./artifacts` | on | (empty) | text logs | report.md, report.json |
| Simple JSON pipe | `json` | — | off | JSON report | (quiet) | — |

### Pipeline.Render strategy — loop externally, no interface change

**Decision:** Do NOT change the `Pipeline` interface. The multi-sink orchestration happens in the caller. Call `Render()` once per format with a different `RenderContext` each time. Each `Render` implementation continues to switch on `rc.Format` — zero changes to the 10 pipeline implementations.

The multi-format helper function (in `cmd/` or `internal/format/`) iterates formats, builds the appropriate `RenderContext` per format (file writer or stdout), and calls `p.Render(rc)` for each. This is a ~30-line function, not a new abstraction layer.

### --post interaction

- `--post` without explicit `--results` coerces default from `pretty` to `markdown` (maintains today's behavior).
- `--post` **prefers markdown** from the results list if present, rather than blindly using the first value. This avoids accidentally posting raw JSON to GitHub when the user writes `--results json,md`. If markdown is not in the list, falls back to the first value with a warning.
- `--post` captures from an in-memory buffer, independent of stdout and `--write-to`. The render phase writes to independent sinks: (a) post buffer (always markdown if available), (b) file(s), (c) stdout. These are decoupled — no more `io.MultiWriter` tee.

### handleError changes

- `handleError()` continues to check `--results` (not `--output`) to decide error encoding on stderr. When `--results json` is the sole format, errors go to stderr as structured `ErrorEnvelope` JSON. This maintains current behavior.
- In Phase 3, `handleError` will switch to checking `--output` instead.

### Warnings and SuppressStderr cleanup

- **Remove `SuppressStderr` global.** Warnings always go to stderr in text mode (matching today's behavior for pretty/markdown).
- **Remove `WarnUnlessJSON`.** Replace all 15 call sites with direct `log.Warn()`. Warnings always appear on stderr AND in `--results json` payloads (`StatsResult.Warnings`).
- **For `--results json` (single value, stdout):** Keep current behavior where stderr warnings are suppressed to avoid corrupting the JSON stream that agents parse. But now this is handled by the render orchestration layer, not a global flag: when the only output destination is stdout and the format is JSON, suppress stderr warnings. When `--write-to` is set, stderr is always active (stdout is empty, so no corruption risk).

#### Research Insight: Dual-path documentation

Document in AGENTS.md: "Warnings appear in both stderr (when active) and result JSON `warnings` array. The result JSON is the authoritative source for data-related warnings. Stderr includes operational warnings (rate limits, retries) that may not appear in the result payload."

### Migration approach for --format

**Use `MarkDeprecated` bridge, not hard break.** Research shows this is the idiomatic Cobra pattern and what the codebase already uses for `--project` → `--project-url`:

1. Register `--results` as the new persistent flag
2. Keep `--format` with `cmd.Flags().MarkDeprecated("format", "use --results/-r instead")`
3. In `PersistentPreRunE`, if `--format` was `Changed` but `--results` was not, copy the value
4. If both are set, error with a clear message

This gives existing users a smooth migration path with actionable deprecation messages, rather than breaking all scripts at once.

### Output config grouping

Group the output-related fields into a nested struct within `Deps`:

```go
type OutputConfig struct {
    Results  []format.Format
    WriteTo  string
    // OutputMode output.Mode  // Phase 3
}
```

This keeps `Deps` from growing unboundedly and makes it easy to pass output config to the render helper.

### Provenance

New flags must be captured in Provenance output so consumers can reproduce exact invocations. The existing `cmd.Flags().Visit()` pattern automatically captures explicitly-set flags. Verify `--results` and `--write-to` appear in Provenance when set.

## System-Wide Impact

### Interaction graph

`PersistentPreRunE` → parses `--results` into `[]format.Format`, `--write-to` into validated dir path → stored in `Deps.OutputConfig` → flows to render orchestration → calls `Pipeline.Render(rc)` once per format with per-sink `RenderContext`.

### Error propagation

- `--write-to` directory creation failures: `AppError{Code: ErrConfigInvalid}` with clear message.
- `pretty` + `--write-to`: `AppError{Code: ErrConfigInvalid}`.
- Multi-format without `--write-to`: `AppError{Code: ErrConfigInvalid}`, exit code 2.
- Render-phase file write errors: fatal (command returns non-zero).

### API surface parity

Every command that currently accepts `--format` will accept `--results` and `--write-to`. The `Pipeline.Render` interface is unchanged. The multi-format loop lives in the calling layer.

### Integration test scenarios

1. `--results md,json --write-to ./out` produces both files, empty stdout, correct content in each.
2. `--post --results md,json --write-to ./out` posts markdown AND writes both files.
3. `--results md,json` without `--write-to` returns error exit code 2.
4. `--format json` produces deprecation warning and still works.
5. `--results json` without `--write-to` suppresses stderr warnings (JSON stream purity).
6. `--results json --write-to ./out` does NOT suppress stderr (stdout is empty, no corruption risk).
7. Provenance includes `--results` and `--write-to` when set.

## Acceptance Criteria

### Functional Requirements

- [ ] `--results`/`-r` registered as persistent flag. Accepts comma-separated values via `StringSliceVar`.
- [ ] `--format`/`-f` kept as deprecated alias with bridge in `PersistentPreRunE`.
- [ ] `--results` with single value sends output to stdout (when `--write-to` not set).
- [ ] `--results` with multiple values requires `--write-to` (error otherwise).
- [ ] `--write-to <dir>` writes all result formats as files, silences stdout.
- [ ] `--write-to` rejects `pretty` format with `AppError{Code: ErrConfigInvalid}`.
- [ ] `--write-to` creates directory if it doesn't exist.
- [ ] File naming follows full command path: `flow-lead-time.md`, `report.json`, etc.
- [ ] `--post` prefers markdown from results list; falls back to first value with warning.
- [ ] `--post` uses independent in-memory buffer (decoupled from stdout and `--write-to`).
- [ ] `--artifact-dir` removed from report command (replaced by `--write-to`).
- [ ] `SuppressStderr` global removed from `internal/log`.
- [ ] `WarnUnlessJSON` removed; all 15 call sites replaced with `log.Warn()`.
- [ ] Warnings embedded in `--results json` payloads (`StatsResult.Warnings`) regardless.
- [ ] `--results json` to stdout suppresses stderr warnings (stream purity).
- [ ] `--results json --write-to` does NOT suppress stderr (no corruption risk).
- [ ] Provenance captures `--results` and `--write-to` flags.
- [ ] `Pipeline.Render` interface unchanged — multi-format loop is external.

### Testing Requirements

- [ ] All 24 smoke tests updated from `-f`/`--format` to `-r`/`--results`.
- [ ] Smoke tests also verify `--format` still works with deprecation warning.
- [ ] Unit tests for `ParseResults()` (single, multi, dedup, invalid, pretty-with-write-to rejection).
- [ ] Unit tests for `handleError` using `--results` for error encoding.
- [ ] Integration test: multi-format `--write-to` produces correct files.
- [ ] Integration test: `--results json` to stdout produces pure JSON (no stderr leakage).
- [ ] Showcase script updated and tested.
- [ ] Verify `WarnUnlessJSON` does not appear in any `.go` file after Phase 1.

### Documentation Requirements

- [ ] `AGENTS.md` updated with new flags and warnings dual-path documentation.
- [ ] All Hugo site guides updated (`agent-integration.md`, `recipes.md`, `quick-start.md`, `troubleshooting.md`, etc.).
- [ ] `README.md` flag table updated.
- [ ] Command examples in all `cmd/*.go` files updated.

## Implementation Phases

### Phase 1: Rename --format to --results + SuppressStderr cleanup

**Goal:** `--results` replaces `--format` with deprecation bridge. Fix the `SuppressStderr` hack. Single-format mode works identically to today.

**Files:**
- `cmd/root.go` — add `--results`/`-r` flag via `StringSliceVar`, deprecate `--format`, bridge in `PersistentPreRunE`, update `Deps` with `OutputConfig` struct, remove `SuppressStderr = true` line, remove `WarnUnlessJSON` method
- `internal/format/formatter.go` — `ParseResults()` returning `[]format.Format` with dedup and validation
- `internal/log/log.go` — remove `SuppressStderr` global. Warnings always go to stderr.
- `internal/log/log_test.go` — remove SuppressStderr test
- `cmd/root_test.go` — update `newTestRoot`, handleError tests, remove WarnUnlessJSON tests
- 15 `WarnUnlessJSON` call sites → `log.Warn()`

**Acceptance:** `--results json`, `--results md`, `--results pretty` all work. `--format json` works with deprecation warning. SuppressStderr removed. Warnings go to stderr in all modes (and remain in JSON payloads).

### Phase 2: --write-to and multi-format rendering

**Goal:** `--results md,json --write-to ./out` produces files from a single data-gathering pass.

**Files:**
- `cmd/root.go` — add `--write-to` persistent flag, validate in `PersistentPreRunE` (reject `pretty`, require `--write-to` for multi-format)
- `cmd/render.go` (new, ~30 lines) — multi-format render helper: iterates formats, builds per-sink `RenderContext`, calls `p.Render(rc)` in a loop. Handles stdout vs file routing.
- `cmd/report.go` — remove `--artifact-dir` and `writeReportArtifacts`, use shared render helper with `--write-to`
- `cmd/post.go` — decouple post buffer from stdout; render to independent in-memory buffer, prefer markdown
- Every `cmd/*.go` command — wire through render helper (replaces direct `switch deps.Format` + `p.Render(rc)` calls)

**Note:** Pipeline.Render interface is UNCHANGED. No changes to any `internal/pipeline/*/` file.

**Acceptance:** `--results md,json --write-to ./out` writes both files, empty stdout. `--post --write-to ./out` posts markdown. Single-format commands unchanged.

### Phase 3: --output mode (DEFERRED)

**Goal:** `--output json` gives agents structured JSON diagnostics on stderr.

**Status:** Deferred until a concrete agent consumer exists. When implemented:
- Add `--output text|json` persistent flag
- Add `output.Mode` to `Deps.OutputConfig`
- Update `internal/log` to encode based on mode
- `handleError` checks `--output` instead of `--results`
- JSON line schema: `{"v":1, "level":"warn", "code":"RATE_LIMITED", "message":"...", "ts":"..."}`

### Phase 4: Migration sweep

**Goal:** All docs, tests, scripts, and workflows updated.

**Files:**
- `scripts/smoke-test.sh` — 24 `-f`/`--format` → `-r`/`--results`, add `--format` deprecation tests
- `scripts/showcase.sh` — update flags, replace `--artifact-dir` with `--write-to`
- `.github/workflows/showcase.yaml` — verify no flag references
- `.github/workflows/velocity.yaml` — update `--debug` usage if needed
- `AGENTS.md` — update flag documentation, add warnings dual-path docs
- `README.md` — update flag table
- All `site/content/**/*.md` — update flag references
- All `cmd/*.go` Example strings — update `-f` to `-r`

**Acceptance:** `task quality` passes. Smoke tests pass. Hugo site builds. `WarnUnlessJSON` grep returns zero results.

## Alternative Approaches Considered

1. **Keep `--format` and add `--write-to` alongside it.** Rejected: `--format json` still triggers `SuppressStderr`. Renaming breaks the conflation.

2. **Hard break: remove `--format` entirely.** Rejected after research: Cobra's `MarkDeprecated` provides a smooth migration path at zero cost. The codebase already uses this pattern for `--project` → `--project-url`.

3. **Multi-format for `report` only, not all commands.** Considered (simplicity reviewer's recommendation): keep `--write-to` as report-only. Decided against because the implementation cost of making it persistent is minimal (the render helper works for all commands) and individual commands benefit from `--write-to` for CI artifact generation.

4. **Implement `--output json` now (Phase 3).** Rejected per YAGNI: no concrete agent consumer exists today. Warnings already embedded in JSON payloads provide self-containment. Defer until needed.

(see brainstorm: `docs/brainstorms/2026-03-16-output-results-separation-brainstorm.md`)

## Dependencies & Risks

- **Risk:** Large surface area (24 smoke tests, ~30 doc pages, 15 WarnUnlessJSON sites). Mitigation: phased approach, Phase 1 is a rename + cleanup with no new features.
- **Risk:** `--post` decoupling from stdout is the most architecturally complex change. Mitigation: Phase 2 tackles it with explicit independent-buffer design.
- **Risk:** Deprecation bridge for `--format` adds temporary complexity. Mitigation: remove after 2-3 releases once migration is complete.
- **Risk:** Adjacent test code may break during migration (learned from tableprinter migration). Mitigation: update smoke tests proactively in Phase 4, check for `((FAIL++))` under `set -e` bash issues.

## Success Metrics

- Single `report` invocation with `--results md,json --write-to ./artifacts` produces both files (showcase uses this).
- No command ever makes an API call during the render phase.
- `--format json` still works with deprecation warning (smooth migration).
- All existing human workflows work unchanged (with `--results` replacing `--format`).

## Sources & References

### Origin

- **Brainstorm document:** [docs/brainstorms/2026-03-16-output-results-separation-brainstorm.md](docs/brainstorms/2026-03-16-output-results-separation-brainstorm.md) — Key decisions: three orthogonal axes, --debug as independent verbosity, file naming with full command path.

### Internal References

- Complete JSON output pattern: `docs/solutions/architecture-patterns/complete-json-output-for-agents.md`
- Command output shape & Provenance: `docs/solutions/architecture-patterns/command-output-shape.md`
- Terminal detection pattern (single detection point in PersistentPreRunE): `docs/solutions/go-gh-tableprinter-migration.md`
- Orthogonal field design: `docs/solutions/three-state-metric-status-pattern.md`
- Flag registration: `cmd/root.go:292`
- Format type: `internal/format/formatter.go:24-31`
- SuppressStderr: `internal/log/log.go:12-15`
- handleError: `cmd/root.go:113-141`
- Pipeline interface: `internal/pipeline/pipeline.go:30-32`
- Artifact-dir prototype: `cmd/report.go:330-360`
- WarnUnlessJSON: `cmd/root.go:75-82` (15 call sites across 5 files)
- MarkDeprecated precedent: `cmd/preflight.go:157-158`
- Smoke tests: `scripts/smoke-test.sh` (24 format flag invocations)

### External References

- pflag StringSliceVar comma handling: [pflag#57](https://github.com/spf13/pflag/issues/57)
- Custom Cobra flag types: [Applejag blog](https://applejag.eu/blog/go-spf13-cobra-custom-flag-types/)
- gh CLI stderr convention: [cli/cli#7447](https://github.com/cli/cli/issues/7447)
- Go structured logging: [go.dev/blog/slog](https://go.dev/blog/slog)

### Related Work

- PR #69: Disk cache + N+1 fix (enables zero-API second render)
- PR #71: Showcase JSON + Markdown artifacts (first multi-format need)
- PR #72: Report-only showcase + `--artifact-dir` prototype
- PR #73: Fail-fast discussion permissions
