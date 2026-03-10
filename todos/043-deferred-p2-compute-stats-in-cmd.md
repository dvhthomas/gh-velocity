---
status: pending
priority: p2
issue_id: "043"
tags: [code-review, architecture, testability]
dependencies: ["042"]
---

# computeStats Business Logic in cmd/ Package

## Problem Statement
`computeStats()` in `cmd/stats.go` is 200+ lines of orchestration and business logic. The `cmd/` package should only handle CLI wiring; orchestration logic should live in an internal package for testability and reuse.

## Findings
- Location: `cmd/stats.go:computeStats()` (lines 100-304)
- Agents: architecture-strategist, code-simplicity-reviewer
- The function handles: API fetching, metric computation, quality calculation, WIP filtering
- Not unit-testable without spinning up full CLI context

## Proposed Solutions

### Option 1: Extract to `internal/dashboard/` package
- **Pros**: Testable, reusable, clean separation
- **Cons**: New package, needs interface for API client
- **Effort**: Medium
- **Risk**: Low

### Option 2: Extract to `internal/metrics/dashboard.go`
- **Pros**: Keeps it in existing package, metrics-adjacent
- **Cons**: Metrics package currently has no API dependencies
- **Effort**: Medium
- **Risk**: Medium (changes metrics package contract)

## Recommended Action
_To be filled during triage_

## Technical Details
- **Affected Files**: cmd/stats.go, new internal package
- **Related Components**: metrics, format, github client
- **Database Changes**: No

## Acceptance Criteria
- [ ] Business logic extracted from cmd/stats.go
- [ ] Unit tests for dashboard computation
- [ ] cmd/stats.go is thin CLI wiring only

## Work Log

### 2026-03-10 - Created from code review
**By:** Claude Code Review
**Actions:**
- Finding identified by architecture-strategist and code-simplicity-reviewer
