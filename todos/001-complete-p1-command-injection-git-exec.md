---
status: complete
priority: p1
issue_id: 001
tags: [code-review, security, git, exec-command]
dependencies: []
---

# Command Injection Risk via exec.Command for Git Operations

## Problem Statement

The plan uses `exec.Command` for local git operations (tags, commit ranges) but does not specify input sanitization for values passed to git. Tag names from `--since <tag>`, issue numbers, and config values could be injected into git commands. This is the single most critical security finding.

**Raised by:** Security Sentinel (CRITICAL), Pattern Recognition (HIGH)

## Findings

- User-supplied tag names via `--since <tag>` flag could contain shell metacharacters or `--` prefixed strings interpreted as git flags
- Config-sourced values (`project.id`, status names) are attacker-controlled (config lives in the repo)
- The plan creates `internal/git/` package but does not specify sanitization rules

## Proposed Solutions

### Option A: Strict validation + `--` separator (Recommended)
- Validate tag names against `^[a-zA-Z0-9._\-/]+$`
- Always use `exec.Command("git", arg1, arg2, ...)` — never `exec.Command("sh", "-c", ...)`
- Always use `--` separator before positional args: `exec.Command("git", "log", "--", tagName)`
- Validate `--repo` format with `^[a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+$`
- **Pros:** Simple, comprehensive, zero runtime cost
- **Effort:** Small
- **Risk:** Low

### Option B: Allowlist wrapper function
- Create `internal/git/safe.go` with `SafeArg(s string) (string, error)` that validates any string before passing to exec
- **Pros:** Centralized validation
- **Effort:** Small
- **Risk:** Low

## Recommended Action

Option A — bake validation into the `internal/git/` package from Phase 1.

## Acceptance Criteria

- [ ] No `exec.Command("sh", "-c", ...)` anywhere in codebase
- [ ] All git commands use `--` to separate flags from positional args
- [ ] Tag names validated against strict pattern before use
- [ ] Table-driven tests for injection attempts (semicolons, backticks, `--upload-pack`)
