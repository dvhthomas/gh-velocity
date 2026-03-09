---
status: complete
priority: p2
issue_id: 024
tags: [code-review, quality, data-integrity]
dependencies: []
---

# Fix Silent Time Parse Error in git.go

## Problem Statement

In `parseCommitLog`, the time parse error is silently discarded: `authored, _ := time.Parse(...)`. A malformed date produces a zero-value `time.Time` that silently corrupts cycle time and lead time calculations downstream.

**Raised by:** Pattern Recognition Specialist

## Findings

- `internal/git/git.go:93` — `authored, _ := time.Parse(time.RFC3339, parts[1])`
- Zero-value time would make cycle time appear as a very large negative or positive duration

## Proposed Solutions

### Option A: Skip commit with warning (Recommended)
- If parse fails, skip the commit and log a warning
- **Effort:** Small
- **Risk:** Low

### Option B: Return error
- Propagate parse error up
- **Effort:** Small
- **Risk:** Low (but one bad commit stops all processing)

## Acceptance Criteria

- [x] Malformed dates in git log output don't produce zero-time commits
- [x] Test covers malformed date handling
