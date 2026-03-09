---
status: complete
priority: p1
issue_id: "029"
tags: [code-review, agent-native, error-handling]
dependencies: []
---

# Wire ErrorEnvelope and ExitCode into Execute()

## Problem Statement

The `Execute()` function in `cmd/root.go` always returns exit code 1 on error and never emits structured JSON errors. The `AppError`, `ErrorEnvelope`, and `ExitCode()` machinery in `internal/model/errors.go` is fully defined but never wired in. When `--format json` is used and an error occurs, the user/agent gets empty stdout and a generic exit code, making errors unparseable by automation.

**Raised by:** Agent-Native Reviewer (CRITICAL), Architecture Strategist

## Findings

- `Execute()` at `cmd/root.go:40-46` always returns 1 on error regardless of error type
- `AppError.ExitCode()` at `internal/model/errors.go:43-48` maps error codes to exit codes but is never called
- `ErrorEnvelope` at `internal/model/errors.go:52-54` provides JSON error output but is never used in the error path
- `--post` rejection at `cmd/root.go:70-71` returns plain `fmt.Errorf` instead of `AppError`
- Lead-time and cycle-time commands return plain errors that aren't wrapped in AppError

## Proposed Solutions

### Option A: Wire Execute() to check for AppError and emit JSON (Recommended)
- In `Execute()`, type-assert returned error to `*model.AppError`
- If `--format json` was requested, write `ErrorEnvelope` JSON to stderr
- Use `AppError.ExitCode()` for the process exit code
- Wrap key error sites (auth, not-found, config) in `AppError`
- **Effort:** Medium
- **Risk:** Low

### Option B: Cobra error handler middleware
- Use Cobra's `PersistentPostRunE` or custom error handler
- More complex, same outcome
- **Effort:** Medium
- **Risk:** Medium — Cobra error handling is subtle

## Acceptance Criteria

- [ ] `Execute()` uses `AppError.ExitCode()` when error is `*AppError`, falls back to 1
- [ ] When `--format json` and error occurs, structured JSON error is written to stdout
- [ ] `--post` rejection uses `AppError` with appropriate code
- [ ] Tests verify JSON error output and correct exit codes
- [ ] Non-AppError errors still return exit code 1

## Work Log

### 2026-03-09 - Created from code review
**By:** Review synthesis
**Actions:** Combined findings from Agent-Native Reviewer and Architecture Strategist
