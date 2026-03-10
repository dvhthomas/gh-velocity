---
status: pending
priority: p2
issue_id: "039"
tags: [code-review, pattern, simplicity, duplication]
dependencies: []
---

# Duplicate buildStrategy / buildReleaseStrategy Functions

## Problem Statement
`buildStrategy` in `cmd/cycletime.go` and `buildReleaseStrategy` in `cmd/release.go` are near-identical functions that construct a cycle-time strategy from config. Both also accept dead parameters (`ctx`, `issueNumber`) that are never used.

## Findings
- Location: `cmd/cycletime.go:buildStrategy()`
- Location: `cmd/release.go:buildReleaseStrategy()`
- Agents: pattern-recognition-specialist, code-simplicity-reviewer
- Both functions read `deps.Config.CycleTime.Strategy` and build the same strategy object

## Proposed Solutions

### Option 1: Extract shared `newStrategy(deps *Deps) cycletime.Strategy`
- **Pros**: Single source of truth, removes dead params
- **Cons**: Minor refactor across two files
- **Effort**: Small
- **Risk**: Low

## Recommended Action
_To be filled during triage_

## Technical Details
- **Affected Files**: cmd/cycletime.go, cmd/release.go, cmd/stats.go
- **Related Components**: cycletime strategy construction
- **Database Changes**: No

## Acceptance Criteria
- [ ] Single strategy builder function used by all three commands
- [ ] Dead parameters removed
- [ ] All tests pass

## Work Log

### 2026-03-10 - Created from code review
**By:** Claude Code Review
**Actions:**
- Finding identified by pattern-recognition-specialist and code-simplicity-reviewer
