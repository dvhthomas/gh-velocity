---
title: "fix: Warn when defaulting to local repo context"
type: fix
status: completed
date: 2026-03-11
issue: https://github.com/dvhthomas/gh-velocity/issues/32
---

# fix: Warn when defaulting to local repo context

## Enhancement Summary

**Deepened on:** 2026-03-11
**Agents used:** SpecFlow analyzer, pattern-recognition-specialist, code-simplicity-reviewer, architecture-strategist, learnings-researcher

### Key Improvements from Deepening
1. Simplified approach: no `resolveRepo` signature change — detect auto-detection at call site
2. Use existing Hints pattern instead of inline config comments
3. Scoped notice to config commands only (preflight, discover) to avoid notice fatigue
4. Added `config discover` as a third caller that was missing from the original plan

## Overview

When a user runs `gh velocity config preflight --project-url <URL>` without `-R`, the tool silently defaults to the local git repo context. If the project URL is associated with a different repo, the user gets misleading results with no indication that the wrong repo was analyzed.

## Problem Statement

`resolveRepo()` in `cmd/root.go:281-306` has a three-level fallback: `--repo` flag > `GH_REPO` env > `repository.Current()` (git remote). The third fallback is silent — no notice is emitted. This is confusing when:

1. The user runs preflight from a directory whose git remote differs from the project they're analyzing
2. The generated config has a `scope.query: "repo:wrong/repo"` without any indication it was auto-detected

## Proposed Solution

Emit `log.Notice` from preflight and discover commands when repo was auto-detected from git remote. Add a hint to the generated config via the existing `Hints` mechanism. Keep `resolveRepo()` pure — no signature change, no side effects.

### Design Decisions

- **No `resolveRepo` signature change**: The caller already has `repoFlag` in scope. Checking `repoFlag == "" && os.Getenv("GH_REPO") == ""` at the call site is simpler than threading a boolean through all 3 callers. (Per simplicity and architecture reviews.)
- **Notice only on config commands**: Preflight and discover are where silent auto-detection causes real confusion (config gets baked in). Metric commands show the repo in their output. Emitting on every command causes notice fatigue.
- **Use Hints, not inline comments**: All other advisory messages in preflight flow through `result.Hints`, which renders as YAML comment block header and appears in JSON output. Adding a hint is more consistent than conditional inline comments in `renderPreflightConfig`.
- **Enhance `--debug` output**: For metric commands, append "(auto-detected from git remote)" to the existing debug line at `cmd/root.go:194`.

### Changes

#### 1. `cmd/preflight.go` — Detect auto-detection and emit notice + hint

In the `RunE` function, after `resolveRepo` succeeds:

```go
repoFlag, _ := cmd.Root().PersistentFlags().GetString("repo")
owner, repo, err := resolveRepo(repoFlag)
if err != nil {
    return err
}

// Detect auto-detection from git remote.
repoAutoDetected := repoFlag == "" && os.Getenv("GH_REPO") == ""
if repoAutoDetected {
    log.Notice("Using repo %s/%s from git remote (use --repo to override)", owner, repo)
}
```

Then after `runPreflight`, add a hint:

```go
if repoAutoDetected {
    result.RepoAutoDetected = true
}
```

In `runPreflight` or after it returns, prepend a hint:

```go
if result.RepoAutoDetected {
    result.Hints = append([]string{
        fmt.Sprintf("Repo %s auto-detected from git remote. Use -R owner/repo to target a different repository.", result.Repo),
    }, result.Hints...)
}
```

Add the `RepoAutoDetected` field to `PreflightResult`:

```go
type PreflightResult struct {
    // ... existing fields ...
    RepoAutoDetected bool `json:"repo_auto_detected"` // true when repo came from git remote
}
```

#### 2. `cmd/config.go` — Same pattern for discover command

The `config discover` command at `cmd/config.go:196` also calls `resolveRepo(repoFlag)` independently. Apply the same auto-detection check and `log.Notice`.

#### 3. `cmd/root.go` — Enhance debug output (no `resolveRepo` changes)

In `PersistentPreRunE`, after resolving repo, enhance the debug line:

```go
if debugFlag {
    repoSource := ""
    if repoFlag == "" && os.Getenv("GH_REPO") == "" {
        repoSource = " (auto-detected from git remote)"
    }
    log.Debug("repo:         %s/%s%s", owner, repo, repoSource)
    // ... rest of debug output
}
```

`resolveRepo()` itself stays unchanged — no signature change, no side effects.

### Files to modify

- `cmd/preflight.go:56-68` — `RunE`: add auto-detection check, `log.Notice`, set hint
- `cmd/preflight.go:141` — `PreflightResult`: add `RepoAutoDetected` field
- `cmd/config.go:196` — discover `RunE`: add same auto-detection check and notice
- `cmd/root.go:193-194` — `PersistentPreRunE`: enhance debug line with source info
- `cmd/preflight_test.go` — test that `RepoAutoDetected` appears in JSON and hint in YAML

## Acceptance Criteria

- [x] Running `gh velocity config preflight` without `-R` in a git repo emits a notice to stderr: `Using repo owner/repo from git remote (use --repo to override)`
- [x] The generated config YAML includes a hint comment noting the repo was auto-detected
- [x] JSON output includes `"repo_auto_detected": true` when auto-detected
- [x] Running with explicit `-R owner/repo` emits no notice and `repo_auto_detected` is false
- [x] Running with `GH_REPO=owner/repo` emits no notice
- [x] `config discover` also emits the notice when auto-detected
- [x] `--debug` output shows "(auto-detected from git remote)" when applicable
- [x] Metric commands (lead-time, report, etc.) do NOT emit the notice (only show in `--debug`)
- [x] The notice goes to stderr (via `log.Notice`), not stdout — does not pollute JSON or piped output
- [x] All existing tests pass (`task test`)
- [x] `task quality` passes

## Technical Considerations

- **Keep `resolveRepo` pure**: No logging side effect in the resolution function. Callers decide whether/how to notify. This preserves testability and single responsibility.
- **Notice fatigue**: Only config-generation commands (preflight, discover) emit `log.Notice`. These are run infrequently and produce files that bake in the repo — the highest-risk path.
- **CI behavior**: In GitHub Actions, `log.Notice` emits `::notice::` annotations. Preflight/discover runs are infrequent in CI, so this is informative rather than noisy.
- **Hints pattern consistency**: Using `result.Hints` for the config warning follows the established pattern — all other advisory messages (strategy detection, label analysis, posting readiness) flow through hints.
- **No breaking changes**: No signature changes, no new dependencies.

## Edge Cases

- **`GH_REPO` set but forgotten**: Not warned — this is standard `gh` CLI convention. Visible in `--debug` output.
- **Cross-repo project URL mismatch**: Out of scope for this fix. Could be a follow-up: compare `ParseProjectURL` owner against resolved repo owner and emit `log.Warn`.
- **stderr always written to**: `log.Notice` writes unconditionally to stderr regardless of TTY. This is fine — stderr is the correct channel for notices and doesn't pollute stdout.

## Sources

- Issue: [#32](https://github.com/dvhthomas/gh-velocity/issues/32)
- `cmd/root.go:281-306` — current `resolveRepo()` implementation
- `cmd/preflight.go:58-62` — preflight's repo resolution
- `cmd/config.go:196` — discover's repo resolution
- `internal/log/log.go:42-51` — `log.Notice` implementation
- `docs/solutions/evidence-driven-preflight-config.md` — preflight hints pattern
