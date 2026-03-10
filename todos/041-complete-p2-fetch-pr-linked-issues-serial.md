---
status: pending
priority: p2
issue_id: "041"
tags: [code-review, performance, concurrency]
dependencies: []
---

# FetchPRLinkedIssues Runs Serially in Stats Command

## Problem Statement
In `cmd/stats.go`, `FetchPRLinkedIssues` runs after all parallel fetches complete (Phase 2), instead of being chained inside the merged-PR goroutine. This adds unnecessary latency since the linked-issue fetch could start as soon as merged PRs are available.

## Findings
- Location: `cmd/stats.go:computeStats()` lines ~235-254
- Agent: performance-oracle
- Current flow: Wait for all 3 fetches → then fetch linked issues serially
- Better flow: Chain linked-issue fetch inside merged-PR goroutine

## Proposed Solutions

### Option 1: Chain FetchPRLinkedIssues inside merged-PR goroutine
- **Pros**: Overlaps with other fetches, reduces wall-clock time
- **Cons**: Slightly more complex goroutine, closingPRs needs mutex protection
- **Effort**: Small
- **Risk**: Low

## Recommended Action
_To be filled during triage_

## Technical Details
- **Affected Files**: cmd/stats.go
- **Related Components**: stats parallel fetch phase
- **Database Changes**: No

## Acceptance Criteria
- [ ] FetchPRLinkedIssues runs inside the merged-PR goroutine
- [ ] closingPRs map properly guarded with mutex
- [ ] Tests pass, no race conditions

## Work Log

### 2026-03-10 - Created from code review
**By:** Claude Code Review
**Actions:**
- Finding identified by performance-oracle
