---
title: "feat: Separate --results, --write-to flags (replacing --format)"
type: feat
status: active
date: 2026-03-18
origin: docs/brainstorms/2026-03-16-output-results-separation-brainstorm.md
supersedes: docs/plans/2026-03-16-001-feat-results-output-writeto-flag-separation-plan.md
---

# feat: Separate --results and --write-to flags

## Overview

Replace the overloaded `--format` flag with orthogonal flags that separate what the CLI produces from where output goes. `--results` controls format(s), `--write-to` controls file destination. `--output json` (structured diagnostics) is deferred per YAGNI.

This plan supersedes the 2026-03-16 plan after codebase validation and flow analysis surfaced critical gaps around stderr suppression mechanics, per-section artifact continuity, and Render idempotency.

(see origin: `docs/brainstorms/2026-03-16-output-results-separation-brainstorm.md`)

## Problem Statement

`--format json` serves double duty: it changes the report data format AND suppresses stderr (`log.SuppressStderr = true`). A CI pipeline that needs both JSON and markdown artifacts must run the command twice, doubling all API calls. The `--artifact-dir` flag on `report` already proved multi-format rendering from a single pass works — this generalizes that to all commands.

## Proposed Solution

### `--results` / `-r` (replaces `--format` / `-f`)

What format(s) for the report data.

- Values: `pretty`, `markdown` (alias `md`), `json`
- Default: `pretty`
- Multiple values: `--results md,json` (comma-separated via `StringSliceVar`)
- Deduplication: `--results md,md,json` silently becomes `[markdown, json]`
- Parse-time validation rejects unknown values immediately

### `--write-to <dir>`

Where result files are written.

- When set: all `--results` formats written as files. **Nothing to stdout** for results.
- When not set + single format: goes to stdout (today's behavior).
- When not set + multiple formats: error — `"multiple --results formats require --write-to <dir>"` (exit code 2).
- `pretty` format disallowed with `--write-to` — returns `AppError{Code: ErrConfigInvalid}`.
- Creates directory with `MkdirAll` if it doesn't exist. **Validation in `PersistentPreRunE`** — fail fast before API calls.
- File naming: full command path below root, joined by hyphens — `report.md`, `flow-lead-time.json`, `quality-release.md`.
- Overwrites existing files silently (CI-friendly).
- Atomic writes via temp file + rename (matches `diskcache.go` pattern).

### `--output text|json` (DEFERRED)

Structured diagnostics for agents. Deferred until a concrete consumer exists per YAGNI. Phases 1-2 deliver all concrete value.

### --format removal (clean break)

**Decision: Clean break, not deprecation bridge.**

The project is pre-1.0. Both the original brainstorm and institutional learnings (`docs/solutions/architecture-refactors/cobra-command-hierarchy-thematic-grouping.md`) say "pre-1.0: prefer clean breaks over deprecated aliases." A `MarkDeprecated` bridge adds complexity and creates a conflict: Cobra's deprecation warning on stderr breaks JSON stream purity for existing `--format json` consumers — the exact scenario the bridge is meant to prevent.

Clean break: remove `--format`/`-f` entirely. Replace with `--results`/`-r`. Update all docs, tests, scripts in the same phase. No deprecated alias, no bridge logic, no `Changed("format")` checks.

## Technical Considerations

### Stderr Suppression: Replacing SuppressStderr

**This is the architectural linchpin.** The 2026-03-16 plan said "remove SuppressStderr, render orchestration layer handles it." SpecFlow analysis revealed this is insufficient: warnings are emitted during `GatherData` and `ProcessData` (15 `WarnUnlessJSON` call sites), not just during Render.

**Resolution: Replace the global with a narrower, scoped condition on `Deps`.**

```go
// In Deps (cmd/root.go)
type OutputConfig struct {
    Results       []format.Format
    WriteTo       string
    SuppressWarn  bool // true when JSON is sole result format going to stdout
}
```

Set in `PersistentPreRunE`:
```go
cfg.SuppressWarn = len(results) == 1 && results[0] == format.JSON && writeTo == ""
```

Replace `log.SuppressStderr` global with `deps.Output.SuppressWarn` checks. Replace all 15 `deps.WarnUnlessJSON()` calls with:
```go
if !deps.Output.SuppressWarn {
    log.Warn(msg)
}
```

This is functionally identical to today's behavior but:
- Not a global — scoped to the command's `Deps`
- More precise condition — only when JSON goes to stdout (not when `--write-to` is set)
- Named for what it does (`SuppressWarn`) not what triggers it (`SuppressStderr`)

When `--write-to` is set, stdout is empty so stderr is always active — no corruption risk.

### --debug interaction

`--debug` controls verbosity (level), not encoding. When `SuppressWarn` is true (JSON to stdout), `--debug` output still goes to stderr. This breaks strict JSON stream purity but is intentional: `--debug` is an explicit opt-in to diagnostic noise. Document this trade-off.

### Interaction Matrix (Phases 1-2)

| Scenario | `--results` | `--write-to` | `--debug` | stdout | stderr | files |
|----------|-------------|--------------|-----------|--------|--------|-------|
| Human default | `pretty` | — | off | pretty report | quiet | — |
| Human verbose | `pretty` | — | on | pretty report | debug + warnings | — |
| Agent JSON | `json` | — | off | JSON report | quiet (SuppressWarn) | — |
| Agent JSON debug | `json` | — | on | JSON report | debug lines only | — |
| CI multi-format | `md,json` | `./out` | on | empty | debug + warnings | *.md, *.json |
| Single md | `md` | — | off | markdown | warnings | — |

### Pipeline.Render — loop externally, no interface change

Call `Render()` once per format with a different `RenderContext`. Each pipeline's Render continues to switch on `rc.Format`.

**Render idempotency requirement:** Multi-format requires Render to be safe for multiple calls. Add a test that calls Render twice per pipeline and asserts identical output. Current implementations read from internal state without mutation — but this must be verified, not assumed.

### --post interaction

- `--post` without explicit `--results` coerces default from `pretty` to `markdown`.
- **`--post` requires markdown in the results list.** If markdown is not present, return `AppError{Code: ErrConfigInvalid}` with message: `"--post requires markdown in --results (got: json)"`. This prevents silently posting JSON blobs and avoids the invisible-warning problem (stderr may be suppressed).
- `--post` renders to an independent in-memory buffer, decoupled from stdout and `--write-to`. No more `io.MultiWriter` tee.
- `--post --results md,json --write-to ./out`: posts markdown, writes both files, stdout empty.

**Migration detail:** The current coercion at `cmd/root.go:180` checks `cmd.Flags().Changed("format")`. With the clean break, this becomes `cmd.Flags().Changed("results")`. No bridge logic needed.

### handleError changes

- `handleError()` checks `--results` (not `--output`) to decide error encoding. When `results` is `[json]` and `writeTo` is empty, errors go to stderr as structured `ErrorEnvelope` JSON.
- Current code at `cmd/root.go:127` does `root.PersistentFlags().GetString("format")` — must change to read from `Deps.Output.Results`.

### Report command: per-section artifacts

**`--write-to` on `report` preserves per-section artifact behavior.** When `--write-to` is set on `report`, it writes:
1. Top-level files: `report.json`, `report.md` (per `--results`)
2. Per-section files: `flow-lead-time.json`, `flow-lead-time.md`, etc. (same as today's `--artifact-dir`)

This is report-specific behavior, not part of the generic `--write-to` contract. Other commands write only their single output in each requested format.

**Report rendering is NOT pipeline-based.** It has custom rendering logic (`cmd/report.go:315-400`). The multi-format render helper must accommodate this — either report calls the helper differently, or report keeps its own multi-format loop using the existing `writeReportArtifacts` pattern adapted for `--write-to`.

### Provenance

New flags captured automatically by the existing `cmd.Flags().Visit()` pattern. Verify `--results` and `--write-to` appear in Provenance when set.

### Warnings dual-path

Warnings appear in both stderr (when `SuppressWarn` is false) and in result JSON `warnings` arrays. The result JSON is authoritative for data-related warnings. Stderr includes operational warnings (rate limits, retries) that may not appear in the result payload. Document this in AGENTS.md.

## System-Wide Impact

### Interaction graph

`PersistentPreRunE` → parses `--results` into `[]format.Format`, `--write-to` into validated dir → `MkdirAll` if needed → stored in `Deps.OutputConfig` → `SuppressWarn` computed → flows to render orchestration → calls `Render(rc)` once per format with per-sink `RenderContext`.

### Error propagation

- `--write-to` dir creation failure: `AppError{Code: ErrConfigInvalid}` in PersistentPreRunE (fail fast)
- `pretty` + `--write-to`: `AppError{Code: ErrConfigInvalid}`
- Multi-format without `--write-to`: `AppError{Code: ErrConfigInvalid}`, exit code 2
- `--post` without markdown in `--results`: `AppError{Code: ErrConfigInvalid}`
- Render-phase file write errors: fatal (non-zero exit)
- Partial file write: use temp file + rename for atomicity

### API surface parity

Every command that currently accepts `--format` accepts `--results` and `--write-to`. Pipeline.Render interface unchanged. Multi-format loop lives in the calling layer.

## Acceptance Criteria

### Functional Requirements

- [ ] `--results`/`-r` registered as persistent flag via `StringSliceVar`, default `["pretty"]`
- [ ] `--format`/`-f` removed entirely (clean break)
- [ ] `--results` with single value sends output to stdout (when `--write-to` not set)
- [ ] `--results` with multiple values requires `--write-to` (error otherwise)
- [ ] `--write-to <dir>` writes all result formats as files, silences stdout
- [ ] `--write-to` rejects `pretty` format with `AppError{Code: ErrConfigInvalid}`
- [ ] `--write-to` creates directory if needed, validated in `PersistentPreRunE`
- [ ] `--write-to` uses atomic writes (temp file + rename)
- [ ] File naming: full command path below root joined by hyphens
- [ ] `--post` requires markdown in `--results`, error if absent
- [ ] `--post` uses independent in-memory buffer (decoupled from stdout/`--write-to`)
- [ ] `--artifact-dir` removed from report (replaced by `--write-to`)
- [ ] `report --write-to` preserves per-section artifact files
- [ ] `SuppressStderr` global removed, replaced by `Deps.Output.SuppressWarn`
- [ ] `WarnUnlessJSON` removed; all 15 call sites use conditional `log.Warn()`
- [ ] `--results json` to stdout suppresses warnings (SuppressWarn = true)
- [ ] `--results json --write-to` does NOT suppress warnings
- [ ] `--debug` always goes to stderr regardless of SuppressWarn
- [ ] Provenance captures `--results` and `--write-to` flags
- [ ] `Pipeline.Render` interface unchanged — multi-format loop is external
- [ ] `handleError` reads from `Deps.Output.Results`, not flag string literals
- [ ] `md` accepted as alias for `markdown`
- [ ] Duplicate formats in `--results` deduplicated silently

### Testing Requirements

- [ ] All smoke tests updated from `-f`/`--format` to `-r`/`--results`
- [ ] Negative smoke tests: `--format` returns "unknown flag" error
- [ ] Unit tests for `ParseResults()` (single, multi, dedup, alias, invalid, pretty+write-to rejection)
- [ ] Unit tests for `handleError` using `--results`
- [ ] Render idempotency test: every pipeline Render called twice, identical output
- [ ] Integration test: `--results md,json --write-to ./out` produces correct files
- [ ] Integration test: `--results json` to stdout produces pure JSON, no stderr
- [ ] Integration test: `--post --results json` returns validation error
- [ ] Integration test: `report --write-to` includes per-section artifacts
- [ ] Showcase script updated and tested

### Documentation Requirements

- [ ] AGENTS.md: new flags, warnings dual-path, `--debug` trade-off
- [ ] Hugo site guides updated (agent-integration, recipes, quick-start, troubleshooting)
- [ ] README.md flag table updated
- [ ] Command examples in all `cmd/*.go` files updated

## Implementation Phases

### Phase 1: `--results` replaces `--format` + SuppressStderr replacement

**Goal:** `--results`/`-r` works identically to today's `--format` for single-format mode. `--format` removed. SuppressStderr replaced with scoped `Deps.Output.SuppressWarn`.

**Files:**
- `cmd/root.go` — register `--results`/`-r` via `StringSliceVar`, remove `--format`/`-f`, add `OutputConfig` to `Deps`, replace `SuppressStderr = true` with `SuppressWarn` logic, update `handleError`, remove `WarnUnlessJSON`
- `internal/format/formatter.go` — `ParseResults()` returning `[]format.Format` with dedup, alias, validation
- `internal/log/log.go` — remove `SuppressStderr` global and its checks in `Warn()`/`Debug()`
- 15 `WarnUnlessJSON` call sites → conditional `log.Warn()` using `deps.Output.SuppressWarn`
  - `cmd/helpers.go` (2 sites)
  - `cmd/report.go` (5 sites)
  - `cmd/cycletime.go` (4 sites)
  - `cmd/release.go` (3 sites)
  - `cmd/myweek.go` (1 site)
- All `cmd/*.go` Example strings — update `-f`/`--format` to `-r`/`--results`
- Tests: update `cmd/root_test.go`, remove WarnUnlessJSON/SuppressStderr tests, add ParseResults tests

**Acceptance:** `--results json`, `--results md`, `--results pretty` all work. `--format` returns unknown flag error. SuppressWarn replaces SuppressStderr. Warnings go to stderr when SuppressWarn is false.

### Phase 2: `--write-to` and multi-format rendering

**Goal:** `--results md,json --write-to ./out` produces files from a single data-gathering pass.

**Sub-phase 2a: Generic render helper + simple commands**
- `cmd/root.go` — add `--write-to` persistent flag, validate in `PersistentPreRunE`
- `cmd/render.go` (new, ~40 lines) — multi-format render helper: iterates formats, builds per-sink `RenderContext`, calls `p.Render(rc)`. Handles stdout vs file routing with atomic writes.
- Wire all non-report commands through render helper

**Sub-phase 2b: Report command + --post decoupling**
- `cmd/report.go` — remove `--artifact-dir`, adapt `writeReportArtifacts` to use `--write-to`, preserve per-section artifact behavior
- `cmd/post.go` (or post logic in root) — decouple post buffer from stdout, require markdown in `--results`, independent in-memory buffer
- Add `--post --results json` validation error

**Acceptance:** `--results md,json --write-to ./out` writes both files, empty stdout. `report --write-to` includes per-section artifacts. `--post` works with decoupled buffer. Single-format commands unchanged.

### Phase 3: `--output` mode (DEFERRED)

Deferred until a concrete agent consumer exists. When implemented: `--output text|json` for structured diagnostics on stderr.

### Phase 4: Migration sweep

**Goal:** All docs, tests, scripts, and workflows updated.

**Files:**
- `scripts/smoke-test.sh` — ~24 `-f`/`--format` → `-r`/`--results`, add negative test for `--format`
- `scripts/showcase.sh` — update flags, replace `--artifact-dir` with `--write-to`
- `.github/workflows/showcase.yaml` — verify flags
- `AGENTS.md` — update flag docs, add warnings dual-path, debug trade-off
- `README.md` — update flag table
- All `site/content/**/*.md` — update flag references

**Acceptance:** `task quality` passes. Smoke tests pass. Hugo site builds. No `WarnUnlessJSON` or `SuppressStderr` in any `.go` file.

## Alternative Approaches Considered

1. **`MarkDeprecated` bridge for `--format`.** Rejected: pre-1.0 project, clean breaks preferred (per institutional learnings). Bridge creates complexity and Cobra's deprecation warning on stderr breaks JSON stream purity — defeating the bridge's purpose.

2. **Multi-format for `report` only.** Rejected: implementation cost of making `--write-to` persistent is minimal, and individual commands benefit from CI artifact generation.

3. **Remove SuppressStderr concept entirely.** Rejected by SpecFlow analysis: warnings during GatherData/ProcessData would corrupt JSON stdout. The concept survives as a scoped, narrower condition (`SuppressWarn`) rather than a global.

4. **`--post` falls back to first format with warning.** Rejected: warning may be invisible when stderr is suppressed. Cleaner to require markdown when `--post` is set.

5. **Implement `--output json` now.** Rejected per YAGNI: no concrete agent consumer.

(see origin: `docs/brainstorms/2026-03-16-output-results-separation-brainstorm.md`)

## Dependencies & Risks

- **Risk:** Large surface area (~24 smoke tests, ~30 doc pages, 15 WarnUnlessJSON sites). Mitigation: phased approach, Phase 1 is rename + cleanup with no new features.
- **Risk:** Render idempotency not currently tested. Mitigation: Phase 1 adds idempotency tests before multi-format wiring in Phase 2.
- **Risk:** Report's custom rendering is the most complex migration. Mitigation: Phase 2 split into 2a (simple commands) and 2b (report + post).
- **Risk:** Clean break for `--format` breaks any external scripts using it. Mitigation: pre-1.0 project, documented in release notes.
- **Risk:** Smoke test bash issues (`((FAIL++))` under `set -e`). Mitigation: use `FAIL=$((FAIL + 1))` per tableprinter migration lesson.

## Success Metrics

- Single `report` invocation with `--results md,json --write-to ./artifacts` produces all files (showcase uses this).
- No command makes API calls during the render phase.
- All existing human workflows work unchanged (with `--results` replacing `--format`).
- `--results json` produces clean stdout with no stderr leakage.
- Render idempotency verified for all pipelines.

## Sources & References

### Origin

- **Origin document:** [docs/brainstorms/2026-03-16-output-results-separation-brainstorm.md](docs/brainstorms/2026-03-16-output-results-separation-brainstorm.md) — Key decisions: three orthogonal axes, file naming with full command path, rendering is zero-API.

### Superseded Plan

- [docs/plans/2026-03-16-001-feat-results-output-writeto-flag-separation-plan.md](docs/plans/2026-03-16-001-feat-results-output-writeto-flag-separation-plan.md) — Original plan. This version resolves: SuppressStderr replacement gap, per-section artifact continuity, Render idempotency requirement, clean break vs MarkDeprecated, --post validation.

### Internal References

- Complete JSON output pattern: `docs/solutions/architecture-patterns/complete-json-output-for-agents.md`
- Command output shape & Provenance: `docs/solutions/architecture-patterns/command-output-shape.md`
- Cobra hierarchy & flag migration: `docs/solutions/architecture-refactors/cobra-command-hierarchy-thematic-grouping.md`
- Render-layer patterns: `docs/solutions/architecture-patterns/render-layer-linking-and-insight-quality.md`
- Tableprinter migration (smoke test lesson): `docs/solutions/go-gh-tableprinter-migration.md`
- Flag registration: `cmd/root.go:297`
- Format type: `internal/format/formatter.go:25-31`
- SuppressStderr: `internal/log/log.go:15`
- handleError: `cmd/root.go:117-141`
- Pipeline interface: `internal/pipeline/pipeline.go:19-33`
- Artifact-dir: `cmd/report.go:66` (flag), `cmd/report.go:491-557` (writeReportArtifacts)
- WarnUnlessJSON: `cmd/root.go:75-82` (15 call sites across 5 files)
- Post coercion: `cmd/root.go:180` (`Flags().Changed("format")`)
- MarkDeprecated precedent: `cmd/preflight.go:158`
- Smoke tests: `scripts/smoke-test.sh` (~24 format flag invocations)

### External References

- pflag StringSliceVar comma handling: [pflag#57](https://github.com/spf13/pflag/issues/57)
- gh CLI stderr convention: [cli/cli#7447](https://github.com/cli/cli/issues/7447)

### Related Work

- Issue #85: Parent issue for this work
- Issue #67: --artifact-dir on report (proves multi-format from single pass)
- PR #72: Report-only showcase + `--artifact-dir` prototype
