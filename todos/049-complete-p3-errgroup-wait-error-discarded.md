---
status: pending
priority: p3
issue_id: "049"
tags: [code-review, pattern, error-handling]
dependencies: []
---

# g.Wait() Error Discarded Silently

## Problem Statement
In `cmd/stats.go`, `_ = g.Wait()` discards the errgroup error. While current goroutines never return errors (graceful degradation returns nil), discarding the error silently means future changes that do return errors would be silently lost.

## Findings
- Location: cmd/stats.go line ~210
- Agent: pattern-recognition-specialist

## Proposed Solutions

### Option 1: Log if g.Wait() returns non-nil
- **Effort**: Small
- **Risk**: Low

## Acceptance Criteria
- [ ] errgroup error is at least logged to stderr

## Work Log

### 2026-03-10 - Created from code review
