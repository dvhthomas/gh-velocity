---
status: pending
priority: p2
issue_id: "040"
tags: [code-review, pattern, duplication, performance]
dependencies: []
---

# Duplicate Closing PR Map Building Logic

## Problem Statement
~20 lines of identical code for building a `closingPRs` map (merged PRs → linked issues) exists in both `cmd/cycletime.go` and `cmd/stats.go`. This maps PR numbers to issues via `FetchPRLinkedIssues`.

## Findings
- Location: `cmd/cycletime.go` (bulk mode, PR strategy branch)
- Location: `cmd/stats.go:computeStats()` (lines ~235-254)
- Agents: pattern-recognition-specialist, performance-oracle, architecture-strategist
- Same pattern: build prNumbers slice, call FetchPRLinkedIssues, build reverse map

## Proposed Solutions

### Option 1: Extract `buildClosingPRMap(ctx, client, mergedPRs) map[int]*model.PR`
- **Pros**: DRY, testable in isolation, single place to optimize
- **Cons**: Need to decide package placement
- **Effort**: Small
- **Risk**: Low

## Recommended Action
_To be filled during triage_

## Technical Details
- **Affected Files**: cmd/cycletime.go, cmd/stats.go
- **Related Components**: PR-linked cycle time computation
- **Database Changes**: No

## Acceptance Criteria
- [ ] Single helper function used by both commands
- [ ] Tests cover the extracted helper
- [ ] No behavior change

## Work Log

### 2026-03-10 - Created from code review
**By:** Claude Code Review
**Actions:**
- Finding identified by pattern-recognition, performance-oracle, architecture-strategist
