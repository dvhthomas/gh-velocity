---
status: pending
priority: p2
issue_id: "045"
tags: [code-review, simplicity, json]
dependencies: []
---

# Always-Zero commitCount in Cycle Time Output

## Problem Statement
`outputCycleTime` accepts a `commitCount` parameter that is always passed as 0. The JSON output includes `"commits": 0` which is misleading — it suggests we counted commits and found none, when we simply never counted.

## Findings
- Location: `cmd/cycletime.go:outputCycleTime()` parameter
- Agent: code-simplicity-reviewer
- The parameter was likely a placeholder for future functionality

## Proposed Solutions

### Option 1: Remove commitCount parameter, omit from JSON
- **Pros**: Accurate output, no misleading data, simpler code
- **Cons**: Breaking JSON schema change (minor)
- **Effort**: Small
- **Risk**: Low

## Recommended Action
_To be filled during triage_

## Technical Details
- **Affected Files**: cmd/cycletime.go, internal/format/cycletime.go
- **Related Components**: Cycle time JSON output
- **Database Changes**: No

## Acceptance Criteria
- [ ] commitCount parameter removed
- [ ] JSON output does not include misleading `"commits": 0`
- [ ] Smoke tests updated if needed

## Work Log

### 2026-03-10 - Created from code review
**By:** Claude Code Review
**Actions:**
- Finding identified by code-simplicity-reviewer
