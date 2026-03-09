---
status: complete
priority: p3
issue_id: "035"
tags: [code-review, simplicity]
dependencies: []
---

# Replace indexOf with strings.IndexByte

## Problem Statement

`indexOf` in `internal/github/tags.go:94-101` reimplements `strings.IndexByte` from the standard library.

**Raised by:** Architecture Strategist, Performance Oracle

## Proposed Solutions

### Option A: Replace with strings.IndexByte (Recommended)
- Replace `indexOf(msg, '\n')` with `strings.IndexByte(msg, '\n')`
- Delete the `indexOf` function
- **Effort:** Small
- **Risk:** Low

## Acceptance Criteria

- [ ] `indexOf` replaced with `strings.IndexByte`
- [ ] All tests pass
