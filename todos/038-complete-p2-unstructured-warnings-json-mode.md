---
status: pending
priority: p2
issue_id: "038"
tags: [code-review, agent-native, json, pattern]
dependencies: []
---

# Unstructured Warnings in JSON Mode

## Problem Statement
The stats and cycle-time bulk commands write warnings to stderr as plain text (`fmt.Fprintf(os.Stderr, "warning: ...")`). When `--format json` is used, these warnings are invisible to JSON consumers and machine agents. A structured `"warnings"` array in the JSON output would make these discoverable.

## Findings
- Location: `cmd/stats.go` (lines 129, 142, 156, 182, 246)
- Location: `cmd/cycletime.go` (bulk mode warnings)
- Agents: agent-native-reviewer, pattern-recognition-specialist
- Impact: Machine consumers parsing JSON stdout miss degraded-mode signals

## Proposed Solutions

### Option 1: Add `warnings` array to JSON output
- **Pros**: Clean, machine-readable, consistent with existing error envelope pattern
- **Cons**: Requires plumbing warnings through `StatsResult` and format layer
- **Effort**: Medium
- **Risk**: Low

### Option 2: Structured stderr JSON warnings
- **Pros**: Keeps stdout clean, mirrors existing error pattern
- **Cons**: Agents may not read stderr
- **Effort**: Small
- **Risk**: Medium (agents may ignore stderr)

## Recommended Action
_To be filled during triage_

## Technical Details
- **Affected Files**: cmd/stats.go, cmd/cycletime.go, internal/format/stats.go
- **Related Components**: All bulk/stats commands with graceful degradation
- **Database Changes**: No

## Acceptance Criteria
- [ ] JSON output includes `"warnings": [...]` array when degraded
- [ ] Warnings omitted from JSON when empty (omitempty)
- [ ] Tests validate warning propagation
- [ ] Smoke tests updated

## Work Log

### 2026-03-10 - Created from code review
**By:** Claude Code Review
**Actions:**
- Finding identified by agent-native-reviewer and pattern-recognition-specialist
- Created as pending todo for triage

## Resources
- Related: #037 (unify-warnings-in-json-output — completed, but only for error envelope)
