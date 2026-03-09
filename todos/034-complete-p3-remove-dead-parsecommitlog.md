---
status: complete
priority: p3
issue_id: "034"
tags: [code-review, dead-code]
dependencies: []
---

# Remove dead parseCommitLog function from git.go

## Problem Statement

`parseCommitLog()` in `internal/git/git.go:153-179` is dead code — superseded by `streamCommits()`. It's unexported and only referenced by its own tests.

**Raised by:** Performance Oracle, Architecture Strategist

## Proposed Solutions

### Option A: Delete function and its tests (Recommended)
- Remove `parseCommitLog` from `git.go`
- Remove associated test cases
- **Effort:** Small
- **Risk:** Low

## Acceptance Criteria

- [ ] `parseCommitLog` removed
- [ ] All tests pass
