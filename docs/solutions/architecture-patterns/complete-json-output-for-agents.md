---
title: Complete JSON output for agentic consumers
category: architecture-patterns
date: 2026-03-13
tags: [json, agents, stderr, warnings, errors, format]
related: [ErrorEnvelope, WarnUnlessJSON, SuppressStderr]
---

# Complete JSON output for agentic consumers

## Problem

When `-f json` is active, agents parsing stdout get clean JSON data, but stderr leaks human-readable noise: `warning:` lines from command-level code, `[debug]` cache/throttle messages from the HTTP client, and rate-limit retry messages. Agents can't reliably parse the output because they have to filter text noise from two streams.

Additionally, non-`AppError` errors (e.g., raw `fmt.Errorf`) were emitted as plain text on stderr instead of structured JSON, breaking the contract for agentic consumers.

## Root Cause

Warnings were emitted via `log.Warn()` at two layers:
1. **Command layer** (`cmd/*.go`) — warnings about data quality, missing config, etc.
2. **Client layer** (`internal/github/search.go`) — rate limit retries, cache diagnostics

The command layer has access to `deps.Format` and can check if JSON is active. The client layer does not — it's below the dependency injection boundary.

## Solution

**Two-layer suppression:**

1. **`log.SuppressStderr` global flag** — set in `PersistentPreRunE` when `-f json` is detected. This suppresses ALL `log.Warn()` and `log.Debug()` calls globally, including deep in the HTTP client.

```go
// internal/log/log.go
var SuppressStderr bool

func Warn(format string, args ...any) {
    if SuppressStderr { return }
    // ... existing implementation
}

func Debug(format string, args ...any) {
    if SuppressStderr { return }
    // ... existing implementation
}
```

```go
// cmd/root.go — PersistentPreRunE
if f == format.JSON {
    log.SuppressStderr = true
}
```

2. **`deps.WarnUnlessJSON()` helper** — belt-and-suspenders at command layer. Commands call this instead of `log.Warn()` directly.

```go
func (d *Deps) WarnUnlessJSON(format string, args ...any) {
    if d.Format != "json" {
        log.Warn(format, args...)
    }
}
```

3. **`handleError` wraps all errors as JSON** — non-`AppError` errors get wrapped as `{"error":{"code":"INTERNAL","message":"..."}}`

```go
func handleError(root *cobra.Command, err error) int {
    var appErr *model.AppError
    if !errors.As(err, &appErr) {
        appErr = &model.AppError{Code: "INTERNAL", Message: err.Error()}
    }
    // JSON envelope on stderr when -f json
}
```

**Where warnings go in JSON mode:**
- Warnings are NOT lost — they're included in each command's JSON payload via `"warnings"` field
- Every JSON output struct already has `Warnings []string \`json:"warnings,omitempty"\``
- Errors go to stderr as structured `ErrorEnvelope` JSON

## Prevention

- All new command-level warnings must use `deps.WarnUnlessJSON()`, never `log.Warn()` directly
- All new JSON output structs must include `Warnings []string` field
- The `log.SuppressStderr` flag catches client-layer logging automatically — no per-call changes needed there
- Test: `go run . <command> -f json 2>&1` should produce only valid JSON on stdout and nothing (or JSON error) on stderr
