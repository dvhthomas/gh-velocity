---
status: complete
priority: p2
issue_id: 004
tags: [code-review, agent-native, error-handling]
dependencies: []
---

# Structured JSON Errors and Exit Code Table

## Problem Statement

Agents parsing JSON output cannot reliably distinguish error types. The plan specifies human-readable error strings but doesn't fully define what agents receive when `--format json` is used and an error occurs. Exit codes are partially defined but need to be a first-class contract.

**Raised by:** Agent-Native Reviewer (CRITICAL), Security Sentinel (MEDIUM)

## Findings

- The deepened plan already includes an exit code table (0-4) and structured JSON errors — good
- Missing: an error code enum (NOT_FOUND, AUTH_MISSING_SCOPE, CONFIG_INVALID, RATE_LIMITED, NO_TAGS, NOT_GIT_REPO)
- Missing: a `details` field in JSON errors for machine-parseable context

## Proposed Solutions

### Option A: Error envelope with code enum (Recommended)
```json
{
  "error": {
    "code": "NOT_FOUND",
    "message": "issue #42 not found in owner/repo",
    "details": { "resource": "issue", "number": 42 }
  }
}
```
- Define error codes as Go constants in `internal/model/errors.go`
- Map to exit codes: NOT_FOUND→4, AUTH_*→3, CONFIG_*→2, others→1
- **Effort:** Small
- **Risk:** Low

## Acceptance Criteria

- [ ] JSON errors include `code`, `message`, and optional `details`
- [ ] Error codes are documented constants
- [ ] Exit codes match the documented table
- [ ] Agents can branch on error type without string parsing
